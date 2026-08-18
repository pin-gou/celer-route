# rtk-phase2-grouping-assistant 设计

## 架构概览

变更全部位于 `plugins/rtk/` Go 模块内（PreLLMHook 请求体改写路径），外加 `transports/config.schema.json` 配置 schema 同步。无新增 HTTP 端点、无 DB 变更、无 UI 变更。

```
Chat 请求 → PreLLMHook (hooks.go)
  └─ applyRtkCompression (compression.go:38)
      ├─ role=tool / Anthropic tool_result block ──→ processRtkTextWithCommand（现状）
      ├─ [NEW] role=assistant ─┬─ apply_to_assistant_messages=true ──→ 全文压缩
      │                        └─ apply_to_code_blocks=true 且含 ``` 围栏 ──→ code-only 压缩
      └─ processRtkTextWithCommand 管线 (compression.go:338)：
           stripANSI → 短错误消息跳过 → 命令检测 → isDocumentLikeRead 分支
             ├─ 文档保护路径：dedup → [NEW] 分组 → 字符硬限
             └─ 正常路径：filter 匹配 → applyLineFilter → applyDedup
                  → [NEW] groupSimilarLines（enable_grouping 开关）
                  → [CHANGED] scaleFilterForIntensity（补 minimal 档 + effectiveMaxLines 公式）
                  → applySmartTruncate（[CHANGED] filter 无 max_lines 时 fallback MaxLinesPerResult）
                  → 字符硬限
```

Responses API 路径（`applyRtkCompressionResponses`）不变——仍只处理 `function_call_output`。

## API 设计（如有）

无。本变更不新增/修改任何 HTTP 端点；行为通过插件配置字段（见数据模型）控制，配置经既有 plugins CRUD 端点写入，无需端点改动。

## 数据模型（如有）

无 DB 表变更。插件配置新增字段（`plugins/rtk/config.go` Config 结构体 + `transports/config.schema.json` rtk 块同步）：

| 字段（json tag） | Go 字段 | 类型 | 默认值 | 校验 | 说明 |
|------------------|---------|------|--------|------|------|
| `enable_grouping` | `EnableGrouping` | bool | `false` | — | 模糊分组总开关 |
| `grouping_threshold` | `GroupingThreshold` | int | `3`（零值填默认） | 运行时 clamp 下限 2；schema `minimum: 2` | 归并触发阈值（runLength >= threshold） |
| `apply_to_assistant_messages` | `ApplyToAssistantMessages` | bool | `false` | — | assistant 消息全文压缩开关 |

既有字段 `apply_to_code_blocks`（现未被管线消费）在本变更后获得语义：与 `apply_to_assistant_messages` 组合决定 assistant 消息压缩模式（对齐 OmniRoute `shouldCompressMessage`）。

归并标记格式（与 OmniRoute `grouper.ts:89` 逐字对齐）：`{首行原文} [rtk:grouped ×{runLength}]`。

## 组件设计（如有）

无前端组件。

## 关键约束与契约

### 前置条件

- 阶段一已落地（`isDocumentLikeRead` 保护 + 截断/去重标记行），本次在其管线上叠加，不回改阶段一行为
- Go workspace 1.26.x（`go.work` 要求 ≥1.26.6）
- OmniRoute 参考源码位于 `/home/ubuntu/workspace/OmniRoute/open-sse/services/compression/engines/rtk/`（grouper.ts / index.ts / types.ts），实现时逐条对照避免 drift

### 影响面

- 新增文件：`plugins/rtk/grouper.go`、`plugins/rtk/grouper_test.go`
- 修改文件：`plugins/rtk/config.go`（新字段+默认值+clamp）、`plugins/rtk/compression.go`（分组阶段、assistant 分支、effectiveMaxLines、MaxLinesPerResult fallback）、`plugins/rtk/compression_test.go`、`plugins/rtk/config_test.go`、`transports/config.schema.json`（rtk 配置块 3134-3189 行区域）
- 函数签名变化：`scaleFilterForIntensity`（compression.go:442）行为扩展（新增 minimal 分支）；`processRtkTextWithCommand` 内部管线步骤增加，对外签名不变
- 是否破坏对外 API：否。新配置字段全部增量且默认值保持现状行为（enable_grouping=false / apply_to_assistant_messages=false / intensity 默认 standard 不变）

### 性能契约

- `normalizeLine` 正则全部包级预编译（`regexp.MustCompile`），禁止在热路径编译
- `groupSimilarLines` 单遍 O(行数) 扫描；归一化串复用，禁止每对行重复归一化
- 分组关闭（默认）时管线零额外开销：开关检查在函数入口，不进分组代码路径
- assistant code-only 模式的 fence 切分为单遍扫描，不引入额外全文拷贝（除压缩改写本身）

### 错误码与编号段

- 不新增错误码。配置非法走既有 `Config.Validate()` 路径（`Init` 快速失败）；`grouping_threshold < 2` 不报错，运行时 clamp 到 2（对齐 OmniRoute `Math.max(2, floor(...))` 运行时语义）

### 环境限制与验证策略

> 依据 `.pg/changes/rtk-phase2-grouping-assistant/env-description.yaml`（local 环境）与 `0-define/define-summary.yaml` 定界结论。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 模糊分组语义（归一化/阈值/标记格式/管线位置） | ❌ | 单元测试 | degraded：压缩语义经 API 无法精确断言，降级为 plugins 模块单测（go test plugins/rtk），scenario 标 @skip |
| V-plugins-2 assistant 消息压缩（全文/code-only/不误伤） | ❌ | 单元测试 | degraded：请求体内部改写经 API 不可直接断言，降级为 plugins 模块单测，scenario 标 @skip |
| V-plugins-3 强度缩放三档 + MaxLinesPerResult fallback | ❌ | 单元测试 | degraded：纯函数语义，降级为 plugins 模块单测（表驱动三档），scenario 标 @skip |
| V-plugins-4 既有压缩行为无回归 | ❌ | 单元测试 | degraded：回归完全由 go test 覆盖（既有 ~70 用例全绿），无环境依赖 |
| V-plugins-5 真实网关链路压缩冒烟 | ✅ | scenario | n/a——经 {env.business_systems[name=pg-gateway-api]} 注入请求、经 {env.data_resources[name=logs-db]} 断言压缩效果 |
| V-transports-1 rtk 配置 schema 块更新 | ✅ | 单元测试 | n/a——transports 模块单测 + plugins 配置解析测试覆盖新字段 |

### 可观测性

- 分组生效时向 `ProcessStats.Techniques` 追加 `rtk-grouping`（对齐 OmniRoute techniquesUsed），CompressionState 沿用既有 token 统计路径（PostLLMHook 重写 prompt tokens）
- `grouping_threshold` 配置 <2 被 clamp 时记录 WARN 日志（含配置值与 clamp 结果）
- 无新增指标；RequestId 追踪沿用既有 stateStore 机制，不需额外埋点

## Verification Criteria

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | 模糊分组语义正确 | DefaultConfig() + enable_grouping=true；种子数据取自 OmniRoute rtk-grouping.test.ts（数字/hex/时间戳/版本变体） | go test：TestGroupSimilarLines 表驱动 | 6 行 `Downloaded chunk N` 归并为 `Downloaded chunk 1 [rtk:grouped ×6]`；2 行（threshold=3）不合并；threshold=2 时 2 行合并；时间戳/hex/版本变体可归并；threshold=1 被 clamp 为 2；enable_grouping=false 时输出与输入逐字一致 |
| V-plugins-2 | assistant 消息压缩生效且不误伤 | 构造 OpenAI assistant ContentStr 消息 + Anthropic text/tool_use 混合 ContentBlocks；apply_to_assistant_messages=true / apply_to_code_blocks=true 两组配置 | go test：TestApplyToAssistantMessages | 全文模式：assistant 文本被压缩且重复行归并；code-only 模式：仅 ``` 围栏内部被压缩、围栏外文本逐字保留；tool_use block、reasoning、带 cache_control 的块字节级不变；两开关均 false 时 assistant 消息逐字不变 |
| V-plugins-3 | 强度缩放三档生效 + MaxLinesPerResult fallback | 构造 Filter{MaxLines:100} 与无 MaxLines 的 filter；intensity=minimal/standard/aggressive 三档 | go test：TestIntensityScaling 表驱动 | effectiveMaxLines(100, minimal)=150 / standard=100 / aggressive=50；base=1 时 aggressive 结果 ≥1；minimal 档 Head/Tail 不变；filter 无 max_lines 时 smartTruncate 使用 MaxLinesPerResult×强度系数；maxChars 不受强度缩放 |
| V-plugins-4 | 既有压缩行为无回归 | 现有全部测试夹具（52 个内置 filter + cache_control 夹具） | go test：plugins/rtk 全量 | 既有 ~70 个用例全绿：cache_control 字节级保护、`[rtk:truncated N lines]` / `[line repeated Nx]` 标记、文档保护路径、tool 消息压缩行为不变 |

### dev transports Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | rtk 配置 schema 块正确更新 | transports/config.schema.json rtk 块含三个新字段定义 | go test：transports 模块单测（runner 注入）+ plugins 配置解析断言 | 含 `enable_grouping`/`grouping_threshold`/`apply_to_assistant_messages` 的 rtk 配置可解析；`additionalProperties: false` 语义保持；`grouping_threshold` schema minimum=2 |

### int scr Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-5 | 真实网关链路压缩冒烟 | local 环境 prepare_env 完成；经 plugins API 启用 rtk 插件（enable_grouping=true） | scenario-scr.yaml：POST {env.business_systems[name=pg-gateway-api].endpoints[name=api].url} chat completions 请求（含 6 行重复 tool 输出），查询 {env.data_resources[name=logs-db]} 请求日志 | 请求被路由且日志中请求体可观察到分组标记 `[rtk:grouped` 或压缩统计生效；rtk 插件配置经 API 读写往返一致 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 无 core/ 下文件改动 |
| framework | ❌ | 无 framework/ 下文件改动 |
| transports | ✅ | `transports/config.schema.json` rtk 配置块新增 3 字段（JSON 数据文件，无 Go 代码改动） |
| plugins | ✅ | `plugins/rtk/` 新增 grouper.go + 修改 config.go/compression.go + 测试 |
| ui | ❌ | 无前端改动 |
| scr | ⚠️ scenario | V-plugins-5 verifiable 需端到端冒烟（跨 plugins+transports 模块观察压缩链路） |

**affected_tracks**：`[plugins, transports]`

**scenario track(s) 启用决策**：`scr=true`——V-plugins-5（verifiable）要求跨 pg-gateway-api 与 logs-db 的端到端观察，单模块单测无法覆盖配置解析→PreLLMHook→压缩→日志落库的完整链路。
