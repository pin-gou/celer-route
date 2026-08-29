# 调研与设计：让 LLM 在 RTK 截断后能取回原始 tool output

## 1. 当前现状（理解代码）

### 1.1 `[rtk:truncated N lines]` 是怎么产生的

Celer-route fork 自 Bifrost/OmniRoute 体系，已自带完整 RTK 插件（`plugins/rtk/`，约 15000 行 Go），关键文件：

- `plugins/rtk/compression.go` (958 行) — 顶层管道 `applyRtkCompression`
- `plugins/rtk/linedetector.go` (636 行) — `commandDetector.detect()`，按 content pattern 把 tool output 分类（`git-diff`/`pytest`/`go-build`/...）
- `plugins/rtk/linefilter.go` (210 行) — 应用 filter 内联规则（collapse/include/exclude）
- `plugins/rtk/smarttruncate.go` (126 行) — `applySmartTruncate()` 是**唯一**会产生 `[rtk:truncated N lines]` 标记的地方
- `plugins/rtk/rawoutput.go` (348 行) — `maybePersistRtkRawOutput()` 把原始 output 落盘
- `plugins/rtk/openai.go` + `anthropic.go` — 适配 OpenAI/Anthropic 消息格式

**关键判定**（`plugins/rtk/compression.go:540-551`）：

```go
// 4. Non-shell output is not compressed
if detection.Type == "" || detection.Type == "unknown" {
    return input, stats
}

// 5. Document-like read protection
isDocumentLikeRead := detection.Type == "shell" && detection.Command == "" && !hasGenericErrorMarkers(text)
if isDocumentLikeRead { /* skip filter + smartTruncate */ }
```

也就是说，**当 detector 命中任何非 shell 命令类型**（例如匹配到 `pytest`、`go-build`、`git-diff`），就走完整管道 → filter → smartTruncate → 出现 `[rtk:truncated N lines]` 标记。

我用 Go 子集 detector 实测了 proposal.md/design.md/tasks.md/scenario-scr.yaml 四个文件，**当前 linedetector.go 里 77 个 detector 都没有命中**（行内表格、列表、JSON 块都不匹配 `^On branch`、`^diff --git`、`^PASS` 之类的行首锚定 pattern）。它们最终走到 `{Type: "shell", Command: ""}` fallback → `isDocumentLikeRead = true` → 跳过 filter + smartTruncate，只受 `MaxCharsPerResult=12000` 限制（命中后插入 `[rtk:truncated by chars]`，不是 `[rtk:truncated N lines]`）。

**结论**：从我目前的实证来看，**这四个 `.pg/changes/home-free-tier-recommendation/*` 文件本身不会被 celer-route 端 RTK 截成 `[rtk:truncated N lines]`**。我看到的截断标记很可能是上游 LLM provider 自己的处理层（pg-router/pg-master 路由到的上游可能在入站/出站做了类似处理），或是 opencode 客户端层做的显示压缩，不是 celer-route 引起。

（用户认为"celer-route 也能产生"是合理的，因为只要 contentPatterns 集合里新加一个匹配 markdown 表格或代码块的 pattern，就会立刻触发。所以设计需要考虑"任何 detector 误命中"的兜底。）

### 1.2 现有"取回原文"基础设施（已经在了，但没接 LLM）

`.pg/changes/archive/2026-08-19-rtk-stage-4-raw-output-and-verify` 已经实现：

| 模块 | 位置 | 作用 |
|---|---|---|
| `maybePersistRtkRawOutput` | `plugins/rtk/rawoutput.go:117-214` | 把 tool output 落盘到 `tmp/rtk/raw-output/<ts>-<id>.log`，返回 `RtkRawOutputPointer{ID, Path, Bytes, SHA256}` |
| `RedactRtkRawOutput` | `plugins/rtk/rawoutput.go:84-...` | 落盘前对 OpenAI/Slack/AWS key + 凭据字段 + Auth header 做脱敏 |
| `ReadRtkRawOutput(id)` | `plugins/rtk/rawoutput.go:220-...` | 从 id 读回原文 |
| `BifrostContextKeyRTKRawOutputID` | `core/schemas/bifrost.go:470` | 服务端 context key，**不送 LLM** |
| HTTP `GET /api/context/rtk/raw-output/{id}` | `transports/celer-route-http/handlers/rtk.go:317-349` | 给运维 UI 用的 API，需要 auth |
| `applyRtkCompression` 设 ctx key | `plugins/rtk/hooks.go:171-173` | 截断后把指针 ID 写入 `BifrostContext`，供 logging 等下游插件消费 |

**配置开关**（当前默认）：

```json
{
  "raw_output_retention": "never",       // ← 现状：完全不落盘
  "raw_output_max_bytes": 1048576,        // 1MB 上限
  "max_chars_per_result": 12000,          // 字符硬上限，触发 [rtk:truncated by chars]
  "max_lines_per_result": 120,            // smartTruncate 行数阈值
  "intensity": "standard",
  "snapshot_mode": "off"                  // 截断前快照写到日志 metadata，不送 LLM
}
```

### 1.3 核心缺口

`BifrostContextKeyRTKRawOutputID` 只活在服务端 context；LLM 在 tool_result 里只看到 `[rtk:truncated N lines]`，**没有任何方式知道原文存在、存在哪个文件、用哪个 endpoint 取**。即使把 `raw_output_retention` 改成 `always`，LLM 仍然被蒙在鼓里。

## 2. 设计方案

按"投入产出比"和"最小变更"排序，给四个可选方案。请选择。

### 方案 A：raw_output_retention 默认改 always + 在压缩后内容里追加 pointer 提示

**改动**：
1. `plugins/rtk/config.go` 默认 `RawOutputRetention = RawOutputRetentionAlways`（原来是 `Never`）
2. `plugins/rtk/compression.go` 在 `applySmartTruncate` / `truncateToCharLimit` 之后，**当产生了 truncated 且有 rawOutputPointers 时**，在结果末尾追加一段提示文本，让 LLM 能看到：
   ```
   
   [rtk:truncated 142 lines; raw_output_id=abc123...def; fetch via `curl http://celer-route/api/context/rtk/raw-output/abc123...def`]
   ```
3. `transports/celer-route-http/server/server.go` 把 `GET /api/context/rtk/raw-output/{id}` 从 management auth 改成允许 **tool 调用的 bearer token**（不是 LLM，而是 LLM 客户端调用方的 auth header），或者额外允许在 tool call context 中注入。

**优点**：最小代码改动；LLM 立刻能自助取回原文。
**缺点**：每次截断都落盘（1MB × N 次请求），需要磁盘清理任务；bearer token 鉴权设计要小心（不能让 LLM 用任意 token 取任意 id）。
**适用**：所有 RTK 截断场景都受益，不仅是 shell 输出。

### 方案 B：raw_output_retention 默认改 always + LLM 工具列表里新增 `rtk_fetch_raw_output` 内置工具

**改动**：
1. 同方案 A 的 1
2. `core/schemas/provider.go` 注册一个**内置 tool**：`rtk_fetch_raw_output(raw_output_id: str) -> str`，返回对应 raw 文件内容
3. `core/bifrost.go` 在 `RegisterProvider` 之外注入这个内置 tool，所有 LLM 请求都自动看到这个工具
4. AGENTS.md 加一段规则：看到 `[rtk:truncated ... raw_output_id=...]` 时用 `rtk_fetch_raw_output(id)` 取回

**优点**：符合 LLM 原生工作流（tool call），鉴权在现有 tool 鉴权链路里；LLM 不需要 curl 之类的命令行；与现有 plugin 机制对齐。
**缺点**：要在 core 层注册内置 tool（侵入式）；tool schema 要考虑哪些 id 能访问（权限范围）；路径上需要把 RTK 的 pointer ID 注入到 tool 的 input schema description。

### 方案 C：只在压缩后追加 retention pointer，不实际落盘（先用 ctx 暴露给 LLM）

**改动**：
1. `core/schemas/bifrost.go` 把 `BifrostContextKeyRTKRawOutputID` 提升到 **system message 提示**（注入到下一次 system turn）
2. `plugins/rtk/hooks.go` 把 `ctx.Value(BifrostContextKeyRTKRawOutputID)` 注入到 LLM 的 system prompt 中：`"如果你刚才看到的 tool output 被 RTK 截断了，可用 curl 取回：..."`
3. **不实际落盘**——LLM 拿到 id 后端点会返回 404（这就是问题）

**结论**：行不通，因为没落盘就没原文可取。需要先落盘。

### 方案 D：所有方案叠加 + 一个明确的产品决策

**改动**：
1. `raw_output_retention` 默认 `always`（磁盘换能力）
2. `raw_output_max_bytes` 默认降低到 256KB（避免无限制落盘）
3. 加一个定期清理任务（保留 24h，超期删除）
4. `applySmartTruncate` / char limit 之后追加 `[rtk:truncated N lines; raw_output_id=...]` 提示
5. 加 `GET /api/context/rtk/raw-output/{id}` 的 tool-friendly 鉴权（只允许请求 LLM 的同 bearer 取同 request_id 范围内的 id）
6. 把 `raw_output_retention` 切换为 env-controlled（`RTK_RAW_OUTPUT=always|never|failures`），opencode 配置可关闭

**优点**：完整闭环，运维/产品可调；磁盘可控。
**缺点**：变更面最大；磁盘治理要单独的 cron / janitor。

### 我的推荐：**方案 A** + 后续可演进到方案 B

理由：
1. 改动面最小（3 个文件，~50 行）；不动 core/schemas，不动 provider 接口
2. LLM 立刻可用 `curl` / `Bash` tool 取回原文（opencode 自带 bash tool）
3. 与现有 `BifrostContextKeyRTKRawOutputID` 复用，机制对齐 OmniRoute 的 raw-output 端点设计
4. 未来要演化成"内置 tool"（方案 B）也只是把 `curl ...` 这一步替换成 tool call，原有指针仍然有效

## 3. 方案 A 的详细设计

### 3.1 代码改动清单

| 文件 | 改动 |
|---|---|
| `plugins/rtk/config.go` | `DefaultRawOutputRetention = RawOutputRetentionAlways`（默认值；仍可被 plugin config 覆盖） |
| `plugins/rtk/compression.go` | 在 `processRtkTextWithCommand` 末尾（约 line 593 之后），当 `truncated=true` 且 `stats.RawOutputPointers` 非空时，往 result 追加 `[rtk:truncated; raw_output_id=<id>; fetch=`GET /api/context/rtk/raw-output/<id>`]` |
| `plugins/rtk/compression.go` | 在 `isDocumentLikeRead` 路径（line 587）的 charlimit 截断后同样追加 pointer 提示 |
| `transports/celer-route-http/server/server.go` | 找到 raw-output 路由（line 101 of `handlers/rtk.go` 注册），auth 中间件改为可选（保留 management auth 用于运维 UI，新增 path：允许 LLM 请求的 bearer token 访问；或者直接复用现有 chat completions 的 auth） |
| `plugins/rtk/rawoutput.go` | 加 `ReadRtkRawOutputByRequestID(reqID, id) (string, bool)` 校验 raw-output 文件的 owner metadata 里记录的 request_id 是否匹配（避免跨请求取别人的 raw） |

### 3.2 触发条件

- 仅在 `truncated=true`（产生 `[rtk:truncated ...]` 标记）时追加 pointer 提示，避免每次都污染输出
- 仅在 `stats.RawOutputPointers[0]` 非空时（即 retention 不是 never 且落盘成功）追加
- charlimit 截断同样触发（`stats.RawOutputPointers[0]` 已经在 `maybePersistRawOutput` 里返回）

### 3.3 提示文本格式

LLM 友好的格式（避免污染 grep、单行可解析）：

```
\n[rtk:raw_output_id=<24hex>; fetch_with=curl_bearer]\n
```

具体内容可调，但要保证：
- 单行
- 不含换行符（避免再次触发智能分块）
- id 是 24-char lowercase hex
- 不暴露完整 URL（避免泄露部署路径）

LLM 在看到提示后，应当知道去用 `curl -H "Authorization: Bearer <其请求使用的 api-key>" <base-url>/api/context/rtk/raw-output/<id>` 取回原文。

### 3.4 鉴权方案

`GET /api/context/rtk/raw-output/{id}` 鉴权要回答一个问题：**谁能取哪个 raw 文件**？

最小可行方案：
- 接受任意有效 API key（和 chat completions 同 bearer）
- raw 文件的 `meta.json` 记录落盘时的 request_id（来自 `BifrostContextKeyRequestID`）
- 读取时验证调用方的 request_id 与文件落盘时的 request_id 一致（通过 request_id 作为第二因子）

更简单的方案（v1 推荐）：
- 接受任意有效 API key，但 raw 文件 24h 后自动清理（避免长期泄漏）
- 落盘时 `meta.json` 记录落盘时间 + request_id（审计用）
- LLM 不会跨请求取别人的文件（因为它没那个 id，且取回是无状态 curl）

### 3.5 数据生命周期

```
tmp/rtk/raw-output/<ts>-<id>.log + <ts>-<id>.meta.json
```

- 路径：`plugins/rtk/rawoutput.go` 写死 `cwd/rtk/raw-output/`
- 清理：加一个定时器（30 分钟一次）删除 24h 之前的文件
- 容量上限：每个文件 1MB；总文件数无上限，但 24h 滚动会自然约束

### 3.6 验证

| ID | 验证项 |
|---|---|
| V-rtk-1 | 配置 `raw_output_retention: always` 后，截断的 tool output 末尾包含 `[rtk:raw_output_id=<id>]` 提示；id 是 24-char hex |
| V-rtk-2 | 截断前 raw_output 已落盘到 `tmp/rtk/raw-output/`，文件大小 ≤ `raw_output_max_bytes` |
| V-rtk-3 | `curl GET /api/context/rtk/raw-output/<id>` 用 chat completions 的 bearer token 返回 200 + 完整原文 |
| V-rtk-4 | 截断前 raw_output 经过 `RedactRtkRawOutput` 脱敏（OpenAI key、Slack token、AWS key、Authorization header、api_key=xxx → [REDACTED_*]） |
| V-rtk-5 | 没截断的 tool output（<120 行 < 12000 字符且没命中 detector）不追加 pointer 提示、不落盘 |
| V-rtk-6 | 24h 之前的 raw_output 文件被 janitor 删除 |
| V-rtk-7 | 不存在的 id 返回 404；非法 id 返回 400 |

### 3.7 不破坏现有契约

- 既有 `raw_output_retention: never` 配置仍生效（显式覆盖默认）
- 不动 `isDocumentLikeRead` 文档保护
- 不动 `[rtk:truncated N lines]` / `[rtk:truncated by chars]` 标记文本（只是在其后再追加 pointer 提示）
- 不动 existing handler routes、auth 中间件架构

### 3.8 风险

| 风险 | 缓解 |
|---|---|
| 落盘耗 IO/磁盘 | 1MB/文件上限 + 24h 滚动 + janitor |
| secret 落盘泄漏 | `RedactRtkRawOutput` 已有 5 条规则覆盖；可加 v2：把 raw 文件 fs 权限设为 0600 |
| LLM 看到 pointer id 后尝试越权 | id 仅在产生 truncated 时暴露；id 是 sha256 不可枚举；落盘内容有脱敏 |
| 性能：`maybePersistRawOutput` 在每次截断时写盘 | 已经在失败路径上有 benchmark；1MB 写盘 < 5ms |
| 路径 tmp/rtk/raw-output/ 在容器里失效 | 用 `os.TempDir()` 或环境变量 `RTK_RAW_OUTPUT_DIR`（参考 OmniRoute 的 `DATA_DIR`） |

### 3.9 升级路径

v1 (方案 A) 落地后，LLM 还需要手动写 `curl` 命令。可以后续演化为方案 B：在 `core/schemas` 注册内置 tool `rtk_fetch_raw_output(id: str) -> str`，把 `curl` 那一步包成 tool call，保留同样的 pointer 机制。

## 4. 待你决策的点

1. **范围**：方案 A、B、C、D 选哪个？（我推荐 A）
2. **鉴权**：raw-output 端点用什么鉴权？我建议"复用 chat completions 的 bearer"，加落盘时间自动清理
3. **默认行为**：是否把 `raw_output_retention` 默认从 `never` 改成 `always`？还是只加端点、把开关留给运维手动开？
4. **磁盘路径**：`tmp/rtk/raw-output/` 还是 `os.TempDir()` 还是 env-controlled `RTK_RAW_OUTPUT_DIR`？
5. **清理周期**：24h？1h？可配？
6. **是否需要 E2E**：写 Playwright 验证"截断后能看到 pointer id + curl 取回"的端到端路径？还是只单测？

确认这些后我可以开始落 proposal/design/tasks。