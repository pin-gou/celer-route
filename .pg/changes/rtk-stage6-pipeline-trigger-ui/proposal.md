# rtk-stage6-pipeline-trigger-ui
**关联 issue**：无
**变更类型**：feature

## 背景
oc2-gateway 的 RTK 插件已在阶段一～五完成核心压缩管线（52 个内置过滤器、文档读取保护、模糊分组、assistant 消息压缩、Raw Output 保留 + 密钥脱敏、过滤器内联测试、Learn/Discover 自学习、自定义过滤器 + SHA256 信任模型）。

但 RTK 当前定位仍是"被动挂在 PreLLMHook 上的单一线性管线"——一旦插件启用，所有 tool result 都会被压缩，无法堆叠其他压缩引擎（如未来引入的 llmlingua / headroom），也无法按请求大小主动决定"这次请求是否值得压缩"。同时前端 workspace 还没有 RTK 配置面板，所有 RTK 参数只能通过编辑 config.json 调整。

## 目标
把 RTK 从"单一插件"升级为"压缩中台"形态：
1. 引入 `CompressionEngine` 接口（5 方法：Id/Apply/HealthCheck/IsEnabled/Schema），RTK 自身作为首个注册引擎，通过 `Config.Pipeline` 配置支持多引擎顺序堆叠。
2. 引入按请求大小的主动触发机制：`MinTokensToCompress` 阈值，低于阈值的请求直接跳过整次压缩（避免对小请求做无用功）。
3. 在 `/workspace/plugins` 下新增 RTK 配置 fragment，提供 enabled/intensity/全部 RTK Config 字段的可视化配置入口。

## 范围
### 包含
- `plugins/rtk/engine.go` 新增 `CompressionEngine` 接口（5 方法）+ `EngineCatalog`（注册表）+ pipeline runner（顺序执行 `Config.Pipeline`，累加 `engineBreakdown` 元数据）
- `plugins/rtk/config.go` 新增 `Pipeline []EngineSpec`（默认 `[{id:"rtk"}]`）与 `MinTokensToCompress int`（默认 0=不跳过）
- `plugins/rtk/hooks.go` 在 `PreLLMHook` 入口估算整请求 `estimatedTokens`（复用 `compression.go` 中已有 `estimateTokens`），低于阈值则 `return req, nil, nil` 原样下传
- `transports/config.schema.json` rtk 块增量更新，新增两个字段定义
- `ui/app/workspace/plugins/fragments/rtkFragment.tsx` 基础配置表单：enabled 开关 + intensity 选择 + max_lines/max_chars/dedup_threshold/各 apply_to_*/raw_output_retention/custom_filters_enabled/trust_project_filters/enable_grouping 等全字段
- `ui/lib/types/plugins.ts` 新增 `rtkConfigSchema` (zod) — 复用现有 `useGetPluginQuery("rtk")` + `useUpdatePluginMutation` 模板
- `ui/locales/{en,zh-CN}/plugins.json` 新增 `plugins.rtk.*` i18n key（中英双语同步）
- Playwright e2e 用例 1 条：访问 `/workspace/plugins`，验证 RTK fragment 加载 + enabled 切换 + 表单字段出现
- `plugins/rtk/*_test.go` 增量测试：engine_test.go 验证接口契约、hooks_test.go 验证阈值边界

### 不包含
- raw-output 文件浏览器 UI（前端展示已落盘的 raw output 日志）
- Learn/Discover UI 预览界面（前端展示自学习结果）
- TOML 导入功能（前端导入社区 .toml 过滤器）
- 新增独立引擎实现（llmlingua / headroom / cc / ionizer）
- 跨 transport 层的"按请求大小主动触发"（本次保持 PreLLMHook 内挂载，不改 core/inference.go）
- `renderers/`、codeStripper.ts 移植
- 修改 `core/bifrost.go`、`core/schemas/plugin.go`
- 修改 transport 路由注册表

## 方案概述
**后端管线接口（6.1）**：在 `plugins/rtk/` 内部新增 `engine.go`，定义接口 `CompressionEngine{ Id() string; Apply(ctx, text, config) (EngineResult, error); HealthCheck() error; IsEnabled() bool; Schema() json.RawMessage }`，将 RTK 自身实现注册为 `id="rtk"`。`EngineCatalog` 是 `map[string]CompressionEngine`，按 `Config.Pipeline[i].id` 查找；pipeline runner 顺序执行，已知 engine 输出累加 `engineBreakdown`（含 `id/inputBytes/outputBytes/compressedBy`）。

**后端主动触发（6.2）**：`Config` 新增 `MinTokensToCompress int`（默认 0 = 维持现状不跳过）。`PreLLMHook` 开头先做 `estimateTokens(req)`（复用 `compression.go` 已有工具），若 `MinTokensToCompress > 0 && estimated < MinTokens`，则 `return req, nil, nil`（不下发任何 plugin 修改）；否则走原有 `applyRtkCompression` 路径。

**前端 UI（6.3）**：在 `ui/app/workspace/plugins/fragments/rtkFragment.tsx` 实现，复用 `providercooldownFragment.tsx` 模板（react-hook-form + zod + RTK Query）。`Enabled` 开关复用 `EnabledSwitch` 模式（沿用 `providercooldown`/`EnabledSwitch`），其余字段按 `rtkConfigSchema` 渲染。表单提交走 `useUpdatePluginMutation({name:"rtk", data:{enabled, config:{...}}})`。

## 风险和注意事项
- **配置向后兼容**：新增 `Pipeline` 默认 `[{id:"rtk"}]` 与 `MinTokensToCompress` 默认 0，零值安全，不会改变现有部署行为。
- **管线 fail-soft**：`Pipeline` 配置了未知 engine id 时，必须 warning + skip 而非 panic。
- **Apply 语义一致性**：RTK 自身的 `Apply()` 输入输出语义必须与当前 `applyRtkCompression` 完全一致，避免引入 silent regression（hooks_test + engine_test 双重断言）。
- **MinTokensToCompress 语义**：低阈值意味着"宁可压缩"，高阈值意味着"宁可跳过"——文档明确说明，避免用户混淆。
- **UI 表单提交失败保护**：react-hook-form 失败保留旧值，不丢失用户配置。
- **跨 track 改动**：dev.plugins + dev.transports + dev.ui 三条 track 并行，需协调合并顺序（plugins 接口稳定后再触发 transports 与 ui 实现）。

**约束**：本节列出的每条风险对应 design.md 的 Verification Criteria 至少一条 V-*：
- "配置向后兼容" → V-plugins-1 / V-plugins-2
- "管线 fail-soft" → V-plugins-1
- "Apply 语义一致性" → V-plugins-1
- "MinTokensToCompress 语义" → V-plugins-2
- "UI 表单提交失败保护" → V-ui-1
- "跨 track 改动协调" → 留痕 final-gate 跨 track 依赖审查