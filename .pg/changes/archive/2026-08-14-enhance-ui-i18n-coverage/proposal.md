# enhance-ui-i18n-coverage

**关联 issue**：无
**变更类型**：feature

## 背景

上期 `add-ui-i18n-zh-en` 在 `ui/` 上交付了 i18n 基础设施（react-i18next + I18nProvider + LanguageSwitcher + 7 个 namespace），并对 dashboard / providers / governance 三大路由做了表层翻译。但 `ui/app/workspace/` 下剩余 19 个路由（logs / mcp-logs / mcp-registry / mcp-sessions / model-limits / routing-rules / skills-repo / plugins / observability / webhooks / oauth-grants / custom-pricing / config / model-catalog / complexity-router / docs 等）仍有大量 user-facing 字符串硬编码英文——实测约 500–600 键未翻译。

同时 `ui/components/noPermissionView.tsx` 的 `entity` prop 仍接受字面量字符串而非 i18n key，导致所有 19 个未翻译路由的 layout.tsx 在切到 zh-CN 时无权限视图仍为英文。`common` namespace 的 action / state 键集不足以覆盖新增 CRUD 文案。

中文用户首次访问 Bifrost 管理后台，看到 19 个核心路由全英文界面，使用门槛仍高。

## 目标

按路由补齐全部 19 个未翻译 workspace 路由的表层翻译（路由标题 / tab / 表头 / 表格内文案 / stat 卡片 / empty state / filter sidebar / AlertDialog 标题描述），单 PR 一次性交付 500–600 键；同步改造 NoPermissionView 让其支持 i18n key 输入；补充 common.* 动作/状态键集；不引入第三语言，不翻译 dialog/form 字段。

具体目标：

1. 新增 8 个 namespace（mcp / routing / skills / plugins / observability / webhooks / oauth-grants / model-catalog），扩展 logs / governance / config 容纳新增路由，15 个 namespace 同步注册到 types.ts / config.ts。
2. 改造 `ui/components/noPermissionView.tsx` 支持 entity i18n key 输入，扫全 19 路由 layout.tsx 替换硬编码。
3. 补充 `ui/locales/{en,zh-CN}/common.json` 的 action / state / status 键集（覆盖 CRUD 动作文案）。
4. 翻译 19 个路由表层文案（不含 dialog/form 字段），单 PR 一次性交付。
5. 扩展 Vitest 键完整性测试覆盖 15 个 namespace + Playwright i18n E2E 覆盖 19 路由切换断言。

## 范围

### 包含

- `ui/locales/{en,zh-CN}/*.json` 新增 8 个 namespace（mcp / routing / skills / plugins / observability / webhooks / oauth-grants / model-catalog）+ 扩展 logs（容纳 mcp-logs）+ 扩展 governance（容纳 model-limits）+ 扩展 config（容纳 config 子路由 + custom-pricing + mcp-settings）
- `ui/lib/i18n/{config.ts, types.ts}` 注册 15 个 namespace
- 改造 `ui/components/noPermissionView.tsx` 支持 entity i18n key 输入
- 补充 `ui/locales/{en,zh-CN}/common.json` 的 action / state / status 键集
- 翻译 19 个 workspace 路由的表层字符串：logs / mcp-logs / mcp-registry / mcp-sessions / model-limits / routing-rules / skills-repo / plugins / observability / webhooks / oauth-grants / custom-pricing / config / model-catalog / complexity-router / docs 等
- 19 个路由 layout.tsx 的 NoPermissionView entity prop 改 i18n key
- 扩展 Vitest 键完整性测试覆盖全部 15 个 namespace
- 扩展 Playwright i18n E2E 覆盖 19 路由切换断言
- 更新 `docs/features/i18n.mdx` 补充新覆盖范围说明

### 不包含

- 19 路由 dialog / form 字段 label / placeholder / 错误提示翻译（仍 out-of-scope，按路由另起 PR）
- prompt-repo 路由委托的 `ui/components/prompts/` 共享组件（保持英文，单独决策）
- 第三语言资源文件 / locale switcher（仅 en + zh-CN）
- URL locale 前缀 / `?lang=` 查询参数 / cookie 持久化
- i18next-icu 复数支持
- SSR / Accept-Language header 处理
- 翻译术语表 / 翻译记忆库
- `lib/constants/logs.ts` 的 ProviderLabels / StatusColors i18n 化包装（独立路径）

## 方案概述

沿用上期已交付的 react-i18next 基础设施，按 namespace 拆分路由归属：mcp-registry + mcp-sessions → 新增 `mcp`；routing-rules + complexity-router → 新增 `routing`；skills-repo → 新增 `skills`；plugins → 新增 `plugins`；observability → 新增 `observability`；webhooks → 新增 `webhooks`；oauth-grants → 新增 `oauth-grants`；model-catalog → 新增 `model-catalog`；mcp-logs 扩 `logs`；model-limits 扩 `governance`；config 全部子路由扩 `config`。

NoPermissionView 改造方案：保持 `entity: string` prop API，但新增可选 `entityI18nKey?: string` prop——当传入时组件内部 `t(entityI18nKey)` 而非直接渲染 `entity`。所有 19 路由 layout.tsx 同步切换为 `entityI18nKey="<namespace>:routeName.permissionDenied"`。

common 键补充：扫描 19 路由中复用的动作文案（如 "Install New Plugin"、"Edit Plugin Sequence"、"Revoke Grant"、"Restore defaults"），统一落入 `common:action.*` / `common:state.*` / `common:confirm.*`，避免每个 namespace 重复定义。

PR 体积：单 PR / 单 commit，500–600 键一次性交付。PR 描述中按 namespace / 文件路径分组列出全部改动清单，辅助 code review。

## 风险和注意事项

1. **单 PR 体积风险**——500–600 键翻译，PR 涉及 100+ 文件。Mitigation：PR 描述按 namespace 分组列出文件清单，code review 按 namespace 逐组审视。
2. **namespace 注册同步性**——8 个新 namespace + 3 个扩展 namespace 必须同步更新 `types.ts`（第 16-24 行）和 `config.ts`（第 21 行 `NS` 数组 + 第 33-52 行 `resources` 对象），否则运行期缺失翻译。Mitigation：V-ui-2 构建期校验阻断。
3. **NoPermissionView prop 改造波及全部 19 路由**——layout.tsx 文件数较多，需 rg 全量扫一遍 `import.*NoPermissionView` 站点确认无遗漏。Mitigation：迁移前 `rg "from.*noPermissionView"` 全量搜索 + V-ui-4 E2E 浏览器断言无权限视图中文显示。
4. **common.* 键命名规范必须一致**——避免与上期已存在键名冲突。Mitigation：键名命名规范统一（`common:action.create` / `common:action.delete` / `common:state.active` / `common:confirm.delete`），新增键前 grep 检查。
5. **prompt-repo 共享组件保持英文**——prompt-repo 路由切到 zh-CN 后 `ui/components/prompts/` 下 26 个组件仍为英文，用户体验不完整。Mitigation：在 `docs/features/i18n.mdx` 中明确说明，作为已知妥协项告知用户；后续另起 PR 翻译共享组件。
6. **E2E 启动时间**——19 路由 E2E 启动时间较长（每次启动 Vite dev server + Playwright）。Mitigation：CI 上按 namespace 分 3–5 个 spec 文件并行跑，避免单 spec 启动时间超阈值。
7. **dialog/form 字段翻译仍 out-of-scope**——本期翻译后仍有大量英文 dialog/form 字段，中文用户体验不完整。Mitigation：在 `docs/features/i18n.mdx` 明确说明翻译进度（19 路由表层已覆盖、dialog/form 后续按路由推进）。

**约束**：上述 7 条风险中，1/2/3/4 在 `design.md` 的 Verification Criteria 中对应 V-ui-{N} 验证项（详见 design.md）；5/6/7 由 docs 文档兜底说明，不进入自动 gate。