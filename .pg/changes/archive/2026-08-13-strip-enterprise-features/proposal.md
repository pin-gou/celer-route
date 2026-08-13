# strip-enterprise-features

**关联 issue**：无
**变更类型**：refactor

## 背景

当前 OSS 仓库（`maximhq/bifrost`）以"OSS + 企业版叠加"模式发布：核心引擎在 OSS 仓库中实现完整；企业版通过两条路径叠加：
1. **UI 层**：通过 `@enterprise/*` TypeScript 别名 + `ui/app/_fallbacks/enterprise/` 占位目录，在 OSS 构建中替换为含 "This feature is part of the Bifrost enterprise license" 文案的 placeholder 组件，在企业版构建中替换为真实功能组件。
2. **后端层**：在 OSS 代码中保留 enterprise-only 的钩子、no-op、注释、context key，但实际实现注入到企业版的 `bifrost-enterprise` 闭源包中。

本次变更是为个人使用场景（single-user、单进程、无多租户需求），剥离所有 enterprise 钩子、占位符、入口、schema 字段与部署资产，使仓库成为"纯 OSS"个人版本。

## 目标

剥离后达到：
- **UI 无任何 enterprise 占位符**：30+ 个企业版路由（rbac、scim、audit-logs、alerting、cluster、guardrails、circuit-breaker、branding、adaptive-routing、mcp-tool-groups、mcp-auth-config、access-profiles、prompt-deployments、edge-control、data-connectors、user-groups、user-rankings 等）从 `routeTree.gen.ts`、菜单、`page.tsx`、`layout.tsx` 中完全消失
- **代码无 `@enterprise` 别名引用**：100+ 个 `@enterprise/lib`、`@enterprise/components`、`@enterprise/types` import 重写为本地路径，`tsconfig.json` 与 `vite.config.mts` 中的 alias 配置删除，`ui/app/_fallbacks/enterprise/` 整个目录删除
- **后端无 enterprise 钩子残留**：`enterprisePlugins` 列表、`FeatureFlags.EnterpriseOnly`、`BifrostContextKeyClusterNodeID` 等 enterprise-only context key、`LargePayloadHook/LargeResponseHook/SSEReaderFactory`、`VaultResolveHook` 等可选钩子全部删除
- **config schema 无 enterprise 字段**：`config.schema.json` 中 9 个 enterprise 顶层字段（alerting、access_profiles、audit_logs、cluster_config、guardrails_config、large_payload_optimization、load_balancer_config、scim_config、circuit_breaker_config）+ 6 个嵌套字段（governance.business_units、governance.roles、governance.teams[].business_unit_id、governance.virtual_keys[].access_profile_id、mcp.tool_groups、plugins.kafka、plugins.google_cloud_pubsub）删除
- **helm-charts 无 enterprise 块**：`values.yaml`、`values.schema.json`、`_helpers.tpl` 中所有 enterprise 段删除
- **E2E 测试无 enterprise 引用**：`tests/e2e/features/mcp-tool-groups/spec.ts`、`transports/bifrost-http/handlers/utils_test.go` 中硬编码的 enterprise 表名引用清理
- **构建链简化**：`Makefile` 的 `cleanup-enterprise` 目标与 `install-ui` 对其依赖删除

## 范围

### 包含

- 删除 `ui/app/_fallbacks/enterprise/` 整个目录（含 `components/` 24 个子目录、`lib/` 完整 fallback 实现）
- 删除 `tsconfig.json`、`vite.config.mts` 中的 `@enterprise/*`、`@schemas/*` 别名配置
- 删除 30+ 个企业版路由的 `page.tsx`、`layout.tsx`（覆盖 `ui/app/workspace/` 下 11 个一级目录）
- 删除 `ui/components/sidebar.tsx` 中 3 个 IS_ENTERPRISE 条件菜单项（Proxy、Branding、License Info）+ `ui/lib/constants/config.ts` 中 `IS_ENTERPRISE` 常量
- 删除 `ui/lib/store/store.ts`、`ui/lib/store/slices/index.ts`、`ui/lib/store/apis/baseApi.ts` 中 enterprise middleware/reducer/State 引用
- 删除 8 个 enterprise RTK Query API stub（accessProfileApi、largePayloadApi、scimApi、virtualKeyUsersApi）
- 删除 OAuth tokenManager、baseQueryWithRefresh、rbacContext 等 fallback 实现
- 删除两个注册器 `userPicker`、`modelLimitScopes` 的 fallback 入口
- 删除 `ui/components/entitySelectors/` 下的 `customerSelector.tsx`、`teamSelector.tsx`（依赖被删 API）
- 重写 100+ 个 `@enterprise/*` import：`useRbac` 改为本地 `() => true` 实现、stubs 改为空函数、types 改为本地空 interface
- 后端清理：`transports/bifrost-http/server/server.go` 的 `enterprisePlugins` 列表、`framework/featureflags/featureflags.go` 的 `EnterpriseOnly` 字段与 `ErrFlagEnterpriseOnly`、20+ 个 `core/schemas/bifrost.go` 中的 enterprise-only `BifrostContextKey`、50+ 处 `// enterprise-only` / `// set by enterprise` 注释、`transports/bifrost-http/integrations/router.go` 的 `LargePayloadHook`/`LargeResponseHook`、`core/providers/utils/sse.go` 的 `SSEReaderFactory`、`core/schemas/vault.go` 的 4 个 vault 全局 hook、`Config.IsEnterprise` 字段、`ConfigData` 中的 enterprise 顶层字段
- 删除 `transports/config.schema.json` 中 9 个 enterprise 顶层字段 + 6 个嵌套字段
- 删除 `helm-charts/bifrost/values.yaml`、`values.schema.json`、`_helpers.tpl` 中的 enterprise 块
- 删除 `tests/e2e/features/mcp-tool-groups/spec.ts` 中 enterprise 提及
- 删除 `transports/bifrost-http/handlers/utils_test.go` 中硬编码的 `enterprise_access_profiles`、`enterprise_users` 表名引用（替换为通用 unique-constraint 检测）
- 删除 `Makefile` 的 `cleanup-enterprise` 目标与 `install-ui` 对其依赖
- 删除 `ui/app/workspace/governance/teams`、`customers`、`users`、`business-units` 路由（teamsView.tsx、customerDetailSheet.tsx 含真实 OSS 业务逻辑但属于多租户范畴，与个人使用语义不符）
- 删除 `ui/app/workspace/routing-rules/views/routingRuleSheet.tsx`、`routingRuleInfoSheet.tsx`、`tree/views/node/rfRuleNode.tsx` 中 customer/team/user scope 处理，scope 收窄为 `virtual_key / global`
- 删除 `ui/app/workspace/virtual-keys/views/virtualKeySheet.tsx`、`virtualKeysTable.tsx` 中 customer/team 选项，保留 VK 路由与基本限流功能
- 保留并搬出 `ui/app/_fallbacks/enterprise/components/api-keys/apiKeysIndexView.tsx` → `ui/app/workspace/config/api-keys/views/apiKeysView.tsx`
- 保留并搬出 `ui/app/_fallbacks/enterprise/components/login/loginView.tsx` → `ui/app/login/views/loginView.tsx`

### 不包含

- 重写 BifrostContext 中**非** enterprise-only 键（保留所有 OSS 业务用到的，如 `BifrostContextKeyAccumulatorID`、`BifrostContextKeyRequestID` 等）
- 修改 `core/schemas/provider.go` 中 30+ 方法的 `Provider` 接口（无 enterprise-only 方法）
- 修改 OpenAPI 文档（`docs/openapi/openapi.json` 不在此仓库）
- 修改 `plugins/governance/` 中的非 enterprise 业务逻辑（VK、Team、Customer 基础 CRUD、限流、预算保留）
- 修改 `framework/configstore/tables/` 中的 OSS 表结构（enterprise 表定义在外部 `bifrost-enterprise` 包，OSS 不存在这些表）
- 修改 `terraform/`（无 enterprise 内容）
- 外部 docs 仓库内容
- 重写 BifrostContext 中 governance 插件已使用但非 enterprise-only 的键（如 `BifrostContextKeyGovernance*` 是被 governance 插件中 user 级别 nogo path 使用的运行时字段，不删）
- Provider 实现层（`core/providers/<provider>/` 中各 provider 不依赖 enterprise 钩子）

## 方案概述

**前端**：

1. **别名移除与目录删除**：删除 `ui/app/_fallbacks/enterprise/` 整个目录树（24 个 components 子目录 + lib 全部）；删除 `tsconfig.json` 中 `paths."@enterprise/*"` 与 `paths."@schemas/*"` 配置；删除 `vite.config.mts` 中 `alias` 段、`isEnterpriseBuild` 检测、`BIFROST_IS_ENTERPRISE` 环境变量注入。
2. **路由删除**：删除 30+ 个企业版路由的 `page.tsx` 与 `layout.tsx`。TanStack Router 路由通过 `createFileRoute` 在 `layout.tsx` 中定义，删除 `layout.tsx` 后 vite build 会自动重新生成 `routeTree.gen.ts` 中不包含的条目。
3. **菜单清理**：`ui/components/sidebar.tsx` 删除 Proxy、Branding、License Info 三个 `IS_ENTERPRISE` 条件块；删除 `IS_ENTERPRISE` 常量定义。
4. **import 重写**：100+ 个 `@enterprise/lib`、`@enterprise/components`、`@enterprise/types` import 批量重写：
   - `useRbac(RbacResource.X, RbacOperation.Y)` 改为 `useRbac()`（新本地 hook 永远返回 `true`）—— `RbacResource`、`RbacOperation` 类型从 `@enterprise/lib` 改为本地定义
   - `@enterprise/lib/store/apis/...` RTK Query API 改为本地空 stub
   - `@enterprise/lib/store/slices/...` enterpriseMiddleware/reducers 引用删除
   - `@enterprise/components/...` 占位组件导入整段删除（连同路由删除）
   - `@enterprise/types/...` 类型改为本地空 interface
5. **基础设施清理**：
   - 删除 `ui/lib/store/store.ts` 中 enterprise reducer/middleware 注入
   - 删除 `ui/lib/store/slices/index.ts` 中 enterprise slice 导出
   - 删除 `ui/lib/store/apis/baseApi.ts` 中 `createBaseQueryWithRefresh`、`clearOAuthStorage` 引用
   - 删除 `ui/app/clientLayout.tsx` 中 `RbacProvider`（替换为本地空 provider 或 `<></>`）
   - 删除 `ui/lib/registries/userPicker.tsx`、`modelLimitScopes.tsx` 中 enterprise 注册项
6. **保留并搬出**：`apiKeysIndexView.tsx`、`loginView.tsx` 含真实 OSS 业务逻辑，搬到本地 `ui/app/workspace/config/api-keys/views/` 与 `ui/app/login/views/`，import 路径调整
7. **Selector 删除**：`customerSelector.tsx`、`teamSelector.tsx` 删除（依赖被删的 `useGetTeamsQuery`/`useGetCustomersQuery`，且个人使用不需要多租户分配）
8. **VK/Routing Rules 收紧**：`virtualKeySheet.tsx`、`virtualKeysTable.tsx` 中 customer/team picker 删除；`routingRuleSheet.tsx`、`routingRuleInfoSheet.tsx` 中 scope 选项收窄为 `virtual_key / global`

**后端**：

1. **enterprisePlugins 删除**：`transports/bifrost-http/server/server.go:63-68` 删除 `var enterprisePlugins = []string{...}`；`server/plugins.go:353` 删除 `slices.Contains(enterprisePlugins, cfg.Name)` 检查（改为简单 logger 错误）
2. **FeatureFlags.EnterpriseOnly 清理**：`framework/featureflags/featureflags.go` 删除 `EnterpriseOnly bool` 字段、`ErrFlagEnterpriseOnly` 错误常量、`registry.go` 中的 enterprise-only 分支
3. **enterprise-only context key 清理**：`core/schemas/bifrost.go` 中 20+ 个被 enterprise 设置的键（如 `BifrostContextKeyClusterNodeID`、`BifrostContextKeyGovernanceBusinessUnitID` 等）整体删除——但**保留** governance 插件已使用的非 enterprise 键（如 `BifrostContextKeyGovernance*` 中被非 enterprise 路径引用的键）
4. **LargePayloadHook/LargeResponseHook 删除**：`transports/bifrost-http/integrations/router.go` 删除 `LargePayloadHook`/`LargeResponseHook` 类型与字段，`handlers/integrations.go` 删除 `SetLargePayloadHook`/`SetLargeResponseHook` 方法
5. **SSEReaderFactory 删除**：`core/providers/utils/sse.go` 删除 `SSEReaderFactory` 注入逻辑；`core/schemas/bifrost.go` 删除 `BifrostContextKeySSEReaderFactory`
6. **Vault hooks 删除**：`core/schemas/vault.go` 删除 4 个 `VaultResolveHook`/`VaultRemoveHook`/`VaultStoreHook`/`VaultPrefixHook` 全局变量；`VaultStoreWriteEnabled` 函数删除
7. **注释清理**：全仓 grep `// enterprise-only` / `// set by enterprise` / `// OSS no-op` 标记，整段删除（保留必要的逻辑）
8. **ConfigData 顶层字段删除**：`transports/bifrost-http/lib/config.go` 删除 `Config.StreamingDecompressThreshold` 中 enterprise 引用、`Config.FeatureFlags` 注释清理、`promoteDeprecatedAccessProfileCalendarAligned` 注释删除
9. **schema 字段删除**：`transports/config.schema.json` 删除 enterprise 顶层 + 嵌套字段
10. **Plugin no-op 清理**：`plugins/governance/store.go` 中 `CheckUserBudget`/`CheckUserRateLimit` 改为真实逻辑（OSS 中应该有但目前是 no-op——但**保留 no-op 行为**以避免破坏现有调用方，本变更不实现 user 级别 budget）

**helm-charts**：

1. `helm-charts/bifrost/values.yaml` 删除 `scim_config`、`access_profiles`、`vault_store`、`is_enterprise`、`large_payload_optimization`、`load_balancer_config`、`guardrails_config`、`cluster_config`、`circuit_breaker_config`、`audit_logs`、`alerting`、`enterprise-certificate-proxy` 等块
2. `helm-charts/bifrost/values.schema.json` 同步删除对应 schema 字段
3. `helm-charts/bifrost/templates/_helpers.tpl` 删除 Access Profiles、Vault Store、is_enterprise 注入段
4. `helm-charts/bifrost/README.md` 删除 enterprise 段落与私有仓库示例

**E2E / 测试**：

1. 删除 `tests/e2e/features/mcp-tool-groups/` 整个 spec 目录（OSS 中无对应组件）
2. `transports/bifrost-http/handlers/utils_test.go` 中硬编码的 `enterprise_access_profiles`、`enterprise_users` 表名改为通用 unique-constraint 文本匹配
3. `transports/bifrost-http/handlers/plugins_test.go` 中 `enterprise-governance` plugin 期望从期望列表删除
4. `transports/bifrost-http/handlers/governanceroutes_test.go` 中 `"enterprise"` 覆盖期望改为 `""` 或测试跳过

**Makefile**：

1. 删除 `.PHONY cleanup-enterprise` 与对应 `cleanup-enterprise` 目标
2. `install-ui` 目标移除对 `cleanup-enterprise` 的依赖

## 风险和注意事项

1. **大规模 import 重写风险**：`useRbac` 在 60+ 文件中被调用，重写 import + 替换为新本地 hook 引入编译错误概率高。建议 pg-build 阶段按目录批量替换，分批验证（先 core、framework，后 transports、plugins、ui）。
2. **routeTree.gen.ts 自动重新生成**：删除 30+ 个企业版路由的 layout.tsx 后，第一次 `npm run build` 会自动重生成 routeTree，但可能暂时性破坏其他路由的引用。需在 build 失败时手动调整 `routeTree.gen.ts` 中的 import 顺序。
3. **后端 context key 删除可能影响运行时**：删除 `BifrostContextKeyClusterNodeID` 等前必须双向 grep set/get 站点——例如 `BifrostContextKeyGovernanceBusinessUnit*` 被 governance 插件中 user 级别 nogo path 使用，**必须保留**这些键（仅删除未被任何非 enterprise 代码使用的 enterprise-only 键）。
4. **helm 渲染失败**：删除 helm 块后必须跑 `helm template` 验证；values.schema.json 与 values.yaml 必须同步。
5. **E2E 测试套件被破坏**：enterprise 相关 spec 文件直接删除；其他 spec 中 enterprise 提及按 grep 修正。
6. **个人使用语义**：剥离后部分功能（如 Virtual Keys 中的 customer/team 选项、Routing Rules 中的多租户 scope）将不可用。这是预期行为——个人使用场景无多租户需求。
7. **provider-harness.json 不涉及**：本变更不修改 `tests/e2e/api/collections/provider-harness.json`（覆盖 LLM provider 场景，与 enterprise 管理面无关）。
8. **V-core-4 真实 LLM 推理验证（已 skipped）**：本变更后调用真实 chat completions 路径需付费 provider API key，个人环境跳过；proposal.md "未做" 段列出。
9. **SCIM OAuth 端点降级**：删除 enterprise 钩子后，`/api/scim/oauth/*` 等 middlewares.go 中已声明的公开路径会进入 404 处理（OSS handler 从未实现）。V-transports-2 已标 degraded。