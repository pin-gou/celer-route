> - **environment 选择**：dev → local，int → local

## 1. dev.core:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 1.1 在 `core/schemas/` 与 `core/bifrost.go` 编写 Go 单元测试，确认 enterprise-only BifrostContextKey 已从源代码中删除，且未在 `core/schemas/context.go`、`core/inference.go`、`core/bifrost.go` 中被引用
- [ ] 1.2 在 `core/providers/utils/sse.go` 编写测试，确认 `SSEReaderFactory` 注入逻辑已删除，sse 解析路径回退到默认 bufio.Scanner
- [ ] 1.3 在 `core/schemas/vault.go` 编写测试，确认 4 个 vault 全局 hook 变量已删除，`GetValue` / `StoreVaultSecretVar` / `VaultPrefix` 在 OSS 下为 no-op（无 panic）
- [ ] 1.4 运行 `cd core && go test ./... -short -count=1` 确认全部测试通过

## 2. dev.core:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 2.1 删除 `core/schemas/bifrost.go` 中 20+ 个 enterprise-only `BifrostContextKey`（保留 governance 插件已使用的非 enterprise 键）
- [ ] 2.2 删除 `core/schemas/vault.go` 中 4 个 vault 全局 hook 变量与 `VaultStoreWriteEnabled` 函数
- [ ] 2.3 删除 `core/providers/utils/sse.go` 中 `SSEReaderFactory` 注入逻辑
- [ ] 2.4 清理 `core/` 目录下 `// enterprise-only` / `// set by enterprise` / `// OSS no-op` 注释
- [ ] 2.5 跑 `cd core && go build ./...` 与 `go vet ./...` 验证通过

## 3. dev.core:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.core:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 4.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 4.2 执行测试（runner 通过 modules 注入命令）
- [ ] 4.3 启动服务（如需）
- [ ] 4.4 验证 V-core-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 5. dev.core:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- 无

## 6. dev.framework:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 6.1 在 `framework/featureflags/` 编写测试，确认 `EnterpriseOnly` 字段、`ErrFlagEnterpriseOnly`、`SyncDelegate` 接口已删除，registry 不再检查 enterprise-only 分支

## 7. dev.framework:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 7.1 删除 `framework/featureflags/featureflags.go` 中 `EnterpriseOnly bool` 字段、`ErrFlagEnterpriseOnly` 错误常量
- [ ] 7.2 删除 `framework/featureflags/registry.go` 中 `EnterpriseOnly` 分支
- [ ] 7.3 删除 `framework/featureflags/featureflags.go` 中 `SyncDelegate` 接口与 `Store.delegate` / `delegateM` 字段
- [ ] 7.4 清理 `framework/` 目录下 enterprise 注释
- [ ] 7.5 跑 `cd framework && go build ./...` 与 `go vet ./...` 验证通过

## 8. dev.framework:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 9. dev.framework:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 9.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 9.2 执行测试（runner 通过 modules 注入命令）
- [ ] 9.3 启动服务（如需）
- [ ] 9.4 验证 V-framework-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 10. dev.framework:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- 无

## 11. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 11.1 确认 `transports/bifrost-http/lib/config_test.go` 的 `TestConfigSchemaSync` 与 `enterpriseSchemaPaths` map 引用全部清理（`enterpriseSchemaPaths` map 整体删除）
- [ ] 11.2 确认 `transports/bifrost-http/handlers/utils_test.go` 中硬编码的 `enterprise_access_profiles`、`enterprise_users` 表名引用已改为通用 unique-constraint 文本
- [ ] 11.3 确认 `transports/bifrost-http/handlers/plugins_test.go` 中 `enterprise-governance` plugin 名称期望已删除
- [ ] 11.4 确认 `transports/bifrost-http/handlers/governanceroutes_test.go` 中 `"enterprise"` 覆盖期望已删除
- [ ] 11.5 跑 `cd transports/bifrost-http && go test -short ./lib/ ./handlers/` 确认全部测试通过

## 12. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 12.1 删除 `transports/bifrost-http/server/server.go` 的 `var enterprisePlugins` 列表与对应 `slices.Contains` 跳过逻辑
- [ ] 12.2 删除 `transports/bifrost-http/integrations/router.go` 中 `LargePayloadHook` / `LargeResponseHook` 类型、字段、`SetLargePayloadHook` / `SetLargeResponseHook` 方法
- [ ] 12.3 删除 `transports/bifrost-http/handlers/integrations.go` 中 `SetLargePayloadHook` / `SetLargeResponseHook` 调用
- [ ] 12.4 删除 `transports/bifrost-http/lib/config.go` 中 enterprise 注释（`promoteDeprecatedAccessProfileCalendarAligned` 等），删除 `initVault` 与 `Config.IsEnterprise` 字段
- [ ] 12.5 删除 `transports/config.schema.json` 中 9 个 enterprise 顶层字段（`alerting` / `access_profiles` / `audit_logs` / `cluster_config` / `guardrails_config` / `large_payload_optimization` / `load_balancer_config` / `scim_config` / `circuit_breaker_config`）与 6 个嵌套字段
- [ ] 12.6 删除 `transports/bifrost-http/lib/config_test.go` 中 `enterpriseSchemaPaths` map 与 `enterpriseSchemaFields` map
- [ ] 12.7 修改 `transports/bifrost-http/handlers/utils_test.go` 中 enterprise 表名硬编码
- [ ] 12.8 修改 `transports/bifrost-http/handlers/plugins_test.go` 中 `enterprise-governance` plugin 期望
- [ ] 12.9 修改 `transports/bifrost-http/handlers/governanceroutes_test.go` 中 `"enterprise"` 覆盖期望
- [ ] 12.10 跑 `cd transports/bifrost-http && go build ./...` 与 `go vet ./...` 验证通过

## 13. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 13.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 13.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 13.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 13.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 14. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 14.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 14.2 执行测试（runner 通过 modules 注入命令）
- [ ] 14.3 启动服务（如需）
- [ ] 14.4 验证 V-transports-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 15. dev.transports:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 16. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 16.1 确认 `plugins/governance/store.go` 中 `CheckUserBudget` / `CheckUserRateLimit` 调用站点未被 enterprise 键污染
- [ ] 16.2 确认 `plugins/governance/main.go` 中 `Config.IsEnterprise` 引用已清理
- [ ] 16.3 跑 `cd plugins/governance && go test ./... -short -count=1` 确认通过

## 17. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 17.1 清理 `plugins/governance/` 下所有 `// enterprise-only` / `// set by enterprise` / `// OSS no-op` 注释（保留必要的逻辑分支）
- [ ] 17.2 跑 `for d in plugins/*/; do (cd "$d" && go build ./...); done` 验证通过

## 18. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 18.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 18.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 18.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 18.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 19. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 19.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 19.2 执行测试（runner 通过 modules 注入命令）
- [ ] 19.3 启动服务（如需）
- [ ] 19.4 验证 V-plugins-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 20. dev.plugins:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 21. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 21.1 编写 grep 测试：`grep -r '@enterprise' ui/ --include='*.ts' --include='*.tsx'` 命中数为 0（除 `ui/lib/rbac.ts` 中 `RbacResource` 字面定义外）
- [ ] 21.2 确认 `ui/app/_fallbacks/enterprise/` 整个目录已删除
- [ ] 21.3 编写 grep 测试：`ls ui/app/workspace/{rbac,scim,audit-logs,alerting,cluster,guardrails,circuit-breaker,mcp-tool-groups,mcp-auth-config,access-profiles,business-units,user-rankings}` 不存在
- [ ] 21.4 编写 grep 测试：`grep -n 'IS_ENTERPRISE' ui/components/sidebar.tsx` 命中数为 0

## 22. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 22.1 删除 `ui/app/_fallbacks/enterprise/` 整个目录（24 个 components 子目录 + lib 全部）
- [ ] 22.2 删除 `tsconfig.json` 中 `paths."@enterprise/*"` 与 `paths."@schemas/*"` 配置
- [ ] 22.3 删除 `vite.config.mts` 中 `isEnterpriseBuild` 检测、`alias` 段中的 `@enterprise` 与 `@schemas`、`BIFROST_IS_ENTERPRISE` 环境变量注入
- [ ] 22.4 删除 `ui/app/workspace/{rbac,scim,audit-logs,alerting,cluster,guardrails,circuit-breaker,mcp-tool-groups,mcp-auth-config,access-profiles,business-units,users,teams,customers,prompt-deployments,user-rankings,adaptive-routing,edge-control,config/license,config/branding,config/proxy}` 路由的 `page.tsx` 与 `layout.tsx`
- [ ] 22.5 把 `ui/app/_fallbacks/enterprise/components/api-keys/apiKeysIndexView.tsx` 搬到 `ui/app/workspace/config/api-keys/views/apiKeysView.tsx`，调整 import 路径
- [ ] 22.6 把 `ui/app/_fallbacks/enterprise/components/login/loginView.tsx` 搬到 `ui/app/login/views/loginView.tsx`，调整 import 路径
- [ ] 22.7 删除 `ui/components/entitySelectors/customerSelector.tsx` 与 `teamSelector.tsx`
- [ ] 22.8 创建 `ui/lib/rbac.ts`，定义 `useRbac()` 返回 true、`RbacResource` / `RbacOperation` enum（仅 OSS 用到的子集）
- [ ] 22.9 重写 100+ 个 `@enterprise/lib`、`@enterprise/components`、`@enterprise/types` import
- [ ] 22.10 修改 `ui/app/workspace/virtual-keys/views/virtualKeySheet.tsx` 与 `virtualKeysTable.tsx`，移除 customer/team picker，移除 `CustomerSelector` / `TeamSelector` 引用
- [ ] 22.11 修改 `ui/app/workspace/routing-rules/views/routingRuleSheet.tsx`、`routingRuleInfoSheet.tsx`、`tree/views/node/rfRuleNode.tsx`，scope 收窄为 `virtual_key / global`，删除 customer/team/user scope 分支
- [ ] 22.12 删除 `ui/components/sidebar.tsx` 中 Proxy、Branding、License Info 三个 IS_ENTERPRISE 条件菜单项
- [ ] 22.13 删除 `ui/lib/constants/config.ts` 中 `IS_ENTERPRISE` 常量定义
- [ ] 22.14 清理 `ui/lib/store/store.ts`、`ui/lib/store/slices/index.ts`、`ui/lib/store/apis/baseApi.ts`、`ui/app/clientLayout.tsx` 中 enterprise 引用
- [ ] 22.15 删除 `ui/lib/registries/userPicker.tsx` 与 `modelLimitScopes.tsx` 中 enterprise fallback 注册项
- [ ] 22.16 删除 `ui/app/_fallbacks/enterprise/components/views/contactUsView.tsx`（不再被任何组件引用）
- [ ] 22.17 跑 `cd ui && npm run build` 验证通过
- [ ] 22.18 跑 `cd ui && npm run lint && npm run typecheck` 验证通过

## 23. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 23.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 23.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 23.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 23.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 24. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 24.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 24.2 执行测试（runner 通过 modules 注入命令）
- [ ] 24.3 启动服务（如需）
- [ ] 24.4 验证 V-ui-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 25. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 26. int.core:test - int 测试先行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 26.1 在 `core/schemas/` 与 `core/bifrost.go` 编写 Go 单元测试，确认 enterprise-only BifrostContextKey 已从源代码中删除，且未在 `core/schemas/context.go`、`core/inference.go`、`core/bifrost.go` 中被引用

## 27. int.core:dev - 实现开发

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 27.1 删除 `core/schemas/bifrost.go` 中 20+ 个 enterprise-only `BifrostContextKey`（保留 governance 插件已使用的非 enterprise 键）
- [ ] 27.2 删除 `core/schemas/vault.go` 中 4 个 vault 全局 hook 变量与 `VaultStoreWriteEnabled` 函数
- [ ] 27.3 删除 `core/providers/utils/sse.go` 中 `SSEReaderFactory` 注入逻辑
- [ ] 27.4 清理 `core/` 目录下 `// enterprise-only` / `// set by enterprise` / `// OSS no-op` 注释
- [ ] 27.5 跑 `cd core && go build ./...` 与 `go vet ./...` 验证通过

## 28. int.core:review - 静态代码审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 28.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 28.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 28.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 28.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 29. int.core:verify - int 集成验证

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 29.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 29.2 执行测试（runner 通过 modules 注入命令）
- [ ] 29.3 启动服务（如需）
- [ ] 29.4 验证 V-core-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 30. int.core:gate - int 门控审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- 无

## 31. int.framework:test - int 测试先行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 31.1 在 `framework/featureflags/` 编写测试，确认 `EnterpriseOnly` 字段、`ErrFlagEnterpriseOnly`、`SyncDelegate` 接口已删除，registry 不再检查 enterprise-only 分支

## 32. int.framework:dev - 实现开发

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 32.1 删除 `framework/featureflags/featureflags.go` 中 `EnterpriseOnly bool` 字段、`ErrFlagEnterpriseOnly` 错误常量
- [ ] 32.2 删除 `framework/featureflags/registry.go` 中 `EnterpriseOnly` 分支
- [ ] 32.3 删除 `framework/featureflags/featureflags.go` 中 `SyncDelegate` 接口与 `Store.delegate` / `delegateM` 字段
- [ ] 32.4 清理 `framework/` 目录下 enterprise 注释
- [ ] 32.5 跑 `cd framework && go build ./...` 与 `go vet ./...` 验证通过

## 33. int.framework:review - 静态代码审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 33.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 33.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 33.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 33.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 34. int.framework:verify - int 集成验证

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 34.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 34.2 执行测试（runner 通过 modules 注入命令）
- [ ] 34.3 启动服务（如需）
- [ ] 34.4 验证 V-framework-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 35. int.framework:gate - int 门控审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- 无

## 36. int.transports:test - int 测试先行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 36.1 确认 `transports/bifrost-http/lib/config_test.go` 的 `TestConfigSchemaSync` 与 `enterpriseSchemaPaths` map 引用全部清理（`enterpriseSchemaPaths` map 整体删除）

## 37. int.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 37.1 删除 `transports/bifrost-http/server/server.go` 的 `var enterprisePlugins` 列表与对应 `slices.Contains` 跳过逻辑
- [ ] 37.2 删除 `transports/bifrost-http/integrations/router.go` 中 `LargePayloadHook` / `LargeResponseHook` 类型、字段、`SetLargePayloadHook` / `SetLargeResponseHook` 方法
- [ ] 37.3 删除 `transports/bifrost-http/handlers/integrations.go` 中 `SetLargePayloadHook` / `SetLargeResponseHook` 调用
- [ ] 37.4 删除 `transports/bifrost-http/lib/config.go` 中 enterprise 注释（`promoteDeprecatedAccessProfileCalendarAligned` 等），删除 `initVault` 与 `Config.IsEnterprise` 字段
- [ ] 37.5 删除 `transports/config.schema.json` 中 9 个 enterprise 顶层字段（`alerting` / `access_profiles` / `audit_logs` / `cluster_config` / `guardrails_config` / `large_payload_optimization` / `load_balancer_config` / `scim_config` / `circuit_breaker_config`）与 6 个嵌套字段
- [ ] 37.6 删除 `transports/bifrost-http/lib/config_test.go` 中 `enterpriseSchemaPaths` map 与 `enterpriseSchemaFields` map
- [ ] 37.7 修改 `transports/bifrost-http/handlers/utils_test.go` 中 enterprise 表名硬编码
- [ ] 37.8 修改 `transports/bifrost-http/handlers/plugins_test.go` 中 `enterprise-governance` plugin 期望
- [ ] 37.9 修改 `transports/bifrost-http/handlers/governanceroutes_test.go` 中 `"enterprise"` 覆盖期望
- [ ] 37.10 跑 `cd transports/bifrost-http && go build ./...` 与 `go vet ./...` 验证通过

## 38. int.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 38.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 38.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 38.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 38.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 39. int.transports:verify - int 集成验证

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 39.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 39.2 执行测试（runner 通过 modules 注入命令）
- [ ] 39.3 启动服务（如需）
- [ ] 39.4 验证 V-transports-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 40. int.transports:gate - int 门控审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 41. int.plugins:test - int 测试先行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 41.1 确认 `plugins/governance/store.go` 中 `CheckUserBudget` / `CheckUserRateLimit` 调用站点未被 enterprise 键污染

## 42. int.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 42.1 清理 `plugins/governance/` 下所有 `// enterprise-only` / `// set by enterprise` / `// OSS no-op` 注释（保留必要的逻辑分支）
- [ ] 42.2 跑 `for d in plugins/*/; do (cd "$d" && go build ./...); done` 验证通过

## 43. int.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 43.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 43.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 43.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 43.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 44. int.plugins:verify - int 集成验证

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 44.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 44.2 执行测试（runner 通过 modules 注入命令）
- [ ] 44.3 启动服务（如需）
- [ ] 44.4 验证 V-plugins-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 45. int.plugins:gate - int 门控审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 46. int.ui:test - int 测试先行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 46.1 编写 grep 测试：`grep -r '@enterprise' ui/ --include='*.ts' --include='*.tsx'` 命中数为 0（除 `ui/lib/rbac.ts` 中 `RbacResource` 字面定义外）

## 47. int.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 47.1 删除 `ui/app/_fallbacks/enterprise/` 整个目录（24 个 components 子目录 + lib 全部）
- [ ] 47.2 删除 `tsconfig.json` 中 `paths."@enterprise/*"` 与 `paths."@schemas/*"` 配置
- [ ] 47.3 删除 `vite.config.mts` 中 `isEnterpriseBuild` 检测、`alias` 段中的 `@enterprise` 与 `@schemas`、`BIFROST_IS_ENTERPRISE` 环境变量注入
- [ ] 47.4 删除 `ui/app/workspace/{rbac,scim,audit-logs,alerting,cluster,guardrails,circuit-breaker,mcp-tool-groups,mcp-auth-config,access-profiles,business-units,users,teams,customers,prompt-deployments,user-rankings,adaptive-routing,edge-control,config/license,config/branding,config/proxy}` 路由的 `page.tsx` 与 `layout.tsx`
- [ ] 47.5 把 `ui/app/_fallbacks/enterprise/components/api-keys/apiKeysIndexView.tsx` 搬到 `ui/app/workspace/config/api-keys/views/apiKeysView.tsx`，调整 import 路径
- [ ] 47.6 把 `ui/app/_fallbacks/enterprise/components/login/loginView.tsx` 搬到 `ui/app/login/views/loginView.tsx`，调整 import 路径
- [ ] 47.7 删除 `ui/components/entitySelectors/customerSelector.tsx` 与 `teamSelector.tsx`
- [ ] 47.8 创建 `ui/lib/rbac.ts`，定义 `useRbac()` 返回 true、`RbacResource` / `RbacOperation` enum（仅 OSS 用到的子集）
- [ ] 47.9 重写 100+ 个 `@enterprise/lib`、`@enterprise/components`、`@enterprise/types` import
- [ ] 47.10 修改 `ui/app/workspace/virtual-keys/views/virtualKeySheet.tsx` 与 `virtualKeysTable.tsx`，移除 customer/team picker，移除 `CustomerSelector` / `TeamSelector` 引用
- [ ] 47.11 修改 `ui/app/workspace/routing-rules/views/routingRuleSheet.tsx`、`routingRuleInfoSheet.tsx`、`tree/views/node/rfRuleNode.tsx`，scope 收窄为 `virtual_key / global`，删除 customer/team/user scope 分支
- [ ] 47.12 删除 `ui/components/sidebar.tsx` 中 Proxy、Branding、License Info 三个 IS_ENTERPRISE 条件菜单项
- [ ] 47.13 删除 `ui/lib/constants/config.ts` 中 `IS_ENTERPRISE` 常量定义
- [ ] 47.14 清理 `ui/lib/store/store.ts`、`ui/lib/store/slices/index.ts`、`ui/lib/store/apis/baseApi.ts`、`ui/app/clientLayout.tsx` 中 enterprise 引用
- [ ] 47.15 删除 `ui/lib/registries/userPicker.tsx` 与 `modelLimitScopes.tsx` 中 enterprise fallback 注册项
- [ ] 47.16 删除 `ui/app/_fallbacks/enterprise/components/views/contactUsView.tsx`（不再被任何组件引用）
- [ ] 47.17 跑 `cd ui && npm run build` 验证通过
- [ ] 47.18 跑 `cd ui && npm run lint && npm run typecheck` 验证通过

## 48. int.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 48.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 48.2 review agent 对 git diff feat/pg/strip-enterprise-features 做静态审查
- [ ] 48.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 48.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 49. int.ui:verify - int 集成验证

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 49.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 49.2 执行测试（runner 通过 modules 注入命令）
- [ ] 49.3 启动服务（如需）
- [ ] 49.4 验证 V-ui-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-core-1, V-core-2, V-core-3, V-transports-1, V-plugins-1, V-helm-1
  - degraded: V-ui-4, V-ui-5, V-transports-2
  - skipped: V-core-4

## 50. int.ui:gate - int 门控审查

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 51. int.scenario:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scenario (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scenario.yaml 读取

- [ ] 51.1 确认 `.pg/changes/strip-enterprise-features/scenario-scenario.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 51.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 51.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 51.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 51.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 51.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [ ] 51.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [ ] 51.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [ ] 51.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 52. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 52.1 收集所有 stage 的 Gate Assessment
- [ ] 52.2 检查跨 stage 依赖项
- [ ] 52.3 输出 Final Gate Assessment
