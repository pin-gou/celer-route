# enhance-ui-i18n-coverage 设计

## 架构概览

本变更仅涉及 `ui/` 模块（React + Vite + TanStack Router 前端），不影响后端 Go 模块。沿用上期 `add-ui-i18n-zh-en` 已交付的 i18n 基础设施（react-i18next + I18nProvider + LanguageSwitcher），按路由补齐 19 个未翻译 workspace 路由的表层翻译，新增 8 个 namespace 并扩展 3 个现有 namespace；同步改造 NoPermissionView 让其支持 i18n key 输入；补充 common.* 动作/状态键集。

### 整体数据流

```
┌──────────────────────────────────────────────────────────────────────────┐
│  用户浏览器 (Vite dev server :3008 / 生产构建静态资源)                       │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │  ui/app/main.tsx → ui/app/clientLayout.tsx                          │  │
│  │  ┌──────────────────────────────────────────────────────────────┐  │  │
│  │  │ I18nProvider (上期已交付)                                     │  │  │
│  │  │  ┌────────────────────────────────────────────────────────┐  │  │  │
│  │  │  │ 15 个 namespace 注册：                                  │  │  │  │
│  │  │  │   common / logs / config / governance / providers /    │  │  │  │
│  │  │  │   dashboard / governance-ui                            │  │  │  │
│  │  │  │  ── 本期新增 8 个 ──                                    │  │  │  │
│  │  │  │   mcp / routing / skills / plugins /                   │  │  │  │
│  │  │  │   observability / webhooks /                           │  │  │  │
│  │  │  │   oauth-grants / model-catalog                         │  │  │  │
│  │  │  │  ── 本期扩展 3 个 ──                                    │  │  │  │
│  │  │  │   logs (mcp-logs) / governance (model-limits) /        │  │  │  │
│  │  │  │   config (config 子路由 + custom-pricing + mcp-settings)│  │  │  │
│  │  │  └────────────────────────────────────────────────────────┘  │  │  │
│  │  │  ┌────────────────────────────────────────────────────────┐  │  │  │
│  │  │  │ 19 个未翻译路由的 layout.tsx:                            │  │  │  │
│  │  │  │   NoPermissionView entityI18nKey="<ns>:<route>..."     │  │  │  │
│  │  │  │ 路由内 page.tsx + views/*.tsx:                          │  │  │  │
│  │  │  │   useTranslation('<new-namespace>') → t('...')         │  │  │  │
│  │  │  └────────────────────────────────────────────────────────┘  │  │  │
│  │  └──────────────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │  ui/locales/                                                       │  │
│  │    ├─ en/        15 个 namespace × 2 个 locale = 30 个 JSON       │  │
│  │    └─ zh-CN/     同结构                                             │  │
│  │  加载策略: i18next-http-backend + 懒加载 (仅加载当前路由 namespace) │  │
│  └────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
```

### 涉及的前端组件

| 组件 | 路径 | 改造 |
|------|------|------|
| namespace 注册 | `ui/lib/i18n/config.ts` | `NS` 数组新增 8 项 + `resources` 对象新增 8 对 locale |
| 类型生成产物 | `ui/lib/i18n/types.ts` | `Resources` / `KeysWithNamespace` 扩展至 15 namespace |
| Locale 资源 | `ui/locales/{en,zh-CN}/*.json` | 新增 8 个 namespace × 2 locale = 16 个 JSON；扩展 logs / governance / config |
| NoPermissionView | `ui/components/noPermissionView.tsx` | 新增可选 `entityI18nKey` prop；保留 `entity: string` 向后兼容 |
| 19 路由 layout | `ui/app/workspace/{logs,mcp-logs,mcp-registry,...}/layout.tsx` | `NoPermissionView entityI18nKey` 切换 |
| 19 路由 page + views | `ui/app/workspace/{logs,...}/**/*.tsx` | 引入 `useTranslation` 替换硬编码英文 |

## API 设计

本变更不涉及后端 HTTP / gRPC / WebSocket 端点变更。i18n 资源全部落在前端，无 REST/GraphQL/gRPC 端点新增或修改。

## 数据模型

无数据库 schema 变更，无 SQL 迁移。

`localStorage` schema（沿用上期，无变更）：

| Key | 类型 | 示例值 | 备注 |
|-----|------|--------|------|
| `bifrost.locale` | string | `"zh-CN"` \| `"en"` | 用户上次选择的 locale；缺失时 fallback 到 `navigator.language` |

## 组件设计

### NoPermissionView 组件 API（改造）

```tsx
// ui/components/noPermissionView.tsx
interface NoPermissionViewProps {
  entity: string;                    // 保留向后兼容（fallback 字符串）
  entityI18nKey?: string;            // 新增：i18n key 路径如 "mcp:registry.permissionDenied"
  // ... 其他 props 不变
}

export function NoPermissionView({ entity, entityI18nKey, ...props }: NoPermissionViewProps) {
  const { t } = useTranslation();
  const displayText = entityI18nKey ? t(entityI18nKey) : entity;
  return <div>You don't have permission to view {displayText}</div>;
}
```

挂载调用示例：
```tsx
// ui/app/workspace/mcp-registry/layout.tsx
<NoPermissionView
  entity="mcp registry"                              // 保留作为 fallback
  entityI18nKey="mcp:registry.permissionDenied"      // 本期新增
/>
```

### namespace 归属表

| 路由 | 归属 namespace | 命名 |
|------|---------------|------|
| `logs` | `logs`（现有，容纳 +mcp-logs） | 复用并扩展 |
| `mcp-logs` | `logs`（同上） | 扩展 |
| `mcp-registry` | **`mcp`** | 新增 |
| `mcp-sessions` | `mcp` | 新增 |
| `model-limits` | `governance`（现有，扩展） | 扩展 |
| `routing-rules` | **`routing`** | 新增 |
| `complexity-router` | `routing` | 新增 |
| `skills-repo` | **`skills`** | 新增 |
| `plugins` | **`plugins`** | 新增 |
| `observability` | **`observability`** | 新增 |
| `webhooks` | **`webhooks`** | 新增 |
| `oauth-grants` | **`oauth-grants`** | 新增 |
| `custom-pricing` | `config`（现有，扩展） | 扩展 |
| `mcp-settings` | `config` | 扩展 |
| `config` (12 子路由) | `config` | 扩展 |
| `model-catalog` | **`model-catalog`** | 新增 |
| `virtual-keys` | （redirect，无需 namespace） | — |
| `docs` | （几乎无文案） | — |
| `prompt-repo` | （共享组件保持英文） | — |

## 关键约束与契约

### 前置条件

- 上一期 `add-ui-i18n-zh-en` 已合并（提供 i18n 基础设施）
- Node.js ≥ 18（项目已要求；Vite 8 需要）
- 无新 npm 依赖（仅复用上期已安装的 i18next / react-i18next / i18next-browser-languagedetector / vite-plugin-i18next-typescript）
- 无后端服务依赖，无 schema 迁移，无配置变更

### 影响面

- **修改**：`ui/lib/i18n/config.ts`、`ui/lib/i18n/types.ts`、`ui/components/noPermissionView.tsx`、`ui/locales/{en,zh-CN}/common.json`、`ui/locales/{en,zh-CN}/{logs,governance,config}.json`、19 个未翻译路由的 layout.tsx + page.tsx + views/**/*.tsx、`docs/features/i18n.mdx`
- **新增**：`ui/locales/{en,zh-CN}/{mcp,routing,skills,plugins,observability,webhooks,oauth-grants,model-catalog}.json`（16 个 JSON 文件）
- **不破坏对外 API**：本变更不修改任何 HTTP / gRPC / WebSocket 端点，对 Bifrost API 消费者零影响
- **TypeScript 类型签名保留**：`ui/lib/i18n/types.ts` 的 `Resources` 联合类型按 15 namespace 扩展，所有现有 `t('namespace.key')` 调用兼容

### 性能契约

- 单次首屏加载体积增长 ≤ 10 KB（gzip）—— 8 个新 namespace 资源 json 体量
- Namespace 懒加载：仅加载当前路由所需的 namespace（初始只加载 `common`，路由进入时按需加载新 namespace）
- `useTranslation()` 在 locale 切换时同步触发 `useSuspense: false` 避免 Suspense fallback —— 单次重渲染 < 16ms
- Vitest 键完整性测试运行时 < 10 秒（15 个 namespace × 2 locale × 文件 IO）

### 错误码与编号段

无新增错误码（前端 i18n 不涉及后端错误码体系）。

### 环境限制与验证策略

> **依据 `.pg/changes/enhance-ui-i18n-coverage/env-description.yaml` 中 `local` 环境 6 段判断**。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-ui-1 Vitest 键完整性测试（15 namespace 覆盖） | ✅ | 单元测试（`cd ui && npx vitest run src/__tests__/i18n/`） | n/a |
| V-ui-2 Vite 生产构建 + i18n TS 类型生成器（15 namespace 注册） | ✅ | `cd ui && npm run build` | n/a |
| V-ui-3 ui lint 通过（i18n 引用规范） | ✅ | `cd ui && npm run lint` | n/a |
| V-ui-4 Playwright i18n 切换 E2E（19 路由覆盖） | ✅ | scenario-scr.yaml 执行（int stage，跨 ui-dev + bifrost-api 联调） | n/a（依赖 `runtime_environment[name=localhost]` 的 9080+3008 端口由 int stage 启动；fixture 登录账号由 `data_resources[name=config-db]` 内 `fixture-keys` 提供，capability `sample_dataset` 已声明） |
| V-scr-5 端到端 scenario（int stage 真机执行） | ✅ | scenario-scr.yaml 执行 | n/a（scenario-execute agent 在 int stage 自动通过 `restart_all_instances` env-action 保证 9080+3008 可达；fixture 登录账号同上） |

所有 4 条 V-* 均为 `verifiable`，无需降级处理。详见 `.pg/changes/enhance-ui-i18n-coverage/0-define/define-summary.yaml`。

### 可观测性

- 关键日志点：i18n 初始化失败（`console.error`，含 i18next error code）、locale 切换（dev mode `console.debug`）
- 关键指标：无新增（i18n 切换属 UI 状态，无需 Prometheus 埋点）
- RequestId 追踪：无（i18n 不参与请求链路）

## Verification Criteria

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | Vitest 键完整性测试覆盖全部 15 个 namespace | 无前置；`ui/locales/{en,zh-CN}/*.json` 已生成 15 namespace × 2 locale | `cd ui && npx vitest run src/__tests__/i18n/` | 所有 15 namespace 的 key 集合在 en 与 zh-CN 之间完全一致；全局 sanity 测试通过（namespace 数量 = 15） |
| V-ui-2 | Vite 生产构建 + i18n TS 类型（15 namespace 注册） | 无前置 | `cd ui && npm run build` | 构建无报错；`ui/lib/i18n/types.ts` 自动生成的 `Resources` 联合类型含 15 namespace；`t('newNamespace.key')` 调用补全生效 |
| V-ui-3 | ui lint 通过（i18n 引用规范） | 无前置 | `cd ui && npm run lint` | 0 errors / 0 warnings（i18n key 引用命名合规；NoPermissionView entityI18nKey 全部走 i18n key） |
| V-ui-4 | Playwright i18n 切换 E2E（19 路由覆盖） | bifrost-api(:9080) + ui-dev(:3008) 已启动；fixture 登录账号已 seed（`config-db` 内 `fixture-keys`） | `make run-e2e FLOW=i18n` | 登录后切到 zh-CN，访问 19 个未翻译路由（logs / mcp-logs / mcp-registry / mcp-sessions / model-limits / routing-rules / skills-repo / plugins / observability / webhooks / oauth-grants / custom-pricing / config / model-catalog / complexity-router / docs 等），每个路由顶部导航 / 路由标题 / 表头 / 关键文案切为中文；NoPermissionView 切到无权限路由显示中文；切回 en 后全部恢复英文 |

### int scr Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-scr-5 | 端到端 scenario：登录 → 切语言 → 19 路由切换断言 → 切回 → 还原 + 持久化 + 损坏兜底 | bifrost-api(:9080) 与 ui-dev(:3008) 已启动；`config-db` 内 fixture auth 已启用（admin / bifrost123）；dev stage 全部 V-* 已绿 | scenario-scr.yaml 4 个 Scenario 执行 | critical=true scenario 全绿；S-i18n-19-routes-zh-CN 验证 19 路由全部切到中文；S-i18n-persist-across-page-reload 验证刷新后 locale 仍为 zh-CN；S-i18n-corrupted-localStorage-fallback-to-en 验证损坏 localStorage 兜底为 en；浏览器 console 无 error |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 纯前端 i18n，不涉及 Go 引擎 |
| framework | ❌ | 纯前端 i18n |
| transports | ❌ | 不修改 HTTP handler / SDK 集成 |
| cli | ❌ | 不涉及 CLI |
| plugins | ❌ | 不涉及 Go plugin |
| ui | ✅ | `noPermissionView.tsx` 改造 + `clientLayout.tsx` 已有 Provider 不动；新增 8 个 namespace × 2 locale JSON + 扩展 3 个现有 namespace；19 个未翻译路由 layout.tsx + page.tsx + views/**/*.tsx 翻译；Vitest + Playwright 测试扩展 |
| scenario | ✅（启用 `scr` track，int stage） | 决策依据见 on-conditions-eval.md `scenario_tracks_decision` 段 |

**affected_tracks**：`[ui]`

**scenario track 启用决策**：

- 跨 role 协作验证? 是——登录态由 bifrost-api(:9080) 提供 api 步骤，浏览器切换与断言由 ui-dev(:3008) 提供 browser 步骤
- 新 API 端点? 否——i18n 不引入新 HTTP 端点
- 跨模块联调? 是——前端 i18n 切换 + 后端 fixture 登录态 + localStorage 持久化属于跨层联调

→ **`scr` scenario track 启用**，在 `int` stage 由 scenario-execute agent 真机执行 `scenario-scr.yaml`；scenario-scr.yaml 至少包含 4 个 Scenario（happy / persist / corrupted-fallback / ui-smoke 维度）。