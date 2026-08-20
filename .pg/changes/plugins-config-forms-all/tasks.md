> - **environment 选择**：dev → local，int → local

## 1. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 1.1 编写 ajv 校验脚本 `scripts/validate-config-schema.ts`：用 ajv 装载 `transports/config.schema.json` 后，对 fixture sample 配置（governance + mocker 各 1 份）做 validate（红：governance 缺字段必失败）
- [x] 1.2 编写治理 unit test `transports/pg-gateway-http/handlers/plugins_test.go`：断言 schema 必填字段存在（governance.is_vk_mandatory、governance.required_headers、governance.disable_auto_tool_inject、governance.routing_chain_max_depth）

## 2. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 2.1 同步 `transports/config.schema.json` 中 `governance` 段：补齐 `disable_auto_tool_inject` (boolean) 和 `routing_chain_max_depth` (integer, 1-100)，与 Go config struct 字段对齐
- [x] 2.2 新增 `transports/config.schema.json` 中 `mocker` 段：覆盖 `global_latency`、`rules[]`（含 `conditions`、`responses`、`priority`、`probability`）、`default_behavior` 枚举（passthrough/error/success）
- [x] 2.3 校对 `transports/config.schema.json` 中 `logging` / `semantic_cache` / `compat` 字段名、enum、allOf 条件与 Go config struct 一致
- [x] 2.4 ajv 校验脚本通过：所有 fixture 配置 validation error = 0

## 3. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/plugins-config-forms-all 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 启动服务（如需）
- [x] 4.4 验证 V-transports-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-ui-4, V-ui-5, V-ui-6, V-transports-1, V-plugins-1, V-ui-7

## 5. dev.transports:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 6. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 6.1 编写 `plugins/logging/main_test.go` 单元测试：覆盖 `Config.UnmarshalJSON` 默认值（4 个字段）、`validateWriterConfig` 边界值（MaxBatchSize > 0、BatchInterval > 0、MaxBatchBytes > 0）
- [x] 6.2 编写 `plugins/semanticcache/main_test.go` 单元测试：覆盖 `Init` 中 `Dimension >= 0` 校验、`Config.Dimension > 0` 当 `Provider != ""`、TTL UnmarshalJSON 接受非负字符串与整数
- [x] 6.3 编写 `plugins/mocker/main_test.go` 单元测试：覆盖 `MockerConfig` 默认值（DefaultBehavior = "passthrough"）、`validateRule` 边界值（Priority [-1000, 1000]、Probability [0, 1]、Response.StatusCode [100, 599]）
- [x] 6.4 编写 `plugins/compat/main_test.go` 单元测试：覆盖 `Config.UnmarshalJSON` 默认值（全部 4 个字段 absent 时为 true）、`IsEnabled()` 至少 1 个字段为 true 时返回 true

## 7. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 7.1 验证 `plugins/logging` 现有 `CheckAndSetDefaults` 与 schema 字段名一致：disable_content_logging、retain_content_in_object_storage、allow_per_request_content_storage_override、logging_headers
- [ ] 7.2 验证 `plugins/semanticcache` 现有 `Init` 与 schema allOf 条件一致：provider 设值时 dimension >= 2、provider 留空时 dimension == 1
- [ ] 7.3 验证 `plugins/mocker` 现有 `validateRule` 覆盖 schema 全部约束：rule 至少 1 个 Response、Latency.Type 枚举、SuccessResponse 需要 Message 或 MessageTemplate
- [ ] 7.4 验证 `plugins/compat` 现有 `UnmarshalJSON` 默认值与 schema 字段对齐：4 个字段全部默认 true（除 should_convert_params），与 config.schema.json 中 `default: false` 字段对照

## 8. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 8.2 review agent 对 git diff feat/pg/plugins-config-forms-all 做静态审查
- [x] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 9. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 9.1 执行 lint（runner 通过 modules 注入命令）
- [x] 9.2 执行测试（runner 通过 modules 注入命令）
- [x] 9.3 启动服务（如需）
- [x] 9.4 验证 V-plugins-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-ui-4, V-ui-5, V-ui-6, V-transports-1, V-plugins-1, V-ui-7

## 10. dev.plugins:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 11. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [x] 11.1 编写 vitest `ui/app/workspace/plugins/fragments/__tests__/schema.test.ts`：覆盖 4 个 Zod schema 的 happy path（合法输入通过）、边界值（semantic_cache dimension 边界 1/2、provider 必填字段）、invalid input（mocker 非法 JSON 拒绝）
- [x] 11.2 编写 vitest `ui/app/workspace/plugins/views/__tests__/pluginsView.test.tsx`：覆盖散转逻辑（pluginName → fragment）、占位卡渲染（prompts / modelcatalogresolver / jsonparser）
- [x] 11.3 编写 vitest `ui/lib/types/__tests__/plugins.test.ts`：覆盖 compatibility 字段（compat schema 4 字段默认 true/should_convert_params 默认 false）、logging_headers 数组校验

## 12. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [x] 12.1 在 `ui/lib/types/plugins.ts` 中新增 4 个 Zod schema：`loggingConfigSchema`、`semanticCacheConfigSchema`、`compatConfigSchema`、`mockerConfigSchema`，使用 `.optional()` 容错现有 config 缺失字段
- [x] 12.2 在 `ui/lib/types/plugins.ts` 中新增 1 个 i18n 标签映射：`pluginFragmentLabels`（logging / semantic_cache / mocker / compat / prompts / modelcatalogresolver / jsonparser → display name）
- [x] 12.3 新增 `ui/app/workspace/plugins/fragments/loggingFragment.tsx`：4 字段表单（Switch + TagInput），按钮触发 `useUpdatePluginMutation`
- [x] 12.4 新增 `ui/app/workspace/plugins/fragments/semanticCacheFragment.tsx`：11 字段表单 with `allOf` 条件联动（provider 切换时 validator 实时重算）
- [x] 12.5 新增 `ui/app/workspace/plugins/fragments/mockerFragment.tsx`：Monaco JSON 编辑器 + Zod 实时校验 + 错误提示
- [x] 12.6 新增 `ui/app/workspace/plugins/fragments/compatFragment.tsx`：4 个 Switch + 提交按钮
- [x] 12.7 新增 3 个占位卡 fragment：`promptsFragment.tsx`、`modelcatalogresolverFragment.tsx`、`jsonparserFragment.tsx`（Card + 跳转 link）
- [x] 12.8 修改 `ui/app/workspace/plugins/views/pluginsView.tsx`：在散转逻辑中新增 7 个插件名 → fragment 映射 + 7 个 import
- [x] 12.9 在 `ui/locales/en/plugins.json` 中新增 49 个 i18n 键（pluginNames.* + loggingConfig.* + semanticCacheConfig.* + mockerConfig.* + compatConfig.* + placeholderConfig.*）
- [x] 12.10 在 `ui/locales/zh-CN/plugins.json` 中新增对应 49 个键值，LLM token 译"词元"、auth token 译"令牌"

## 13. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [x] 13.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 13.2 review agent 对 git diff feat/pg/plugins-config-forms-all 做静态审查
- [x] 13.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 13.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 14. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 14.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 14.2 执行测试（runner 通过 modules 注入命令）
- [ ] 14.3 启动服务（如需）
- [ ] 14.4 验证 V-ui-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-ui-4, V-ui-5, V-ui-6, V-transports-1, V-plugins-1, V-ui-7

## 15. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 16. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [ ] 16.1 确认 `.pg/changes/plugins-config-forms-all/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 16.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 16.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 16.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 16.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 16.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [ ] 16.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [ ] 16.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [ ] 16.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 17. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 17.1 收集所有 stage 的 Gate Assessment
- [ ] 17.2 检查跨 stage 依赖项
- [ ] 17.3 输出 Final Gate Assessment
