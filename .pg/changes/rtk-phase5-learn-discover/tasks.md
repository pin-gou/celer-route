> - **environment 选择**：dev → local

## 1. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 `plugins/rtk/discover_test.go`：表驱动测试覆盖 `DiscoverNormalizeLine`（时间戳/hex/版本号/整数/`<PKG>`/`<CODE>`/时间单位/空白折叠）和 `DiscoverRepeatedNoise`（单 sample 过滤/hits 聚合/sort/pattern 转义）。先写测试，确认跑现有 `plugins/rtk` 全集时新增子测试全部 FAIL（红）
- [ ] 1.2 编写 `plugins/rtk/learn_test.go`：表驱动测试覆盖 `SuggestFilter`（命令 slug、错误/摘要识别、阈值过滤、冲突守卫、空样本骨架）和 `CommandToId`（trim/lower/字符替换/首尾去 `-`）。先写测试，确认新增子测试全部 FAIL（红）

## 2. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 2.1 实现 `plugins/rtk/discover.go`：`CommandSample`/`NoiseCandidate` 类型；`discoverNormalizeLine(line)` 复用 `grouper.normalizeLine` 后追加 3 步（npm/pip 包名 → `<PKG>@<N>`、错误码 → `<CODE>`、时间/大小单位 → `<N>`）+ 空白折叠；`discoverRepeatedNoise(samples)` 跨 sample 聚合 + 单 sample 过滤 + `normalizedToPattern` 转 regex 安全字符串（转义特殊字符、`<N>` → `[\S]+`、`<PKG>` → `[\S]+`、`<CODE>` → `[A-Z][A-Z0-9]+`、加 `^` 前缀锚定）+ hits desc + pattern asc 排序
- [ ] 2.2 实现 `plugins/rtk/learn.go`：`SuggestedFilter` canonical JSON 形态（含 `_meta` 块）；`SuggestFilter(command, samples)` 实现 ERROR_PATTERN/SUMMARY_PATTERN 行识别 → preservedRawLines 收集 → DROP_THRESHOLD_RATIO=0.5 阈值过滤 → 冲突守卫（drop candidate 匹配任一 preserved 行则跳过）→ 组装骨架；`CommandToId(command)` slug 算法；空样本骨架分支
- [ ] 2.3 对齐 OmniRoute：对照 `/home/ubuntu/workspace/OmniRoute/open-sse/services/compression/engines/rtk/discover.ts` 与 `learn.ts` 源文件，确认正则、阈值、排序、守卫策略与字段命名完全一致（避免凭记忆改导致 drift）
- [ ] 2.4 跑 `cd plugins/rtk && go test -short -count=1 ./...` 确认新增测试全部 PASS（绿）且既有 27 个 RTK 测试零回归
- [ ] 2.5 跑 `cd plugins/rtk && go vet ./...` 确认无告警；跑 `gofmt -l plugins/rtk/` 确认无格式漂移

## 3. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/rtk-phase5-learn-discover 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 启动服务（如需）
- [x] 4.4 验证 V-plugins-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - skipped: V-plugins-1, V-plugins-2, V-plugins-3, V-plugins-4, V-plugins-5, V-plugins-6

## 5. dev.plugins:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 6. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [x] 6.1 收集所有 stage 的 Gate Assessment
- [x] 6.2 检查跨 stage 依赖项
- [x] 6.3 输出 Final Gate Assessment
