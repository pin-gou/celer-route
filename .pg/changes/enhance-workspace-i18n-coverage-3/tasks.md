> - **environment 选择**：dev → local，int → local

## 1. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 en vs zh-CN key diff 脚本（`ui/scripts/i18n-key-diff.mjs`）：遍历 15 个现有 namespace，断言 en 与 zh-CN 的 key 集合完全一致，新增 key 后脚本能正确检出差异（红）
- [ ] 1.2 为 ESLint 规则编写测试用例（`eslint.no-unlocalized-text` 规则 + 测试 fixture）：断言 `"Save"` 裸英文字面量被标记为错误、`t("common:action.save")` 不被标记、`eslint-disable-next-line` 豁免生效（红）
- [ ] 1.3 为 `schemas.ts` Zod 翻译路径编写 vitest 单测：mock i18next.t() 返回固定中文，断言 `.min(1, t(...))` 生成的错误消息含中文（红）

## 2. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 2.1 扩充 `.pg/hooks/local/describe_env.sh`：在 environments.local.business_systems 段新增 `ui-dev` 条目（type=web-app, capabilities: vite_dev_server + i18n_runtime_switch + jsdom_runtime），同步在 relations 段补 ui-dev→localhost 与 ui-dev→bifrost-api 关系（用户已授权）
- [ ] 2.2 在 `ui/lib/types/schemas.ts` 顶部 import i18n 实例，将 ~70+ 处 Zod 硬编码 message 替换为 `i18next.t("validation.*")` 或 `t("validation.fieldRequired", { field: ... })`；同步在 15 个 namespace 的 locale JSON 中新增 `validation.*` 段（绿）
- [ ] 2.3 Logs 菜单项 i18n 化：`ui/app/workspace/logs/page.tsx`（✓已部分 i18n，需补 COLUMN_LABELS → column_labels 引用调整）+ 子路由 `ui/app/workspace/logs/timeline/page.tsx` + 子组件 `views/logsTimeline.tsx` / `views/timelineToolbar.tsx` / `views/timelineLegend.tsx` / `views/timelineDetail.tsx` + `sheets/logDetailsSheet.tsx` / `sheets/logDetailView.tsx` / `sheets/observabilityConfigSheet.tsx` / `sheets/observabilitySettingsSheet.tsx` / `sheets/sessionDetailsSheet.tsx` + `views/logsTable.tsx` / `views/logsVolumeChart.tsx` / `views/emptyState.tsx` / `views/pluginLogsView.tsx` / `views/logEntryDetailsView.tsx` / `views/logChatMessageView.tsx` / `views/logResponsesMessageView.tsx` / `views/collapsibleBox.tsx` / `views/imageView.tsx` / `views/videoView.tsx` / `views/audioPlayer.tsx` / `views/speechView.tsx` / `views/transcriptionView.tsx` / `views/ocrView.tsx` / `views/blockHeader.tsx` / `views/columns.tsx` / `views/recalculateCostDialog.tsx`；新增 key 追加到 `logs.json`（绿）
- [ ] 2.4 Dashboard 菜单项 i18n 化：`ui/app/workspace/dashboard/page.tsx`（✓已 i18n）+ `components/tabViews/overviewTabView.tsx` / `providerUsageTabView.tsx` / `modelRankingsTabView.tsx` / `dimensionRankingsTabView.tsx` / `mcpTabView.tsx` + `components/exportPopover.tsx` + `components/charts/logVolumeChart.tsx` / `tokenUsageChart.tsx` / `costChart.tsx` / `modelUsageChart.tsx` / `latencyChart.tsx` / `throughputChart.tsx` / `providerCostChart.tsx` / `providerTokenChart.tsx` / `providerLatencyChart.tsx` / `providerThroughputChart.tsx` / `mcpVolumeChart.tsx` / `mcpCostChart.tsx` / `mcpTopToolsChart.tsx` / `localCacheTokenMeterChart.tsx` / `externalCacheTokenMeterChart.tsx` / `chartTypeToggle.tsx` / `chartCard.tsx` / `chartErrorBoundary.tsx` / `gaugeUtils.tsx` / `providerFilterSelect.tsx` / `modelFilterSelect.tsx`；新增 key 追加到 `dashboard.json`（绿）
- [ ] 2.5 Providers 菜单项 i18n 化：`ui/app/workspace/providers/page.tsx`（✓已 i18n）+ `views/providersEmptyState.tsx` / `views/addProviderDropdown.tsx` + `fragments/deploymentsTable.tsx` + `dialogs/addNewKeySheet.tsx` / `dialogs/addNewCustomProviderSheet.tsx`；新增 key 追加到 `providers.json`（绿）
- [ ] 2.6 Browse Providers (new) 菜单项 i18n 化：`ui/app/workspace/providers2/page.tsx`（✗全新页面）+ `views/ProviderFamilyGroup.tsx` / `views/ProviderFilters.tsx` / `views/useProviders2Data.ts`（hook 内 i18n 调用）/ `views/LogsTab.tsx`；新增 key 追加到 `providers.json`（绿）
- [ ] 2.7 Model Catalog 菜单项 i18n 化：`ui/app/workspace/model-catalog/page.tsx`（✗）+ `views/overviewTab.tsx` / `views/modelCatalogTable.tsx` / `views/attributeSheet.tsx`；新增 key 追加到 `model-catalog.json`（绿）
- [ ] 2.8 Routing Rules 菜单项 i18n 化：`ui/app/workspace/routing-rules/page.tsx`（✗）+ `views/routingRulesTable.tsx` / `views/routingRuleSheet.tsx` / `views/routingRuleInfoSheet.tsx` + `components/celBuilder/celRuleBuilder.tsx`；`views/routingRulesView.tsx` 与 `views/routingRulesEmptyState.tsx` 已 ✓ i18n，跳过；新增 key 追加到 `routing.json`（绿）
- [ ] 2.9 Routing Rules → Tree 子路由 i18n 化：`ui/app/workspace/routing-rules/tree/page.tsx`（✗）+ `views/routingTreeView.tsx` / `views/rfChainEdge.tsx` / `views/node/rfRuleNode.tsx` / `views/node/rfSourceNode.tsx` / `views/node/rfConditionNode.tsx` / `views/node/rfEdgeHandle.tsx`；新增 key 追加到 `routing.json`（绿）
- [ ] 2.10 Complexity Router 菜单项 i18n 化：`ui/app/workspace/complexity-router/page.tsx`（✓已 i18n）+ 补缺漏文案（绿）
- [ ] 2.11 Custom Pricing 菜单项 i18n 化：`ui/app/workspace/custom-pricing/page.tsx`（✓已 i18n）+ 补缺漏文案（绿）
- [ ] 2.12 Custom Pricing → Overrides 子路由 i18n 化：`ui/app/workspace/custom-pricing/overrides/page.tsx`（✗）+ `pricingOverrideSheet.tsx` / `scopedPricingOverridesView.tsx` / `pricingOverridesEmptyState.tsx` / `pricingFieldSelector.tsx`；新增 key 追加到 `config.json`（绿）
- [ ] 2.13 Plugins 菜单项 i18n 化：`ui/app/workspace/plugins/page.tsx`（✓已 i18n）+ `views/pluginsView.tsx` + `sheets/pluginSequenceSheet.tsx` / `sheets/addNewPluginSheet.tsx` + `dialogs/confirmDeletePluginDialog.tsx` + `fragments/pluginFormFragments.tsx` / `fragments/providercooldownFragment.tsx`；`views/pluginsEmptyState.tsx` 已 ✓ 跳过；新增 key 追加到 `plugins.json`（绿）
- [ ] 2.14 Settings 父页（`nav.settings`） i18n 化：`ui/app/workspace/config/page.tsx`（✗）；新增 key 追加到 `config.json`（绿）
- [ ] 2.15 Settings → Client Settings 子路由 i18n 化：`/workspace/config/client-settings/page.tsx` + `views/clientSettingsView.tsx`（绿）
- [ ] 2.16 Settings → Compatibility 子路由 i18n 化：`/workspace/config/compatibility/page.tsx` + `views/compatibilityView.tsx`（绿）
- [ ] 2.17 Settings → Caching 子路由 i18n 化：`/workspace/config/caching/page.tsx` + `views/cachingView.tsx`（绿）
- [ ] 2.18 Settings → Security 子路由 i18n 化：`/workspace/config/security/page.tsx` + `views/securityView.tsx`（绿）
- [ ] 2.19 Settings → API Keys 子路由 i18n 化：`/workspace/config/api-keys/page.tsx` + `views/apiKeysView.tsx`（绿）
- [ ] 2.20 Settings → Performance Tuning 子路由 i18n 化：`/workspace/config/performance-tuning/page.tsx` + `views/performanceTuningView.tsx`（绿）
- [ ] 2.21 Settings → Feature Flags 子路由 i18n 化：`/workspace/config/feature-flags/page.tsx` + `views/featureFlagsView.tsx`（绿）
- [ ] 2.22 Settings → Logging 子路由 i18n 化：`/workspace/config/logging/page.tsx` + `views/loggingView.tsx`（绿）
- [ ] 2.23 Settings → MCP Gateway 子路由 i18n 化：`/workspace/config/mcp-gateway/page.tsx` + `views/mcpView.tsx`（绿）
- [ ] 2.24 Settings → Observability 子路由 i18n 化：`/workspace/config/observability/page.tsx` + `views/observabilityView.tsx`（绿）
- [ ] 2.25 Settings → Large Payload 子路由 i18n 化：`/workspace/config/large-payload/page.tsx` + `views/modelSettingsView.tsx`（绿）
- [ ] 2.26 Settings → Pricing Config 子路由 i18n 化：`/workspace/config/pricing-config/page.tsx` + `views/pricingConfigView.tsx`（绿）
- [ ] 2.27 Settings → Proxy 子路由 i18n 化（如 `ui/app/workspace/config/proxy/page.tsx` 存在）：`/workspace/config/proxy/page.tsx` + `views/proxyView.tsx`；新增 key 追加到 `config.json`（绿，若文件不存在则跳过此任务项）
- [ ] 2.28 LogsFilterSidebar 共享 FilterSidebar i18n 化：`ui/components/filters/logsFilterSidebar.tsx`（被 Logs + Dashboard 共用，都可见）；filter 选项（status / provider / model / virtual key / 时间范围 等）走 i18n；新增 key 追加到 `logs.json`（绿）
- [ ] 2.29 修改 `ui/components/sidebar.tsx`：promo card 标题/按钮（"Restart Required" / "Setup checklist incomplete" / "Resume setup" / "View release notes"）/ 搜索 placeholder / Collapse/Expand/Logout aria-label / GitHub Repository externalLinks label 全部走 i18n；新增 key 追加到 `common.json`（绿）
- [ ] 2.30 重命名 `ui/locales/{en,zh-CN}/logs.json` 中 `COLUMN_LABELS` → `column_labels`（顶层 + mcpLogs 子对象）；同步修改 `ui/app/workspace/logs/page.tsx`（15 处）+ `ui/app/workspace/mcp-logs/page.tsx`（6 处）的引用（绿）
- [ ] 2.31 为 toast 文案新增 key 到各 workspace namespace：`providers.toast.*` / `config.toast.*` / `logs.toast.*` / `mcp.toast.*` / `governance-ui.toast.*` / `dashboard.toast.*` / `model-catalog.toast.*` / `routing.toast.*` / `plugins.toast.*` / `webhooks.toast.*` 等；逐文件替换 302 处硬编码 `toast.success("...")` / `toast.error("...")` 为 `toast.success(t("..."))`（绿）
- [ ] 2.32 在 ESLint 配置中新增规则（`eslint-plugin-localized-text` 或自定义规则）：禁止 JSX 文本节点 / `toast.success("...")` / `<Input placeholder="...">` 中出现裸英文字面量；支持 `eslint-disable-next-line` 豁免；将 `npm run lint` 设为 CI 必跑（绿）
- [ ] 2.33 在 `package.json` 新增 `"i18n:check": "node ui/scripts/i18n-key-diff.mjs"`；确认 `npm run lint` + `npm run i18n:check` 双双零错误退出（绿）

## 3. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/enhance-workspace-i18n-coverage-3 做静态审查
- [ ] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 启动服务（如需）
- [x] 4.4 验证 V-ui-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-ui-4

## 5. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 6. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [x] 6.1 确认 `.pg/changes/enhance-workspace-i18n-coverage-3/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [x] 6.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [x] 6.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [x] 6.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [x] 6.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [x] 6.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [x] 6.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [x] 6.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [x] 6.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 7. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 7.1 收集所有 stage 的 Gate Assessment
- [ ] 7.2 检查跨 stage 依赖项
- [ ] 7.3 输出 Final Gate Assessment
