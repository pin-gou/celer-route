# rtk-fetch-tool

**关联 issue / 草案**：`.pg/changes/_draft-rtk-llm-raw-output-recovery.md`（方案 B 落地）
**变更类型**：feature
**前置变更**：`2026-08-18-add-rtk-compression-plugin`（已落地） + `2026-08-18-rtk-stage1-markers-and-doc-protection`（raw-output 落盘 + sentinel bypass 已落地）

## 背景

`plugins/rtk/` 当前通过**两条机制**让 LLM 在收到 RTK 截断后的 tool_result 时拿回原文：

1. **System prompt 注入**（`plugins/rtk/hint.go:19` 的 `rtkRecoveryHintText`）—— 告诉 LLM 通过 `curl` / `Bash` tool 调 `GET /api/context/rtk/raw-output/{id}` 取回原文。
2. **Sentinel 透传**（`plugins/rtk/compression.go:557` 的 `StripRawOutputSentinel`）—— LLM 调完 fetch 后，结果自带 `\x00RTK_RAW_OUTPUT_BEGIN\x00...` 前缀，再次进入 RTK 压缩 pipeline 时被识别并**整体透传**，避免"fetch → 压缩 → 再截断 → 再 fetch"的死循环。

机制 (1) 有一个**关键短板**：依赖 LLM 客户端拥有 `Bash` / `curl` 类 HTTP tool。多数 agent 都有，但**纯 MCP-only agent 或最小化 SDK 直连场景没有**——它们看到 `[rtk:raw_output_id=...]` 标记后无能为力，被迫接受被截断的 tool_result。

设计草案 `.pg/changes/_draft-rtk-llm-raw-output-recovery.md` §方案 B 已经识别并规划了解决方案：在 core 层注册内置 MCP tool `rtk_fetch_raw_output(id)`，**gateway 自动在每次 chat request 的 `tools=` 列表里塞入这个 tool 的 schema**。LLM 看到 tool 后，直接发出 tool_call → gateway 在 MCP agent loop 内执行（不需要外部客户端工具）→ 用 sentinel 包好返回 → 透传。

本次变更实施该方案。

## 目标

让任何 LLM 客户端（含无 Bash tool 的 MCP-only agent）都能通过 gateway **内置的 tool call** 取回 RTK 截断的原文，对 agent **完全透明**：

- gateway 在 `PreLLMHook` 中自动向 `req.ChatRequest.Params.Tools` 注入 `bifrostInternal-rtk_fetch_raw_output` schema
- gateway 启动时通过 `MCPManager.RegisterTool` 注册该 tool，handler 直接读 `<appDir>/rtk/raw-output/<id>.log` 并返回 sentinel-wrapped body
- LLM 调 tool_call 时，gateway 沿用现有 MCP agent loop 自动执行（`autoExecutableTools` 路径）
- handler 返回的 body 与现有 `GET /api/context/rtk/raw-output/{id}` HTTP 端点的 LLM-bound 响应**字节级一致**（同 sentinel 包装）
- 复用现有 sentinel bypass：`processRtkTextWithCommand` 在 `compression.go:557` 识别 sentinel 后透传，无需新增代码
- system prompt 提示词从 "issue a plain GET" 改为 "call the bifrostInternal-rtk_fetch_raw_output tool"，但保留 fallback 说明（SDK 直接调用场景）

## 非目标

- **不动** raw-output 端点的鉴权模型（保持"24-char hex id 即凭据"现状，与 `/api/context/rtk/raw-output/{id}` 一致）。加固到 per-request_id 隔离留作 Phase 2。
- **不动** streaming 路径下的 tool call 自动执行（`ChatCompletionStreamRequest` 不进 agent loop）。hint 文本保留 fallback 说明，streaming 结束后客户端可通过非 stream 请求调用 tool。
- **不动** SDK 直接调用 `bifrost.ChatCompletionRequest` 的路径。SDK 用户不走 agent loop，tool 不会自动执行；hint 文本提示他们用 `curl`。
- **不**在 `tools=` 列表里塞入 RTK 之外的"内置 utility tool"（如文件读、写、执行等）。本次仅 RTK fetch 一个 tool。
- **不**支持 Anthropic prompt cache 的 tool 列表差异化（Anthropic cache key 是 prefix 字节相等；tool schema 是 byte-stable 字符串，已可命中）。

## 范围

### 包含

- `core/bifrost.go`：导出 `GetMCPManager()` 公开方法（plugin 可访问）
- `plugins/rtk/rawoutput.go`：新增 `RawOutputReadHandler(ctx, args) (string, error)` MCP handler 函数
- `plugins/rtk/tool_schema.go`（新文件）：定义 `RtkFetchRawOutputTool schemas.ChatTool` JSON Schema
- `plugins/rtk/rtk.go`（`NewPlugin`）：通过 `MCPManager.RegisterTool` 注册 handler + schema
- `plugins/rtk/hooks.go`（`PreLLMHook`）：向 `req.ChatRequest.Params.Tools`（和 `req.ResponsesRequest.Params.Tools`）注入 tool schema，保持与 MCP manager 同步
- `plugins/rtk/hint.go`：改写 `rtkRecoveryHintText`，主路径改为"call the tool"，保留 fallback 文本（SDK 场景）
- `plugins/rtk/config.go`：新增 `InjectFetchTool bool` 配置项（默认 `true`），允许 opt-out
- `transports/config.schema.json`：增 RTK 块 `inject_fetch_tool` 字段
- `plugins/rtk/rawoutput_test.go`：新增 handler 单元测试（id 格式校验、文件存在/缺失、sentinel 包裹正确性）
- `plugins/rtk/hint_test.go`：更新现有 `TestRecoveryHint_ContainsRecoveryEndpoint` 等用例如需涉及
- `plugins/rtk/hooks_test.go`：新增 `TestPreLLMHook_InjectsFetchToolSchema`、`TestPreLLMHook_SkipsInjectWhenDisabled`
- `plugins/rtk/rtk_test.go`：新增 `TestRegisterFetchTool_Success` / `TestRegisterFetchTool_RejectsDisabledRTK`
- `tests/e2e/api/collections/provider-harness.json`：新增"tool call → sentinel bypass" E2E 用例（参考 `.claude/skills/harness-test-writer/SKILL.md`）

### 不包含

- Raw-output 端点的 per-request_id 隔离（鉴权加固留 Phase 2）
- 自动启用 `inject_fetch_tool` 时的 enable 提示（UI 加 banner），留 Phase 2
- 多 tool 的"工具市场"机制（gateway-side tool catalog）。本次只加 1 个 tool。
- Anthropic `tool_choice` 自动锁到 fetch tool（不需要——tool_choice 留给 LLM 自主决策）

## 方案概述

**核心思路**：复用 `core/mcp` 的 `RegisterTool` + agent loop，不发明新机制。

```
Gateway 启动
  ├─ RTK plugin NewPlugin() 收到 *schemas.BifrostContext（包含 bifrost 引用）
  ├─ bifrost.GetMCPManager().RegisterTool(
  │    "rtk_fetch_raw_output",                    ← 不带 hyphen（RegisterTool 校验）
  │    "Reads previously truncated tool output...",
  │    rtk.RawOutputReadHandler,                   ← Go handler
  │    schemas.RtkFetchRawOutputTool,              ← JSON Schema
  │ )
  └─ RegisterTool 内部：
       ├─ setupLocalHost()（启动 in-process MCP stdio server，仅一次）
       ├─ ToolMap["bifrostInternal-rtk_fetch_raw_output"] = schema  ← 加 prefix
       └─ 把工具注册到本地 MCP server 的 dispatcher

每次 LLM 请求进来
  ├─ transports/celer-route-http/handlers/inference.go chatCompletion (L1013)
  ├─ LLMPlugin.PreLLMHook chain（plugins/rtk/hooks.go）
  │   ├─ 若 p.config.InjectFetchTool && MCPManager 已注册该 tool：
  │   │   ├─ req.ChatRequest.Params.Tools 不含 bifrostInternal-rtk_fetch_raw_output → append
  │   │   └─ req.ResponsesRequest.Params.Tools 同样处理
  │   ├─ 注入 system hint（"call bifrostInternal-rtk_fetch_raw_output"）
  │   └─ 跑 RTK 压缩 pipeline（已有逻辑）
  └─ bifrost.ChatCompletionRequest → provider → 响应

LLM 响应含 tool_calls（name="bifrostInternal-rtk_fetch_raw_output"）
  └─ bifrost.go:951 MCPManager.CheckAndExecuteAgentForChatRequest
       └─ agent.go:186 clientManager.GetClientForTool("bifrostInternal-rtk_fetch_raw_output")
            └─ 返回 bifrostInternal 客户端（state != disabled）
            └─ 归入 autoExecutableTools
       └─ agent.go:executeToolWithHooks → executeToolInternal
            └─ handler 被调用 → 读 raw-output 文件
            └─ WrapRawOutputForHTTP(data, id, len, "")  ← sentinel 包裹
            └─ 返回 ChatMessage（role=tool, content=sentinel+body）
       └─ 把 ChatMessage 加到 conversation，loop 回到 LLM

下一轮 LLM 请求（带 tool_result）
  └─ plugins/rtk/hooks.go PreLLMHook
       └─ processRtkTextWithCommand (compression.go:557)
            └─ StripRawOutputSentinel 识别 → 透传 body
            └─ 不再走 RTK pipeline，body 字节级 == 磁盘原文
```

**承载复用清单**（直接复用、不改）：
- ✅ `MCPManager.RegisterTool`（clientmanager.go:1736）— 已有 typed tool 注册 API
- ✅ `MCPManager.GetClientForTool`（utils.go:115）— agent loop 自动归入 `autoExecutableTools`
- ✅ `prepareToolExecution` + `executeToolWithHooks`（exec.go:61）— plugin pipeline (governance / observability) 自动跑
- ✅ `StripRawOutputSentinel`（compression.go:557）— fetch 后的 tool_result 透传
- ✅ `WrapRawOutputForHTTP`（rawoutput.go:307）— sentinel 包裹
- ✅ `ReadRtkRawOutputByID`（rawoutput.go:351）— 已有 id → 文件查找
- ✅ `IsValidRawOutputID`（rawoutput.go:266）— id 格式校验

**新增承载**：
- 🆕 `core/bifrost.go` `GetMCPManager() *MCPManager` 导出方法
- 🆕 `plugins/rtk/tool_schema.go` `RtkFetchRawOutputTool` schema
- 🆕 `plugins/rtk/rawoutput.go` `RawOutputReadHandler` Go handler
- 🆕 `plugins/rtk/hooks.go` PreLLMHook 中的 tool schema 注入分支
- 🆕 `plugins/rtk/config.go` `InjectFetchTool` 配置字段

**关键设计决策**：

1. **tool name = `rtk_fetch_raw_output`（带下划线不带 hyphen）**：`MCPManager.RegisterTool`（clientmanager.go:1745）拒绝含 hyphen 的名字（`strings.Contains(name, "-")`）。下划线满足 MCP 标准且 LLM 友好。
2. **暴露给 LLM 的 name = `bifrostInternal-rtk_fetch_raw_output`**：`RegisterTool` 内部自动加 `BifrostMCPClientKey-` 前缀（clientmanager.go:1770）。LLM 在 tool_call 里必须用完整带前缀名字。hint 文本要写明确。
3. **handler 返回 sentinel-wrapped body**：与 HTTP 端点的 LLM-bound 响应**字节一致**（`handlers/rtk.go:386` 调用 `WrapRawOutputForHTTP`）。这样无论 LLM 是用 tool call 还是 curl 取回，body 在 RTK 看来都"长得一样"，都触发 `StripRawOutputSentinel` bypass。
4. **handler 不持有锁、不写 ctx**：handler 是 `(ctx, args) → (string, error)` 的纯函数形态。读文件失败时返回 `error`，由 MCP 层映射成 tool_result error message（已有逻辑）。
5. **tool schema 注入与 manager ToolMap 同步**：以 `MCPManager.GetToolPerClient(ctx)` 为 source of truth；PreLLMHook 只查询 + append，不主动创建。这样多个 RTK plugin 实例（如多 gateway worker）不会冲突。

## 风险和注意事项

1. **`MCPManager.RegisterTool` 启动 in-process MCP stdio server**：每次 gateway 启动，RTK plugin enabled + `InjectFetchTool=true` 时，会多起一个 in-process MCP server。开销 < 5MB 内存，可接受，但要在 docs 注明。
2. **tool name prefix 暴露内部命名**：LLM 看到的 function name 是 `bifrostInternal-rtk_fetch_raw_output`，略冗长。可接受 —— `bifrostInternal` 表明这是网关提供的工具，反而是品牌一致性；hint 文本会写完整带前缀的 name 让 LLM 不会拼错。
3. **Anthropic prompt cache 影响**：每次 chat 都多一个 tool schema 项。但 schema 是 byte-stable 字符串（编译时常量），Anthropic cache 仍命中。需在 docs 注明。
4. **streaming 不支持 tool 自动执行**：`ChatCompletionStreamRequest` 不进 agent loop（bifrost.go:986 直接 return）。streaming 场景下 LLM 即使调 tool_call，response stream 也会立即结束。当前 hint 文本保留 fallback（"issue a plain GET"）供 streaming 后客户端手动调用。
5. **多 RTK plugin 实例**：当前架构是 single bifrost instance，多 worker 共享同一 MCPManager。`RegisterTool` 内部有重复检测（clientmanager.go:1774-1777），同名 tool 会返回 error。RTK plugin 在 NewPlugin 时调用一次，整个生命周期不重复注册。
6. **测试矩阵**：handler 是纯函数，单测覆盖率高；PreLLMHook 注入分支需要 mock MCPManager。新增 `mocks.MCPManager` 或 inline stub。
7. **backward compat**：`InjectFetchTool` 默认 `true` 但已有 config 文件不写该字段时——用 `applyConfigDefaults` 兜底成 `true`。`plugins/rtk/config.go:220` 已有类似逻辑。
8. **handler panic 安全**：MCP 层 `executeToolInternal`（toolmanager.go:650）目前没有显式 recover。handler panic 会冒泡到 goroutine 顶层，被 gin/fasthttp middleware 500 兜住。本次不加固，留作后续。

## 验收标准（V-plugins-*, V-core-* (各 track 内递增)）

可验证项（design.md 详细化）：

- **V-plugins-1**：handler 单元测试 — 24-hex id 接受、非 hex 拒绝、文件存在返回 sentinel-wrapped body、文件不存在返回 error
- **V-plugins-2**：tool schema 单测 — JSON Schema 合法、`function.name = bifrostInternal-rtk_fetch_raw_output`、`required: [id]`
- **V-plugins-3**：PreLLMHook 注入 — RTK enabled + InjectFetchTool=true 时，`req.ChatRequest.Params.Tools` 末尾追加 schema；RTK disabled 时不追加；InjectFetchTool=false 时不追加
- **V-plugins-4**：hint 文本更新 — 含完整 tool name `bifrostInternal-rtk_fetch_raw_output`、描述 tool 调用方式、保留 fallback GET URL
- **V-plugins-5**：E2E provider-harness — chat request → LLM 调 tool_call → agent loop 执行 → tool_result 含 sentinel + 原文 → 下轮 request 的 RTK 透传 → LLM 拿到完整原文
- **V-core-1**：bifrost.GetMCPManager 暴露 — plugin 可通过此方法拿到 *MCPManager
- **V-plugins-7**：handler 返回值与 HTTP 端点 LLM-bound 响应字节一致（防止 sentinel 格式漂移）