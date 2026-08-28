# Provider Cooldown × Logging：时序关系与日志细分方案

> **状态**：待定稿（draft）。作者先把分析记录下来，待日后想清楚了再启动实现。
> **适用范围**：celer-route 已实现 `provider-cooldown` plugin 之后，运维观察到的一个看似"虚假"的日志现象。

---

## 1. 问题陈述

### 1.1 观察

路由规则 `pg-master` 配置的目标模型是 `minimax/MiniMax-M3`，并配置回退到 `sensenova/glm-5.2`。

当 `minimax` 提供商的所有 Key 都进入 cooldown 时：

- 实际**没有**向 `minimax` 发起任何 LLM 请求。
- 但日志表中**仍然记录了一条状态为 `cancelled`（cancel）的日志**。
- 接着记录了回退到 `sensenova` 的成功日志。

运维疑问：

1. 为什么会有"假"日志？
2. 这个状态从时间线上看是 cancel（取消），但实际不是客户端主动断开，也不是超时——它是 cooldown 触发的"被压制"。
3. 后续如何区分这些不同语义的 cancelled？

### 1.2 根因（结论先行）

这条 `cancelled` 日志是 **设计意图**，不是 bug：

- `provider-cooldown` 通过 `KeyPoolFilter` 把所有 cooldown 中的 key 过滤掉，导致该 provider 没有可用 key。
- 核心在 `executeRequestWithRetries` 中检测到这种情况，抛出**合成的** `503 no_eligible_keys` 错误。
- 核心仍然把这个错误送入 `RunPostLLMHooks`，目的是关闭 `PreLLMHook` 写入的 `processing` timeline 事件——不写 PostLLMHook 会导致时间线永远灰色运行。
- logging 插件的 `PostLLMHook` 看到 `no_eligible_keys` 错误，状态设为 `cancelled`（不计入 error/cost/latency 聚合）。
- 紧接着 core 调度 fallback，触发第二条 success 日志。

详细时序见 §2。

---

## 2. Cooldown × 日志的完整时序

### 2.1 调用栈

```
handleRequest(ctx, req)
  ├─ PreRequestHook（governance/语义缓存等一次性钩子）
  ├─ tryRequest(ctx, req)  ◄── 主路径 (provider = minimax)
  │    ├─ RunLLMPreHooks
  │    │   └─ logging.PreLLMHook
  │    │       └─ 把 pending log entry 写入内存，状态=processing
  │    │       └─ request_id = R, provider="minimax", model="MiniMax-M3"
  │    ├─ msg → pq.queue
  │    └─ worker 取出 msg → executeRequestWithRetries
  │         └─ keyProvider(): KeyPoolFilter 过滤所有 minimax key
  │              └─ AsFilter() 返回空切片 → 抛 errAllKeysFiltered
  │         └─ 转为 503 BifrostError{ Type: "no_eligible_keys" }
  │         └─ msg.Err ← 该错误
  │    ← case bifrostErrVal := <-msg.Err:
  │       ├─ RunPostLLMHooks(ctx, nil, err503, pluginCount)
  │       │   ├─ cooldown.PostLLMHook
  │       │   │   └─ isQuotaExhausted(503 no_eligible_keys) = false → 直接 return
  │       │   └─ logging.PostLLMHook (Path A: result==nil, bifrostErr!=nil)
  │       │       └─ entry.Status = logStatusForError(err503)
  │       │       └─ isNoEligibleKeysError(err503) = true → 返回 "cancelled"
  │       │       └─ 把日志落库 (status="cancelled", provider="minimax", model="MiniMax-M3")
  │       └─ 返回 (nil, err503)
  ├─ shouldTryFallbacks(req, err503)
  │   └─ err503.Error.Type = "no_eligible_keys" ≠ RequestCancelled
  │   └─ AllowFallbacks = nil（默认允许）
  │   └─ fallbacks 不空 → 返回 true
  └─ for i, fallback := range fallbacks
       ├─ ctx.SetValue(FallbackRequestID, NEW_UUID)
       │   注意：新的 fallback request id 替换了 ctx 上的 RequestID
       ├─ clearCtxForFallback(ctx)
       └─ tryRequest(ctx, fallbackReq)  ◄── fallback (provider = sensenova)
            ├─ RunLLMPreHooks
            │   └─ logging.PreLLMHook（用 NEW_UUID 作为有效 request id
            │      + parent = 原 R，作为 fallback 子行写入 pending）
            ├─ 真实 LLM 请求到 sensenova → 成功
            └─ RunPostLLMHooks → logging.PostLLMHook 写第二条日志
                (status="success", provider="sensenova", model="glm-5.2",
                 parent_request_id = R, fallback_index = 1)
```

### 2.2 关键代码定位

| 阶段 | 文件:行 |
|---|---|
| `KeyPoolFilter` 过滤 → `errAllKeysFiltered` | `core/bifrost.go:5625–5660` |
| 合成 503 `no_eligible_keys` | `core/bifrost.go:6156–6167` |
| PostLLMHook 在 err 路径仍执行（设计意图） | `core/bifrost.go:5624–5640` 的注释明确说明 |
| 状态映射规则 | `plugins/logging/operations.go:29–34, 43–54` |
| logging.PreLLMHook 写入 pending | `plugins/logging/main.go:982–995` |
| logging.PostLLMHook 落库 | `plugins/logging/main.go:1190–1285`（Path A 分支） |
| fallback 编排 | `core/bifrost.go:5242–5298` |
| cooldown 在 `no_eligible_keys` 上不触发 | `plugins/providercooldown/cooldown.go:514–531` |
| `shouldTryFallbacks` 放行 | `core/bifrost.go:5017–5047` |

### 2.3 设计动机（注释佐证）

`core/bifrost.go:5625–5632` 明确写着：

> "Previously this branch skipped PostLLMHooks to avoid logging a 'spurious' 0ms failure, but doing so left the request stuck in 'processing' forever in the timeline because PreLLMHook already pushed a processing event. Always running PostLLMHooks records the terminal status correctly."

---

## 3. 行为是否正确？

**功能上正确**，但仍有可优化点。

### 3.1 现状评估

| 维度 | 现状 |
|---|---|
| 是否会被错误地视为 "客户端取消" | ✓ 是 |
| 是否计入 error 率 | ✗ 否（cancelled 单列） |
| 是否计入 cost/latency 聚合 | ✗ 否（cancelled 单列） |
| cooldown 状态是否会被这条假日志污染 | ✗ 否（PostLLMHook 仅在 quota error 上 mark） |
| fallback 链路是否正确 | ✓ 是（fallback 行带 parent_request_id 串联） |
| timeline 是否能正常关闭 | ✓ 是 |

### 3.2 仍待优化

1. **可视化混淆**：cooldown 触发的"被压制"和"客户端主动断开"在 UI 中颜色与文案完全相同。
2. **运维排查成本高**：需要打开 Error Details 才能分辨原因。
3. **统计语义不清晰**：运维拉 `status='cancelled'` 报表时，cooldown 触发的丢弃和客户端断开混在一起。

---

## 4. 待定方案：LLM 日志模型 C

### 4.1 三种可选模型的对比

#### 模型 A：每用户请求一行（attempts 折叠成子字段）

每个 R 只生成一行，行的 metadata 包含 attempts 数组。

| 优点 | 缺点 |
|---|---|
| UI 一眼看完 | 重写 plugin pipeline 写入语义 |
| 和客户对账清晰 | 需要在 handleRequest 终态 defer write |
| 统计直接对齐账单 | cooldown/MCP/agent 多轮 trace 难表达 |
| | 改动面大 |

#### 模型 B：每 provider attempt 一行（现状）

R 行 + R' 行 + R'' 行 … 由 `parent_request_id` 链成一棵树。

| 优点 | 缺点 |
|---|---|
| 实现最小 | "假 cancelled" 行会一直存在 |
| 和 plugin pipeline 契约完全契合 | 运维需理解 parent/child 折叠 |
| 零迁移成本 | 计费/统计需 SUM 聚合 |
| MCP agent 多轮 trace 好表达 | |

#### 模型 C：现状 + UI 折叠 + 状态细分

保留 attempt 级日志行，但引入新 status 子类型区分 `cancelled_suppressed` / `cancelled_client` / `cancelled_timeout`，UI 默认按 parent 折叠。

| 优点 | 缺点 |
|---|---|
| 现状成本最低 | 多一行 status 子类型枚举 |
| 细节不丢 | UI 折叠是另一个 PR |
| "假 cancelled" 状态可解释 | |
| 兼容旧 `cancelled` 值 | |

**推荐**：模型 C（最小改动，最大信息密度提升）。

### 4.2 选 C 的理由

1. **Plugin 契约不变**：`RunPreLLMHooks` → `RunPostLLMHooks` 的对称设计是 celer-route 性能与正确性的基石。
2. **细节不丢**：MCP agent 多轮 trace、cache hit/fail、retry attempt trail 都依赖 attempt 级日志。
3. **"假 cancelled" 只是 UI 体验问题**：底层语义（"我们没真打 minimax"）已经在 `error.type = no_eligible_keys` 和 `routing_engine_log` 里了。

---

## 5. 模型 C 详细实施计划

### 5.1 后端：logging plugin 引入 status 子类型

**文件**：`plugins/logging/operations.go`

```go
// 新增 3 个常量（保留旧 logStatusCancelled 以兼容）
const (
    logStatusCancelled            = "cancelled"            // 兼容旧值
    logStatusCancelledSuppressed  = "cancelled_suppressed" // no_eligible_keys (KeyPoolFilter 全过滤)
    logStatusCancelledClient      = "cancelled_client"     // 499 / RequestCancelled
    logStatusCancelledTimeout     = "cancelled_timeout"    // ctx deadline
)

// 新增函数
func cancelledSubtype(err *schemas.BifrostError) string {
    // 顺序重要：先查 no_eligible_keys（最具体），再查 status code，再查 type
    if isNoEligibleKeysError(err) {
        return logStatusCancelledSuppressed
    }
    if err.StatusCode != nil && *err.StatusCode == 499 {
        return logStatusCancelledClient
    }
    if err.Error != nil && err.Error.Type != nil {
        switch *err.Error.Type {
        case schemas.RequestCancelled:
            return logStatusCancelledClient
        case schemas.RequestTimedOut:
            if isContextTimeoutLogError(err) {
                return logStatusCancelledTimeout
            }
        }
    }
    return logStatusCancelled // 兜底：保持现有行为
}

// 修改 logStatusForError：返回精确子类型
func logStatusForError(err *schemas.BifrostError) string {
    if isCancelledLogError(err) {
        return cancelledSubtype(err)
    }
    return logStatusError
}
```

**兼容性**：`logStatusCancelled` 常量值仍是 `"cancelled"`，旧值不会消失；兜底返回 `"cancelled"` 兼容未识别情况。

### 5.2 后端：核心测试

**位置**：`core/bifrost_test.go`

新增/扩展 `TestFixedKeyProviderRespectsKeyPoolFilter`：

- 验证 PostLLMHook 仍被调用、status = `"cancelled_suppressed"`
- 验证 cooldown 没误标
- 验证最终日志只有一行 `cancelled_suppressed` + 一行 fallback `success`

### 5.3 Harness 用例

**位置**：`tests/e2e/api/collections/provider-harness.json`

按 AGENTS.md "Every core/ change ships with a provider-harness case" 要求，新增：

```json
{
  "name": "Cooldown triggers fallback with suppressed log entry",
  "request": { ...primary provider request... },
  "expectPrimaryStatus": "cancelled_suppressed",
  "expectFallbackSuccess": true,
  "expectLogEntryPrimaryStatus": "cancelled_suppressed",
  "expectLogEntryFallbackStatus": "success",
  "expectCooldownStateUnchanged": true
}
```

### 5.4 前端：constants/logs.ts

**文件**：`ui/lib/constants/logs.ts:70`

```ts
export const Statuses = [
  "success",
  "error",
  "processing",
  "cancelled",
  "cancelled_suppressed",  // 新增
  "cancelled_client",      // 新增
  "cancelled_timeout",     // 新增
] as const;

export const StatusColors = {
  success: "bg-green-100 text-green-800",
  error: "bg-red-100 text-red-800",
  processing: "bg-blue-100 text-blue-800",
  cancelled: "bg-gray-100 text-gray-800",
  cancelled_suppressed: "bg-blue-100 text-blue-800",  // 蓝灰：系统策略
  cancelled_client: "bg-gray-100 text-gray-800",       // 灰：客户端行为（保持现状）
  cancelled_timeout: "bg-orange-100 text-orange-800",  // 橙：超时
} as const;

export const StatusBarColors = {
  success: "bg-green-500",
  error: "bg-red-500",
  processing: "bg-blue-500",
  cancelled: "bg-gray-400",
  cancelled_suppressed: "bg-indigo-500",  // 靛色：与 processing 蓝色区分
  cancelled_client: "bg-gray-400",
  cancelled_timeout: "bg-orange-500",
} as const;
```

### 5.5 前端：types/logs.ts

**文件**：`ui/lib/types/logs.ts:608, 715, 755`

```ts
status: string;
// "success", "error", "processing", "cancelled",
// "cancelled_suppressed" (KeyPoolFilter 全过滤/cooldown),
// "cancelled_client" (499/RequestCancelled),
// "cancelled_timeout" (ctx deadline)

interface HistogramEntry {
  success: number;
  error: number;
  processing: number;
  cancelled: number;
  cancelled_suppressed: number;  // 新增
  cancelled_client: number;      // 新增
  cancelled_timeout: number;     // 新增
}
```

### 5.6 前端：SSE 终端判定

**文件**：`ui/hooks/useLogsTimelineSSE.ts:88`

```ts
function isTerminalStatus(status: string): boolean {
  return status === "success" ||
         status === "error" ||
         status.startsWith("cancelled"); // 包含所有 cancelled_* 子类型
}
```

### 5.7 前端：详情页 StatusPill

**文件**：`ui/app/workspace/logs/sheets/logDetailView.tsx:783–808`

```tsx
const statusPillStyles: Record<string, string> = {
  success: "bg-green-50 text-green-700 border-green-200 ...",
  error: "bg-red-50 text-red-700 border-red-200 ...",
  processing: "bg-blue-50 text-blue-700 border-blue-200 ...",
  cancelled: "bg-gray-50 text-gray-700 border-gray-200 ...",
  cancelled_suppressed: "bg-indigo-50 text-indigo-700 border-indigo-200 ...",  // 新增
  cancelled_client: "bg-gray-50 text-gray-700 border-gray-200 ...",             // 新增（同 cancelled）
  cancelled_timeout: "bg-orange-50 text-orange-700 border-orange-200 ...",      // 新增
};
const statusDotStyles: Record<string, string> = {
  // ... 同上，新增 3 项
};

const statusSubtitle: Record<string, string> = {
  cancelled_suppressed: "Suppressed by Cooldown",
  cancelled_client: "Cancelled by Client",
  cancelled_timeout: "Request Timed Out",
};

function StatusPill({ status }: { status: Status }) {
  return (
    <div className="flex flex-col items-start gap-0.5">
      <span className={...}>
        <span className={dot} />
        {status}
      </span>
      {statusSubtitle[status] && (
        <span className="text-[10px] text-muted-foreground">{statusSubtitle[status]}</span>
      )}
    </div>
  );
}
```

### 5.8 前端：Volume chart histogram

**文件**：`ui/app/workspace/logs/views/logsVolumeChart.tsx:150,161,177,190,204`

```ts
// bucket.cancelled_suppressed ?? 0 等需要从后端 histogram 返回
// 后端 GROUP BY status 已经自动返回新桶，前端只需读取
```

**后端对应**：`plugins/logging/main.go:1194` `GetHistogram` → `p.store.GetHistogram` 透传 status 分组，无需改动后端逻辑（histogram 已按 status 列 GROUP BY，新 status 子类型会自动成为新桶）。

**前端测试**：`ui/app/workspace/logs/views/logsVolumeChart.test.tsx` 需覆盖新桶。

### 5.9 前端：i18n

**文件**：`ui/locales/en/logs.json`、`ui/locales/zh-CN/logs.json`

```json
{
  "statusCancelledSuppressed": "Suppressed by Cooldown",
  "statusCancelledClient": "Cancelled by Client",
  "statusCancelledTimeout": "Request Timed Out"
}
```

中文：

```json
{
  "statusCancelledSuppressed": "被冷却抑制",
  "statusCancelledClient": "客户端取消",
  "statusCancelledTimeout": "请求超时"
}
```

注：i18n 规范下，"token" 在 LLM 上下文是"词元"，本方案不涉及 token 翻译。

### 5.10 前端：SSE 测试

**文件**：`ui/hooks/useLogsTimelineSSE.test.ts`

新增/扩展测试覆盖新 cancelled_* 终态判定。

### 5.11 回归测试（红 → 绿）

**位置**：`plugins/logging/operations_test.go`

新增三个测试：

- `TestLogStatusForError_NoEligibleKeysReturnsCancelledSuppressed`
- `TestLogStatusForError_Status499ReturnsCancelledClient`
- `TestLogStatusForError_ContextDeadlineReturnsCancelledTimeout`

每个测试构造对应 BifrostError → 调用 `logStatusForError` → 断言返回值。

---

## 6. 影响面评估

| 层 | 影响 | 风险 |
|---|---|---|
| 后端 Go | `operations.go` 函数行为微调 | 低：旧值 `"cancelled"` 仍在兜底；其他 status 路径不变 |
| 后端聚合 | 无（按 status 字段 GROUP BY，不依赖精确字符串过滤） | 无 |
| 前端 enum | Statuses 数组 + 4 处类型字段 | 低：所有现有过滤项仍存在 |
| 前端 SSE | `isTerminalStatus` 改为 `startsWith("cancelled")` | 低：旧值仍能命中 |
| 前端 UI | 4 个新徽章 + 3 个新 i18n key + 可选折叠 | 中：折叠行为可能影响其他视图对多行的假设 |
| 数据库迁移 | 不需要（status 字段是 string 列，新值直接写入） | 无 |
| API 兼容性 | 兼容（v1+v2 endpoint status 字段类型不变） | 无 |
| 向后兼容 | 旧客户端按 `=== "cancelled"` 过滤的代码仍能命中兜底值 | 低 |

---

## 7. 视觉设计对比（最终交付时的样子）

### 7.1 状态徽章

**前**：

```
[SUCCESS] [ERROR] [PROCESSING] [CANCELLED]
  绿       红       蓝           灰
```

**后**：

```
[SUCCESS]  [ERROR]  [PROCESSING]  [CANCELLED_SUPPRESSED]  [CANCELLED_CLIENT]  [CANCELLED_TIMEOUT]  [CANCELLED]
  绿        红       蓝            靛色                   灰(保持现状)         橙                    灰(兜底)
```

颜色选择理由：

- **cancelled_suppressed 靛色**：和 processing 蓝色区分（避免被误以为还在运行），但和 success/error 形成第三类区分
- **cancelled_client 灰色**：保持现状，避免变更用户对"客户端取消"的视觉记忆
- **cancelled_timeout 橙色**：和 warning/error 形成渐进色调，符合"超时比 cancel 严重"的语义

### 7.2 列表页（`/workspace/logs`）

**前**：
```
11:42:03  minimax/MiniMax-M3   [CANCELLED]   ← 视觉上与"客户端断开"无法区分
```

**后**：
```
11:42:03  minimax/MiniMax-M3   [CANCELLED_SUPPRESSED]   ← 鼠标悬停："Suppressed by cooldown"
```

### 7.3 详情页头部

**前**：
```
Request ID: abc-123                      [▏CANCELLED]
Provider: minimax
Model: MiniMax-M3
```

**后**：
```
Request ID: abc-123          [▏CANCELLED_SUPPRESSED]
                              ↳ Suppressed by Cooldown  ← 新副标题

Provider: minimax
Model: MiniMax-M3
Error Details:
   type: no_eligible_keys, status: 503
```

### 7.4 Volume Chart

**前**：4 段堆叠图（success/error/processing/cancelled）

**后**：7 段堆叠图，cancelled 内部细分为 suppressed/client/timeout/other 四段

### 7.5 折叠视图（Phase 3，可选增强）

目前 `columns.tsx:262` 已经有 `groupedView=true` 的折叠逻辑（ChevronRight + child_count 徽章），但默认未启用。Phase 3 把 groupedView 默认开启：

```
11:42:03  ▶ [2]  minimax · MiniMax-M3   [CANCELLED_SUPPRESSED]   ← 主行
         ↳ 11:42:03.123 minimax/MiniMax-M3     [SUPPRESSED]
         ↳ 11:42:03.245 sensenova/glm-5.2       [SUCCESS]
```

---

## 8. 顺序与里程碑

### Phase 1（后端，独立 PR）

1. `plugins/logging/operations.go`：status 子类型 + `cancelledSubtype` + `logStatusForError` 改写
2. `plugins/logging/operations_test.go`：3 个新测试
3. `core/bifrost_test.go`：1 个新测试验证 cooldown 路径
4. `tests/e2e/api/collections/provider-harness.json`：1 个 case
5. 跑 `make test-plugins` 和 `make test-core` 验证

### Phase 2（前端 UI，独立 PR）

1. `ui/lib/constants/logs.ts`：Statuses + 颜色
2. `ui/lib/types/logs.ts`：类型扩展
3. `ui/hooks/useLogsTimelineSSE.ts`：终端判定
4. `ui/app/workspace/logs/sheets/logDetailView.tsx`：详情页徽章 + 副标题
5. `ui/app/workspace/logs/views/logsVolumeChart.tsx`：新桶渲染 + legend
6. `ui/locales/{en,zh-CN}/logs.json`：新 key
7. 跑 `cd ui && npm run build` 验证

### Phase 3（折叠增强，单独 PR）

1. `ui/app/workspace/logs/views/columns.tsx`：默认 groupedView=true
2. 配套 E2E：`tests/e2e/features/logs-fallback.spec.ts`

---

## 9. 待决问题

1. **是否真的需要这么多状态子类型？** 也许仅 `cancelled_suppressed` 一个就足够？
2. **折叠是否默认开启？** 默认开启会影响所有用户（包括没用 cooldown 的简单部署）。
3. **Volume chart 是否需要细分桶？** 是否给运维增加信息密度，还是带来视觉复杂度？
4. **是否需要改动 `error.type` 的语义？** 当前 `no_eligible_keys` 已经表达清楚，但 UI 层是否需要弱化显示？

---

## 10. 关联文件

- `core/bifrost.go` — `executeRequestWithRetries`, `tryRequest`, `shouldTryFallbacks`
- `plugins/providercooldown/cooldown.go` — `AsFilter`, `PostLLMHook`, `IsQuotaExhausted`
- `plugins/logging/operations.go` — `logStatusForError`, `isNoEligibleKeysError`, `isCancelledLogError`
- `plugins/logging/main.go` — `PreLLMHook`, `PostLLMHook` Path A
- `tests/e2e/api/collections/provider-harness.json` — provider-harness 回归用例

---

> **备注**：本文档为方案草稿，待决策者确认范围与优先级后再启动实现。
