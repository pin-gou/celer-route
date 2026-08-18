# add-rtk-compression-plugin 设计

## 架构概览

```
┌────────────────────────────────────────────────────────────────────────────┐
│                        pg-gateway 请求生命周期                              │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ① HTTPTransportPreHook → ② PreRequestHook → ③ PreLLMHook                  │
│                                                                            │
│    [New] RTK Plugin: PreLLMHook                                           │
│    ┌─────────────────────────────────────────────────────────────────┐    │
│    │ 1. 遍历 req.ChatRequest.Input，识别 role=tool 消息               │    │
│    │ 2. 通过 assistant 消息的 tool_calls 构建 lookup 表              │    │
│    │ 3. 跳过 cache_control != nil 的 content block                   │    │
│    │ 4. 对 shell 类工具调用 RTK 引擎：                                │    │
│    │    commandDetector → filterLoader → lineFilter                 │    │
│    │    → deduplicator → smartTruncate                              │    │
│    │ 5. 写入 plugin.sync.Map[requestID] = *compressionState          │    │
│    │ 6. 返回修改后的 req                                             │    │
│    └─────────────────────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ④ Provider Queue → ⑤ Provider API Call                                    │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ⑥ PostLLMHook                                                              │
│                                                                            │
│    [New] RTK Plugin: PostLLMHook                                          │
│    ┌─────────────────────────────────────────────────────────────────┐    │
│    │ 1. 重写 resp.Usage.PromptTokens = compressedTokens              │    │
│    │ 2. 设置 ctx.SetValue(                                           │    │
│    │      BifrostContextKeyOriginalPromptTokens, originalTokens)     │    │
│    │ 3. 异步写 logs-db metadata（与 logging plugin 协作）            │    │
│    └─────────────────────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ⑦ HTTPResponse → 客户端                                                    │
└────────────────────────────────────────────────────────────────────────────┘
```

**涉及模块**：

| 模块 | 路径 | 改动 |
|------|------|------|
| **plugins/rtk**（新） | `plugins/rtk/` | 新建独立 Go module |
| **core/schemas** | `core/schemas/chatcompletions.go` | `BifrostLLMUsage` 增 `OriginalPromptTokens` / `CompressedPromptTokens` 字段 |
| **core/schemas** | `core/schemas/bifrost.go` | 增 `BifrostContextKeyOriginalPromptTokens` 常量 |
| **transports** | `transports/config.schema.json` | 增 rtk plugin 块（`if/then` 分支） |
| **transports** | `transports/pg-gateway-http/server/plugins.go` | 增 RTK plugin init case |
| **transports** | `transports/pg-gateway-http/handlers/logging.go` | 读 `OriginalPromptTokens` ctx key 写 logs-db metadata |

## API 设计（如有）

无新增 HTTP API。本变更仅修改 plugin 内部行为与 schema 字段。

## 数据模型

### 1. `BifrostLLMUsage` 字段扩展

**文件**：`core/schemas/chatcompletions.go`（`BifrostLLMUsage` struct）

```go
type BifrostLLMUsage struct {
    PromptTokens            int                          `json:"prompt_tokens,omitempty"`
    PromptTokensDetails     *ChatPromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
    CompletionTokens        int                          `json:"completion_tokens,omitempty"`
    TotalTokens             int                          `json:"total_tokens"`
    Cost                    *BifrostCost                 `json:"cost,omitempty"`
    // 既有字段
    CachedReadTokens        int  `json:"cached_read_tokens,omitempty"`
    CachedWriteTokens       int  `json:"cached_write_tokens,omitempty"`
    // [New] RTK 压缩字段
    OriginalPromptTokens    *int  `json:"original_prompt_tokens,omitempty"`
    CompressedPromptTokens  *int  `json:"compressed_prompt_tokens,omitempty"`
}
```

**注意**：
- 仅 Pointer + `omitempty`，确保现有 behavior 兼容
- `PromptTokens` 字段保留，但 RTK 插件运行时会被改写为压缩后值
- `OriginalPromptTokens` 用于 analytics / billing 追溯

### 2. `BifrostContext` 字段扩展

**文件**：`core/schemas/bifrost.go`

```go
const BifrostContextKeyOriginalPromptTokens BifrostContextKey = "x-bf-original-prompt-tokens"
```

### 3. RTK Plugin Config

**文件**：`plugins/rtk/config.go`

```go
type Config struct {
    Enabled              bool   `json:"enabled"`
    Intensity            string `json:"intensity"`             // minimal | standard | aggressive
    ApplyToToolResults   bool   `json:"apply_to_tool_results"`
    ApplyToCodeBlocks    bool   `json:"apply_to_code_blocks"`
    MaxLinesPerResult    int    `json:"max_lines_per_result"`  // default 120
    MaxCharsPerResult    int    `json:"max_chars_per_result"`  // default 12000
    DedupThreshold       int    `json:"dedup_threshold"`       // default 3
    PreserveCacheControl bool   `json:"preserve_cache_control"` // default true
}
```

## 组件设计

### 1. RTK Plugin 内部结构

```
plugins/rtk/
├── go.mod
├── config.go              # Config struct + UnmarshalJSON
├── rtk.go                 # Plugin struct + Init() 入口
├── compression.go         # applyRtkCompression + processRtkText
├── filterloader.go        # Filter 加载 + ReDoS 保护 + 优先级匹配
├── linedetector.go        # commandDetector（git/npm/docker/make 等 50+）
├── linefilter.go          # 行级规则执行（strip/keep/collapse/replace/dedup）
├── smarttruncate.go       # 智能截断（head/tail + priority pattern）
├── deduplicator.go        # 连续行去重
├── textoken.go            # token 估算（char/4 + 可选 tiktoken）
├── anthropic.go           # Anthropic adapter（识别 tool_result blocks）
├── openai.go              # OpenAI adapter（tool_call_id 关联链）
├── state.go               # per-request compression state
├── hooks.go               # PreLLMHook + PostLLMHook
├── filters/               # embed.FS 嵌入 50+ JSON 过滤器
│   ├── builtin/
│   │   ├── git-status.json
│   │   ├── npm-install.json
│   │   ├── docker-logs.json
│   │   └── ... (50+)
│   └── generic-output.json
├── filters.go             # 过滤器加载 + 优先级匹配
├── rtk_test.go            # 核心引擎单测
├── filterloader_test.go   # Filter 加载 + ReDoS 保护单测
├── linefilter_test.go     # 行级规则单测
├── hooks_test.go          # PreLLMHook + PostLLMHook 单测
└── anthropic_test.go      # Anthropic 适配单测
```

### 2. Compression Pipeline

```
┌──────────────────────────────────────────────────────────────────┐
│ Input: BifrostRequest.ChatRequest.Input []ChatMessage            │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 1: buildToolCallLookup(messages)                            │
│   - 遍历 assistant 消息提取 tool_calls[]                        │
│   - 提取 toolUseId → {toolName, command}                         │
│   - 通过 toolName (e.g. "bash") 判断 shell vs non-shell         │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 2: messages.map → 逐条处理                                 │
│   对每条 message:                                                │
│     - role=user 含 type=tool_result blocks (Anthropic)         │
│     - role=tool 含 string content (OpenAI)                     │
│     - 跳过 cache_control marked blocks                          │
│     - 通过 tool_call_id 查 lookup → command + skipFilters       │
│     - 调用 processRtkText(content, config)                     │
│     - 测量 originalTokens / compressedTokens                   │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 3: processRtkText(text)                                     │
│   ① commandDetector.detect(text) → {type, command, confidence}  │
│   ② filterLoader.match(type, command) → filter                 │
│   ③ lineFilter.apply(text, filter) → stripped                  │
│   ④ deduplicator.dedup(stripped, threshold=3) → deduped        │
│   ⑤ smartTruncate.truncate(deduped, {head, tail, priority})    │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ Output: 修改后的 messages + CompressionStats                    │
│   stats: {originalTokens, compressedTokens, techniques[], ...}  │
└──────────────────────────────────────────────────────────────────┘
```

### 3. PostLLMHook 设计

```go
func (p *Plugin) PostLLMHook(ctx *BifrostContext, resp *BifrostResponse, err *BifrostError) (
    *BifrostResponse, *BifrostError, error,
) {
    // 1. 取 state
    state := p.getCompressionState(ctx)
    if state == nil || !state.compressed {
        return resp, err, nil  // 未压缩则透传
    }

    // 2. 重写 Usage.PromptTokens
    if resp != nil && resp.BifrostChatResponse != nil &&
       resp.BifrostChatResponse.Usage != nil {
        resp.BifrostChatResponse.Usage.PromptTokens = state.compressedTokens
        resp.BifrostChatResponse.Usage.OriginalPromptTokens = &state.originalTokens
        resp.BifrostChatResponse.Usage.CompressedPromptTokens = &state.compressedTokens
    }

    // 3. 同步 ctx（供 logging plugin 读取）
    ctx.SetValue(BifrostContextKeyOriginalPromptTokens, state.originalTokens)
    ctx.SetValue(BifrostContextKeyCompressedPromptTokens, state.compressedTokens)

    // 4. 清理 state
    p.clearCompressionState(ctx)

    return resp, err, nil
}
```

## 关键约束与契约

### 前置条件

- Bifrost go workspace 1.26.1+ (项目当前 1.26.6)
- Go 1.26.6+ 编译环境
- `embed.FS` 用于打包过滤器 JSON（Go 1.16+ 标准库支持）
- `regexp` 包（Go 标准库）用于过滤器正则匹配
- 可选：`github.com/pkgz/tiktoken-go` 或类似库（当前阶段不引入，留接口）

### 影响面

- **BifrostLLMUsage 字段扩展**（schema 变更，新字段为 Optional）：
  - `core/schemas/chatcompletions.go` 加 `OriginalPromptTokens *int` / `CompressedPromptTokens *int`
- **BifrostContextKey 增一个常量**（不影响 wire）：
  - `core/schemas/bifrost.go` 加 `BifrostContextKeyOriginalPromptTokens`
- **config.schema.json 增一个 plugin 块**（不影响现有 plugin）：
  - `transports/config.schema.json` 加 `if/then` 块（`name == "rtk"`）
- **transport register 增一个 case**：
  - `transports/pg-gateway-http/server/plugins.go` 加 RTK plugin init
- **logging handler 读 ctx key**：
  - `transports/pg-gateway-http/handlers/logging.go` 读 `OriginalPromptTokens` 写 logs-db metadata
- **不破坏任何对外 API**

### 性能契约

- 单条 tool 消息压缩延迟 < 5ms（参考 OmniRoute 实测）
- 50+ 内置过滤器正则编译缓存（plugin 启动时一次性编译）
- 拒绝 ReDoS-prone 正则（filter loader 启动时检测）
- 不在 hot path 上做 JSON 序列化（仅操作内存对象）

### 错误码与编号段

无新增 error code。

### 环境限制与验证策略

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 RTK 核心压缩逻辑正确 | ✅ | 单元测试 | n/a |
| V-plugins-2 RTK Plugin 注册 & PreLLMHook 修改 messages | ✅ | e2e + 单元测试 | n/a |
| V-core-1 BifrostLLMUsage 扩展字段 schema 通过 | ✅ | go build + 单元测试 | n/a |
| V-plugins-3 RTK PostLLMHook 重写 usage + ctx 透传 | ✅ | e2e + logs-db 查询 | n/a |
| V-plugins-4 config.schema.json 注入 rtk 块校验通过 | ✅ | JSON schema 校验 + e2e 启动 | n/a |
| V-plugins-5 cache_control 块保护逻辑 | ✅ | 单元测试 + e2e | n/a |

### 可观测性

- **关键日志点**：
  - `logger.Info("RTK", "request_id", id, "original_tokens", n, "compressed_tokens", m, "ratio", n/m)` — 压缩统计
  - `logger.Warn("RTK", "filter_redos_rejected", name)` — ReDoS 过滤器被拒绝
  - `logger.Error("RTK", "compression_error", err)` — 压缩失败时记录（不阻塞主流程）
- **关键指标**（可选，后续可加 Prometheus）：
  - `rtk_compression_total` (Counter)
  - `rtk_tokens_saved_total` (Counter)
  - `rtk_compression_latency_seconds` (Histogram)
- **RequestId 追踪**：使用 `BifrostContextKeyRequestID` 关联 pre/post hook

### 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ✅ | `BifrostLLMUsage` 字段扩展 + `BifrostContextKey` 常量 |
| framework | ❌ | 无 |
| transports | ✅ | `config.schema.json` 增 rtk 块 + server register 修改 + logging handler 读 ctx |
| plugins | ✅ | 新建 `plugins/rtk/` 独立 module |
| ui | ❌ | 无 |
| scr | ❌ | 无（端到端验证走 e2e） |

**affected_tracks**：`[core, transports, plugins]`

**scenario track 决策**：`scr=false`（本次变更跨 3 个 track，但都通过 e2e/单元测试可验证，无需启多 role 协作的真实场景）

## Verification Criteria

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | RTK 核心压缩逻辑正确 | 无 | `cd plugins/rtk && go test -run TestApplyRtkCompression` | 50+ 测试用例覆盖各类命令输出，token 减少 30%+，关键信息保留 |
| V-plugins-2 | RTK Plugin 注册 & PreLLMHook 修改 messages | pg-gateway 已用 fixture 启动 | `curl -X POST http://localhost:9080/v1/chat/completions -d '{tool_result: <long>}'` | Provider 收到的 tool 消息已压缩，response 中 `compressed_prompt_tokens` 字段存在 |
| V-plugins-3 | RTK PostLLMHook 重写 usage + ctx 透传 | 已通过 plugins-2 阶段 | `sqlite3 .pg/hooks/local/data/logs.db "SELECT metadata FROM logs WHERE metadata LIKE '%original_prompt_tokens%'"` | logs-db metadata JSON 含 `original_prompt_tokens` 字段 |
| V-plugins-4 | config.schema.json 注入 rtk 块校验通过 | config.json 含 `{name: "rtk", config: {enabled: true}}` | `pg-gateway` 启动读取 config 不报错 | 启动成功，RTK plugin 已加载 |
| V-plugins-5 | cache_control 块保护逻辑 | 构造含 cache_control 块的工具消息 | `cd plugins/rtk && go test -run TestCacheControlPreservation` | 压缩后 cache_control 块字节完全相同 |

### dev core Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-core-1 | BifrostLLMUsage 扩展字段 schema 通过 | 无 | `cd core && go build ./...` | 编译通过，无 JSON marshal 错误 |

### dev transports Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | config.schema.json 注入 rtk 块 | config.json 含 rtk 配置 | `pg-gateway` 启动 | rt 注册成功，无 schema 校验错误 |
| V-transports-2 | logging handler 读 ctx key 写 logs-db | 运行 plugins-2 阶段 | `sqlite3 logs.db "SELECT metadata FROM logs ..."` | metadata 含 original_prompt_tokens |

### int scenario Verification Criteria

- （本变更无 scenario track 启用，跳过）

## 风险覆盖

| proposal.md 风险 | design.md V-* 验证 |
|------------------|-------------------|
| BifrostLLMUsage 字段扩展 nil dereference | V-core-1 |
| fallback 重试时重复压缩 | V-plugins-2（多次重试验证） |
| ReDoS 风险 | V-plugins-1（filter 加载测试） |
| token 估算误差 | V-plugins-3（usage 字段验证） |
| Anthropic 适配复杂度 | V-plugins-1（Anthropic 适配单测） |
| cache_control 透传 | V-plugins-5 |
| 运行时性能 | V-plugins-1（latency 断言） |
| logging 集成 | V-plugins-3 + V-transports-2 |
