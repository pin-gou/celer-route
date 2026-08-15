# enhance-workspace-i18n-coverage-3 设计

## 架构概览

本变更完全在前端 TypeScript / React 层完成，不涉及后端 Go 代码、不涉及数据库 schema、不涉及 API 端点。架构图：

```
┌─────────────────────────────────────────────────────────────┐
│                     ui/locales/en/*.json                     │
│                  ui/locales/zh-CN/*.json                     │
│         (15 个 namespace，en 与 zh-CN 严格对齐)              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│        i18next 实例（ui/lib/i18n/config.ts，顶层 init）        │
│  - LanguageDetector (localStorage > navigator > htmlTag)     │
│  - fallbackLng: "en"                                          │
│  - lookupLocalStorage: "bifrost.locale"                      │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
   ┌─────────────────┐ ┌─────────────┐ ┌──────────────────┐
   │  React 组件内    │ │ Zod schema  │ │ sidebar.tsx 自身  │
   │  useTranslation │ │ i18next.t() │ │ promo / search   │
   │  (t("xxx"))     │ │ 直接调用    │ │ aria-label 等    │
   └─────────────────┘ └─────────────┘ └──────────────────┘
              │               │               │
              └───────────────┼───────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  静态闸 (dev 阶段)                          │
│  - ESLint 规则：禁 JSX 文本节点 / toast / placeholder 裸英文 │
│  - key diff 脚本：en vs zh-CN 集合对齐                      │
└─────────────────────────────────────────────────────────────┘
```

**数据流**：
1. 用户切语言 → `useLocale.setLocale("zh-CN")` → `localStorage.setItem("bifrost.locale", "zh-CN")`
2. i18next 重新解析 namespace → React 组件 re-render → 全部文案刷新
3. Zod schema 在表单提交时同步用 i18next.t() 取当前语言
4. dev 阶段 `npm run lint` + `npm run i18n:check` 跑静态闸

## 组件设计

### 新增 / 修改文件

| 文件路径 | 类型 | 说明 |
|---------|------|------|
| `ui/locales/en/<namespace>.json` | 修改 | 15 个 namespace 增量加 key（toast、validation、filter、placeholder、aria-label） |
| `ui/locales/zh-CN/<namespace>.json` | 修改 | 与 en 同步加 key，token 翻译遵守"LLM→词元、auth→令牌"规则 |
| `ui/lib/types/schemas.ts` | 修改 | ~70+ 处 Zod 硬编码 message 改为 `i18next.t("validation.x")` |
| `ui/app/workspace/<visible>/page.tsx`（21 个） | 修改 | 替换硬编码英文为 `t("...")` |
| `ui/app/workspace/<visible>/views/*.tsx` | 修改 | Sheet / Dialog / Form / Table 替换硬编码 |
| `ui/components/filters/logsFilterSidebar.tsx` | 修改 | Logs + Dashboard 共用，本轮范围内；filter 选项走 i18n |
| `ui/app/workspace/mcp-registry/views/mcpClientsFilterSidebar.tsx` | **不修改** | 被隐藏菜单 mcp-registry 引用，落范围外 |
| `ui/app/workspace/mcp-registry/library/views/mcpLibraryFilterSidebar.tsx` | **不修改** | 被隐藏菜单 mcp-registry/library 引用，落范围外 |
| `ui/app/workspace/mcp-sessions/views/mcpSessionsFilterSidebar.tsx` | **不修改** | 被隐藏菜单 mcp-sessions 引用，落范围外 |
| `ui/app/workspace/oauth-grants/views/oauthGrantsFilterSidebar.tsx` | **不修改** | 被隐藏菜单 oauth-grants 引用，落范围外 |
| `ui/components/sidebar.tsx` | 修改 | promo card / search / aria-label 走 i18n |
| `ui/app/workspace/logs/page.tsx` | 修改 | `COLUMN_LABELS` → `column_labels`（15 处） |
| `ui/app/workspace/mcp-logs/page.tsx` | 修改 | `mcpLogs.COLUMN_LABELS` → `mcpLogs.column_labels`（6 处） |
| `eslint.config.js` 或 `eslintrc.*` | 修改 | 新增"禁裸英文字面量"规则 |
| `ui/scripts/i18n-key-diff.mjs`（新增） | 新增 | en vs zh-CN key 集合对齐 diff 脚本 |
| `package.json` | 修改 | 新增 `"i18n:check": "node ui/scripts/i18n-key-diff.mjs"` |
| `.pg/hooks/local/describe_env.sh` | 修改 | 扩充 ui-dev business_system 段（**用户已授权**） |

### i18n key 命名约定（沿用 + 增量）

- 命名空间：`{workspace}.json`（沿用 15 个，不新建除非必要）
- 顶层 key：`page.*` / `form.*` / `columnLabels.*` / `state.*` / `toast.*` / `filter.*` / `placeholder.*` / `ariaLabel.*` / `validation.*`
- 全小写 snake_case + dot-separated
- **重要**：`logs.COLUMN_LABELS` → `logs.column_labels`（修复一致性）

### Zod schema 翻译模式

```typescript
// ui/lib/i18n/config.ts 已有默认导出 i18n 实例
import i18n from "@/lib/i18n/config";

// schemas.ts（项目是纯 CSR，i18next 顶层 init 无 SSR 风险）
const t = (key: string, opts?: Record<string, unknown>) => i18n.t(key, opts);

export const providerSchema = z.object({
  name: z.string().min(1, t("validation.fieldRequired", { field: "Name" })),
  endpoint: z.string().url(t("validation.urlInvalid")),
  weight: z.number().min(0).max(1),
});
```

新增 `validation.*` key 到 `common.json`：
```json
{
  "validation": {
    "fieldRequired": "{field} 为必填项",
    "urlInvalid": "URL 格式无效",
    "minLength": "至少 {n} 个字符",
    "maxLength": "最多 {n} 个字符",
    "rangeOutOfBounds": "{field} 必须在 {min} 到 {max} 之间"
  }
}
```

## 关键约束与契约

### 前置条件
- `ui/lib/i18n/config.ts` 已存在并 init 完成（v0.0.0 起即有，0 新依赖）
- 现有 15 个 namespace 的 en/zh-CN 已严格对齐 793 key（v0.0.0 阶段产物）
- 用户已授权修改共享的 `.pg/hooks/local/describe_env.sh`

### 影响面
- 影响的 locale 文件：15 个 namespace 的 en/zh-CN，en/zh-CN 必须**同步**追加同结构 key
- 影响的 React 组件：约 60-80 个文件（含 21 个 page.tsx + ~50 个 views/Sheet/Dialog/Filter/Table/Sidebar）
- 影响的 Zod schema：1 个文件（`ui/lib/types/schemas.ts`，~70+ 处 message）
- 影响的 React 测试：无（项目无 React 测试）
- 是否破坏任何对外 API：**否**（仅前端 UI 改造）

### 性能契约
- i18next 加载策略不变：build 时静态 import locale JSON，无运行时网络开销
- toast / Zod / aria-label 走 `t()` 的额外开销：单次字符串查找（Map lookup），< 1µs，对 60fps 渲染无可观测影响
- bundle size：locale JSON 增量约 5-15 KB（视新增 key 数量）

### 错误码与编号段
- 不涉及（前端 i18n 改造，无后端错误码）

### 可观测性
- **关键日志**：无（前端 i18n 切换是静默的；i18next console warning 由 Chrome DevTools MCP 采证）
- **关键指标**：无业务指标；可在 telemetry plugin 增加 `i18n.locale.switch.count` 计数器（**本轮不做，留待后续**）
- **RequestId 追踪**：N/A（前端用户操作，无 RequestId）

### 环境限制与验证策略

依据 `.pg/changes/enhance-workspace-i18n-coverage-3/env-description.yaml`（v1.4 协议 1.6 阶段产出），本 change 在 `local` 环境的可验证性：

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-ui-1 Vite dev server 启动 + 中英文运行时切换验证 | ✅ | scenario（浏览器冒烟） | n/a |
| V-ui-2 ESLint + key 对齐 dev 阶段静态闸验证 | ✅ | scenario（dev 阶段自检） | n/a |
| V-ui-3 Zod schema i18n 翻译在表单错误中显示 | ✅ | scenario（浏览器冒烟 + Zod 触发） | n/a |
| V-ui-4 Chrome DevTools MCP 采证 21 个可见菜单项中英文切换 | ✅ | scenario（浏览器冒烟 + console 采证） | n/a |

**降级策略**：4 个 V-* 全部 verifiable，无需 degraded 降级路径；本变更无 skipped V-*。

**SSOT**：所有 V-* 均引用 `env-description.yaml` 中已声明的资源：
- `{env.business_systems[name=ui-dev]}`（type=web-app, capabilities: vite_dev_server + i18n_runtime_switch + jsdom_runtime）
- `{env.runtime_environment[name=localhost].config.ports[role=ui-dev]}`（port 3008）

## Verification Criteria

按 stages 顺序遍历（project.yaml 仅有 `dev` stage + tracks=[core, framework, transports, cli, plugins, ui]），本变更仅影响 `ui` track：

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | Vite dev server 启动 + 中英文运行时切换 | 需 `npm run dev` 起 3008 端口，浏览器访问任一可见菜单项页面 | scenario（type=browser，访问 `/workspace/logs`，切 zh-CN → en → zh-CN） | 所有可见英文文案切换为中文；console 无 `i18next: key 'xxx' not found` 警告 |
| V-ui-2 | ESLint + key 对齐 dev 阶段静态闸 | 需所有改动已提交到工作树 | `npm run lint` + `npm run i18n:check`（key diff 脚本） | 两条命令均零错误；en/zh-CN key 集合完全一致 |
| V-ui-3 | Zod schema i18n 在表单错误中显示中文 | 需 ui-dev 起来 + 打开任一 Sheet/Dialog 表单（如 provider 配置 Sheet），故意留空必填字段后提交 | scenario（type=browser，触发 Zod 验证，截图） | 错误消息显示中文（如"端点为必填项"）而非英文（如"Endpoint is required"） |
| V-ui-4 | Chrome DevTools MCP 采证 21 个可见菜单项中英文切换 | 需 chrome-devtools MCP 工具 + ui-dev 起来；按 21 个菜单项路径列表逐个采证 | scenario（type=browser，遍历 21 个 menu path，每个 path 切 zh-CN/en 截图 + console 日志） | 所有路径下可见英文文案切换为中文；console 无 missing key；es 路径下无英文残留 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | Go 后端引擎，无前端相关改动 |
| framework | ❌ | 数据持久化、流式累加器，无前端相关改动 |
| transports | ❌ | HTTP 网关传输层，无前端相关改动 |
| cli | ❌ | CLI 工具，无前端相关改动 |
| plugins | ❌ | 9 个 Go plugin，无前端相关改动 |
| ui | ✅ | 修改 25 个 page.tsx（含 Settings 13 子路由）+ ~90 个 views + **1 个共享 FilterSidebar（LogsFilterSidebar）** + sidebar.tsx + ui/lib/types/schemas.ts + 15 个 locale JSON + 新增 ESLint 规则 + key diff 脚本 |
| scr（scenario） | ✅（启用） | 跨 ui 模块冒烟（Chrome DevTools MCP），需本地 Vite dev server 起来 |

**affected_tracks**：`[ui]`（仅 ui 模块受影响）

**scenario track 启用决策**：`scr=true`（启用）—— 本变更需在 ui-dev 环境跑 scenario 全链路验证：dev 阶段静态闸（V-ui-2）+ 浏览器冒烟（V-ui-1 / V-ui-3 / V-ui-4）。四问答复：
1. 跨 role 协作验证？**是**—— ui-dev（Vite dev server）必须起，且 Chrome DevTools MCP 需真实浏览器渲染。
2. 新 API 端点？**否**—— 仅前端改造，无 API 变更。
3. 跨模块联调？**否**—— 仅 ui 模块内。
4. ui 模块本身使用场景？**是**—— scenario 集合必须含 ≥1 个 `type=api` 与 ≥1 个 `type=browser`（design 含 frontend track 的 V-*），本项目 frontend 即 ui track。

**scenario-decisions**：`scr=true`
**scenario-reason**：跨 role 协作验证（ui-dev + chrome-devtools）；design 含 ui track 的 4 个 V-* 需 scenario 覆盖；前端 i18n 改造必须在真实浏览器中采证（不依赖 vitest jsdom 模拟）。