# rtk-phase2-grouping-assistant
**关联 issue**：无（内部规划 temp/rtk.md 阶段二）
**变更类型**：feature

## 背景

oc2-gateway `plugins/rtk/` 已移植 OmniRoute RTK 核心压缩管线（阶段一已落地：`isDocumentLikeRead` 文档读取保护 + `[rtk:truncated N lines]` / `[line repeated Nx]` / `[rtk:truncated by chars]` 标记行），但对照参考实现 OmniRoute `open-sse/services/compression/engines/rtk/` 仍缺三项能力：

1. **模糊分组缺失**：仅时间戳/hex/版本号/数字不同的相似行无法归并（OmniRoute `grouper.ts` 未移植），典型如 `Downloaded chunk 1..6` 这类重复进度日志只能靠精确去重折叠
2. **assistant 消息永不压缩**：`applyRtkCompression` 只处理 `role=tool` / Anthropic `tool_result` block / Responses `function_call_output`，assistant 消息内容被完全跳过；OmniRoute 支持 `applyToAssistantMessages` 全文压缩与 `applyToCodeBlocks` 围栏感知 code-only 压缩
3. **强度缩放不完整**：`scaleFilterForIntensity` 仅 aggressive 档生效（minimal/standard 无动作）；`Config.MaxLinesPerResult` 在管线中完全未接线（OmniRoute 用它作 `filter.maxLines` 的 fallback：`effectiveMaxLines(filter.maxLines || config.maxLinesPerResult, intensity)`）

## 目标

对齐 OmniRoute RTK 阶段二能力，补齐生态压缩面：

- 模糊分组：归一化相似行并按阈值归并，输出 `首行 [rtk:grouped ×N]` 标记
- assistant 消息压缩：`apply_to_assistant_messages` 全文模式 + `apply_to_code_blocks` 围栏感知 code-only 模式（仅 Chat 路径）
- 强度缩放修正：三档 `effectiveMaxLines` 公式 + `MaxLinesPerResult` fallback 接线

默认行为不变（新开关全部默认关闭/对齐现状），既有 ~70 个测试用例保持全绿。

## 范围

### 包含

- `plugins/rtk/grouper.go` 新文件：`normalizeLine` + `groupSimilarLines`（移植 OmniRoute `grouper.ts`）
- `plugins/rtk/config.go` 新增 `enable_grouping` / `grouping_threshold` / `apply_to_assistant_messages` 字段、默认值与校验（threshold 运行时 clamp 下限 2）
- `plugins/rtk/compression.go` 管线集成：去重后、smartTruncate 前插入分组阶段（含文档保护路径）；Chat 路径 assistant 消息压缩分支（OpenAI ContentStr + Anthropic text block）；`effectiveMaxLines` 强度缩放 + `MaxLinesPerResult` fallback
- `transports/config.schema.json` rtk 配置块新增三个字段（该块 `additionalProperties: false`，新字段必须同步否则配置校验拒绝）
- 单测：`grouper_test.go` 新文件 + compression/config 测试增补（种子用例取自 OmniRoute `rtk-grouping.test.ts`）

### 不包含

- Responses API 路径 assistant 消息压缩（`applyRtkCompressionResponses` 维持只处理 `function_call_output`）
- `codeStripper.ts` 完整移植（代码注释剥离/docstring 保留等）
- renderers 语义渲染器、TOML 兼容、外部自定义过滤器与 trust 信任模型（阶段三）
- Raw Output 保留、密钥脱敏、过滤器内联测试 benchmark（阶段四）
- Learn/Discover 过滤器自学习（阶段五）
- 跨引擎堆叠、主动触发、UI 管理面板（阶段六）
- `filterloader.go` 加载逻辑与 52 个内置 filter JSON 内容变更

## 方案概述

对照 OmniRoute 源码移植三项能力：

1. **模糊分组**：`normalizeLine` 按 7 步顺序归一化（ISO 时间戳→`<N>`、`[日期 时间]`→`[<N>]`、hex 6-40→`<N>`、semver→`<N>`、整数→`<N>`、空白折叠、trim）；`groupSimilarLines` 对连续 `runLength >= threshold`（默认 3、运行时下限 2）的归一化相等行合并为 `首行 [rtk:grouped ×N]`，返回压缩文本与被归并行数。插入管线位置＝去重之后、smartTruncate 之前；文档保护（isDocumentLikeRead）路径在 dedup 之后同样应用；受 `enable_grouping` 开关控制（默认关）。分组生效时向 `ProcessStats.Techniques` 追加 `rtk-grouping`。
2. **assistant 消息压缩**（仅 Chat 路径）：`apply_to_assistant_messages=true` → 压缩 assistant 全文；否则 `apply_to_code_blocks=true` 且消息含 ``` 围栏 → 轻量 fence 切分仅压缩代码块内部（不移植完整 codeStripper）。Anthropic 仅处理 text block，跳过 `tool_use`/reasoning/带 `cache_control` 的块（沿用 `PreserveCacheControl` 字节级保护语义）。
3. **强度缩放**：新增 `effectiveMaxLines(base, intensity)`（minimal×1.5 / standard×1 / aggressive×0.5，`max(1, round)`）；`scaleFilterForIntensity` 补 minimal 分支（仅缩放 MaxLines，Head/Tail/maxChars 不动）；匹配 filter 无 `max_lines` 时回退 `Config.MaxLinesPerResult` 再应用强度系数，对齐 OmniRoute 公式。

## 风险和注意事项

1. **MaxLinesPerResult fallback 行为变更**：接线后「匹配 filter 无 max_lines」时的 smartTruncate 行为会改变，需全量回归 52 个内置 filter 相关用例。验证方式：V-plugins-3（fallback 语义）+ V-plugins-4（既有 ~70 用例全绿回归）。
2. **assistant 压缩误伤风险**：必须跳过带 `cache_control` 的块与 reasoning/tool_use 内容，否则破坏 Anthropic prompt 缓存命中或请求结构。验证方式：V-plugins-2 单测覆盖 tool_use/cache_control/reasoning 不可触碰断言。
3. **grouping_threshold 非法值**：需运行时 clamp 下限 2（对齐 OmniRoute Zod 校验 `groupingThreshold < 2` 拒绝语义），配置 0/1/负数不得导致行为漂移。验证方式：V-plugins-1 阈值边界用例 + V-plugins-3 配置校验用例。
