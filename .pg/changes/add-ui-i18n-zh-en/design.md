# add-ui-i18n-zh-en 设计

## 架构概览

本变更仅涉及 `ui/` 模块（React + Vite + TanStack Router 前端），不影响后端 Go 模块。引入 react-i18next 作为 i18n 引擎，在 `clientLayout.tsx` Provider 栈最外层注入 `I18nProvider`，所有用户可见字符串统一从 `ui/locales/{en,zh-CN}/*.json` namespace 文件读取。

### 整体数据流

```
┌──────────────────────────────────────────────────────────────────────────┐
│  用户浏览器 (Vite dev server :3008 / 生产构建静态资源)                       │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │  ui/app/main.tsx → ui/app/clientLayout.tsx                          │  │
│  │  ┌──────────────────────────────────────────────────────────────┐  │  │
│  │  │ I18nProvider (新增, 最外层)                                   │  │  │
│  │  │   ├─ init i18next (react-i18next)                            │  │  │
│  │  │   ├─ 检测顺序: localStorage > navigator.language > en        │  │  │
│  │  │   └─ react: { useSuspense: false } 避免 Suspense fallback 闪烁│  │  │
│  │  │  ┌────────────────────────────────────────────────────────┐  │  │  │
│  │  │  │ ProgressProvider > ThemeProvider > Toaster >           │  │  │  │
│  │  │  │   ReduxProvider > NuqsAdapter > RbacProvider >         │  │  │  │
│  │  │  │   AppContent > DevProfiler                              │  │  │  │
│  │  │  │   ↓                                                    │  │  │  │
│  │  │  │  useTranslation() → t('namespace.key', { ns: '...' })  │  │  │  │
│  │  │  └────────────────────────────────────────────────────────┘  │  │  │
│  │  └──────────────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐  │
│  │  ui/locales/                                                       │  │
│  │    ├─ en/        en.json (common / logs / config / governance /   │  │
│  │    │              providers / dashboard / governance-ui)           │  │
│  │    └─ zh-CN/     同结构                                            │  │
│  │    加载策略: i18next-http-backend + 懒加载 (仅加载当前路由 namespace) │  │
│  └────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
```

### 涉及的前端组件

| 组件 | 路径 | 改造 |
|------|------|------|
| 客户端布局 | `ui/app/clientLayout.tsx` | 在 ProgressProvider 之前插入 `<I18nProvider>` |
| 用户菜单 | `ui/components/Sidebar/SidebarUserMenu.tsx`（待定位） | 在 "Sign out" 上方增加 `<LanguageSwitcher>` 下拉 |
| 国际化基础设施 | `ui/lib/i18n/config.ts` (新增) | i18next init、fallback、detection、namespace 注册 |
| 国际化 Provider | `ui/lib/i18n/I18nProvider.tsx` (新增) | `<I18nProvider>{children}</I18nProvider>` |
| Locale hook | `ui/lib/i18n/useLocale.ts` (新增) | `{ locale, setLocale, availableLocales }` |
| 类型生成产物 | `ui/lib/i18n/types.ts` (新增, 构建期生成) | `Resources` / `KeysWithNamespace` |
| Locale 资源 | `ui/locales/{en,zh-CN}/*.json` (新增) | 七个 namespace × 两个 locale |
| 英文常量迁移 | `ui/lib/constants/{logs,config,governance}.ts` | `*Labels` Record 的 value 改为 `t('namespace.key')` 调用 |
| 三大路由表层 | `ui/app/workspace/{dashboard,providers,governance}/` | 翻译路由标题 / tab / 表头 / 表格内文案 |

## API 设计

本变更不涉及后端 HTTP API 变更。i18n 资源全部落在前端，无 REST/GraphQL/gRPC 端点。

## 数据模型

无数据库 schema 变更，无 SQL 迁移。

`localStorage` schema（仅前端持久化）：

| Key | 类型 | 示例值 | 备注 |
|-----|------|--------|------|
| `bifrost.locale` | string | `"zh-CN"` \| `"en"` | 用户上次选择的 locale；缺失时 fallback 到 `navigator.language` |

## 组件设计

### I18nProvider 组件 API

```tsx
// ui/lib/i18n/I18nProvider.tsx
export function I18nProvider({ children }: { children: React.ReactNode }) {
  // 内部: useEffect 监听 localStorage 变化以同步跨 tab
  return <I18nextProvider i18n={i18nInstance}>{children}</I18nextProvider>;
}
```

### useLocale hook API

```tsx
// ui/lib/i18n/useLocale.ts
export interface UseLocaleReturn {
  locale: SupportedLocale;       // "en" | "zh-CN"
  setLocale: (locale: SupportedLocale) => void;  // 写 localStorage + i18n.changeLanguage
  availableLocales: readonly SupportedLocale[];
}

export function useLocale(): UseLocaleReturn;
```

### LanguageSwitcher 组件

```tsx
// ui/components/LanguageSwitcher/LanguageSwitcher.tsx
export function LanguageSwitcher() {
  const { locale, setLocale, availableLocales } = useLocale();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger data-testid="language-switcher-trigger">
        <GlobeIcon /> {locale}
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        {availableLocales.map(l => (
          <DropdownMenuItem
            key={l}
            data-testid={`language-switcher-option-${l}`}
            onSelect={() => setLocale(l)}
          >
            {LOCALE_LABELS[l]}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

挂载位置：`SidebarUserMenu.tsx` 内 `<DropdownMenuContent>` 的 "Sign out" `<DropdownMenuItem>` 之前。

## 关键约束与契约

### 前置条件

- Node.js ≥ 18（项目已要求；Vite 8 需要）
- `npm install` 新增依赖：`i18next`、`react-i18next`、`i18next-browser-languagedetector`、`vite-plugin-i18next-typescript`（devDep）
- 无后端服务依赖，无 schema 迁移，无配置变更

### 影响面

- **修改**：`ui/app/clientLayout.tsx`、`ui/lib/constants/{logs,config,governance}.ts`、`ui/app/workspace/{dashboard,providers,governance}/**/*.tsx`、`ui/components/Sidebar/SidebarUserMenu.tsx`、`ui/package.json`、`ui/vite.config.mts`、`ui/tsconfig.json`
- **新增**：`ui/lib/i18n/`（4 文件）、`ui/locales/{en,zh-CN}/`（14 json）、`ui/components/LanguageSwitcher/`、`ui/__tests__/i18n/`（Vitest）、`tests/e2e/features/i18n/i18n.spec.ts`、`docs/features/i18n.mdx`
- **不破坏对外 API**：本变更不修改任何 HTTP / gRPC / WebSocket 端点，对 Bifrost API 消费者零影响
- **TypeScript 类型签名保留**：`ui/lib/constants/*.ts` 中的 `ProviderLabels`、`RequestTypeLabels`、`StatusColors` 等 TS 类型导出（`Record<ProviderName, string>` 等）完整保留；仅其 `value` 字段（运行时字符串）改为 `t('...')` 调用

### 性能契约

- I18n Provider 注入后单次首屏加载体积增长 ≤ 30 KB（gzip）—— i18next + react-i18next core
- Namespace 懒加载：仅加载当前路由所需的 namespace（初始只加载 `common`，路由进入时按需加载 `dashboard` / `providers` / `governance` 等）
- `useTranslation()` 在 locale 切换时同步触发 `useSuspense: false` 避免 Suspense fallback —— 单次重渲染 < 16ms

### 错误码与编号段

无新增错误码（前端 i18n 不涉及后端错误码体系）。

### 环境限制与验证策略

> **依据 `.pg/changes/add-ui-i18n-zh-en/env-description.yaml` 中 `local` 环境 6 段判断**。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-ui-1 Vitest 键完整性测试 | ✅ | Vitest 单元测试（`cd ui && npx vitest run src/__tests__/i18n/`） | n/a |
| V-ui-2 Vite 生产构建 + i18n TS 类型生成器 | ✅ | `npm run build` | n/a |
| V-ui-3 ui lint 通过 | ✅ | `npm run lint` | n/a |
| V-ui-4 Playwright i18n 切换 E2E | ✅ | `make run-e2e FLOW=i18n` | n/a（依赖 `runtime_environment[name=localhost]` 的 9080+3008 端口由 int stage 启动；fixture 登录账号由 `data_resources[name=config-db]` 提供，capability `sample_dataset` 已声明） |
| V-ui-5 端到端 scenario（int stage 真机执行） | ✅ | scenario-scr.yaml 执行 | n/a（scenario-execute agent 在 int stage 自动通过 `restart_all_instances` env-action 保证 9080+3008 可达；fixture 登录账号同上） |

所有 4 条 V-* 均为 `verifiable`，无需降级处理。详见 `.pg/changes/add-ui-i18n-zh-en/0-define/define-summary.yaml`。

### 可观测性

- 关键日志点：i18n 初始化失败（`console.error`，含 i18next error code）、locale 切换（dev mode `console.debug`）
- 关键指标：无新增（i18n 切换属 UI 状态，无需 Prometheus 埋点）
- RequestId 追踪：无（i18n 不参与请求链路）

## Verification Criteria

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | Vitest 键完整性测试 | 无前置；`ui/locales/{en,zh-CN}/*.json` 已生成 | `cd ui && npx vitest run src/__tests__/i18n/` | 所有 namespace 的 key 集合在 en 与 zh-CN 之间完全一致；全局 sanity 测试通过 |
| V-ui-2 | Vite 生产构建 + TS 类型生成 | 无前置；`vite-plugin-i18next-typescript` 已装 | `cd ui && npm run build` | 构建无报错；`ui/lib/i18n/types.ts` 自动生成 `Resources` / `KeysWithNamespace` 类型；`t('namespace.key')` 调用补全生效 |
| V-ui-3 | ui lint 通过 | 无前置 | `cd ui && npm run lint` | 0 errors / 0 warnings（i18n key 引用命名合规） |
| V-ui-4 | Playwright i18n 切换 E2E | bifrost-api(:9080) + ui-dev(:3008) 已启动；fixture 登录账号已 seed（`config-db` 内 `fixture-keys`） | `make run-e2e FLOW=i18n` | 登录后切到 zh-CN，顶部导航栏出现中文（"仪表板"、"提供商"、"治理"等），关键路由 `/workspace/dashboard` 标题切为中文；切回 en 后全部恢复英文 |

### int scr Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-5 | 端到端 scenario：登录 → 切语言 → 跨路由断言翻译 → 切回 → 还原 + 持久化 + 损坏兜底 | bifrost-api(:9080) 与 ui-dev(:3008) 已启动；`config-db` 内 `fixture-keys` 登录账号已 seed；dev stage 全部 V-* 已绿 | scenario-scr.yaml 4 个 Scenario 执行 | critical=true scenario 全绿；S-i18n-persist-across-page-reload 验证刷新后 locale 仍为 zh-CN；S-i18n-corrupted-localStorage-fallback-to-en 验证损坏 localStorage 兜底为 en；浏览器 console 无 error |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 纯前端 i18n，不涉及 Go 引擎 |
| framework | ❌ | 纯前端 i18n |
| transports | ❌ | 不修改 HTTP handler / SDK 集成 |
| cli | ❌ | 不涉及 CLI |
| plugins | ❌ | 不涉及 Go plugin |
| ui | ✅ | `clientLayout.tsx` 注入 Provider；新增 `ui/lib/i18n/` 与 `ui/locales/`；`ui/lib/constants/*Labels` 值迁移；dashboard / providers / governance 三大路由表层翻译；用户菜单新增语言切换；新增 Vitest + Playwright 测试 |
| scenario | ✅（启用 `scr` track，int stage） | 决策依据见 on-conditions-eval.md `scenario_tracks_decision` 段 |

**affected_tracks**：`[ui]`

**scenario track 启用决策**：

- 跨 role 协作验证? 是——登录态由 bifrost-api(:9080) 提供 api 步骤，浏览器切换与断言由 ui-dev(:3008) 提供 browser 步骤
- 新 API 端点? 否——i18n 不引入新 HTTP 端点
- 跨模块联调? 是——前端 i18n 切换 + 后端 fixture 登录态 + localStorage 持久化属于跨层联调

→ **`scr` scenario track 启用**，在 `int` stage 由 scenario-execute agent 真机执行 `scenario-scr.yaml`；scenario-scr.yaml 至少包含 3 个 Scenario（覆盖 happy / negative / ui-smoke 维度）。