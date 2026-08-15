# enhance-workspace-i18n-coverage-3
**关联 issue**：无
**变更类型**：feature

## 背景

Bifrost 管理界面已完成两轮 i18n 改造（en + zh-CN 双语、15 个 namespace、793 key 完全对齐、运行时切换 + localStorage 持久化），但覆盖范围有限——仅覆盖 sidebar 菜单 label + 9 个 workspace 页面的主结构。**21 个可见菜单项对应的页面、子页面、Sheet/Dialog/Drawer/Form/Toast/FilterSidebar/aria-label/placeholder 中仍有大量硬编码英文**。

切到中文后，用户仍然看到大量英文 toast、英文 Zod 表单验证消息、英文 filter 标签、英文占位符，以及 sidebar 自身 promo card / 搜索框 / aria-label 的英文。语言切换的用户体验是割裂的。

## 目标

完成**菜单栏未隐藏的 21 个可见菜单项**对应页面的全栈 i18n 化，使切换到中文后，这些页面及其子组件（对话框、抽屉、表单、toast、验证消息、filter 侧栏、占位符、aria-label）不再出现硬编码英文。验证标准：Chrome DevTools MCP 采证 21 个页面中英文切换、console 无 missing key 警告、ESLint 零硬编码警告、en/zh-CN key 集合对齐。

## 范围

### 包含

- 21 个可见菜单项对应页面与子页面（logs / dashboard / providers / providers2 / model-catalog / routing-rules / complexity-router / custom-pricing / plugins / config 及其 **13 个**子项：api-keys / caching / client-settings / compatibility / feature-flags / large-payload / logging / mcp-gateway / observability / performance-tuning / pricing-config / proxy / security）
- 这些页面涉及的 Sheet / Dialog / Drawer / Form / Toast / Input placeholder / aria-label / **LogsFilterSidebar**（被 Logs + Dashboard 共用，本轮范围内）
- sidebar.tsx 自身 UI（promo card 标题/按钮/搜索 placeholder/Collapse/Expand/Logout aria-label）
- Zod schema 中硬编码的 validation message（约 70+ 处集中在 `ui/lib/types/schemas.ts`）
- `logs.COLUMN_LABELS` → `logs.column_labels` 命名一致性修复（21 处引用）
- 新增 ESLint 规则（禁 JSX 文本节点 / toast / placeholder 中裸英文，可豁免注释）
- 新增 en vs zh-CN key diff 脚本 + CI/dev 闸
- `.pg/hooks/local/describe_env.sh` 扩充 ui-dev business_system 段（含 vite_dev_server + i18n_runtime_switch capability）——已授权

### 不包含

- 19 个**隐藏**菜单项对应页面（mcp-logs / observability / model-limits / pricing-overrides / mcp-gateway / mcp-catalog / mcp-library / auth-sessions / oauth-grants / mcp-settings / governance / virtual-keys / webhooks / prompt-repo / skills-repo / evals / feature-flags）
- 技术性硬编码英文：URL 示例（http://pushgateway:9091）、变量名占位（env.PUSHGATEWAY_URL / sk-xxxxxx）、代码示例、第三方 selector（recharts chart type、Radix selector、CSS font format）、query param value
- 新增语言（仅 en + zh-CN）
- RTL 支持
- 新增 locale namespace（仅在 toast 跨域共享场景必要时新建，否则沿用 15 个现有 namespace）
- 后端 Go 代码（core / framework / transports / cli / plugins）

## 方案概述

沿用现有 i18n 基础设施（react-i18next + useLocale + localStorage 持久化），无新依赖：

1. **toast key 按 workspace domain 落位**：各 workspace 的 toast 进各自 namespace（如 `providers.toast.*`、`config.toast.*`、`logs.toast.*`），common.json 仅保留通用 success / error / copied 等。
2. **Zod validation message 直接调 `i18next.t()`**：项目为纯 CSR + vitest jsdom，i18next 实例在 config.ts 顶层 init，schema 模块可直接调用，无 SSR 风险、零运行时保护。
3. **LogsFilterSidebar + sidebar.tsx 自身 UI 一并覆盖**：filter 选项、promo card、搜索框、aria-label 全部走 i18n。被隐藏菜单引用的另外 4 个 FilterSidebar（mcpClients / mcpLibrary / mcpSessions / oauthGrants）落范围外，留待后续 change。
4. **修复 `logs.COLUMN_LABELS` 命名不一致**：全大写 → snake_case，21 处引用同步改。
5. **新增 ESLint 规则 + key diff 脚本**：禁止 JSX 文本节点 / toast / placeholder 中裸英文字面量（可 eslint-disable 豁免）；`en vs zh-CN` key 同步 diff 脚本，接入 dev 阶段 `npm run lint` / `npm run i18n:check`。

## 风险和注意事项

- **Zod schema 顶层 `i18next.t()` 的 SSR 风险**：当前项目为纯 CSR + vitest jsdom，i18next.init() 不报错；但若未来引入 SSR（Next.js/Remix），schema 顶层调用会因 document / localStorage 缺失而崩溃。本变更零运行时保护，需在引入 SSR 前重构为 lazy init。*（验证方式：V-ui-3）*
- **ESLint 规则误伤技术性字符串**：可能误伤 CSS 类名、inline style、aria-hidden、query param 等技术性字符串。需提供 `eslint-disable-next-line` 豁免机制。*（验证方式：V-ui-2）*
- **toast 跨域复用性下降**：归到各 workspace namespace 后，"Copied to clipboard" 等跨域通用 toast 复用需保留在 common.json。*（验证方式：V-ui-4）
- **`logs.COLUMN_LABELS` 重命名的爆炸半径**：21 处引用集中在 2 文件（logs/page.tsx + mcp-logs/page.tsx），引用方式为 `t('COLUMN_LABELS.x')` 默认 namespace 写法，无其他隐藏引用。*（验证方式：V-ui-4）*
- **扩充共享 describe_env.sh 脚本**：影响所有 change 的 env-description 探测，已与用户授权确认。*（验证方式：V-ui-1）*
- **providers2（新设计版）页面仍在迭代**：若未来大改 key 结构，本轮新增 key 需同步迁移。*（验证方式：V-ui-4）*

## 未做

- 无 V-* 标记为 skipped。所有 4 个验证点均可在 local 环境验证（V-ui-1/2/3/4 全为 verifiable）。
