> - **environment 选择**：dev → local，int → local

## 1. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 1.1 编写 transports 配置 schema 校验测试（红）：仿照 `transports/pg-gateway-http/lib/validator_test.go` 的 loadLocalSchema 模式，断言 rtk 插件配置含 `enable_grouping`(bool)/`grouping_threshold`(int, minimum 2)/`apply_to_assistant_messages`(bool) 时校验通过；`grouping_threshold: 1` 被 schema 拒绝；未声明字段仍被 `additionalProperties: false` 拒绝（V-transports-1）

## 2. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 2.1 更新 transports/config.schema.json rtk 配置块（if/then name=rtk 段，约 3134-3189 行）：properties 新增 `enable_grouping`（boolean, default false）、`grouping_threshold`（integer, minimum 2, default 3）、`apply_to_assistant_messages`（boolean, default false），保持 `additionalProperties: false` 与既有 8 个字段定义不变，使 1.1 测试转绿

## 3. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/rtk-phase2-grouping-assistant 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 启动服务（如需）
- [x] 4.4 验证 V-transports-1：来自 design.md 的 Verification Criteria

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-5
  - degraded: V-plugins-1, V-plugins-2, V-plugins-3, V-plugins-4

## 5. dev.transports:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 6. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 6.1 新建 plugins/rtk/grouper_test.go TestNormalizeLine（红）：对照 OmniRoute grouper.ts 7 步规则逐条断言——ISO 时间戳/括号时间 `[2024-01-01 10:00:00]`/hex 6-40 位/semver（含 v 前缀与多段）/独立整数归一为 `<N>`，连续空白折叠为单空格，首尾 trim
- [ ] 6.2 grouper_test.go TestGroupSimilarLines（红）：种子用例取自 OmniRoute rtk-grouping.test.ts——6 行 `Downloaded chunk N` 归并为 `Downloaded chunk 1 [rtk:grouped ×6]`（grouped=5）；2 行低于默认 threshold=3 不合并；threshold=2 时 2 行合并；hex 变体 3 行合并；时间戳变体 3 行合并；版本号变体 8 行合并；threshold=1 被 clamp 为 2；非相似行原样保留
- [ ] 6.3 compression_test.go 新增 TestApplyToAssistantMessages（红）：apply_to_assistant_messages=true 时 OpenAI assistant ContentStr 与 Anthropic text block 被压缩；apply_to_code_blocks=true 且含 ``` 围栏时仅围栏内部被压缩、围栏外逐字保留；tool_use block、reasoning、带 cache_control 的块字节级不变；两开关均 false 时 assistant 消息逐字不变（V-plugins-2）
- [ ] 6.4 compression_test.go 新增 TestIntensityScaling（红）：effectiveMaxLines(100) minimal/standard/aggressive = 150/100/50；base=1 时 aggressive 结果 ≥1；minimal 档 Head/Tail 不变；filter 无 max_lines 时回退 Config.MaxLinesPerResult 再乘强度系数；maxChars 不受强度缩放（V-plugins-3）
- [ ] 6.5 config_test.go 新增分组字段用例（红）：零值填默认（EnableGrouping=false/GroupingThreshold=3/ApplyToAssistantMessages=false）；GroupingThreshold<2 clamp 为 2；非法 intensity 拒绝（V-plugins-1/V-plugins-3）

## 7. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 7.1 新建 plugins/rtk/grouper.go：normalizeLine（7 步归一化，正则包级 regexp.MustCompile 预编译，对照 OmniRoute grouper.ts:39-56 逐条移植）+ groupSimilarLines（threshold=max(2, 配置值)，连续 runLength>=threshold 归一化相等行合并为「首行 [rtk:grouped ×N]」，单遍 O(行数) 扫描，返回压缩文本与归并行数）
- [ ] 7.2 config.go：Config 新增 EnableGrouping/GroupingThreshold/ApplyToAssistantMessages 字段（json tag：enable_grouping/grouping_threshold/apply_to_assistant_messages）+ applyConfigDefaults 填默认值 + threshold<2 clamp 为 2（WARN 日志记录配置值与 clamp 结果）
- [ ] 7.3 compression.go 管线集成分组：processRtkTextWithCommand 在 applyDedup 后、scaleFilterForIntensity/applySmartTruncate 前插入 groupSimilarLines（enable_grouping 开关，默认关时零开销）；归并行数>0 时向 ProcessStats.Techniques 追加 rtk-grouping；isDocumentLikeRead 文档保护路径 dedup 后同样应用分组（V-plugins-1）
- [ ] 7.4 compression.go assistant 分支：applyRtkCompression 主循环对 role=assistant 增加压缩路径——apply_to_assistant_messages=true 时全文压缩（OpenAI ContentStr / Anthropic text block）；否则 apply_to_code_blocks=true 且含 ``` 围栏时轻量 fence 切分仅压缩代码块内部（单遍扫描）；跳过 tool_use block、reasoning、带 cache_control 的块；Responses API 路径不动（V-plugins-2）
- [ ] 7.5 compression.go 强度缩放修正：新增 effectiveMaxLines(base, intensity)（minimal×1.5/standard×1/aggressive×0.5，max(1, round)）；scaleFilterForIntensity 补 minimal 分支（仅缩放 MaxLines，Head/Tail/maxChars 不动）；applySmartTruncate 前 filter 无 MaxLines 时 fallback Config.MaxLinesPerResult 再应用强度系数（对齐 OmniRoute index.ts:250 公式）（V-plugins-3）
- [ ] 7.6 使 6.1-6.5 全部测试转绿，并全量回归 plugins/rtk 既有 ~70 个用例（cache_control 字节级保护、截断/去重标记、文档保护路径行为不变）（V-plugins-4）

## 8. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/rtk-phase2-grouping-assistant 做静态审查
- [ ] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 9. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 9.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 9.2 执行测试（runner 通过 modules 注入命令）
- [ ] 9.3 启动服务（如需）
- [ ] 9.4 验证 V-plugins-1, V-plugins-2, V-plugins-3, V-plugins-4：来自 design.md 的 Verification Criteria（V-plugins-1~4 均为 degraded，验证方式=plugins 模块单元测试；V-plugins-5 为 verifiable，由 int.scr scenario 验证，不在本章节执行）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-5
  - degraded: V-plugins-1, V-plugins-2, V-plugins-3, V-plugins-4

## 10. dev.plugins:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 11. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [ ] 11.1 确认 `.pg/changes/rtk-phase2-grouping-assistant/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 11.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 11.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 11.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 11.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 11.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [ ] 11.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [ ] 11.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [ ] 11.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 12. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 12.1 收集所有 stage 的 Gate Assessment
- [ ] 12.2 检查跨 stage 依赖项
- [ ] 12.3 输出 Final Gate Assessment
