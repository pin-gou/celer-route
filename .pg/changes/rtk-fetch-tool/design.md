# rtk-fetch-tool 设计

## 架构概览

```
┌────────────────────────────────────────────────────────────────────────────┐
│                        Gateway 启动 (一次)                                  │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ transports/celer-route-http/server/plugins.go: Init RTK plugin             │
│                                                                            │
│   p := rtk.NewPlugin(config, logger, ...)                                   │
│     └─ rtk/rtk.go NewPlugin                                                 │
│          └─ bifrost.GetMCPManager().RegisterTool(                          │
│               "rtk_fetch_raw_output",                                       │
│               description,                                                  │
│               rtk.RawOutputReadHandler,  ← Go handler                       │
│               schemas.RtkFetchRawOutputTool, ← JSON Schema                  │
│             )                                                               │
│             └─ core/mcp/clientmanager.go:1736 RegisterTool                  │
│                  ├─ setupLocalHost() → 启动 in-process MCP stdio server    │
│                  ├─ ToolMap["bifrostInternal-rtk_fetch_raw_output"]         │
│                  │     = schema  ← 加 prefix (line 1770)                    │
│                  └─ 把 handler 注册到本地 MCP server                        │
└────────────────────────────────────────────────────────────────────────────┘


┌────────────────────────────────────────────────────────────────────────────┐
│                     每次 Chat Completion 请求                              │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ① HTTP handler:                                                            │
│    transports/celer-route-http/handlers/inference.go:1013 chatCompletion   │
│    prepareChatCompletionRequest → BifrostChatRequest{ Tools: client.tools }│
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ② LLMPlugin.PreLLMHook chain                                               │
│                                                                            │
│   [RTK plugin] plugins/rtk/hooks.go:73                                     │
│   ┌────────────────────────────────────────────────────────────────────┐  │
│   │ 1. 若 p.config.Enabled && p.config.InjectFetchTool:                │  │
│   │    - bifrost.GetMCPManager().GetToolPerClient(ctx) 看有无 tool     │  │
│   │    - 若有 + req.ChatRequest.Params.Tools 不含 → append              │  │
│   │    - req.ResponsesRequest.Params.Tools 同样处理                    │  │
│   │ 2. injectRtkRecoveryHint(ctx, req)  ← 改写后的 hint (主路径 tool)  │  │
│   │ 3. applyRtkCompression(...)           ← 既有逻辑, 行为不变         │  │
│   └────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ③ Provider Queue → Provider API Call                                       │
│    工具列表里多一项 bifrostInternal-rtk_fetch_raw_output, LLM 可见            │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ④ LLM 响应: 含 tool_calls [{name="bifrostInternal-rtk_fetch_raw_output"}]  │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ⑤ bifrost.go:951 CheckAndExecuteAgentForChatRequest                        │
│                                                                            │
│    agent.go:executeAgent → 拆分 tool_calls:                                 │
│    ┌──────────────────────────────────────────────────────────────────┐    │
│    │ clientManager.GetClientForTool(name)  → bifrostInternal 客户端    │    │
│    │   state.State = healthy (因 RegisterTool 后已 healthy)             │    │
│    │   → 归入 autoExecutableTools                                     │    │
│    └──────────────────────────────────────────────────────────────────┘    │
│                                                                            │
│    并行执行 (agent.go:301) → mcpRequest = {ChatAssistantMessageToolCall}    │
│    → bifrost.MCPManager.ExecuteChatTool(ctx, toolCall)                     │
│    → executeToolWithHooks → prepareToolExecution                            │
│    → ToolsManager.ExecuteTool → executeToolInternal (toolmanager.go:650)    │
│    → handler(*context.Context, args) → (string, error)                      │
│      ┌────────────────────────────────────────────────────────────────┐    │
│      │ plugins/rtk/rawoutput.go: RawOutputReadHandler                 │    │
│      │   1. IsValidRawOutputID(args.ID) — 24-char hex 校验            │    │
│      │   2. ReadRtkRawOutputByID(id, p.appDir) — 读文件               │    │
│      │   3. WrapRawOutputForHTTP(data, id, len, "")  ← sentinel 包裹 │    │
│      │   4. return body, nil                                          │    │
│      └────────────────────────────────────────────────────────────────┘    │
│    → tool_result 注入 conversation                                          │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ⑥ LLM 第二轮调用 (带 tool_result)                                          │
│                                                                            │
│    plugins/rtk/hooks.go PreLLMHook                                         │
│    └─ processRtkTextWithCommand → StripRawOutputSentinel (compression.go:557)│
│         └─ 识别 sentinel → stats.Techniques += "rtk-raw-output-bypass"     │
│         └─ body 透传 (字节级 == 磁盘原文)                                  │
│                                                                            │
│    LLM 看到完整原文 + 无新 [rtk:raw_output_id=] marker → 任务完成         │
└────────────────────────────────────────────────────────────────────────────┘
```

**涉及模块**：

| 模块 | 路径 | 改动 |
|------|------|------|
| **core** | `core/bifrost.go` | 导出 `GetMCPManager()` 方法 |
| **plugins/rtk** | `plugins/rtk/tool_schema.go`（新） | `RtkFetchRawOutputTool schemas.ChatTool` |
| **plugins/rtk** | `plugins/rtk/rawoutput.go` | 新增 `RawOutputReadHandler` Go 函数 |
| **plugins/rtk** | `plugins/rtk/rtk.go` | `NewPlugin` 注册 tool |
| **plugins/rtk** | `plugins/rtk/hooks.go` | PreLLMHook 注入 tool schema |
| **plugins/rtk** | `plugins/rtk/config.go` | `InjectFetchTool` 字段 |
| **plugins/rtk** | `plugins/rtk/hint.go` | 改写 `rtkRecoveryHintText` |
| **transports** | `transports/config.schema.json` | RTK 块增 `inject_fetch_tool` 字段 |
| **transports** | `transports/celer-route-http/server/plugins.go` | 不动（RTK plugin 已在 Init 路径） |
| **E2E** | `tests/e2e/api/collections/provider-harness.json` | 新增 tool-call bypass 用例 |

## API 设计

### MCP Tool 形态

`RegisterTool` 接受的 Go 函数签名是 `MCPToolFunction[any]`，即 `func(args any) (string, error)`。handler 自行做 JSON unmarshal。

```go
// RawOutputReadArgs matches the JSON schema declared in RtkFetchRawOutputTool.
type RawOutputReadArgs struct {
    ID string `json:"id"`
}

// RawOutputReadHandler is the Go handler backing the rtk_fetch_raw_output MCP tool.
// It reads <appDir>/rtk/raw-output/<unix-ms>-<slug>-<id>.log, wraps the body in
// the RTK raw-output sentinel so the next RTK compression pass strips it back
// out (anti-recursion), and returns the wrapped body. Same wire shape as the
// LLM-bound response of /api/context/rtk/raw-output/{id} (handlers/rtk.go:386).
//
// Authorization: today the endpoint has no per-request_id isolation — anyone
// with the 24-hex id can read. The tool inherits the same surface; adding
// owner isolation is a separate task (Phase 2).
func (p *Plugin) RawOutputReadHandler(ctx context.Context, args any) (string, error) {
    var a RawOutputReadArgs
    if err := mapstructure.Decode(args, &a); err != nil {
        return "", fmt.Errorf("invalid args: %w", err)
    }
    if !IsValidRawOutputID(a.ID) {
        return "", fmt.Errorf("invalid id %q: must be 24 lowercase hex characters", a.ID)
    }
    data, found := ReadRtkRawOutputByID(a.ID, p.AppDir())
    if !found {
        return "", fmt.Errorf("raw output %q not found or expired (24h TTL)", a.ID)
    }
    return WrapRawOutputForHTTP(data, a.ID, len(data), ""), nil
}
```

`mapstructure` 是 stdlib-equivalent 的 JSON 解析，handler 不需要 import；这里用 `encoding/json`（已在 rawoutput.go:1 引入）。

```go
func (p *Plugin) RawOutputReadHandler(ctx context.Context, args any) (string, error) {
    argsMap, ok := args.(map[string]any)
    if !ok {
        return "", fmt.Errorf("invalid args type: %T", args)
    }
    id, _ := argsMap["id"].(string)
    if !IsValidRawOutputID(id) {
        return "", fmt.Errorf("invalid id %q: must be 24 lowercase hex characters", id)
    }
    data, found := ReadRtkRawOutputByID(id, p.AppDir())
    if !found {
        return "", fmt.Errorf("raw output %q not found or expired (24h TTL)", id)
    }
    return WrapRawOutputForHTTP(data, id, len(data), ""), nil
}
```

### Tool Schema

`RtkFetchRawOutputTool` 是 byte-stable 常量（满足 Anthropic / OpenAI prompt cache 命中）。`function.name` 字段必须已经是**带 prefix 的完整名** `bifrostInternal-rtk_fetch_raw_output`，因为 PreLLMHook 注入到 `req.Params.Tools` 时不再二次包装。

```go
// plugins/rtk/tool_schema.go
package rtk

import "github.com/pin-gou/celer-route/core/schemas"

const rtkFetchRawOutputToolName = "bifrostInternal-rtk_fetch_raw_output"

var rtkFetchRawOutputToolDescription = strings.Join([]string{
    "Reads the original (untruncated) output of a tool_result that the RTK",
    "compression plugin previously truncated. Use this when you see a marker",
    "like [rtk:raw_output_id=<24hex>; orig=<size>; ...] at the end of a",
    "tool_result and you need the full content. The 24-char hex id is the",
    "value after raw_output_id=. The body is automatically unwrapped by RTK",
    "on the next request so the response you receive is the raw file content.",
}, " ")

// RtkFetchRawOutputTool is the schemas.ChatTool declaration for the
// rtk_fetch_raw_output MCP tool. Exposed to the LLM as a regular function
// in the chat request's tools= array. The function name carries the
// bifrostInternal- prefix because MCPManager.RegisterTool (clientmanager.go:1770)
// prefixes the tool name with BifrostMCPClientKey when storing it.
//
// Byte-stable: keep this struct literal unchanged across releases so
// Anthropic and OpenAI prompt caches still hit on the system prefix when
// the same tool schema is appended to req.Params.Tools.
var RtkFetchRawOutputTool = schemas.ChatTool{
    Type: "function",
    Function: schemas.ChatToolFunction{
        Name: rtkFetchRawOutputToolName,
        Description: rtkFetchRawOutputToolDescription,
        Parameters: schemas.ChatToolFunctionParameters{
            Type: "object",
            Properties: map[string]any{
                "id": map[string]any{
                    "type":        "string",
                    "description": "24-char lowercase hex id from the [rtk:raw_output_id=...] marker",
                    "pattern":     "^[0-9a-f]{24}$",
                },
            },
            Required: []string{"id"},
        },
    },
}
```

### `core/bifrost.go` GetMCPManager

```go
// GetMCPManager returns the MCP manager used by this Bifrost instance.
// Plugins can use this to register internal tools (e.g. the RTK raw-output
// fetch tool) or to register client-provided MCP servers. Returns nil when
// the Bifrost was constructed without an MCP manager (rare — e.g. unit
// tests that bypass MCP entirely).
func (bifrost *Bifrost) GetMCPManager() *mcp.MCPManager {
    return bifrost.MCPManager
}
```

`mcp` 包已经是 bifrost.go 的依赖（`core/mcp`），不需要新增 import 路径。

## 组件设计

### 1. RTK plugin `NewPlugin` 注册 tool

```go
// plugins/rtk/rtk.go
func NewPlugin(config Config, logger schemas.Logger, bifrost *schemas.Bifrost) (*Plugin, error) {
    applyConfigDefaults(&config)

    p := &Plugin{
        config:  &config,
        logger:  logger,
        bifrost: bifrost,   // 新增字段
    }

    // ... 既有初始化 ...

    // Register the raw-output fetch tool with MCPManager so the LLM can call
    // it via tool call instead of issuing a plain GET. No-op when the plugin
    // is disabled, when InjectFetchTool is off, or when no MCPManager is
    // configured (e.g. unit tests).
    if config.Enabled && config.InjectFetchTool {
        if mgr := bifrost.GetMCPManager(); mgr != nil {
            if err := mgr.RegisterTool(
                "rtk_fetch_raw_output", // 内部名（无 hyphen）
                rtkFetchRawOutputToolDescription,
                p.rawOutputReadHandlerMCPTool, // MCPToolFunction[any] adapter
                RtkFetchRawOutputTool,         // 已带 prefix 的 schema
            ); err != nil {
                p.logger.Warn("rtk", "failed to register rtk_fetch_raw_output MCP tool: %v", err)
            }
        }
    }

    return p, nil
}

// rawOutputReadHandlerMCPTool adapts RawOutputReadHandler to MCPToolFunction[any].
// The MCP layer passes args as map[string]any (deserialized from JSON Schema).
func (p *Plugin) rawOutputReadHandlerMCPTool(args any) (string, error) {
    return p.RawOutputReadHandler(context.Background(), args)
}
```

### 2. PreLLMHook tool schema 注入

```go
// plugins/rtk/hooks.go PreLLMHook 末尾追加
if p.config.Enabled && p.config.InjectFetchTool && p.bifrost != nil {
    if mgr := p.bifrost.GetMCPManager(); mgr != nil {
        // Source of truth: 询问 MCP manager 是否有这个 tool
        // (多实例 / 多 worker / 测试场景都安全)
        if p.mcpManagerHasFetchTool(mgr) {
            p.injectFetchToolSchema(req)
        }
    }
}

// mcpManagerHasFetchTool returns true when the tool is currently registered
// in the manager's tool map (under its prefixed name). Avoids duplicating
// the registration list in the plugin.
func (p *Plugin) mcpManagerHasFetchTool(mgr MCPManagerLike) bool {
    for _, tools := range mgr.GetToolPerClient(context.Background()) {
        for _, t := range tools {
            if t.Function.Name == rtkFetchRawOutputToolName {
                return true
            }
        }
    }
    return false
}

// injectFetchToolSchema appends RtkFetchRawOutputTool to req.Params.Tools
// when not already present. Idempotent: a no-op when the schema is already
// in the list (e.g. client already supplied it).
func (p *Plugin) injectFetchToolSchema(req *schemas.BifrostRequest) {
    switch req.RequestType {
    case schemas.ChatCompletionRequest, schemas.ChatCompletionStreamRequest:
        if req.ChatRequest == nil || req.ChatRequest.Params == nil {
            return
        }
        if !hasTool(req.ChatRequest.Params.Tools, rtkFetchRawOutputToolName) {
            req.ChatRequest.Params.Tools = append(req.ChatRequest.Params.Tools, RtkFetchRawOutputTool)
        }
    case schemas.ResponsesRequest, schemas.ResponsesStreamRequest:
        if req.ResponsesRequest == nil || req.ResponsesRequest.Params == nil {
            return
        }
        if !hasTool(req.ResponsesRequest.Params.Tools, rtkFetchRawOutputToolName) {
            req.ResponsesRequest.Params.Tools = append(req.ResponsesRequest.Params.Tools, RtkFetchRawOutputTool)
        }
    }
}

func hasTool(tools []schemas.ChatTool, name string) bool {
    for _, t := range tools {
        if t.Function.Name == name {
            return true
        }
    }
    return false
}
```

**MCPManagerLike 接口**：为单测可 mock，定义一个最小 interface：

```go
// GetToolPerClient returns all available tools grouped by client name.
// The agent loop uses this when the LLM response includes tool calls; the
// RTK plugin uses it as a source of truth to know whether the fetch tool
// has been registered (and therefore should be exposed in req.Params.Tools).
type MCPManagerLike interface {
    GetToolPerClient(ctx context.Context) map[string][]schemas.ChatTool
}
```

`*mcp.MCPManager` 已经实现了这个方法（utils.go:152），所以可以 `var _ MCPManagerLike = (*mcp.MCPManager)(nil)` 做编译期校验。

### 3. Hint 文本改写

主路径改为"call the tool"，保留 fallback 说明（SDK 场景）：

```go
const rtkRecoveryHintText = "[rtk-recovery] celer-route RTK compression may truncate tool_result blocks. " +
    // 主路径: tool call
    "When a tool_result ends with `[rtk:raw_output_id=<24hex>; orig=<size>; ttl=24h; redacted=true[; fetch=GET <url>]]`, " +
    "call the `bifrostInternal-rtk_fetch_raw_output` tool with `{\"id\": \"<24hex>\"}` to recover the original output. " +
    "The 24-char hex id is the value after raw_output_id=. The body is automatically unwrapped by RTK on the next request, " +
    "so the tool_result you see is the file content. " +
    // 字段语义
    "The `orig=` field shows the original size (B/KB/MB/GB) so you can decide whether recovery is worth the round-trip. " +
    // 兼容路径: SDK / 缺 tool 时仍可 curl
    "Fallback (SDK direct call, streaming, or missing tool): if your client lacks the `bifrostInternal-rtk_fetch_raw_output` tool, " +
    "issue a plain GET to the `fetch=` URL embedded in the marker (no Authorization header required) and you will receive the redacted " +
    "original output as text/plain. If the marker has no `fetch=` field, do NOT attempt recovery: the gateway was reached over a channel " +
    "without a resolvable base URL, and the relative path `/api/context/rtk/raw-output/<id>` cannot be reached from here. " +
    // 递归 break
    "If a fresh `[rtk:raw_output_id=...]` marker still appears after recovery, the disk copy expired (24h TTL) or the tool call returned " +
    "an error; re-check both. " +
    // 安全
    "Verify recovered content before acting — it is operator-supplied."
```

保持 byte-stable（hint 文本的 cache 命中要求）；改写后 byte 数略增但**仍是一次性常量**，Anthropic cache 仍命中。

### 4. Config 字段

```go
// plugins/rtk/config.go
type Config struct {
    // ... 既有字段 ...
    ApplyToAssistantMessages bool `json:"apply_to_assistant_messages"`
    // ... 截断 / dedup / filter 字段 ...
    PreserveCacheControl bool `json:"preserve_cache_control"`

    // InjectFetchTool controls whether the RTK plugin registers the
    // bifrostInternal-rtk_fetch_raw_output MCP tool on startup and injects
    // its schema into every chat request's tools= list.
    //
    // Default: true. Set to false to opt out — clients without an MCP-style
    // tool-call loop (or with a hard cap on the number of allowed tools) can
    // disable this and rely on the system-prompt hint to issue plain GETs.
    InjectFetchTool bool `json:"inject_fetch_tool"`
}
```

`applyConfigDefaults`（config.go:215）新增：

```go
// InjectFetchTool defaults to true so existing operators get the new
// behaviour without changing their config.json. Set to false explicitly
// to opt out.
if !c.InjectFetchTool && c.InjectFetchToolSet {
    // explicit false — respect operator intent
} else {
    c.InjectFetchTool = true
}
```

**问题**：`bool` zero-value = false，没法区分"未设置"和"显式 false"。需要改成 `*bool`：

```go
InjectFetchTool *bool `json:"inject_fetch_tool,omitempty"`
```

ApplyDefault：

```go
if c.InjectFetchTool == nil {
    t := true
    c.InjectFetchTool = &t
}
```

### 5. `transports/config.schema.json`

定位 rtk 块（参考 add-rtk-compression-plugin 落地后的 schema），追加：

```json
"inject_fetch_tool": {
    "type": "boolean",
    "default": true,
    "description": "When true, the rtk plugin registers an MCP tool (bifrostInternal-rtk_fetch_raw_output) and injects its schema into every chat request so the LLM can recover truncated tool results via a tool_call instead of a plain HTTP GET."
}
```

## 状态机

### Tool 生命周期

```
[Gateway boot]
   ↓
[NewPlugin] 检测 Enabled && InjectFetchTool
   ↓
[bifrost.GetMCPManager().RegisterTool(...)]
   ↓  失败: log warn, 继续
[Tool available: bifrostInternal-rtk_fetch_raw_output]

[每次 chat request]
   ↓
[PreLLMHook] 检测 p.config.Enabled && InjectFetchTool
   ↓
[mgr.GetToolPerClient] 是否含此 tool
   ↓ yes
[req.Params.Tools append]  ← idempotent

[LLM 调 tool_call]
   ↓
[agent.go:executeAgent → autoExecutableTools]
   ↓
[executeToolWithHooks → prepareToolExecution]
   ↓ state.State == healthy
[executeToolInternal → handler]
   ↓
[handler 返回 sentinel-wrapped body]
   ↓
[tool_result 注入 conversation, loop LLM]

[下一轮 PreLLMHook]
   ↓
[processRtkTextWithCommand → StripRawOutputSentinel]
   ↓ 识别 sentinel
[body 透传, 不再压缩]
```

### 错误状态

| 触发点 | 行为 | 用户感知 |
|---|---|---|
| `RegisterTool` 失败（已注册/重名） | `p.logger.Warn`, 跳过注册 | 启动日志 warn; tool 不可用, hint 提示用户 curl |
| `RegisterTool` 失败（MCPManager nil） | 静默 no-op | tool 不可用, hint 提示用户 curl |
| handler 收到非法 id | 返回 `error` | LLM tool_result 是 error message, LLM 可重试或放弃 |
| handler 读文件失败（不存在/过期） | 返回 `error` | LLM tool_result 是 error message, 含 "24h TTL" 提示 |
| handler panic | 冒泡到 goroutine 顶层 | fasthttp 500; 但 agent loop 会把这个 error 反馈给 LLM, 通常不致命 |
| PreLLMHook 注入时 Tools 已含此 tool | 跳过 | 幂等, 无副作用 |
| PreLLMHook 时 `req.ChatRequest.Params == nil` | 跳过 | 老 client 不会崩, 不会注入 |

## 行为约束

1. **byte-stable schema**：`RtkFetchRawOutputTool` 是编译期常量。任何 release 都不应修改其 JSON 序列化结果（Anthropic cache key 依赖）。
2. **handler 幂等**：相同 id 多次调用 → 同样结果（直到 janitor 回收）。
3. **handler 不会修改 ctx**：纯函数 `(ctx, args) → (string, error)`。
4. **PreLLMHook idempotent**：重复注入时基于 `hasTool` 跳过。
5. **schema 注入顺序无关**：append 到末尾，LLM 看到 list 顺序由 client 决定。
6. **handler 错误格式**：`fmt.Errorf` 输出，MPG 层映射为 `mcp.NewToolResultError`（已有逻辑）。

## 配置变更

### `transports/config.schema.json`

新增 `inject_fetch_tool`（RTK 块下）：

```json
{
  "type": "boolean",
  "default": true,
  "description": "..."
}
```

### 环境变量（如需要）

无。RTK 配置走 config.json，不走 env（与既有 `enabled` / `intensity` 等一致）。

## 兼容性 / 迁移

### Backward compat

- 旧 config.json 不写 `inject_fetch_tool`：`*bool == nil` → default `true` → 新行为生效，**对所有现有 operator 是 additive change**
- `RegisterTool` 失败但启动成功：tool 不可用，hint 提示 curl（退化为方案 A）
- hint 文本 byte-stable（虽然有改写）：Anthropic cache 第一次会 miss，后续命中；OpenAI 端无 prefix cache

### Forward compat

- 如果未来要新增更多"内置 utility tool"（如 `rtk_search` / `rtk_stats`），复用 `p.bifrost.GetMCPManager().RegisterTool(...)` 模式；`injectFetchTool` 可拆为 `InjectTools []string`
- 鉴权加固（per-request_id isolation）做在 handler 里，schema 不变

## 演进路径

### Phase 2（本次不做）

- **per-request_id 隔离**：handler 校验 ctx.BifrostContextKeyRequestID == 文件 header 里记录的 request_id。需要修改 `persistRawOutput` 落盘时写入 request_id（文件 header 第一行）。
- **UI banner**：gateway 启动时如 `inject_fetch_tool=true` 且有 tool 注入到 chat，在 workspace 顶部加小提示
- **tool stats 暴露**：通过现有 `/api/context/rtk/stats` 增 `fetch_tool_invocations` 字段
- **tool_choice 友好**：当 hint 文本触发时，临时 `tool_choice={"type":"function","function":{"name":"bifrostInternal-rtk_fetch_raw_output"}}` 强制 LLM 调用（不需要本次）

### Phase 3（远期）

- **多 tool 化**：`rtk_compress` / `rtk_redact` / `rtk_stats` 等；新增 `InjectTools []string` 配置
- **inline tool result**：MCP 协议原生支持的"直接在 tool_call response 里返回原值"——可绕过一轮 LLM，省 token

## 验收映射

| V-* | 在 tasks.md 的位置 | 验证方式 |
|---|---|---|
| V-plugins-1 | `dev.plugins:test` §1.1-1.4 | 单元测试（id 校验、文件存在/缺失、sentinel 包裹） |
| V-plugins-2 | `dev.plugins:test` §1.5-1.6 | 单元测试（schema 字段、JSON 合法） |
| V-plugins-3 | `dev.plugins:test` §1.7-1.9 | 单元测试（注入分支、opt-out 分支） |
| V-plugins-4 | `dev.plugins:test` §1.10 | hint_test.go 字符串包含断言 |
| V-plugins-5 | `int.scr:scenario-execute` §16 | E2E provider-harness 集成用例 |
| V-core-1 | `dev.core:test` §1.1 + `dev.core:dev` §2.1 | 单测 + 编译 |
| V-plugins-7 | `dev.plugins:test` §1.11 | handler 输出 vs HTTP 端点输出 byte-equal |