> - **environment 选择**：dev → local，int → local

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
- [ ] 2.3 创建 `ui/locales/en/{common,logs,config,governance,providers,dashboard,governance-ui}.json` 七个 namespace 文件，填充英文初始值
- [ ] 2.4 创建 `ui/locales/zh-CN/` 七个对应 namespace 文件，填充中文翻译
- [ ] 2.5 创建 `ui/lib/i18n/config.ts`：导出 i18next init 配置（resources、fallback lng=en、detection order=localStorage>navigator>en、ns=common 等 7 个、defaultNS=common、react.useSuspense=false）
- [ ] 2.6 创建 `ui/lib/i18n/I18nProvider.tsx`：`<I18nextProvider i18n={i18nInstance}>{children}</I18nextProvider>` 包装组件
- [ ] 2.7 创建 `ui/lib/i18n/useLocale.ts`：导出 `useLocale()` hook（基于 i18next useTranslation + 自定义 setLocale 写 localStorage + 跨 tab storage 事件监听）；try/catch 兜底损坏 localStorage
- [ ] 2.8 修改 `ui/app/clientLayout.tsx`：在 `ProgressProvider` 之前插入 `<I18nProvider>` 包装
- [ ] 2.9 创建 `ui/components/LanguageSwitcher/LanguageSwitcher.tsx`：基于 Radix DropdownMenu；挂入用户菜单组件的 "Sign out" 项之前
- [ ] 2.10 迁移 `ui/lib/constants/logs.ts`：`ProviderLabels` / `RequestTypeLabels` / `StatusColors` / `RequestTypeColors` 的 value 由英文 string 改为 `t('logs.providers.openai.label')` 等调用；保留 TS 类型签名
- [ ] 2.11 迁移 `ui/lib/constants/config.ts`：`ModelPlaceholders` 等 value 改 `t('config.providers.openai.placeholder')` 调用；保留 TS 类型
- [ ] 2.12 迁移 `ui/lib/constants/governance.ts`：`resetDurationLabels` 等 value 改 `t('governance.reset.1h.label')` 调用；保留 TS 类型
- [ ] 2.13 翻译 dashboard 路由表层：`ui/app/workspace/dashboard/` 内路由标题、tab、组件内部文案；`useTranslation('dashboard')`
- [ ] 2.14 翻译 providers 路由表层：`ui/app/workspace/providers/` 内 provider 列表/详情/路由规则组件的表头、操作按钮、tooltip、空状态；`useTranslation('providers')`
- [ ] 2.15 翻译 governance 路由表层：`ui/app/workspace/governance/` + `virtual-keys/` 子路由的表头、按钮、空状态；`useTranslation('governance-ui')` + `useTranslation('governance')`
- [ ] 2.16 翻译全局共享组件：顶部导航栏、命令面板、Toaster (sonner)、通用确认对话框、通用按钮、通用空状态；`useTranslation('common')`
- [ ] 2.17 创建 `docs/features/i18n.mdx`：简述当前支持的语言、如何在用户菜单切换、如何贡献翻译
- [ ] 2.18 全量 `rg "from.*constants/(logs|config|governance)"` 扫一遍，验证所有 import 仍能找到对应类型导出

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

## 6. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [ ] 6.1 确认 `.pg/changes/add-ui-i18n-zh-en/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 6.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 6.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 6.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 6.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 6.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [ ] 6.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [ ] 6.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [ ] 6.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 7. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 7.1 收集所有 stage 的 Gate Assessment
- [ ] 7.2 检查跨 stage 依赖项
- [ ] 7.3 输出 Final Gate Assessment
