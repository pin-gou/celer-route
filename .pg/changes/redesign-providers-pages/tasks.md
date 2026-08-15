> - **environment 选择**：dev → local，int → local

## 1. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 1.1 编写单元测试：`transports/bifrost-http/handlers/providers_test.go` 新增 `TestListProvidersAggregatedFields` 用例：mock 一个 provider + 3 keys + 5 models + 当日 100 条 logs（含 3 条错误），断言 `GET /api/providers` 响应中 `keys_count=3`、`models_count=5`、`keys_health_status`、`today_requests=100`、`today_errors=3`、`last_used_at`、`last_error_at`、`uptime`、`avg_latency_ms` 字段全部存在且值正确（红）
- [x] 1.2 编写单元测试：`transports/bifrost-http/handlers/providers_test.go` 新增 `TestBatchUpdateProviderKeys` 用例 happy path：mock provider 含 3 keys，POST `{"key_ids":[k1,k2,k3],"enabled":true}` 断言响应 200 + `updated=3`，DB 内 3 个 key 全部 `enabled=true`（红）
- [x] 1.3 编写单元测试：`transports/bifrost-http/handlers/providers_test.go` 新增 `TestBatchUpdateProviderKeys_RollbackOnMissing` 用例：mock provider 含 3 keys，POST 包含不存在 key_id "k-bad"，断言响应 400 + `missing_key_ids:["k-bad"]`，且 DB 内**没有任何** key 被更新（事务回滚验证，红）
- [x] 1.4 编写单元测试：`transports/bifrost-http/handlers/providers_test.go` 新增 `TestProviderResponse_BackwardCompat` 用例：解析 fixture providers（来自 `.pg/hooks/local/data/config.db`）的响应，断言所有原有字段（`name` / `network_config` / `provider_status` 等）仍存在，且 9 个新字段**缺省时为 zero value**（不破坏现有 wire 契约，红）

## 2. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 2.1 `transports/bifrost-http/handlers/providers.go` 的 `ProviderResponse` struct 追加 9 个只读字段：`KeysCount int`、`ModelsCount int`、`KeysHealthStatus string`、`TodayRequests int`、`TodayErrors int`、`LastUsedAt *time.Time`、`LastErrorAt *time.Time`、`Uptime float64`、`AvgLatencyMs int`，全部加 `json` tag 与 `omitempty`（除 `omitempty` 不适用的基础类型）
- [ ] 2.2 `transports/bifrost-http/handlers/providers.go` 新增私有方法 `aggregateProviderStats(ctx, providerName) (ProviderStats, error)`：单次 GROUP BY 批拉 keys 计数 + models 计数 + 当日请求/错误 + 24h avg latency。**禁止 N+1 循环**——批查询必须按 provider 分组一次性返回
- [ ] 2.3 修改 `listProviders` handler：在循环中调用 `aggregateProviderStats`，将结果填入 `ProviderResponse` 新增字段；`getProvider` handler 同样填充
- [ ] 2.4 `transports/bifrost-http/handlers/provider_keys.go` 新增 `batchUpdateProviderKeys(ctx)` handler：解析 body `{key_ids:[]string, enabled:bool}`，开启 DB 事务 → 校验所有 key_id 属于该 provider（`missing_key_ids` 收集）→ `UPDATE keys SET enabled=? WHERE id IN (...)` → 提交
- [ ] 2.5 `transports/bifrost-http/handlers/provider_keys.go` 的 `RegisterRoutes` 注册 `r.POST("/api/providers/{provider}/keys/batch", lib.ChainMiddlewares(h.batchUpdateProviderKeys, middlewares...))`
- [ ] 2.6 在 `transports/bifrost-http/handlers/providers_test.go` 中跑通 1.1-1.4 写的单测（绿）
- [ ] 2.7 `cd transports && go build ./...` 确认编译通过
- [ ] 2.8 `cd transports && go vet ./...` 确认 lint 通过

## 3. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/redesign-providers-pages 做静态审查
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
  - verifiable: V-transports-1, V-transports-2, V-transports-3, V-transports-4, V-transports-5, V-transports-6

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

- [ ] 6.1 编写组件单元测试：`ui/app/workspace/providers2/views/ProviderCard.test.tsx` 新增测试：mock 1 个 active provider 数据，断言卡片渲染 Provider 图标 + 名称 + 健康度 Badge + "X keys" + "Y models" + "Z reqs" + Toggle + Quick test 按钮（红）
- [ ] 6.2 编写组件单元测试：`ui/app/workspace/providers2/views/ProviderFilters.test.tsx` 新增测试：输入搜索词 + 切换健康度 chip，断言 `onChange` 回调被正确触发且过滤参数符合预期（红）
- [ ] 6.3 编写组件单元测试：`ui/app/workspace/providers2/[id]/views/OverviewTab.test.tsx` 新增测试：mock provider 数据，断言 6 个内联编辑 fragment（Network/Proxy/Performance/Governance/Beta Headers/OpenAI Config）全部渲染（红）
- [ ] 6.4 编写 Playwright e2e spec：`tests/e2e/features/providers2/providers2-list.spec.ts` 新增用例：访问 `/workspace/providers2`，断言 5 个 fixture provider 分组卡片可见 + 名称搜索能过滤 + 健康度 chip 能切换（红）
- [ ] 6.5 编写 Playwright e2e spec：`tests/e2e/features/providers2/providers2-detail.spec.ts` 新增用例：访问 `/workspace/providers2/openai`，依次点击 6 个 Tab，断言每个 Tab 切换后内容区更新且无 console.error（红）

## 7. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 7.1 创建 `ui/app/workspace/providers2/layout.tsx`：使用 `createFileRoute("/workspace/providers2")` 注册新路由（与 `ui/app/workspace/providers/layout.tsx` 同级但独立文件）
- [ ] 7.2 创建 `ui/app/workspace/providers2/page.tsx`：列表页主组件，包含顶部 Toolbar + 分组 section 列表。复用 `useGetProvidersQuery` 但扩展 9 个聚合字段的 TS 类型
- [ ] 7.3 创建 `ui/app/workspace/providers2/views/ProviderFamilyGroup.tsx`：单个厂商家族 section 组件，接收一组 provider，按 family 名渲染折叠/展开列表
- [ ] 7.4 创建 `ui/app/workspace/providers2/views/ProviderCard.tsx`：单张 provider 卡片，含图标 + 名称 + CUSTOM 标签 + 健康度 Badge + Keys 数 + Models 数 + 今日请求 + 上次错误时间 + 批量启用/禁用 Toggle + Quick test 按钮（POST /refresh-models）
- [ ] 7.5 创建 `ui/app/workspace/providers2/views/ProviderFilters.tsx`：名称搜索 + provider 多选 + 健康度 chips（all/active/error）
- [ ] 7.6 创建 `ui/app/workspace/providers2/dialogs/TryLegacyViewButton.tsx`：顶部 "Try legacy view" 按钮，`onClick` 调 `router.navigate({ to: "/workspace/providers", search: { provider: currentProvider } })`
- [ ] 7.7 创建 `ui/app/workspace/providers2/[id]/layout.tsx` + `[id]/page.tsx`：详情路由 `createFileRoute("/workspace/providers2/$id")` + 主组件（Tabs 容器）
- [ ] 7.8 创建 `ui/app/workspace/providers2/[id]/views/OverviewTab.tsx`：内联编辑 Network / Proxy / Performance / Governance / Beta Headers / OpenAI Config 共 6 个 fragment（复用 `ui/app/workspace/providers/fragments/`）
- [ ] 7.9 创建 `ui/app/workspace/providers2/[id]/views/KeysTab.tsx`：复用 `modelProviderKeysTableView` + 新增 `useBatchUpdateProviderKeysMutation` hook（RTK Query mutation 调用新批量端点）
- [ ] 7.10 创建 `ui/app/workspace/providers2/[id]/views/ModelsTab.tsx` + `UsageTab.tsx` + `GovernanceTab.tsx` + `LogsTab.tsx`：分别复用 modelcatalog / logs 聚合 API / providerGovernanceTable / Logs 跳转入口
- [ ] 7.11 创建 `ui/app/workspace/providers2/[id]/dialogs/OpenLegacyConfigSheetButton.tsx`：触发现有 `ProviderConfigSheet` 的"Open legacy config sheet"按钮
- [ ] 7.12 修改 `ui/lib/store/apis/providersApi.ts`：扩展 `ProviderResponse` TS 类型追加 9 字段；新增 `useBatchUpdateProviderKeysMutation` hook
- [ ] 7.13 修改 `ui/lib/constants/nav.ts`：在 `Providers` 菜单下新增子项 `{ label: "Browse Providers (New)", to: "/workspace/providers2" }`
- [ ] 7.14 修改 `ui/app/workspace/providers/dialogs/confirmRedirection.tsx` 或新增按钮：在旧 `/workspace/providers` 页面顶部加 "Try new view" 按钮，链接到 `/workspace/providers2`
- [ ] 7.15 所有新增 React 组件加 `data-testid`（命名空间 `providers2-*`，与现有 testid 不冲突）
- [ ] 7.16 跑通 6.1-6.5 的单测与 e2e（绿）
- [ ] 7.17 `cd ui && npm run format` 确认格式通过
- [ ] 7.18 `cd ui && npm run build` 确认 build 通过

## 8. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/redesign-providers-pages 做静态审查
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
- [ ] 9.4 验证 V-transports-N：来自 design.md（N 由 design.md 决定；本次 UI E2E 验证项 V-transports-5、V-transports-6 由 dev.ui:verify 负责）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-transports-1, V-transports-2, V-transports-3, V-transports-4, V-transports-5, V-transports-6

## 10. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 11. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [ ] 11.1 确认 `.pg/changes/redesign-providers-pages/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 11.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 11.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 11.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 11.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 11.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [ ] 11.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [ ] 11.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [ ] 11.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 12. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 12.1 收集所有 stage 的 Gate Assessment
- [ ] 12.2 检查跨 stage 依赖项
- [ ] 12.3 输出 Final Gate Assessment
