> - **environment 选择**：dev → local，int → local

## 1. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 Vitest 键完整性测试：扩展 `ui/src/__tests__/i18n/key-completeness.test.ts`，遍历 `ui/locales/{en,zh-CN}/*.json` 全部 15 个 namespace 文件，断言每个 namespace 内部 key 集合在 en 与 zh-CN 之间完全一致（红——新 namespace 未注册）
- [ ] 1.2 编写 Vitest namespace sanity 测试：扩展 `ui/src/__tests__/i18n/namespace-sanity.test.ts`，断言 en 与 zh-CN 各有且仅有 15 个 namespace（common / logs / config / governance / providers / dashboard / governance-ui / mcp / routing / skills / plugins / observability / webhooks / oauth-grants / model-catalog）（红——新 namespace 尚未建出）
- [ ] 1.3 编写 Playwright i18n 切换 E2E：扩展 `tests/e2e/features/i18n/i18n.spec.ts`，新增 19 路由遍历断言（登录态下访问每个路由 → 切到 zh-CN → 断言关键中文文案出现 → 切回 en → 断言恢复英文）（红——19 路由尚未翻译）
- [ ] 1.4 编写 Vitest NoPermissionView 组件测试：创建 `ui/src/__tests__/components/noPermissionView.test.tsx`，覆盖 (a) 传入 `entityI18nKey` 时显示翻译文案；(b) 仅传 `entity` 时保持向后兼容显示原始字符串；(c) I18nProvider 未挂载时 fallback（红——NoPermissionView 尚未改造）

## 2. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 2.1 在 `ui/lib/i18n/config.ts` 的 `NS` 数组（line 21 附近）新增 8 项：`mcp`, `routing`, `skills`, `plugins`, `observability`, `webhooks`, `oauth-grants`, `model-catalog`
- [ ] 2.2 在 `ui/lib/i18n/config.ts` 的 `resources` 对象（line 33-52 附近）注册 8 个新 namespace 的 en + zh-CN locale 对
- [ ] 2.3 验证 `ui/lib/i18n/types.ts`（构建期由 vite-plugin-i18next-typescript 自动生成）包含全部 15 namespace 的 `Resources` / `KeysWithNamespace` 类型；如未自动生成则手动添加
- [ ] 2.4 创建 `ui/locales/en/{mcp,routing,skills,plugins,observability,webhooks,oauth-grants,model-catalog}.json` 八个新 namespace 文件，填充英文初始值
- [ ] 2.5 创建 `ui/locales/zh-CN/` 八个对应 namespace 文件，填充中文翻译
- [ ] 2.6 扩展 `ui/locales/en/logs.json` 添加 mcp-logs 路由专属键（statCards / COLUMN_LABELS / emptyState / filterLabels）
- [ ] 2.7 扩展 `ui/locales/zh-CN/logs.json` 同上
- [ ] 2.8 扩展 `ui/locales/en/governance.json` 添加 model-limits 路由专属键
- [ ] 2.9 扩展 `ui/locales/zh-CN/governance.json` 同上
- [ ] 2.10 扩展 `ui/locales/en/config.json` 添加 config 全部 12 子路由 + custom-pricing + mcp-settings 专属键
- [ ] 2.11 扩展 `ui/locales/zh-CN/config.json` 同上
- [ ] 2.12 扩展 `ui/locales/{en,zh-CN}/common.json` 补充 action / state / status / confirm 键集（覆盖 CRUD 动作文案：Install / Edit / Delete / Revoke / Restore / Activate / Deactivate 等；状态文案：Active / Inactive / Pending / Failed / Success / Error / Warning；确认文案：Delete / Discard / Cancel 等）
- [ ] 2.13 改造 `ui/components/noPermissionView.tsx`：新增可选 `entityI18nKey?: string` prop；保留 `entity: string` 向后兼容；当 `entityI18nKey` 存在时使用 `useTranslation` 渲染翻译文案
- [ ] 2.14 全量 `rg "from.*noPermissionView"` 扫描 19 个未翻译路由的 layout.tsx，替换为 `entityI18nKey="<namespace>:<route>.permissionDenied"` 模式
- [ ] 2.15 翻译 `ui/app/workspace/logs/` 全部 .tsx 表层（route title / tab / COLUMN_LABELS / statCards / emptyState / filterLabels），引入 `useTranslation('logs')`
- [ ] 2.16 翻译 `ui/app/workspace/mcp-logs/` 全部 .tsx 表层，`useTranslation('logs')`
- [ ] 2.17 翻译 `ui/app/workspace/mcp-registry/` 全部 .tsx 表层，`useTranslation('mcp')`
- [ ] 2.18 翻译 `ui/app/workspace/mcp-sessions/` 全部 .tsx 表层，`useTranslation('mcp')`
- [ ] 2.19 翻译 `ui/app/workspace/model-limits/` 全部 .tsx 表层，`useTranslation('governance')`
- [ ] 2.20 翻译 `ui/app/workspace/routing-rules/` 全部 .tsx 表层，`useTranslation('routing')`
- [ ] 2.21 翻译 `ui/app/workspace/skills-repo/` 全部 .tsx 表层，`useTranslation('skills')`
- [ ] 2.22 翻译 `ui/app/workspace/plugins/` 全部 .tsx 表层，`useTranslation('plugins')`
- [ ] 2.23 翻译 `ui/app/workspace/observability/` 全部 .tsx 表层，`useTranslation('observability')`
- [ ] 2.24 翻译 `ui/app/workspace/webhooks/` 全部 .tsx 表层，`useTranslation('webhooks')`
- [ ] 2.25 翻译 `ui/app/workspace/oauth-grants/` 全部 .tsx 表层，`useTranslation('oauth-grants')`
- [ ] 2.26 翻译 `ui/app/workspace/custom-pricing/` 全部 .tsx 表层，`useTranslation('config')`
- [ ] 2.27 翻译 `ui/app/workspace/config/` 全部 12 个子路由（api-keys / caching / client-settings / compatibility / feature-flags / large-payload / logging / mcp-gateway / observability / performance-tuning / pricing-config / security）的 views/*.tsx，`useTranslation('config')`
- [ ] 2.28 翻译 `ui/app/workspace/model-catalog/` 全部 .tsx 表层，`useTranslation('model-catalog')`
- [ ] 2.29 翻译 `ui/app/workspace/complexity-router/` 全部 .tsx 表层（tier 名称 / 边界标签 / transition labels），`useTranslation('routing')`
- [ ] 2.30 翻译 `ui/app/workspace/docs/` 全部 .tsx 表层，`useTranslation('common')`
- [ ] 2.31 更新 `docs/features/i18n.mdx` 补充本期 19 路由覆盖范围 + prompt-repo 共享组件英文保持说明 + dialog/form 字段 out-of-scope 说明

## 3. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/enhance-ui-i18n-coverage 做静态审查
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

- [ ] 6.1 确认 `.pg/changes/enhance-ui-i18n-coverage/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
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