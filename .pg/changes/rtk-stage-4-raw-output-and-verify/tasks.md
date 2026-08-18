> - **environment 选择**：dev → local，int → local

## 1. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 plugins/rtk/rawoutput_test.go：覆盖 5 类脱敏正则（OpenAI key / Slack token / AWS key / key=value credential field / Authorization: Bearer|Basic），覆盖 retention=never/failures/always 三策略的文件落盘/不落盘行为，覆盖 sidecar .meta.json 5 字段（command/timestamp/failure/redacted/bytes），覆盖 isLikelyFailureOutput 9 关键词，触发 chmod 0 验证 EACCES best-effort 降级不 panic（红）
- [ ] 1.2 编写 plugins/rtk/verify_test.go：覆盖 RunRtkFilterTests 返回 Passed/Outcomes/Benchmark/FiltersWithoutTests 四段，覆盖 benchmark 按 category 聚合数学，覆盖 trimComparable 去除尾部换行（红）
- [ ] 1.3 编写 plugins/rtk/filterloader_test.go：覆盖 52 个 builtin JSON 的 `tests` 字段解析兼容 legacy + canonical 双格式（红）
- [ ] 1.4 编写 plugins/rtk/rtk_test.go：覆盖 processRtkTextWithCommand 在 stats.CompressedTokens < stats.OriginalTokens 时触发 MaybePersistRtkRawOutput 并把 pointer 累加到 CompressionState.RawOutputPointers（红）

## 2. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 2.1 新建 plugins/rtk/rawoutput.go：实现 RtkRawOutputRetention 枚举、RtkRawOutputPointer 结构、5 条脱敏正则（预编译 var reSecretOpenAI/Slack/AWS/CredField/AuthHeader）、RedactRtkRawOutput/IsLikelyFailureOutput/MaybePersistRtkRawOutput/ReadRtkRawOutput 公开 API；落盘路径 `<appDir>/rtk/raw-output/<ts_ms>-<command_slug>-<sha256_id24>.log` + 同名 `.meta.json` sidecar；UTF-8 字节级截断 safeUtf8Slice；磁盘错误 best-effort 返回 nil 不 panic
- [ ] 2.2 新建 plugins/rtk/verify.go：实现 FilterTest/FilterTestOutcome/FilterBenchmarkRow/VerifyResult 类型、RunRtkFilterTests(opts *VerifyOptions) VerifyResult 主入口（遍历 loader.cachedFilters、跑 applyLineFilter 比对 expected、按 category 聚合 benchmark）、trimComparable 工具函数；VerifyOptions 含 RequireAll/CustomFiltersEnabled/TrustProjectFilters/AppDir 字段
- [ ] 2.3 修改 plugins/rtk/filterloader.go：Filter struct 新增 `Tests []FilterTest \`json:"tests,omitempty"\`` 顶层字段（双格式 JSON 自动接管，阶段三 UnmarshalJSON 仲裁无需改）
- [ ] 2.4 修改 plugins/rtk/config.go：新增 `RawOutputRetention string \`json:"raw_output_retention"\`` (default "never") + `RawOutputMaxBytes int \`json:"raw_output_max_bytes"\`` (default 1048576, min 1024)；Validate 新增枚举校验 + 数值范围校验；applyConfigDefaults 填默认值
- [ ] 2.5 修改 plugins/rtk/compression.go：processRtkTextWithCommand 在 stats.CompressedTokens < stats.OriginalTokens 时（实际压缩，严格对齐 OmniRoute D1 决策）调 MaybePersistRtkRawOutput(text, {retention: config.RawOutputRetention, command: cmd, maxBytes: config.RawOutputMaxBytes})；拿到的 pointer 累加到 ProcessStats.RawOutputPointers
- [ ] 2.6 修改 plugins/rtk/state.go：CompressionState 新增 `RawOutputPointers []*RtkRawOutputPointer` 字段；applyRtkCompression 与 applyRtkCompressionResponses 主循环把 ProcessStats.RawOutputPointers 累加到 state.RawOutputPointers
- [ ] 2.7 修改 52 个 plugins/rtk/filters/builtin/*.json：每个 JSON 加 `tests: [{name, command?, input, expected}]` 字段，每个至少 1 个最小用例（无 rules 的 filter 验证 head/tail 截断 + 标记行，有 rules 的 filter 覆盖对应 strip/keep/collapse 行为）
- [ ] 2.8 修改 transports/config.schema.json rtk 配置块：新增 raw_output_retention (enum: never|failures|always, default "never") + raw_output_max_bytes (integer, minimum 1024, default 1048576)
- [ ] 2.9 跑 `cd plugins/rtk && go build ./...` + `go test ./... -short -count=1 -run TestRetentionPolicies/TestRedactRtkRawOutput/TestIsLikelyFailureOutput/TestSidecarMetadata/TestDiskErrorGracefulDegradation/TestRunRtkFilterTests/TestBuiltinFiltersHaveTests/TestBenchmarkAggregation/TestCompressionTriggersRawOutput/TestStateRawOutputPointersPropagation`，确认红测试转绿

## 3. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 3.2 review agent 对 git diff feat/pg/rtk-stage-4-raw-output-and-verify 做静态审查
- [ ] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 4.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 4.2 执行测试（runner 通过 modules 注入命令）
- [ ] 4.3 启动服务（如需）
- [ ] 4.4 验证 V-plugins-N：来自 design.md（N 由 design.md 决定，非章节号）

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-plugins-3, V-plugins-4, V-plugins-5, V-plugins-6, V-plugins-7, V-plugins-8, V-plugins-9, V-plugins-10

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

- [ ] 6.1 收集所有 stage 的 Gate Assessment
- [ ] 6.2 检查跨 stage 依赖项
- [ ] 6.3 输出 Final Gate Assessment
