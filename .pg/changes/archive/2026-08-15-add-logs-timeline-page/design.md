# add-logs-timeline-page 设计

## 架构概览

本变更在 Bifrost 中引入"Request Timeline"能力，分三层实现：

```
┌──────────────────────────────────────────────────────────────────┐
│  UI 层 (ui/app/workspace/logs/timeline/)                          │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ Gantt 总览（横向时间轴）· 新组件 LogsTimeline                 │  │
│  │  - follow/live/pan 三模式 · lane 分配 · 缩放 · hover tooltip  │  │
│  │  - 点击 bar → LogDetailSheet（新增 Timeline 标签页）         │  │
│  │  - SSE 活跃请求实时合并                                      │  │
│  └────────────────────────────────────────────────────────────┘  │
└───────────────┬──────────────────────────────────────────────────┘
                │ HTTP: GET /api/logs, /api/logs/{id}, GET /api/logs/active/stream (SSE)
                ▼
┌──────────────────────────────────────────────────────────────────┐
│  HTTP 层 (transports/bifrost-http/handlers/logging.go)            │
│  - GetLogTimeline(): 聚合 Log + timeline_events + 存量 JSON 字段  │
│  - SSE 端点：推送 processing→success/error 的活跃请求             │
└───────────────┬──────────────────────────────────────────────────┘
                │
                ▼
┌──────────────────────────────────────────────────────────────────┐
│  采集层 (plugins/logging) · 存储层 (framework/logstore)            │
│  - PreLLMHook / PostLLMHook 写入 timeline_events（按 log_id 关联）│
│  - 新增 timeline_events 表（SQLite + Postgres）                  │
└──────────────────────────────────────────────────────────────────┘
```

### 数据流

1. 客户端发起 LLM 请求 → `plugins/logging.PreLLMHook` 写入一条 `Log`（status=processing）并记录 PreLLM 阶段事件到 `timeline_events`。
2. 请求完成 → `plugins/logging.PostLLMHook` 更新 `Log`（status=success/error）并写入 PostLLM 阶段事件。
3. UI 打开 timeline 页：`GET /api/logs` 拉列表渲染 Gantt；订阅 `GET /api/logs/active/stream` 实时接收活跃请求变化；点击 bar → `GET /api/logs/{id}/timeline` 拉阶段事件。
4. `GetLogTimeline` 后端聚合 `timeline_events` + `RoutingEngineLogs` + `PluginLogs` + `AttemptTrail`（存量字段反序列化）为统一事件列表。

## API 设计（如有）

### GET /api/logs/{id}/timeline

获取单条日志的阶段时间线事件列表。

#### GET /api/logs/{id}/timeline - Response Body (200)

```json
{
  "log_id": "b8f2a1c3-...",
  "total_duration_ms": 1234.56,
  "events": [
    {
      "time_ms_offset": 0.0,
      "duration_ms": 8.2,
      "phase": "pre_llm",
      "source": "plugin_logging",
      "message": "pre-llm hook executed",
      "level": "info",
      "plugin_name": "logging"
    },
    {
      "time_ms_offset": 20.1,
      "duration_ms": 1100.0,
      "phase": "upstream_call",
      "source": "routing_engine",
      "message": "provider=ali model=qwen-max attempt=0",
      "level": "info",
      "plugin_name": ""
    },
    {
      "time_ms_offset": 20.1,
      "duration_ms": 0.0,
      "phase": "key_attempt",
      "source": "attempt_trail",
      "message": "key_id=xxxx status=success",
      "level": "info",
      "plugin_name": ""
    },
    {
      "time_ms_offset": 1128.0,
      "duration_ms": 6.5,
      "phase": "post_llm",
      "source": "plugin_logging",
      "message": "post-llm hook executed",
      "level": "info",
      "plugin_name": "logging"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `log_id` | string | 日志主记录 ID |
| `total_duration_ms` | float | 请求总耗时（ms） |
| `events[]` | array | 有序阶段事件（按 time_ms_offset 升序） |
| `events[].time_ms_offset` | float | 相对请求开始的偏移（ms） |
| `events[].duration_ms` | float | 该阶段耗时（ms），瞬时事件为 0 |
| `events[].phase` | string | 阶段名：tree 见下 |
| `events[].source` | string | 数据来源：`plugin_logging` / `routing_engine` / `plugin_logs` / `attempt_trail` |
| `events[].message` | string | 人类可读消息 |
| `events[].level` | string | 级别：info/warn/error |
| `events[].plugin_name` | string | 插件名（source=plugin_log* 时） |

`phase` 取值：`pre_llm`（PreLLMHook）、`post_llm`（PostLLMHook）、`upstream_call`（RoutingEngineLogs 中 provider 调用）、`key_attempt`（AttemptTrail 中 key 尝试）、`plugin_log`（PluginLogs 各条）。

#### GET /api/logs/{id}/timeline - Response Body (404)

```json
{
  "error": {
    "code": "log_not_found",
    "message": "log not found"
  }
}
```

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | log 存在，返回聚合事件 | 0 |
| 404 | log_id 不存在 | log_not_found |
| 500 | 内部错误 | internal_error |

### GET /api/logs/active/stream (SSE)

以 Server-Sent Events 推送活跃请求状态变化。客户端首次连接下发全量 processing 日志，此后在 Log 状态变化（processing→success/error 或新增 processing）时推送增量。

Content-Type: `text/event-stream`

#### 首个事件 (handshake) 定义

```
event: active_logs
data: [{"id":"...","status":"processing","provider":"ali","model":"qwen-max","latency_ms":null}, ...]
```

#### 后续事件（增量推送）

```
event: log_updated
data: {"id":"...","previous_status":"processing","status":"success","latency_ms":1234.0}
```

连接关闭（客户端断开）→ 服务端清理订阅并关闭 stream。无 4xx/5xx 业务错误（连接层面错误走 HTTP 基础设施）。

## 数据模型（如有）

### 新表 `timeline_events`（framework/logstore）

`Log` 主记录（一请求多 fallback 尝试 → 每次尝试一行 Log）下挂关联的阶段事件。

**SQLite / Postgres 共享列定义**（`framework/logstore/tables.go` 中新增 struct，`gorm` tag 对齐现有风格）：

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | string (UUID, PK) | 事件主键 |
| `log_id` | string (index) | 关联的 Log 主记录 ID |
| `phase` | string | 阶段：pre_llm / post_llm |
| `source` | string | `plugin_logging`（本期仅此来源） |
| `plugin_name` | string | 插件名 |
| `level` | string | info / warn / error |
| `message` | string | 消息文本 |
| `time_offset_ms` | float | 相对请求开始偏移（ms） |
| `duration_ms` | float | 阶段耗时（ms） |
| `timestamp` | time.Time | 事件发生时间 |

**迁移**：在 SQLite + Postgres logstore 的 schema 自动迁移列表中加入该表（对齐现有 `Log` / `MCPToolLog` 表的 AutoMigrate 模式）。本期不新增 `timeline_events` 的 ClickHouse 实现。

### 存量 JSON 字段复用（GetLogTimeline 读取）

- `Log.RoutingEngineLogs`（string，JSON `[]RoutingEngineLogEntry`）→ 上游 provider 决策事件
- `Log.PluginLogs`（string，JSON `map[string][]PluginLogEntry`）→ 各插件日志事件
- `AttemptTrailParsed`（`[]KeyAttemptRecord`）→ key 尝试事件

这三类在 `GetLogTimeline` 中反序列化后追加到 `events[]`，与 `timeline_events` 表行合并排序（按 `time_offset_ms`）。

## 组件设计（如有）

### UI 路由与组件

```
ui/app/workspace/logs/timeline/
├── layout.tsx              # createFileRoute（顶级路由，复用 logs 布局）
├── page.tsx                # 组合 LogsTimeline + 详情 Sheet
└── views/
    ├── logsTimeline.tsx    # 横向 Gantt 时间轴组件（核心）
    ├── timelineToolbar.tsx # follow/live/pan 模式切换、时间窗口、刷新
    └── timelineLegend.tsx  # 图例（bar 颜色 = 状态码）
```

- `logsTimeline.tsx`：核心交互（参考 omniroute `RequestTimeline.tsx`）——lane 分配、缩放平移、hover tooltip、bar 状态着色、SSE 合并、点击回调。
- 详情面板：**复用**现有 `sheets/logDetailsSheet.tsx` + `logDetailView.tsx`，在 `logDetailView.tsx` 内新增一个"Timeline"标签页区块，渲染 `GET /api/logs/{id}/timeline` 返回的事件流（纵向列表：时间偏移 + 阶段名 + 插件 + 消息 + 耗时）。
- 数据获取：复用现有 RTK Query logs API slice（`ui/lib/api/` 下现有 logs 相关 query），新增 timeline query + SSE 订阅 hook。

### 数据获取与状态

- Gantt 列表：现有 `GET /api/logs`（带分页/过滤），前端按时间窗口过滤。
- 活跃请求：新增 SSE hook，`EventSource` 订阅 `GET /api/logs/active/stream`，收到 `active_logs` / `log_updated` 后 merge 到本地 timeline 状态；关闭页面自动 `EventSource.close()`。
- 详情 timeline：新增 RTK Query 端点调用 `GET /api/logs/{id}/timeline`。

## 关键约束与契约

### 前置条件
- golang 环境可编译 core / framework / plugins / transports 模块（Go 1.26.1，go.work）。
- ui 模块可执行 `npm run build` + `npm run lint`（前端环境）。
- local 环境：`bifrost-api`（localhost:9080）+ `ui-dev`（localhost:3008）+ `logs.db`（SQLite）。
- `timeline_events` 表迁移 forward-only 不可回滚，发布前在 logs.db 上完整跑通一次 AutoMigrate。

### 影响面
- **新增表**：`timeline_events`（SQLite + Postgres）。
- **新增 handler 方法**：`LoggingHandler.GetLogTimeline()`、`LoggingHandler.GetActiveLogStream()`（SSE）。
- **新增路由**：`GET /api/logs/{id}/timeline`、`GET /api/logs/active/stream`。
- **新增采集**：`plugins/logging` 的 PreLLMHook / PostLLMHook 写入事件。
- **修改日志接口**：`framework/logstore` 的 `LogStore` 接口增加 `timeline_events` 相关读写方法（或独立 store 方法），需同步 SQLite / Postgres 两个实现。
- **是否破坏对外 API**：否——仅新增端点与表，不改现有端点语义。

### 性能契约
- `GET /api/logs/{id}/timeline`：单条 log 查询 + timeline_events 查询（按 log_id 索引）→ 常数级，禁止 N+1；单次响应 < 500ms。
- `GET /api/logs/active/stream`：限订阅者（仅 logs 页面打开时）；丢失更新策略为 `active_logs` 全量握手 + `log_updated` 增量；不全局常驻。
- 写入路径：`timeline_events` 写入与 `Log` 写入同一事务（plugins/logging 内），避免半写。

### 错误码与编号段
- 新增 `log_not_found`（GET /timeline 404）。沿用现有 `BifrostError` 结构，不新增独立错误码段。

### 环境限制与验证策略

> 依据 `.pg/changes/add-logs-timeline-page/env-description.yaml`（local 环境，logstore 为 SQLite）。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-framework-1 timeline_events 表+迁移 | ✅ | scenario + 单元测试 | n/a |
| V-framework-2 plugin 阶段点采集 | ✅ | scenario（发起请求→查表） | n/a |
| V-transports-1 /timeline 端点 | ✅ | scenario（curl + 校验 JSON） | n/a |
| V-transports-2 /active/stream SSE | ✅ | scenario（curl -N 订阅） | n/a |
| V-ui-1 Gantt 页面 | ✅ | scenario（浏览器冒烟）+ vitest | 多浏览器留待 CI |
| V-ui-2 fallback 链展开 | ✅ | scenario（构造 fallback 请求） | n/a |
| V-ui-3 UI build + lint | ⚠️ degraded | 单元测试（npm run build） | CI 完整门禁 |

> **degraded 说明（V-ui-3）**：环境 `bifrost-build` capability 已声明于 `{env.business_systems[name=bifrost-api]}`，但 `ui` 模块的 `npm run build` / `npm run lint` 依赖前端 toolchain 在本地 env 的可用性。若本地 node 环境不可用 → 降级为仅在 verify 阶段跑 `npm run build` + `npm run lint` 单元级校验，scenario 对 V-ui-3 标 @skip。

### 可观测性
- 关键日志点：`GetLogTimeline` 命中/未命中（WARN）、SSE 订阅建立/关闭（INFO）、`timeline_events` 写入失败（WARN，不阻断主请求）。
- 关键指标：无新增 Counter/Gauge（复用现有 logs 指标）。
- RequestId 追踪：`GET /api/logs/{id}/timeline` 复用请求 id 上下文，无需额外埋点。

## Verification Criteria

按 stages 顺序组织；V-* 编号与 define-summary 保持一致（三态契约，不重编号）。

### dev framework Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-framework-1 | timeline_events 表存在且 schema 正确 | 本地启动 bifrost-api（logs.db 为 SQLite） | 检查 logs.db 中 timeline_events 表结构 | 表存在，列与设计一致；AutoMigrate 无报错 |
| V-framework-2 | plugin 阶段点采集写入事件 | 发起一次 LLM 请求 | 查 timeline_events 表按 log_id 过滤 | 存在 pre_llm / post_llm 各至少 1 条，log_id 关联正确 |

### dev plugins Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| （plugins 采集行为由 framework 验证项覆盖，无独立新 V-*；详见 dev framework Verification Criteria） | | | | |

### dev transports Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | /api/logs/{id}/timeline 返回结构化事件 | 已有带阶段事件的 log | curl GET /api/logs/{id}/timeline | 200 + events[] 数组，含 timeline_events + RoutingEngineLogs + PluginLogs + AttemptTrail 聚合事件 |
| V-transports-2 | /api/logs/active/stream SSE 推送 | bifrost-api 运行中且正在处理请求 | curl -N 订阅 SSE 端点，期间发请求 | 收到 active_logs 握手 + 状态从 processing→success 的 log_updated |

### dev ui Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | Gantt 总览页面可访问并渲染 | logs 目录有数据 | 浏览器访问 localhost:3008/workspace/logs/timeline | 渲染 Gantt bar；点击 bar 打开 LogDetailSheet 且显示 Timeline 标签页 |
| V-ui-2 | fallback 链展开为多 bar | 构造带 fallback 的请求 | 浏览器查看 Gantt | 同原始请求的多条 bar 按 fallback_index 并排 |
| V-ui-3 | UI build + lint 通过 | node 环境 | npm run build + npm run lint | 无错误（degraded：见「环境限制」段） |

### int scr Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-scr-1 | 跨模块联调：发请求→采集→teline→Gantt 全链路 | local 环境全部服务就绪 | 浏览器 + curl 走完整流程 | 无报错、数据正确 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 本变更不动 core 引擎；插件采集在 plugins/logging 层做 |
| framework | ✅ | 新增 timeline_events 表 + LogStore 读写方法 |
| transports | ✅ | 新增 GetLogTimeline + SSE 端点 |
| cli | ❌ | CLI 不涉及 |
| plugins | ✅ | plugins/logging 增加 Pre/Post hook 事件采集 |
| ui | ✅ | 新增 timeline 路由 + Gantt 组件 + 详情 Timeline 标签 |
| scr | ✅ | 启用：跨 framework/plugins/transports/ui 联调场景 |

**affected_tracks**：framework, plugins, transports, ui
**scenario 决策**：scr = enabled（详见 on-conditions-eval.md）
