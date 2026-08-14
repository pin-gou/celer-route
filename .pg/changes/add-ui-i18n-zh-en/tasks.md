> - **environment 选择**：dev → local

## 1. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 Vitest 键完整性测试：创建 `ui/src/__tests__/i18n/key-completeness.test.ts`，遍历 `ui/locales/{en,zh-CN}/*.json`，断言所有 namespace 文件的 key 集合在 en 与 zh-CN 之间完全一致
- [ ] 1.2 编写 Vitest namespace sanity 测试：创建 `ui/src/__tests__/i18n/namespace-sanity.test.ts`，断言 en 与 zh-CN 各有且仅有 7 个 namespace（common / logs / config / governance / providers / dashboard / governance-ui）
- [ ] 1.3 编写 Playwright i18n 切换 E2E 骨架：创建 `tests/e2e/features/i18n/i18n.spec.ts`，包含登录 → 切语言 → 断言导航栏中文 → 切回 en → 断言恢复（红——尚无 i18n Provider 实现，切语言无效果）
- [ ] 1.4 编写 Vitest `useLocale` hook 单测：创建 `ui/src/__tests__/i18n/useLocale.test.ts`，覆盖 locale 初始化、setLocale 写 localStorage、损坏 localStorage 兜底为 en（红）

## 2. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 2.1 在 `ui/package.json` 新增依赖：`i18next`、`react-i18next`、`i18next-browser-languagedetector`（runtime），`vite-plugin-i18next-typescript`（devDependency）；执行 `npm install`
- [ ] 2.2 在 `ui/vite.config.mts` 集成 `vite-plugin-i18next-typescript` 插件，配置 typescriptOptions 输出至 `ui/lib/i18n/types.ts`
- [ ] 2.3 创建 `ui/locales/en/{common,logs,config,governance,providers,dashboard,governance-ui}.json` 七个 namespace 文件，填充英文初始值（common 必填 dashboard / providers / governance 等高频 key）
- [ ] 2.4 创建 `ui/locales/zh-CN/` 七个对应 namespace 文件，填充中文翻译
- [ ] 2.5 创建 `ui/lib/i18n/config.ts`：导出 i18next init 配置（resources、fallback lng=en、detection order=localStorage>navigator>en、ns=common 等 7 个、defaultNS=common、react.useSuspense=false、interpolation.escapeValue=react 自带防 XSS）
- [ ] 2.6 创建 `ui/lib/i18n/I18nProvider.tsx`：`<I18nextProvider i18n={i18nInstance}>{children}</I18nextProvider>` 包装组件，对外暴露 `<I18nProvider>`
- [ ] 2.7 创建 `ui/lib/i18n/useLocale.ts`：导出 `useLocale()` hook（基于 `i18next` 的 `useTranslation` + 自定义 `setLocale` 写 localStorage + 跨 tab `storage` 事件监听）；try/catch 兜底损坏 localStorage
- [ ] 2.8 修改 `ui/app/clientLayout.tsx`：在 `ProgressProvider` 之前插入 `<I18nProvider>` 包装
- [ ] 2.9 创建 `ui/components/LanguageSwitcher/LanguageSwitcher.tsx`：基于 Radix `DropdownMenu`，列出 `availableLocales`；挂入 `SidebarUserMenu.tsx`（或同级用户菜单组件）的 "Sign out" `<DropdownMenuItem>` 之前
- [ ] 2.10 迁移 `ui/lib/constants/logs.ts`：`ProviderLabels` / `RequestTypeLabels` / `StatusColors` / `RequestTypeColors` 的 value 由英文 string 改为 `t('logs.providers.openai.label')` 等调用；保留 TS 类型签名 `Record<ProviderName, string>` 完整
- [ ] 2.11 迁移 `ui/lib/constants/config.ts`：`ModelPlaceholders` 等用户可见字符串 value 改 `t('config.providers.openai.placeholder')` 调用；保留 TS 类型
- [ ] 2.12 迁移 `ui/lib/constants/governance.ts`：`resetDurationLabels` 等 value 改 `t('governance.reset.1h.label')` 调用；保留 TS 类型
- [ ] 2.13 翻译 dashboard 路由表层：`ui/app/workspace/dashboard/` 内路由标题、tab、组件内部文案（图表标题、统计卡片标签、空状态提示）；`useTranslation('dashboard')`
- [ ] 2.14 翻译 providers 路由表层：`ui/app/workspace/providers/` 内 provider 列表/详情/路由规则等组件的表头、操作按钮、tooltip、空状态；`useTranslation('providers')`
- [ ] 2.15 翻译 governance 路由表层：`ui/app/workspace/governance/` + `virtual-keys/` 子路由的表头、按钮、空状态；`useTranslation('governance-ui')` + `useTranslation('governance')`
- [ ] 2.16 翻译全局共享组件：顶部导航栏 (`Sidebar` / `Topbar`)、命令面板 (`CommandPalette`)、Toaster (sonner)、通用确认对话框 (Radix `AlertDialog`)、通用按钮 (`Button`)、通用空状态 (`EmptyState`)；`useTranslation('common')`
- [ ] 2.17 创建 `docs/features/i18n.mdx`：简述当前支持的语言、如何在用户菜单切换、如何贡献翻译（如何新增 namespace、如何新增 key）
- [ ] 2.18 全量 `rg "from.*constants/(logs|config|governance)"` 扫一遍，验证所有 import 仍能找到对应类型导出（迁移值不破坏类型推导链）

## 3. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/add-ui-i18n-zh-en 做静态审查
- [ ] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 4.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 4.2 执行测试（runner 通过 modules 注入命令）
- [ ] 4.3 启动服务（如需）
- [ ] 4.4 验证 V-ui-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-ui-1, V-ui-2, V-ui-3, V-ui-4

## 5. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 6. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 6.1 收集所有 stage 的 Gate Assessment
- [ ] 6.2 检查跨 stage 依赖项
- [ ] 6.3 输出 Final Gate Assessment
