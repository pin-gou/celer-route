> - **environment 选择**：dev → local，int → local

## 1. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写单元测试：在 `transports/bifrost-http/server/plugins_test.go` 新增 `TestLoadBuiltinPlugins_ProviderCooldown_DefaultOn` 用例：构造空 `PluginConfigs`（无 provider-cooldown entry），调 `loadBuiltinPlugins()`，断言 `providercooldown.NewPlugin` 被实例化、`s.KeyPoolFilter != nil`、插件状态为 `active`
- [ ] 1.2 编写 schema 验证单元测试：在 `transports/config.schema.json` 校验流程的测试用例里加 3 条 fixture：
  - `default_ttl_seconds` 为负数 → 期望 validation 失败
  - `ttl_overrides` 的 value 为负数 → 期望 validation 失败
  - `quota_patterns` 为空数组 → 期望 validation 失败
- [ ] 1.3 编写 enabled=false 显式禁用单元测试：在 `TestLoadBuiltinPlugins_ProviderCooldown_ExplicitDisabled` 验证：`PluginConfigs` 中含 `{name: "provider-cooldown", enabled: false}` 时，插件不加载 + `KeyPoolFilter` 为 nil

## 2. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 2.1 修改 `transports/bifrost-http/server/plugins.go` loadBuiltinPlugins 中 providercooldown case（约 line 289-315）：从 `if cooldownCfg != nil && cooldownCfg.Enabled` 改为"无 entry 时构造等价默认 cfg（Enabled=true），仍允许 entry.enabled=false 覆写"
- [ ] 2.2 保持 `s.KeyPoolFilter = plugin.State.AsFilter(logger)` 绑定逻辑不变（不破坏 reload 链路）
- [ ] 2.3 修改 `transports/config.schema.json` plugins 数组 name 字段描述（约 line 2739-2742），把 `provider-cooldown` 加入 built-in 列表
- [ ] 2.4 新增 allOf/if/then 专用 config schema：当 `name=provider-cooldown` 时，`config` 字段必须有 `default_ttl_seconds` (integer ≥ 1)、`ttl_overrides` (object, value ≥ 1)、`quota_patterns` (array of string, minItems ≥ 1)
- [ ] 2.5 grep 校验无外部破坏性引用：`rg "loadBuiltinPlugins|MaxIdleConnDuration|MaxConnsPerHost" transports/bifrost-http/` 仅返回本次变更范围内的引用

## 3. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/providercooldown-builtin-ui 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> 1. `IsBuiltinPlugin()` 决策函数未改，`builtinPluginNames` 列表未改
> 2. `KeyPoolFilter` 绑定 fallback 路径在 enabled=false 时不应触发
> 3. configschema allOf/if/then 描述字段类型与 providercooldown.Config struct 一致

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
  - verifiable: V-transports-1, V-transports-4, V-plugins-1, V-ui-1, V-ui-2, V-ui-3, V-ui-4
  - degraded: V-transports-2, V-transports-3

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

- [ ] 6.1 编写单元测试：在 `plugins/providercooldown/config_test.go` 补充 `TestConfig_DefaultOnInitialization` 用例：验证 `providercooldown.NewPlugin(logger).Init(nil)` 能用默认配置初始化（覆盖默认开启路径）
- [ ] 6.2 编写单元测试：在 `plugins/providercooldown/cooldown_test.go` 补充 `TestAsFilter_EnabledFilter` 用例：验证 `AsFilter` 返回的 `KeyPoolFilter` 在 KeyPoolFilter 通过 nil 路径时不会 panic

## 7. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 7.1 保持 `plugins/providercooldown/cooldown.go` 核心逻辑不变（CooldownState / PostLLMHook / AsFilter）
- [ ] 7.2 保持 `plugins/providercooldown/config.go` Config struct 与 ParseConfig 不变
- [ ] 7.3 （如需）调整 `Init(rawConfig any)` 允许 rawConfig 为 nil 时使用默认配置

## 8. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/providercooldown-builtin-ui 做静态审查
- [ ] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> 1. providercooldown 核心逻辑改动最小化（仅允许 Init 容忍 nil 入口）
> 2. AsFilter 返回的 function 不在 enabled=false 路径被调用

## 9. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 9.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 9.2 执行测试（runner 通过 modules 注入命令）
- [ ] 9.3 启动服务（如需）
- [ ] 9.4 验证 V-plugins-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-transports-1, V-transports-4, V-plugins-1, V-ui-1, V-ui-2, V-ui-3, V-ui-4
  - degraded: V-transports-2, V-transports-3

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

- [ ] 11.1 编写 Vitest：在 `ui/app/workspace/plugins/fragments/__tests__/providercooldownFragment.test.tsx` 编写专用 fragment 单元测试，验证：
  - 3 字段（default_ttl_seconds / ttl_overrides / quota_patterns）都能正确渲染 form input
  - zod schema 校验非法输入（default_ttl_seconds 负数 / ttl_overrides 负数 / quota_patterns 空数组）
  - Switch 切换触发 useUpdatePluginMutation
- [ ] 11.2 编写 Vitest：监控面板组件渲染 state / stats 列表 + 解冻按钮触发 DELETE
- [ ] 11.3 编写 Vitest：RTK Query endpoint 扩展（3 个 cooldown API）的 request shape 与 response type 符合现有 pluginsApi 约定

## 12. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 12.1 新建 `ui/app/workspace/plugins/fragments/providercooldownFragment.tsx` 三段组件：
  - `<EnabledSwitch>` 顶部 Switch（受控 + RBAC 控制）
  - `<ConfigForm>` react-hook-form + zod 表单（3 字段）
  - `<MonitoringPanel>` 监控面板（state / stats / 解冻按钮）
- [ ] 12.2 扩展 `ui/lib/types/plugins.ts` pluginFormSchema：新增 providercooldown 字段 schema（default_ttl_seconds / ttl_overrides / quota_patterns）
- [ ] 12.3 扩展 `ui/lib/store/apis/pluginsApi.ts`：新增 3 个 RTK Query endpoint
  - `useGetCooldownStateQuery` → GET /api/plugins/provider-cooldown/state
  - `useGetCooldownStatsQuery` → GET /api/plugins/provider-cooldown/stats
  - `useUnfreezeCooldownMutation` → DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId}
- [ ] 12.4 修改 `ui/app/workspace/plugins/views/pluginsView.tsx`：检测到 `name === "provider-cooldown"` 时切换到 `<ProvidercooldownFragment />`
- [ ] 12.5 新建 `docs/features/provider-cooldown.mdx`：含 3 字段语义、默认值、UI 入口、监控 API 说明

## 13. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 13.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 13.2 review agent 对 git diff feat/pg/providercooldown-builtin-ui 做静态审查
- [ ] 13.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 13.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> 1. UI 端 zod schema 与 config.schema.json 字段类型一致（无 drift）
> 2. RTK Query 5s 轮询监控面板不引入 N+1
> 3. RBAC 控制按钮可见性

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
  - verifiable: V-transports-1, V-transports-4, V-plugins-1, V-ui-1, V-ui-2, V-ui-3, V-ui-4
  - degraded: V-transports-2, V-transports-3

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

- [ ] 16.1 确认 `.pg/changes/providercooldown-builtin-ui/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
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
