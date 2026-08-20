> - **environment 选择**：dev → local，int → local

## 1. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [x] 1.1 在 `ui/app/workspace/plugins/fragments/__tests__/` 下新增 `governanceFragment.test.tsx`，验证 EnabledSwitch 点击触发 updatePlugin + RBAC 拦截
- [x] 1.2 验证 ConfigForm 4 字段初始值从 plugin.config 正确填充（含 `*bool` 未设置时回退到 `false`、`*[]string` 未设置时回退到 `[]`、`*int` 未设置时回退到 `5`）
- [x] 1.3 验证 ConfigForm 提交后调用 updatePlugin({ name: GOVERNANCE_PLUGIN, data: { enabled, config } })，断言 payload 含 4 字段且值正确
- [x] 1.4 验证 `governanceConfigSchema` zod 校验：缺字段不报错、类型不匹配报错、`routing_chain_max_depth` 超 100 报错

## 2. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 2.1 在 `ui/lib/types/plugins.ts` 中新增 `governanceConfigSchema`（zod object 覆盖 4 字段）+ `GOVERNANCE_PLUGIN = "governance"` + `OTEL_PLUGIN = "otel"` 常量
- [ ] 2.2 新建 `ui/app/workspace/plugins/fragments/governanceFragment.tsx`，导出 `EnabledSwitch`（独立 toggle + RBAC）、`ConfigForm`（react-hook-form + zodResolver）、`GovernanceFragment`（默认导出，组合两者）。ConfigForm 含 2 个 fieldset：「访问控制」含 is_vk_mandatory(Switch) + required_headers(TagInput)、「行为」含 disable_auto_tool_inject(Switch) + routing_chain_max_depth(Input number)
- [ ] 2.3 在 `ui/app/workspace/plugins/views/pluginsView.tsx` 的 if-chain（PROVIDER_COOLDOWN_PLUGIN / RTK_PLUGIN 之后）追加 GOVERNANCE_PLUGIN → GovernanceFragment、OTEL_PLUGIN → OtelView 两个分支
- [ ] 2.4 从 `ui/app/workspace/observability/views/plugins/otelView.tsx` 导入 OtelView，在 pluginsView.tsx 中传入 selectedPlugin 复用
- [ ] 2.5 在 `ui/locales/en/plugins.json` 添加 governance 段（enableTitle / enableDescription / settingsTitle / isVkMandatoryLabel / requiredHeadersLabel / requiredHeadersPlaceholder / disableAutoToolInjectLabel / routingChainMaxDepthLabel / saveConfiguration / savedToast / updateFailedToast 等）
- [ ] 2.6 在 `ui/locales/zh-CN/plugins.json` 添加 governance 段，中文翻译（确认无「令牌」误用；LLM token → 词元，不适用；access control → 访问控制）

## 3. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/add-plugins-governance-form 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> - governanceFragment 是否完全沿用 rtkFragment 模式（命名 / 提交语义 / RBAC 门控）
> - pluginsView.tsx 的 if-chain 顺序是否正确（governance / otel 必须在 fallback 之前）
> - i18n 键命名是否与现有 plugins.json 命名风格一致（snake_case + 段落前缀）
> - TagInput 实现是否正确处理「空字符串不添加」与「重复不添加」边界

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

- [ ] 6.1 确认 `.pg/changes/add-plugins-governance-form/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 6.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 6.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 6.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 6.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 6.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`
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
