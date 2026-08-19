> - **environment 选择**：dev → local，int → local

## 1. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 transports Go 单测：plugin CRUD PATCH 透传新增 pipeline/min_tokens_to_compress 字段（红）

## 2. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 2.1 修改 `transports/config.schema.json` rtk 块（第 3130-3225 行附近），新增 `pipeline`（array of {id, config}，默认 `[{id:"rtk"}]`）与 `min_tokens_to_compress`（integer，默认 0）两个字段定义，附 description
- [ ] 2.2 验证现有 plugin handlers 无需修改即可透传（plugins.go 处理 config 为 json.RawMessage）

## 3. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/rtk-stage6-pipeline-trigger-ui 做静态审查
- [ ] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 4.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 4.2 执行测试（runner 通过 modules 注入命令）
- [ ] 4.3 启动服务（如需）
- [ ] 4.4 验证 V-transports-1：plugin PATCH 透传新增字段（curl PATCH /api/plugins/rtk -d '{"config":{"pipeline":[{"id":"rtk"}],"min_tokens_to_compress":500}}' → 返回 200；GET 见新字段）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-ui-1

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

- [ ] 6.1 编写 `plugins/rtk/engine_test.go`：CompressionEngine 接口契约 + EngineCatalog 注册 + Pipeline runner 顺序执行 + 未知 id fail-soft（红）
- [ ] 6.2 编写 `plugins/rtk/hooks_test.go` 增量：MinTokensToCompress 阈值边界（0=全压、低阈值跳过、高阈值压缩）（红）
- [ ] 6.3 编写 `plugins/rtk/config_test.go` 增量：Config 新字段默认值零值安全（Pipeline 空自动补 `[{id:"rtk"}]`，MinTokensToCompress=0 不跳过）

## 7. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 7.1 新增 `plugins/rtk/engine.go`：
  - 定义 `CompressionEngine` 接口（Id/Apply/HealthCheck/IsEnabled/Schema 5 方法）
  - 定义 `EngineConfig` / `EngineResult` / `EngineBreakdown` / `PipelineResult` 结构
  - 实现 `EngineCatalog` 全局注册表（`RegisterEngine(e)` + `map[string]CompressionEngine`）
  - 实现 `RunPipeline(ctx, text, pipeline, cfg)` 顺序执行，累加 engineBreakdown
  - 未知 engine id → log.Warnf + skip 而非 panic
- [ ] 7.2 新增 `plugins/rtk/rtk_engine.go`（避免与 rtk.go 冲突）：将 RTK 自身实现为 `CompressionEngine`，在 `RegisterEngine(rtkEngine{})` 注册 id="rtk"
  - `Apply()` 委托给现有 `applyRtkCompression`，保持输入输出语义一致
- [ ] 7.3 修改 `plugins/rtk/config.go`：新增 `Pipeline []EngineSpec`（JSON tag `pipeline`，omitempty）+ `MinTokensToCompress int`（JSON tag `min_tokens_to_compress`）
  - 在 `applyConfigDefaults` 中：空 Pipeline 自动补 `[{id:"rtk"}]`；MinTokensToCompress 默认 0
- [ ] 7.4 修改 `plugins/rtk/hooks.go`：`PreLLMHook` 开头增加 `estimateRequestTokens(req)` + 阈值判断
  - `MinTokensToCompress == 0` → 维持现状，全压
  - `MinTokensToCompress > 0 && estimated < MinTokensToCompress` → `return req, nil, nil`，DEBUG 日志含 estimated/threshold
- [ ] 7.5 在 `plugins/rtk/rtk.go:Init` 末尾调用 `RegisterEngine(rtkEngine{})` 完成自注册

## 8. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/rtk-stage6-pipeline-trigger-ui 做静态审查
- [ ] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 9. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 9.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 9.2 执行测试（runner 通过 modules 注入命令）
- [ ] 9.3 启动服务（如需）
- [ ] 9.4 验证 V-plugins-1：CompressionEngine 接口注册与堆叠行为（`go test ./plugins/rtk/... -run TestEngine` 通过：EngineCatalog 含 id="rtk"；Pipeline runner 顺序执行累加 engineBreakdown；未知 id warn+skip）
- [ ] 9.5 验证 V-plugins-2：PreLLMHook 主动触发 token 阈值跳过（`go test ./plugins/rtk/... -run TestHooksMinTokens` 通过：MinTokens=0 全压；MinTokens=1000000 + req tokens=10 跳过压缩输出字节与输入一致）
- [ ] 9.6 验证 V-plugins-3：Config 默认值零值安全（`go test ./plugins/rtk/... -run TestConfigDefaults` 通过：applyConfigDefaults 不 panic；空 Pipeline 自动补 `[{id:"rtk"}]`）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-ui-1

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

- [ ] 11.1 编写 `tests/e2e/features/plugins/rtk-config.spec.ts` Playwright e2e（红）：访问 `/workspace/plugins` → 选中 rtk → 断言 fragment 渲染 enabled/intensity/max_lines_per_result/max_chars_per_result/raw_output_retention 等字段；enabled 切换后 submit 断言 API 返回 200
- [ ] 11.2 编写 Vitest 单元测试：`ui/lib/types/plugins.test.ts` 验证 `rtkConfigSchema` (zod) 校验逻辑（pipeline 是数组、min_tokens_to_compress 是非负整数）

## 12. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 12.1 修改 `ui/lib/types/plugins.ts`：新增 `rtkConfigSchema` (zod)：
  - `pipeline: z.array(z.object({id: z.string(), config: z.unknown().optional()})).default([{id:"rtk"}])`
  - `min_tokens_to_compress: z.number().int().min(0).default(0)`
  - 其余字段沿用现有 rtk 字段定义（intensity/max_lines_per_result/...等）
- [ ] 12.2 新增 `ui/app/workspace/plugins/fragments/rtkFragment.tsx`：
  - 复用 `providercooldownFragment.tsx` 模板（react-hook-form + zodResolver + RTK Query）
  - `useGetPluginQuery("rtk")` 读取 + `useUpdatePluginMutation` 提交
  - `EnabledSwitch` 复用现有模式
  - 字段分组（启用与强度 / 行字符上限 / 作用范围 / 分组 / 过滤器 / 原始输出 / 高级），按 design.md 表格
  - `pipeline` 字段以 JSON textarea 渲染（高级配置，避免嵌套 array 表单复杂度）
- [ ] 12.3 修改 `ui/app/workspace/plugins/page.tsx` 或 `views/pluginsView.tsx`：将 `rtkFragment` 接入到 RTK plugin 选中时的渲染路径（按 `plugin.name === 'rtk'` 分发）
- [ ] 12.4 修改 `ui/locales/en/plugins.json` 和 `ui/locales/zh-CN/plugins.json`：新增 `plugins.rtk.*` i18n key（约 15 个：title、enableTitle、settingsTitle、intensity、maxLines、maxChars、dedupThreshold、applyToToolResults、applyToCodeBlocks、applyToAssistantMessages、enableGrouping、groupingThreshold、customFiltersEnabled、trustProjectFilters、rawOutputRetention、pipeline、minTokensToCompress）

## 13. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 13.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 13.2 review agent 对 git diff feat/pg/rtk-stage6-pipeline-trigger-ui 做静态审查
- [ ] 13.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 13.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 14. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 14.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 14.2 执行测试（runner 通过 modules 注入命令）
- [ ] 14.3 启动服务（如需）
- [ ] 14.4 验证 V-ui-1：RTK fragment 渲染与字段出现（Playwright e2e `tests/e2e/features/plugins/rtk-config.spec.ts` 通过：进入 `/workspace/plugins` → 选中 rtk → fragment 渲染字段；enabled 切换 submit → API 返回 200）
- [ ] 14.5 验证 V-ui-2：i18n 中英文渲染（Playwright e2e 切换 zh-CN 后断言字段标签中文正确显示）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-ui-1

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

- [ ] 16.1 确认 `.pg/changes/rtk-stage6-pipeline-trigger-ui/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
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