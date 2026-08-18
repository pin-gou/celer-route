# rtk-stage1-markers-and-doc-protection
**关联 issue**：无
**变更类型**：feature

## 背景

oc2-gateway `plugins/rtk/` 已实现 RTK 核心压缩管线（参见 `add-rtk-compression-plugin` 变更，archive/2026-08-18），通过 `applyRtkCompression` 在 `PreLLMHook` 中对 `role=tool` / `tool_result` / `function_call_output` 三类工具输出做命令感知的结构化裁剪。但当前管线存在两个**可观测性盲点**：

1. **文档/文件读取被静默截断**：`defaultDetector.detect()` 对未匹配任何已知命令的输出兜底返回 `{Type:"shell", Command:""}`，filterloader 在 `command==""` 时命中 `generic` filter（Head=10/Tail=5），会把一个 ~147 行的代码/prose 读取结果强行 head/tail 截断，**中间内容被静默丢弃**，LLM 既不知道被截断也不知道中间有什么。
2. **截断/去重/字符硬限对 LLM 不可见**：当前管线在三个步骤削减文本但**不向输出注入任何标记**。LLM 看到的最终文本与原文片段难以区分，不知道哪些行被砍了、砍了多少。

OmniRoute RTK 引擎（`open-sse/services/compression/engines/rtk/`）通过 4 项可观测性产出解决了这两个问题：文档保护 + 截断/去重/字符限三处 marker。本次 `temp/rtk.md` 把这 4 项划为"阶段一"。

## 目标

在 `plugins/rtk/` 压缩管线上落地 4 项可观测性产出：

- **文档读取保护**：对未识别命令且不含错误标记的输出，跳过 filter matching + smartTruncate，仅保留 ANSI 剥离 + 去重
- **截断 marker**：smartTruncate 因 head/tail 窗口或 MaxLines cap 丢弃中间行时，在 head 与 tail 之间插入 `[rtk:truncated N lines]`
- **去重 marker**：applyDedup 命中 `runLen>=threshold` 时，在首行后追加 `[line repeated Nx]` 与 `[rtk:dropped N repeated lines]`
- **字符截断 marker**：truncateToCharLimit 命中（result 长度超 MaxCharsPerResult）时，追加 `[rtk:truncated by chars]`

效果：
- 文档/文件读取输出不再被 generic filter 误命中
- LLM 可看到完整的"被截断事实"（多少行被砍）
- 不引入新 config 字段、不改 hooks / openai / anthropic 集成

## 范围

### 包含

- `plugins/rtk/linedetector.go` 新增 `hasGenericErrorMarkers(text) bool` 辅助函数（正则 `Error:|Exception:|Traceback \(most recent call last\):`）
- `plugins/rtk/compression.go` 在 `processRtkTextWithCommand` 的 step 4 后插入 isDocumentLikeRead 判定，命中时跳过 step 5/6/8/9，保留 step 1-3 + step 7 dedup
- `plugins/rtk/smarttruncate.go` `applySmartTruncate` 返回签名改为 `(string, int)`，在 head/tail 之间插入 `[rtk:truncated N lines]`（N = dropped 行数）
- `plugins/rtk/deduplicator.go` `applyDedup` 返回签名改为 `(string, int)`，对 runLen>=threshold 行追加 `[line repeated Nx]` + `[rtk:dropped N repeated lines]`
- `plugins/rtk/compression.go` step 9 `truncateToCharLimit` 命中后追加 `[rtk:truncated by chars]`
- 更新 `linefilter_test.go` 中 3 个受影响的现有测试断言：`TestLineFilterDedup` / `TestLineFilterHeadAndTail` / `TestLineFilterMaxLines`
- 新增 5 个测试：`TestDocumentReadNotTruncated` / `TestIsDocumentLikeReadWithErrorMarkers` / `TestTruncateMarker` / `TestDedupMarker` / `TestCharTruncateMarker`

### 不包含

- 模糊分组（`grouper.ts`，阶段二）
- `apply_to_assistant_messages` assistant 消息压缩（阶段二）
- `intensity` 缩放修正：minimal 档 maxLines×1.5（阶段二）
- 自定义过滤器 + project/global/builtin 三级加载 + `trust.json` SHA256 校验（阶段三）
- Canonical `RtkFilterPack` 双格式 schema（阶段三）
- Raw Output 落盘 + 密钥脱敏 + 失败检测（阶段四）
- 内联过滤器测试 + savings benchmark（阶段四）
- Learn/Discover 过滤器自学习（阶段五）
- 跨引擎堆叠管线 / 主动触发 / UI（阶段六）
- 修改 `filterloader.go` / `config.go` / `hooks.go` / `openai.go` / `anthropic.go`
- 新增 config 字段 / env vars / config.schema.json 变更
- 修改 `core/schemas/` 任何类型

## 方案概述

**isDocumentLikeRead 判定**：在 `processRtkTextWithCommand` step 4（非 shell 早返）之后、step 5（filter matching）之前插入：

```go
isDocumentLikeRead := detection.Type == "shell" && detection.Command == "" && !hasGenericErrorMarkers(text)
if isDocumentLikeRead {
    // 跳过 step 5 (loader.Match) / 6 (applyLineFilter) / 8 (applySmartTruncate) / 9 (truncateToCharLimit)
    // 保留 step 7 (applyDedup)
}
```

**截断 marker 插入点**：`applySmartTruncate` 返回 (text string, dropped int)。在 head 之后、tail 之前插入 `[rtk:truncated N lines]` 单行。

**字符截断 marker 插入点**：在 `compression.go` step 9 `truncateToCharLimit` 调用后 append `[rtk:truncated by chars]`。

**签名变更**：`applySmartTruncate(text, filter) (string, int)` 与 `applyDedup(text, threshold) (string, int)`。blast radius：仅 `compression.go` step 7/8 调用点 + 直接调用这两个函数的测试。

## 风险和注意事项

1. **签名变更 blast radius**：`applySmartTruncate` 与 `applyDedup` 返回类型从 string 改为 (string,int)，compression.go 调用点已识别（step 7/8）；直接调用这两个函数的现有测试需同步接收第二个返回值。
2. **isDocumentLikeRead 语义差**：oc2 `defaultDetector` 兜底类型是 `"shell"`，OmniRoute 是 `"unknown"`——本实现判定条件 `Type=="shell" && Command==""` 比 OmniRoute 的 `type=="unknown" && !command` 更宽。任何未识别命令的 shell 输出都会进入保护路径，与 OmniRoute 的精确"未知类型"行为有别。需要在 design.md 中显式说明。
3. **marker token 计入**：marker 文本（~50 字符）会被 `estimateTokens` 计入 `CompressedTokens`。边界 case 下 5% 压缩率阈值（compression.go:84）更难达到——属于 token accounting drift，不影响功能正确性。
4. **priority pattern 与 marker 交互**：被抢救的中间 priority line 会出现在 marker 之后，与 OmniRoute 行为一致，但需要测试明确验证。
5. **未做项**：不引入 learn/discover，不改 hooks 接口，不改 anthropic/openai 集成——下一阶段处理。