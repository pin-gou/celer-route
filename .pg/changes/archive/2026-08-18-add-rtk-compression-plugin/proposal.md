# add-rtk-compression-plugin
**关联 issue**：无
**变更类型**：feature

## 背景

pg-gateway 作为 AI 网关，接收到的 LLM 请求中常包含大量冗长的工具输出（`git status` / `npm install` / `docker logs` 等）。这些输出通常只有关键信息（错误、变更摘要）值得 LLM 关注，但绝大多数篇幅是冗余的格式化输出，会显著拉高 input token 消耗与成本。Bifrost 现有 `plugins/semanticcache/` 只能对**完全相同的请求**做缓存短路，对**相似但不相同**的请求无法压缩，导致 token 浪费。

OmniRoute 项目实现了 RTK（Rule-based Tool-output Kompression）规则引擎，通过命令感知的过滤器集合对工具输出做结构化裁剪，在保留关键信息的前提下大幅降低 input token 数。本次希望将该能力移植到 pg-gateway。

## 目标

新建 `plugins/rtk/` 独立 Go plugin 模块，在 `PreLLMHook` 中检测并压缩 BifrostRequest 中 `role: tool` 类型的消息以及 `role: user` 含 `tool_result` content block 的消息（Anthropic 风格），达到：

- 节省 30-70% 的 input token（视命令类型）
- 对响应正确性零影响（保留所有错误信息、变更摘要、关键元数据）
- 通过 `BifrostContext` 透传压缩统计，logging plugin 写入 logs-db
- 重写 `Usage.PromptTokens` 为压缩后值，计费按压缩后结算

## 范围

### 包含

- 新建 `plugins/rtk/` 独立 Go module，参考 `plugins/semanticcache/` 模板
- 移植 OmniRoute 的 RTK 核心引擎（`applyRtkCompression` / `processRtkText`）
- 移植 50+ 内置 JSON 过滤器（`filters/*.json`），按 `project > global > builtin` 优先级匹配
- 移植命令检测器（`commandDetector.ts` → Go）：git / npm / docker / make 等 50+ 内置检测器
- 移植行级规则引擎（`lineFilter`）：strip / keep / collapse / replace / dedup / head/tail 截断
- 移植智能截断（`smartTruncate`）：保留 head/tail 窗口 + priority pattern 保护
- 移植去重（`deduplicator`）：连续重复行合并
- 移植 filterLoader（含 ReDoS 保护）
- Anthropic provider 格式适配（识别 `content[].type == "tool_result"` 块）
- OpenAI provider 格式适配（识别 `tool_call_id` 关联链）
- `cache_control` 块保护：跳过 `ChatContentBlock.CacheControl != nil` 的块
- 扩展 `core/schemas/chatcompletions.go` 的 `BifrostLLMUsage` 增 `OriginalPromptTokens` / `CompressedPromptTokens` 字段
- `PostLLMHook` 重写 `Usage.PromptTokens` 为压缩后值
- 通过 `ctx.SetValue` 透传 `originalPromptTokens` 给 logging plugin
- 扩展 `transports/config.schema.json` 增 rtk 配置块（`if/then` 块）
- 在 `transports/pg-gateway-http/server/plugins.go` 注册 RTK plugin
- 单元测试覆盖：核心引擎、过滤器匹配、cache_control 保护、Anthropic 适配
- transport 层集成测试（fixture 场景）

### 不包含

- response 侧压缩（思考过程折叠、output 去重）
- LLMLingua 等模型化压缩（仅做规则化）
- tiktoken 精确计数（仅留 `char/4` fallback，预留接口）
- stacked pipeline（rtk + 其他压缩器串行）
- UI 改版（plugin 配置页面、压缩统计图表）
- 原始输出 raw retention 持久化（OmniRoute 的 `rawOutputRetention`）
- 自定义过滤器上传 / 信任管理 UI

## 方案概述

**插件形态**：参考 `plugins/semanticcache/main.go` 的 `LLMPlugin` 注册模式，实现 `Init(config Config, logger, ...) (schemas.LLMPlugin, error)`。

**核心数据流**：

```
Client HTTP Request
  → HTTP layer
    → PreRequestHook / PreLLMHook pipeline
      → [RTK Plugin] PreLLMHook
        - 遍历 req.ChatRequest.Input
        - 对 role=tool 消息或 Anthropic 风格的 tool_result blocks
          - 通过 tool_call_id → tool metadata lookup 识别 shell vs non-shell
          - 应用 commandDetector → filterLoader → lineFilter → dedup → smartTruncate
          - 跳过 cache_control 块
        - 写入 compressionState (sync.Map keyed by requestID)
      → Provider API Call
      → [RTK Plugin] PostLLMHook
        - 改写 resp.Usage.PromptTokens = compressedTokens
        - ctx.SetValue(BifrostContextKeyOriginalPromptTokens, originalTokens)
        - 异步写 logs-db metadata
    → HTTP Response
```

**过滤器存储**：使用 `embed.FS` 把 50+ JSON 过滤器打包进 plugin 二进制，无需外部文件依赖。

**配置项**（用于 `transports/config.schema.json` 注入）：

```yaml
{
  "enabled": true,
  "intensity": "standard",  # minimal | standard | aggressive
  "apply_to_tool_results": true,
  "apply_to_code_blocks": false,
  "max_lines_per_result": 120,
  "max_chars_per_result": 12000,
  "deduplicate_threshold": 3,
  "preserve_cache_control": true
}
```

## 风险和注意事项

1. **BifrostLLMUsage 字段扩展是 schema 变更**：所有改动字段需 `omitempty` + 指针类型，避免现有 logging / governance plugin nil dereference。
2. **fallback 重试时重复压缩**：PreLLMHook 修改是 in-place，fallback 重试会被反复压缩。需要在 `compressionState` 中记录已压缩 requestID，或在 PreRequestHook 一次性完成。
3. **ReDoS 风险**：50+ 内置过滤器包含大量正则表达式，必须从 OmniRoute 移植 `isReDoSProne` 保护，每次 filter loader 启动时拒绝可疑正则。
4. **token 估算误差**：默认 `char/4` 对 Gemini 等非 4 字节/token 的模型计费可能偏差。预留 tiktoken 接口，待后续迭代。
5. **Anthropic 适配复杂度**：Bifrost `ChatContentBlock` 没有 `type=tool_use` 枚举，需要在 plugin 内做 shape 识别（`content[].type == "tool_result"`）。
6. **cache_control 透传**：压缩发生在 PreLLMHook，Anthropic prompt caching 依赖 `cache_control` 标记被 provider converter 正确透传；plugin 内必须保留 `CacheControl` 字段原样。
7. **运行时性能**：每条 tool 消息都会被 `commandDetector` + `filterLoader` + `lineFilter` 流水线处理一次。目标延迟 < 5ms（参考 OmniRoute）。需要在 plugin 内做正则缓存。
