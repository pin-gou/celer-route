# strip-enterprise-features 设计

## 架构概览

本变更同时涉及**前端 UI 层**与**后端 Go 多模块**与**部署资产（helm-charts）**，跨 6 个模块（`core` / `framework` / `transports` / `cli` / `plugins` / `ui`）。

### 前端架构

```
┌─────────────────────────────────────────────────────────────┐
│                    ui/app/_fallbacks/enterprise/            │  ← 整个目录删除
│   ┌──────────────┬──────────────┬──────────────────┐         │
│   │ components/  │ lib/         │ types/           │         │
│   │ (24 dirs)    │ contexts/    │ UserAccessProfile│         │
│   │ placeholders │ registrations│ LargePayloadConfig│        │
│   │              │ store/      │ User             │         │
│   │              │ utils/      │                  │         │
│   │              │ index.ts    │                  │         │
│   └──────────────┴──────────────┴──────────────────┘         │
└─────────────────────────────────────────────────────────────┘
                          ↓ 删除后
┌─────────────────────────────────────────────────────────────┐
│                  ui/lib/rbac.ts (新本地实现)                │
│   - useRbac = () => true                                    │
│   - RbacResource enum (OSS 用到的子集)                       │
│   - RbacOperation enum                                      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                       ui/app/workspace/                     │
│   ┌─────────────────────────────────────────────────┐        │
│   │ 删除的路由目录（11 个一级）:                    │        │
│   │  alerting/    audit-logs/   brandings/         │        │
│   │  circuit-breaker/  cluster/  config/branding/  │        │
│   │  config/license/  config/proxy/  edge-control/ │        │
│   │  governance/access-profiles/                   │        │
│   │  governance/business-units/                    │        │
│   │  governance/customers/  governance/rbac/       │        │
│   │  governance/teams/  governance/users/          │        │
│   │  guardrails/  mcp-auth-config/                 │        │
│   │  mcp-tool-groups/  prompt-deployments/         │        │
│   │  scim/  user-rankings/  adaptive-routing/      │        │
│   └─────────────────────────────────────────────────┘        │
│   ┌─────────────────────────────────────────────────┐        │
│   │ 保留并调整:                                      │        │
│   │  virtual-keys/ (移除 customer/team 选项)        │        │
│   │  routing-rules/ (scope 收窄为 virtual_key/global)│       │
│   │  config/api-keys/ (搬出 fallback)               │        │
│   │  login/ (搬出 fallback)                         │        │
│   └─────────────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│            ui/lib/store/  &  ui/lib/registries/             │
│   - 移除 enterprise middleware/reducers/State               │
│   - 移除 OAuth stub (tokenManager / baseQueryWithRefresh)  │
│   - 移除注册器 fallback (userPicker / modelLimitScopes)    │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                 ui/components/sidebar.tsx                   │
│   - 删除 Proxy / Branding / License Info 三项 IS_ENTERPRISE│
│     条件菜单                                                │
│   - 删除 IS_ENTERPRISE 常量定义                             │
└─────────────────────────────────────────────────────────────┘
```

### 后端架构

```
┌─────────────────────────────────────────────────────────────┐
│ core/schemas/bifrost.go                                     │
│   - 删除 20+ enterprise-only BifrostContextKey:              │
│     BifrostContextKeyClusterNodeID                          │
│     BifrostContextKeyGovernanceBusinessUnit{ID,Name}        │
│     BifrostContextKeyGovernanceTeam{IDs,Names}              │
│     BifrostContextKeyGovernanceBusinessUnit{IDs,Names}      │
│     BifrostContextKeyGovernanceCustomer{IDs,Names}          │
│     BifrostContextKeyGovernanceScopedCustomerID             │
│     BifrostContextKeyUser{ID,Name,Email}                    │
│     BifrostContextKeyGuardrailDebug                         │
│     BifrostContextKeyRedactionData                          │
│     BifrostContextKeyLargePayloadContentType                │
│     BifrostContextKeyLargePayloadRequestThreshold           │
│     BifrostContextKeyLargeResponseThreshold                 │
│     BifrostContextKeyLargePayloadPrefetchSize               │
│     BifrostContextKeyDeferredLargePayloadMetadata           │
│     BifrostContextKeySSEReaderFactory                       │
│     BifrostContextKeyLargePayloadReader                     │
│     BifrostContextKeyLargePayloadContentLength              │
│     BifrostContextKeyLargePayloadMetadata                   │
│     BifrostContextKeyLargeResponseReader                    │
│     BifrostContextKeyDeferredUsage                          │
│   - 保留 governance 插件已使用的非 enterprise 键:            │
│     BifrostContextKeyGovernance*  (除上述外)                │
│     BifrostContextKeyNumberOfRetries / FallbackIndex         │
│     BifrostContextKeyStreamEndIndicator                     │
│     BifrostContextKeySelectedKeyID/Name                     │
│     BifrostContextKeyRequestID                              │
│     BifrostContextKeyAccumulatorID                          │
│     BifrostContextKeyExtraHeaders                           │
│     BifrostContextKeyURLPath                                │
│     BifrostContextKeySkipKeySelection                       │
│     BifrostContextKeyUseRawRequestBody                      │
│     BifrostContextKeyVirtualKey                            │
│     BifrostContextKeyAPIKeyName/ID                          │
│     BifrostContextKeyTrace*/Span*                            │
│     BifrostContextKeyIsEnterprise (运行时判断, 保留但去除 │
│                                       set 站点)             │
│   - 删除 VaultResolveHook/VaultRemoveHook/                   │
│     VaultStoreHook/VaultPrefixHook 全局变量                 │
│   - 删除 VisibilityFilter 中 EntityUser/Role/BusinessUnit/  │
│     AuditLog/AccessProfile/MCPToolGroup/                    │
│     PromptDeployment 枚举值                                │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ transports/bifrost-http/server/server.go                    │
│   - 删除 var enterprisePlugins = []string{                  │
│       "datadog", "bigquery", "pubsub", "kafka"}             │
│   - 删除 plugin 加载时的 slices.Contains 跳过逻辑           │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ framework/featureflags/featureflags.go                      │
│   - 删除 EnterpriseOnly bool 字段                           │
│   - 删除 ErrFlagEnterpriseOnly 错误                         │
│   - 删除 registry.go 中 EnterpriseOnly 分支                 │
│   - 删除 SyncDelegate 接口（仅 enterprise 实现）             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ transports/bifrost-http/integrations/router.go              │
│   - 删除 LargePayloadHook / LargeResponseHook 类型          │
│   - 删除 GenericRouter.largePayloadHook/largeResponseHook   │
│   - 删除 SetLargePayloadHook/SetLargeResponseHook 方法      │
│   - 删除 providerUtils.BuildStreamingClient 中 large 引用  │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ core/providers/utils/sse.go                                 │
│   - 删除 SSEReaderFactory 注入逻辑                          │
│   - 删除 BifrostContextKeySSEReaderFactory 引用             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ transports/bifrost-http/lib/config.go                       │
│   - 清理 enterprise 注释（promoteDeprecatedAccessProfile*） │
│   - 删除 initVault 引用（OSS 本就是 no-op）                  │
│   - 删除 Config.StreamingDecompressThreshold                │
│     中 enterprise 引用（值仍由 OSS 决定）                   │
│   - 删除 Config.FeatureFlags.SyncDelegate 注释              │
│   - 删除 Config.IsEnterprise 字段                           │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ transports/config.schema.json                               │
│   - 删除 9 个顶层字段:                                       │
│     alerting / access_profiles / audit_logs /                │
│     cluster_config / guardrails_config /                    │
│     large_payload_optimization / load_balancer_config /      │
│     scim_config / circuit_breaker_config                     │
│   - 删除 6 个嵌套字段:                                       │
│     governance.business_units / governance.roles /          │
│     governance.teams.items.business_unit_id /                │
│     governance.virtual_keys.items.access_profile_id /        │
│     mcp.tool_groups / plugins.items.kafka /                  │
│     plugins.items.google_cloud_pubsub /                      │
│     plugins.items.config.properties.is_enterprise             │
│   - 删除 enterpriseSchemaPaths /                            │
│     enterpriseSchemaFields 测试 map 引用                     │
└─────────────────────────────────────────────────────────────┘
```

### 部署资产

```
helm-charts/bifrost/
├── values.yaml        # 删除: scim_config, access_profiles, vault_store,
│                      #        is_enterprise, alerting, audit_logs,
│                      #        load_balancer_config, large_payload_optimization,
│                      #        guardrails_config, cluster_config,
│                      #        circuit_breaker_config
├── values.schema.json # 同步删除对应 schema
├── templates/_helpers.tpl # 删除 Access Profiles, Vault Store, is_enterprise
│                          # 注入段
└── README.md          # 删除 enterprise 段落与私有仓库示例
```

### 数据流

无运行时数据流变化（删除 enterprise 后，所有原本走 enterprise 钩子的路径简化为：

- `SetLargePayloadHook(enterpriseHook)` → 直接删除，路径上不再有 large payload 优化
- `CheckUserBudget()` → 保持现状（OSS 中已返回 `DecisionAllow`），删除 enterprise-only 注释
- `ReconcileOauthAfterMCPChange()` → OSS 中是 no-op，删除 enterprise-only 注释
- `externalQuotaBudgetResolver` → 删除 `RegisterExternalQuotaBudgetResolver` 调用（OSS 无注册）
- `RegisterScopeNameResolver("virtual_key", ...)` → 保留，但删除 governance.go:114-116 注释中 enterprise 说明

## API 设计

**无新增 API。** 本变更仅删除 enterprise-only 端点（OSS 后端从未实现这些端点，删除仅是清理 schema 与注释）。

不适用 Response Body 设计段。

## 数据模型

**无数据库 schema 变更。** `framework/configstore/` 中**不包含** enterprise 表（如 `enterprise_users`、`enterprise_access_profiles`）——这些表在外部 `bifrost-enterprise` 闭源包中定义。本变更删除的是以下注释与测试硬编码：

- `framework/configstore/migrations.go:9782-9837` 的 `migrationAddVKAccessProfileIDColumn` 与 `migrationDropVKAccessProfileIDColumn`（删除 `access_profile_id` 字段迁移——该字段在 OSS 中从未启用，迁移也是 no-op）
- `transports/bifrost-http/handlers/utils_test.go` 中硬编码的 `enterprise_access_profiles`、`enterprise_users` 表名引用

不修改 `framework/configstore/tables/` 中 OSS 表结构。

## 组件设计

### 前端组件拆分

**删除组件**（30+ 占位组件全部删除）：
```
ui/app/_fallbacks/enterprise/components/
├── access-profiles/    (3 files)   # 纯占位符
├── adaptive-routing/   (1 file)    # 纯占位符
├── alerting/           (4 files)   # 纯占位符
├── audit-logs/         (1 file)    # 纯占位符
├── branding/           (1 file)    # 纯占位符
├── circuit-breaker/    (1 file)    # 纯占位符
├── cluster/            (1 file)    # 纯占位符
├── data-connectors/    (4 dirs × 1 file)  # 纯占位符
├── edge-control/       (3 files + fallbackWrapper)  # 纯占位符
├── guardrails/         (2 files)   # 纯占位符
├── license/            (1 file)    # 纯占位符
├── mcp-auth-config/    (1 file)    # 纯占位符
├── mcp-tool-groups/    (1 file)    # 纯占位符
├── prompt-deployments/ (2 files)   # 纯占位符
├── rbac/               (1 file)    # 纯占位符
├── scim/               (2 files)   # 纯占位符
├── user-groups/        (4 files)   # usersView/businessUnitsView 纯占位符
│                                  # teamsView/customerDetailSheet 真实业务逻辑
│                                  # （删除路由但不删除组件，搬到非 fallback 位置）
└── user-rankings/      (1 file)    # 纯占位符
```

**保留并搬出**（2 个含真实 OSS 业务逻辑的组件）：
- `ui/app/_fallbacks/enterprise/components/api-keys/apiKeysIndexView.tsx` → `ui/app/workspace/config/api-keys/views/apiKeysView.tsx`
- `ui/app/_fallbacks/enterprise/components/login/loginView.tsx` → `ui/app/login/views/loginView.tsx`

搬出后 import 路径调整：
- `import { useGetCoreConfigQuery } from "@/lib/store"` → 不变
- `import ContactUsView from "../views/contactUsView"` → 删除（搬出后无引用）
- `import { useCopyToClipboard } from "@/hooks/useCopyToClipboard"` → 不变
- 删除 ContactUsView 整个文件（不再被任何组件引用）

**本地新增**（rbac 无操作版本）：
- `ui/lib/rbac.ts` —— 包含 `useRbac()`（返回 true）、`RbacResource` enum（仅保留 OSS 用到的子集）、`RbacOperation` enum

```typescript
// ui/lib/rbac.ts
export enum RbacResource {
  VirtualKeys = "VirtualKeys",
  ModelProvider = "ModelProvider",
  Settings = "Settings",
  Logs = "Logs",
  Observability = "Observability",
  // ... 仅 OSS 路由用到的子集
}

export enum RbacOperation {
  Read = "Read", View = "View", Create = "Create",
  Update = "Update", Delete = "Delete", Reveal = "Reveal", Download = "Download",
}

export function useRbac(_resource?: RbacResource, _operation?: RbacOperation): boolean {
  return true;
}
```

**删除 Selector**：
- `ui/components/entitySelectors/customerSelector.tsx`
- `ui/components/entitySelectors/teamSelector.tsx`

### 交互逻辑

**Virtual Keys sheet** 删除以下交互：
- customer 选择器入口（删除）
- team 选择器入口（删除）

**Routing Rules sheet** 收窄 scope 选项：
- 删除 `team` / `customer` / `user` 三个 scope 选项
- 仅保留 `virtual_key` / `global`

**Routing Rules tree 视图** 删除对 team/customer/user scope 的渲染分支：
- `tree/views/node/rfRuleNode.tsx` 中 `rule.scope !== "global" && rule.scope_id` 渲染逻辑改为仅 `rule.scope === "virtual_key"` 分支

## 关键约束与契约

### 前置条件

- 本地需已构建 `tmp/bifrost-http`（`make setup-workspace && make build`）
- Go workspace `go.work` 已就绪
- npm 工具链可用
- `describe_env` 已成功执行（env-description.yaml 已落盘）

### 影响面

**前端**：
- 哪些 UI 组件删除：`ui/app/_fallbacks/enterprise/components/` 全部 24 个子目录（`api-keys/` 与 `login/` 保留并搬出）
- 哪些路由删除：30+ 个企业版路由
- 哪些菜单项删除：sidebar.tsx 中 3 个 IS_ENTERPRISE 条件项
- 是否破坏对外 API：否（OSS 前端组件仅供 UI 内部消费）

**后端**：
- 哪些 Go 文件修改：`core/schemas/bifrost.go`、`transports/bifrost-http/server/server.go`、`framework/featureflags/featureflags.go`、`transports/bifrost-http/integrations/router.go`、`core/providers/utils/sse.go`、`transports/bifrost-http/lib/config.go`、`plugins/governance/store.go`、`plugins/governance/main.go`
- 哪些 context key 删除：20+ 个 enterprise-only BifrostContextKey（详见架构概览）
- 哪些 schema 字段删除：9 顶层 + 6 嵌套
- 是否破坏对外 API：否（OSS 后端从未注册 enterprise 端点）

**部署资产**：
- 哪些 helm 块删除：`values.yaml` 中 11 个 enterprise 段、`values.schema.json` 中对应字段、`_helpers.tpl` 中 4 段 enterprise 注入

**E2E**：
- 删除 `tests/e2e/features/mcp-tool-groups/` 整个 spec 目录
- 修改 `transports/bifrost-http/handlers/utils_test.go` 中 enterprise 表名硬编码
- 修改 `transports/bifrost-http/handlers/plugins_test.go` 中 enterprise-governance plugin 期望
- 修改 `transports/bifrost-http/handlers/governanceroutes_test.go` 中 "enterprise" 覆盖期望

### 性能契约

无性能契约变化（删除 enterprise 钩子不影响主路径性能）。

### 错误码与编号段

无新增错误码。

### 环境限制与验证策略

> **环境资源引用**（来自 `.pg/changes/strip-enterprise-features/0-define/define-summary.yaml` 的 `env_resource_refs`）：所有 verifiable V-* 引用 `{env.data_resources[name=config-db]}` 作为验证锚点（fixture 数据存在与否的判定依据）。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-ui-1 无 @enterprise 引用残留 | ✅ | 单元测试（grep） | n/a |
| V-ui-2 UI build 通过 | ✅ | 单元测试（`npm run build`） | n/a |
| V-ui-3 UI lint 与 typecheck 通过 | ✅ | 单元测试（`npm run lint` + `npm run typecheck`） | n/a |
| V-ui-4 enterprise 路由不存在 | ⚠️ degraded | 单元测试（grep workspace/ 子目录）+ 浏览器手动验证 | E2E 未在本环境跑 |
| V-ui-5 侧边栏无 enterprise 菜单项 | ⚠️ degraded | 单元测试（grep sidebar.tsx） + 浏览器手动验证 | E2E 未在本环境跑 |
| V-core-1 Go 模块 build 通过 | ✅ | 单元测试（`go build ./...`） | n/a |
| V-core-2 Go vet 通过 | ✅ | 单元测试（`go vet ./...`） | n/a |
| V-core-3 Go 单元测试通过 | ✅ | 单元测试（`go test ./... -short`） | n/a |
| V-transports-1 config.schema.json 同步校验通过 | ✅ | 单元测试（`config_test.go`） | n/a |
| V-transports-2 enterprise HTTP 端点返回 404 | ⚠️ degraded | prepare_env 启动 bifrost-api 后 curl 验证；OSS handler test 不引用 enterprise 路由注册 | prepare_env 未启动运行时，E2E 未跑 |
| V-plugins-1 governance 插件不引用被删键 | ✅ | 单元测试（go build） | n/a |
| V-helm-1 helm template 渲染通过 | ✅ | 单元测试（`helm template`） | n/a |
| V-core-4 真实 LLM provider 推理验证 | ❌ skipped | scenario 不写 | proposal.md "未做" 段列出 |

### 可观测性

无新增可观测性要求。

## Verification Criteria

### dev core Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-core-1 | Go 模块 build 通过 | go.work 已就绪 | `cd core && go build ./...` | 退出码 0，无错误 |
| V-core-2 | Go vet 通过 | go.work 已就绪 | `cd core && go vet ./...` | 退出码 0，无 vet warning |
| V-core-3 | Go 单元测试通过 | go.work 已就绪 | `cd core && go test ./... -short -count=1` | 所有测试通过 |

### dev framework Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-framework-1 | Go 模块 build 通过 | go.work 已就绪 | `cd framework && go build ./...` | 退出码 0，无错误 |

### dev transports Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | config.schema.json 同步校验通过 | 无 | `cd transports/bifrost-http && go test -run TestConfigSchemaSync ./lib/` | 通过 |
| V-transports-2 | enterprise HTTP 端点返回 404 | prepare_env 已启动 bifrost-http 实例 | curl GET /api/rbac /api/scim 等 | 全部返回 404 |

### dev cli Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-cli-1 | Go 模块 build 通过 | go.work 已就绪 | `cd cli && go build ./...` | 退出码 0，无错误 |

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | governance 插件不引用被删键 | go.work 已就绪 | `cd plugins/governance && go build ./...` | 退出码 0，无 undefined identifier |

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | 无 @enterprise 引用残留 | ui/ 目录就绪 | `grep -r '@enterprise' ui/ \| wc -l` | 命中数为 0（除 rbac.ts 中的 RbacResource 字面定义） |
| V-ui-2 | UI build 通过 | node_modules 已安装 | `cd ui && npm run build` | 退出码 0，产出 out/ |
| V-ui-3 | UI lint 与 typecheck 通过 | node_modules 已安装 | `cd ui && npm run lint && npm run typecheck` | 退出码 0，无错误 |
| V-ui-4 | enterprise 路由不存在 | ui/ 目录就绪 | `ls ui/app/workspace/{rbac,scim,audit-logs,alerting,cluster,guardrails,circuit-breaker,mcp-tool-groups,mcp-auth-config,access-profiles,business-units,user-rankings}` | 所有目录不存在 |
| V-ui-5 | 侧边栏无 enterprise 菜单项 | ui/ 目录就绪 | `grep -n 'IS_ENTERPRISE' ui/components/sidebar.tsx` | 命中数为 0 |

### int scenario Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-scenario-1 | 完整变更后端到端 build + lint 验证 | 全部 track 已完成 | `make build LOCAL=1 && cd ui && npm run build` | 退出码 0 |
| V-scenario-2 | helm template 渲染通过 | helm-cli 可用 | `helm template helm-charts/bifrost/` | 退出码 0，无渲染错误 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ✅ | 删除 20+ enterprise-only BifrostContextKey、Vault hooks、SSEReaderFactory |
| framework | ✅ | 删除 FeatureFlags.EnterpriseOnly、ErrFlagEnterpriseOnly、SyncDelegate 接口 |
| transports | ✅ | 删除 enterprisePlugins 列表、LargePayloadHook、注释清理、config_test.go enterpriseSchemaPaths、handlers/utils_test.go enterprise 表名硬编码 |
| cli | ❌ | 无 enterprise 引用，无需修改 |
| plugins | ✅ | 删除 enterprise-only 注释、确认 governance 插件不引用被删键 |
| ui | ✅ | 删除 30+ 路由、100+ import 重写、删除 fallback 目录、删除 sidebar 条件、删除 selector |

**affected_tracks**：`[core, framework, transports, plugins, ui]`

**scenario_tracks_decision**：

- 跨 role 协作验证？**否** —— 这是纯 refactor，不引入新 API 端点，不涉及跨 service 协作
- 新 API 端点？**否** —— 仅删除，不新增
- 跨模块联调？**是** —— 涉及 core/framework/transports/plugins/ui 6 模块联合编译验证

`scenario_decisions`：仅 scenario track 启用（用于跨模块联调验证），其他都按 standard 处理。
`scenario_reason`：跨模块联调场景需要 scenario track 验证 core/framework/transports/plugins/ui 联合 build 通过；纯 refactor 无新 API 端点也无跨 service 协作。