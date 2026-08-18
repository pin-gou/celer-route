> - **environment 选择**：dev → local

## 1. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 1.1 在 `plugins/rtk/linedetector.go` 加 `hasGenericErrorMarkers(text) bool` 函数（正则 `Error:|Exception:|Traceback \(most recent call last\):`），与 `isShortErrorMessage` 风格一致，编译期预编译 `*regexp.Regexp`（红）
- [ ] 1.2 改 `plugins/rtk/smarttruncate.go` 的 `applySmartTruncate` 返回 `(string, int)`，并在 head/tail 之间插入 `[rtk:truncated N lines]`，同时**更新 3 处现有测试断言**：`TestLineFilterHeadAndTail`、`TestLineFilterMaxLines` 接收新签名与新 marker 文本（红：旧测试断言会因 marker 字符串变化而失败；改完签名后旧调用点会因接收元组失败——预期红）
- [ ] 1.3 改 `plugins/rtk/deduplicator.go` 的 `applyDedup` 返回 `(string, int)`，对 runLen>=threshold 行追加 `[line repeated Nx]` + `[rtk:dropped N repeated lines]`，同时**更新现有测试断言**：`TestLineFilterDedup`（红：旧断言期望 `"Compiling...\nDone!\n"`，新断言应含 marker）
- [ ] 1.4 新增 5 个测试到 `plugins/rtk/linefilter_test.go`：
  - `TestDocumentReadNotTruncated`：~147 行无错误标记代码，detection 兜底为 `{Type:"shell", Command:""}` → 输出保留全文，无 `[rtk:truncated ...]` marker
  - `TestIsDocumentLikeReadWithErrorMarkers`：含 `Traceback (most recent call last):` 的输入 → 走正常管线（filter + smartTruncate + char limit 全跑）
  - `TestTruncateMarker`：filter.Head=3 + Tail=2 + 10 行 → 输出含 `[rtk:truncated 5 lines]`
  - `TestDedupMarker`：threshold=3 + 5 个连续相同行 + 1 个不同行 → 输出含 `[line repeated 4x]` + `[rtk:dropped 4 repeated lines]`
  - `TestCharTruncateMarker`：调用 `processRtkTextWithCommand` 配 `MaxCharsPerResult=100` + 超长 input → 输出末尾含 `[rtk:truncated by chars]`

## 2. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 2.1 在 `plugins/rtk/linedetector.go` 实现 `hasGenericErrorMarkers(text)`：正则 `regexp.MustCompile(`Error:|Exception:|Traceback \(most recent call last\):`)`，调用 `.MatchString(text)` 返回 bool
- [ ] 2.2 改 `plugins/rtk/compression.go` 的 `processRtkTextWithCommand`：在 step 4 之后、step 5 之前加 isDocumentLikeRead 分支
  ```go
  isDocumentLikeRead := detection.Type == "shell" && detection.Command == "" && !hasGenericErrorMarkers(text)
  if isDocumentLikeRead {
      // 跳过 step 5 (loader.Match) / 6 (applyLineFilter) / 8 (applySmartTruncate) / 9 (truncateToCharLimit)
      // 保留 step 7 (applyDedup)
  }
  ```
- [ ] 2.3 改 `plugins/rtk/compression.go` step 7：接收 `applyDedup` 返回的 `(string, int)`，丢弃 int
- [ ] 2.4 改 `plugins/rtk/compression.go` step 8：接收 `applySmartTruncate` 返回的 `(string, int)`，丢弃 int
- [ ] 2.5 改 `plugins/rtk/compression.go` step 9：在 `truncateToCharLimit` 返回值后 append `\n[rtk:truncated by chars]\n`，且仅当实际发生截断（`len(result) > config.MaxCharsPerResult`）时追加
- [ ] 2.6 改 `plugins/rtk/smarttruncate.go`：在 `applySmartTruncate` 内部当 head+tail < len(content) 时，把保留 head 与保留 tail 之间插入单行 `[rtk:truncated N lines]`（N = len(content) - 保留的 head 数 - 保留的 tail 数 - 抢救的 priority 行数）；返回签名改为 `(string, int)`
- [ ] 2.7 改 `plugins/rtk/deduplicator.go`：在 `applyDedup` 内部对 runLen>=threshold 的行，在首行后 append `[line repeated Nx]` + `[rtk:dropped N repeated lines]`（N = runLen-1）；返回签名改为 `(string, int)`
- [ ] 2.8 跑 `go build ./...` 与 `go vet ./...` 在 `plugins/rtk/` 目录，确认签名变更无遗漏调用点

## 3. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/rtk-stage1-markers-and-doc-protection 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 4.1 执行 lint（runner 通过 modules.plugins.lint 注入命令）：`for d in plugins/*/; do (cd "$d" && go vet ./...); done`
- [ ] 4.2 执行测试（runner 通过 modules.plugins.test.unit 注入命令）：`for d in plugins/*/; do (cd "$d" && go test ./... -short -count=1); done`
- [ ] 4.3 启动服务：无需启动（纯 plugins 单元测试驱动）
- [ ] 4.4 验证 V-plugins-N：来自 design.md Verification Criteria（N 由 design.md 决定，非章节号）

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-plugins-3, V-plugins-4, V-plugins-5, V-plugins-6, V-plugins-7

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - V-plugins-1～4：测试输出（含 marker 文本断言）来自 `go test -v -run TestDocument|TestTruncate|TestDedup|TestChar` 跑分
  - V-plugins-5/6：build / vet 命令退出码 0 + 编译日志摘要
  - V-plugins-7：测试结果 `Tests run: N, Failures: 0, Errors: 0` 必须有日志摘要（N = 现有 24 + 更新 3 + 新增 5 = 32 个测试用例）

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
