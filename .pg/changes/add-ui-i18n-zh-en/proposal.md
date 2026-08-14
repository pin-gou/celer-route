# add-ui-i18n-zh-en

**关联 issue**：无
**变更类型**：feature

## 背景

Bifrost UI 当前所有用户可见字符串均为硬编码英文（实测约 1500-3000 处，分布在 446 个 `.tsx` 文件中）。
项目零 i18n 库依赖、零多语言资源文件、零 locale 切换机制。
国内运维 / 业务团队首次打开管理后台看到全英文界面，使用门槛高；现有企业客户对中文支持的诉求反复出现在反馈中。

## 目标

引入前端国际化能力，初期支持英文（en）与简体中文（zh-CN）两种语言。用户首次访问根据浏览器语言自动选择，记忆在 localStorage，用户菜单底部可手动切换；切换后全局 UI 同步更新。

具体目标：

1. 建立 i18n 基础设施（react-i18next 引擎、locale 资源目录、Provider 注入、语言检测/持久化/切换）。
2. 完成 MVP 翻译范围：所有全局共享 UI（顶部导航、用户菜单、命令面板、Toaster 等）+ `ui/lib/constants/*Labels` 中 provider/request-type/状态等英文常量。
3. 完成三大目标路由（dashboard / providers / governance）表层翻译：路由标题、tab、表头、表格内文案（空状态、tooltip、操作按钮）。
4. 提供键完整性自动化保障（Vitest）+ 切换体验端到端保障（Playwright）。

## 范围

### 包含

- `ui/locales/{en,zh-CN}/*.json` 七个 namespace 资源文件（common / logs / config / governance / providers / dashboard / governance-ui）。
- `ui/lib/i18n/{config.ts, I18nProvider.tsx, useLocale.ts, types.ts}` 三件套基础设施。
- `ui/app/clientLayout.tsx` 注入 I18nProvider（最外层，ProgressProvider 之前）。
- 用户菜单底部增加语言切换下拉。
- 翻译 `ui/lib/constants/{logs,config,governance}.ts` 中 `*Labels` Record 的值；保留 `.ts` 文件中的 TS 类型签名供其他模块 `import type` 使用。
- 翻译三大目标路由（dashboard / providers / governance）的 user-facing 字符串（路由标题、tab、表头、表格内文案）。
- `vite-plugin-i18next-typescript` 构建期自动生成 `Resources` / `KeysWithNamespace` TS 类型。
- Vitest 键完整性测试（按 namespace 拆 + 全局 sanity），CI 强制通过。
- Playwright i18n 切换 E2E（复用 `tests/e2e/core/fixtures/base.fixture` 已有的认证流程）：登录态下切语言 → 断言导航栏 / 关键路由标题切为中文 → 切回 en 恢复。
- `docs/features/i18n.mdx` 一页简述（如何切换、如何贡献翻译）。

### 不包含

- dialog / form 字段 label / placeholder / 错误提示的翻译（按路由 PR 后续单独推进，本期 200+ form 字段单独成 PR 更易 review）。
- 其他 23 个 workspace 路由（model-limits / routing-rules / mcp-* 等）的翻译（同样按路由后续推进）。
- URL 路径带 locale 前缀或 `?lang=` 查询参数、cookie 持久化（dashboard 后台无 SEO 需求，localStorage 足够）。
- i18next-icu 复数支持（dashboard 复数场景极少，`{{count}}` 简单插值 + 浏览器原生 `Intl.NumberFormat` / `DateTimeFormat` 已覆盖 95% 场景）。
- 第三语言（ja-JP / es-ES 等）资源文件——目录与 fallback 链按 N locale 设计但本期不交付第三 locale。
- SSR、`Accept-Language` header 后端处理（纯客户端 `navigator.language`）。
- 完整翻译贡献工具链（仅 docs 简述）。

## 方案概述

技术栈选择 **react-i18next + i18next**（生态最旺、Vite/TanStack Router 集成案例多、TypeScript 路径生成器成熟）。命名空间按 `lib/constants/` 子目录映射到独立 json（懒加载），扁平 key + 自动生成 TS 类型提供编译期补全。

Provider 注入点选 `ui/app/clientLayout.tsx` 最外层——与现有 `ThemeProvider` / `RbacProvider` / `ReduxProvider` 同级但更外，确保 Toaster 等所有下游都能用 `t()`。语言检测走 `localStorage > navigator.language > en` fallback 链。切换 UI 选用户菜单底部下拉（零新增 UI 位置、符合后台产品惯例）。

迁移策略：保留 `ui/lib/constants/*.ts` 的 TS 类型签名（其他模块 `import type` 仍可用），把 `value` 字段由英文 string 改为 `t('xxx.yyy.zzz')` 调用，资源数据落在 `ui/locales/`。

验证：Vitest 机械保证 zh-CN 与 en 永远 key 对齐（按 namespace 拆测 + 全局 sanity），Playwright E2E 覆盖切换机制端到端（复用 base fixture 登录态），`npm run build` 验证 Vite + TS 类型生成器产物。

## 风险和注意事项

1. **i18n 切换与 Radix UI 组件 ARIA 属性冲突**——`aria-label` 等需翻译。Mitigation：review 时检查所有 Radix wrapper component 的 ARIA props。
2. **`vite-plugin-i18next-typescript` 构建期失败会阻断 dev server**——新增/删除 key 后必须确保 `npm run build` 通过，否则 dev server 起不来。Mitigation：CI 强校验 + Vite plugin watch 模式。
3. **`localStorage` 损坏兜底**——若用户手动改坏 `localStorage.locale` 字段，需兜底为 `en` 而不是抛错。Mitigation：在 `useLocale` 初始化 try/catch + fallback。
4. **`ui/lib/constants/*Labels` 的 TS 类型签名被多文件 import**——迁移值时需 grep 全量引用确认 `.ts` 类型导出保留完整。Mitigation：迁移前 `rg "from.*constants/(logs|config|governance)"` 全量搜索。
5. **`ProviderLabels` 等枚举型 Record 必须保留 TS 类型完整**——下游 `Record<ProviderName, string>` 强依赖类型签名；本期策略是保留 `.ts` 类型 + 迁值，i18n key 仍以 `t('providers.openai.label')` 形式写在调用处，不破坏类型推导链。
6. **locale 切换瞬时空白闪烁**——react-i18next 在 locale 切换瞬间可能有未翻译字符串闪现。Mitigation：切换瞬间保持当前 locale 渲染完成后再切换，避免在 Suspense fallback 期间暴露原始 key。

**约束**：上述 6 条风险中，1/2/3/4 在 `design.md` 的 Verification Criteria 中对应 V-ui-{N} 验证项（详见 design.md）；5 由静态代码 review 兜底；6 由人工冒烟验证（不进入自动 gate）。