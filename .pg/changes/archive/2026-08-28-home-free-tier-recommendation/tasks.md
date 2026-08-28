> - **environment 选择**：dev → local，int → local

## 1. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 `transports/celer-route-http/handlers/catalog_test.go` 单元测试：覆盖 `GET /api/catalog/bundles` 三路径——成功（mock server 返回合法 JSON）、空 bundles（mock 返回空数组）、上游失败（mock 返回 500 或无效 JSON），断言响应永远 200 + 不返 5xx（红：失败路径当前实现若返 5xx 必失败）
- [ ] 1.2 编写 `transports/celer-route-http/handlers/catalog_test.go` 单元测试：覆盖 ETag 协商——首次返回 200 + ETag 头；客户端带相同 `If-None-Match` 第二次返回 304 且 body 为空
- [ ] 1.3 编写 `transports/celer-route-http/handlers/logs_recent_test.go` 单元测试：覆盖 `GET /api/logs/recent-routing-rules` 三路径——正常聚合（100 条日志，5 个不同 routing_rule_id 去重）、空规则（所有日志 routing_rule_id=null）、limit 越界（limit=0/limit=99999 返 400 + INVALID_LIMIT）

## 2. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 2.1 在 `transports/config.schema.json` 新增 `remote_catalog` 段定义：`url_template` (string), `refresh_interval_seconds` (integer, default 3600, min 60), `max_bundles` (integer, default 100), `max_bundle_size_bytes` (integer, default 1048576), `max_provider_models` (integer, default 50)
- [ ] 2.2 修改 `transports/celer-route-http/lib/config.go` 解析 `remote_catalog` 段到 `Config.RemoteCatalog` 字段，加 `CheckAndSetDefaults` 设默认值
- [ ] 2.3 新增 `transports/celer-route-http/handlers/catalog.go`：定义 `bundleCatalog` 结构（含 sync.RWMutex + map + etags），实现 `startCatalogRefresher(ctx)` goroutine 周期拉取 + `getBundlesHandler(ctx)` 端点（ETag 协商 + 永远 200）
- [ ] 2.4 在 `transports/celer-route-http/handlers/catalog.go` 复用 `network.SSRFSafeDialContext` 防止 SSRF；连续 3 次拉取失败后停止 goroutine，INFO 日志输出原因
- [ ] 2.5 在 `transports/celer-route-http/handlers/logs.go` 追加 `recentRoutingRulesHandler(ctx)` 端点：单 SQL 查询 `SELECT routing_rule_id, routing_rule_name, MAX(timestamp) AS last_used_at, COUNT(*) AS use_count FROM logs WHERE routing_rule_id IS NOT NULL GROUP BY routing_rule_id ORDER BY last_used_at DESC LIMIT ?`，参数 limit 校验 [1, 1000]
- [ ] 2.6 在 `transports/celer-route-http/server/server.go` 注册 2 条新路由：`GET /api/catalog/bundles` 和 `GET /api/logs/recent-routing-rules`，权限中间件复用现有 auth
- [ ] 2.7 所有单测通过：`cd transports && go test ./celer-route-http/handlers/...`

## 3. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/home-free-tier-recommendation 做静态审查
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
- [ ] 4.4 验证 V-transports-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-transports-2, V-transports-3, V-ui-1, V-ui-2, V-ui-4
  - degraded: V-transports-1, V-ui-3, V-ui-5

## 5. dev.transports:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 6. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 6.1 编写 `ui/app/workspace/home/components/freeTierRecommendationCard.test.tsx` 单测（vitest + jsdom + MSW）：覆盖三种渲染状态——正常 bundle 列表、空 bundles 数组（渲染空状态 + 重试按钮）、网络错误（同上空状态）
- [ ] 6.2 编写 `ui/app/workspace/home/components/freeTierOneKeyConfigDialog.test.tsx` 单测：覆盖三种提交路径——成功（POST providers 200 → POST keys 200）、409 已存在（POST providers 409 → 继续 POST keys）、keyless provider（跳过 POST keys）
- [ ] 6.3 编写 `ui/app/workspace/home/hooks/useRecentRoutingRulesQuery.test.ts` 单测：mock RTK Query 返回 fixture 数据，断言 hook 返回结构正确
- [ ] 6.4 中英文 i18n 键值对齐静态扫描脚本：在 `ui/locales/en/home.json` 和 `ui/locales/zh-CN/home.json` 中遍历 `freeTier.*` 段，断言 key 集合完全一致

## 7. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 7.1 修改 `ui/app/workspace/home/views/homePage.tsx`：移除 quickStartCard import 与渲染，替换为 FreeTierRecommendationCard；保留其余 4 个 card
- [ ] 7.2 新增 `ui/app/workspace/home/components/freeTierRecommendationCard.tsx`：RTK Query 调 `GET /api/catalog/bundles?lang=<currentLocale>`，三种渲染分支（正常 / 空 / 错误），data-testid="home-free-tier-card"
- [ ] 7.3 新增 `ui/app/workspace/home/components/bundleApplyCard.tsx`：单个 bundle 渲染（标题 / 描述 / providers / 申请链接 / 一键配置按钮 / 最近路由 footer），data-testid=`home-free-tier-bundle-${bundle.id}`
- [ ] 7.4 新增 `ui/app/workspace/home/components/freeTierOneKeyConfigDialog.tsx`：弹窗调 `useCreateProviderMutation` + `useCreateProviderKeyMutation`，409 翻译为 "已配置" toast，跳过 keyless provider，data-testid="home-free-tier-config-dialog"
- [ ] 7.5 新增 `ui/app/workspace/home/hooks/useRecentRoutingRulesQuery.ts`：包装 `useGetRecentRoutingRulesQuery`，参数 limit 默认 100
- [ ] 7.6 新增 `ui/lib/store/apis/catalogApi.ts`：RTK Query slice，含 `useGetBundlesQuery` + `useCreateProviderMutation` + `useCreateProviderKeyMutation` + `useGetRecentRoutingRulesQuery`
- [ ] 7.7 在 `ui/locales/en/home.json` 和 `ui/locales/zh-CN/home.json` 增补 `freeTier.*` 段（25 键）：title / updatedAt / applyNow / configureNow / noBundles / retry / recentRoutingRules / configSuccess / configFailed / alreadyConfigured / keylessNote / 等
- [ ] 7.8 所有单测通过 + 格式化：`cd ui && npm run format && npm run build && npm test`

## 8. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/home-free-tier-recommendation 做静态审查
- [ ] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 9. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 9.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 9.2 执行测试（runner 通过 modules 注入命令）
- [ ] 9.3 启动服务（如需）
- [ ] 9.4 验证 V-ui-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-transports-2, V-transports-3, V-ui-1, V-ui-2, V-ui-4
  - degraded: V-transports-1, V-ui-3, V-ui-5

## 10. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 11. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (scenario track; enabled by scenario_tracks_decision)
-->

- [ ] 11.1 确认 `.pg/changes/home-free-tier-recommendation/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 11.2 校验 scenario_id 全局唯一、critical 字段为 bool
- [ ] 11.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 11.4 校验 scenario given/then 中所有 `{env.*}` 占位符引用真实资源（pg-validate-proposal.py 已在阶段 3 校验）
- [ ] 11.5 启动 scenario-execute agent：按 critical=true 优先执行，failure → record(scenario-execute, "escalate") → 调度 scenario-fix agent → 修复后重跑
- [ ] 11.6 收集所有 Scenario 执行结果到 evidence 段路径（`<change-dir>/2-build/<report_seq>-<scenario_id>-evidence.{json,png}`）
- [ ] 11.7 scenario-execute 报告 critical=true 全 PASS → 该 track 进入 gate
- [ ] 11.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")

## 12. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [x] 12.1 收集所有 stage 的 Gate Assessment
- [x] 12.2 检查跨 stage 依赖项
- [x] 12.3 输出 Final Gate Assessment
