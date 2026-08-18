> - **environment 选择**：dev → local，int → local

## 1. dev.core:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 1.1 编写 `core/schemas/chatcompletions_test.go` 单元测试：验证 `BifrostLLMUsage` 的 `OriginalPromptTokens` / `CompressedPromptTokens` 字段可正常序列化（JSON tag 路径正确）+ 与 `PromptTokens` 共存（omitempty 指针）

## 2. dev.core:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 2.1 在 `core/schemas/chatcompletions.go` 的 `BifrostLLMUsage` struct 上新增 `OriginalPromptTokens *int` 与 `CompressedPromptTokens *int` 字段（JSON tag `original_prompt_tokens,omitempty` / `compressed_prompt_tokens,omitempty`）
- [x] 2.2 在 `core/schemas/bifrost.go` 的 `BifrostContextKey` 常量块新增 `BifrostContextKeyOriginalPromptTokens` 与 `BifrostContextKeyCompressedPromptTokens`（值：`x-bf-original-prompt-tokens` / `x-bf-compressed-prompt-tokens`）
- [ ] 2.3 跑 `cd core && go build ./...` 确认编译通过

## 3. dev.core:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/add-rtk-compression-plugin 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> - `BifrostLLMUsage` 新增字段是否为 Pointer + `omitempty`（避免 nil deref）
> - JSON 字段名是否与 proposal.md 描述一致
> - `BifrostContextKey` 类型是否一致（用 `BifrostContextKey` 常量类型）

## 4. dev.core:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 启动服务（如需）
- [x] 4.4 验证 V-core-1：来自 design.md Verification Criteria

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-core-1, V-plugins-3, V-plugins-4, V-plugins-5

## 5. dev.core:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- 无

## 6. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 6.1 编写 `transports/pg-gateway-http/server/plugins_test.go` 单元测试：验证 RTK plugin init case 正确加载 + config schema 校验通过
- [x] 6.2 编写 `transports/pg-gateway-http/handlers/logging_test.go` 单元测试：验证 logging handler 读取 `BifrostContextKeyOriginalPromptTokens` 并写入 logs-db metadata

## 7. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 7.1 修改 `transports/config.schema.json` 在 `plugins.items` 数组的 `if/then` 块下新增 `name: "rtk"` 的 `then` 分支，包含 `enabled` / `intensity` / `apply_to_tool_results` / `apply_to_code_blocks` / `max_lines_per_result` / `max_chars_per_result` / `deduplicate_threshold` / `preserve_cache_control` 等属性
- [x] 7.2 修改 `transports/pg-gateway-http/server/plugins.go` 增加 RTK plugin init case（参考 semanticcache 模式），调用 `rtk.Init(ctx, config, logger)`
- [x] 7.3 修改 `transports/pg-gateway-http/handlers/logging.go` 在 log entry 写入前读取 `BifrostContextKeyOriginalPromptTokens` / `BifrostContextKeyCompressedPromptTokens`，若存在则注入到 log metadata JSON
- [x] 7.4 跑 `cd transports/pg-gateway-http && go build ./...` 确认编译通过

## 8. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 8.2 review agent 对 git diff feat/pg/add-rtk-compression-plugin 做静态审查
- [x] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> - config.schema.json 注入位置是否正确（与 `semantic_cache` 块同级）
> - logging handler 读取 ctx 的路径是否覆盖 streaming 路径
> - plugin init 是否有 panic 防护（恶意 config）

## 9. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 9.1 执行 lint（runner 通过 modules 注入命令）
- [x] 9.2 执行测试（runner 通过 modules 注入命令）
- [x] 9.3 启动服务（如需）
- [x] 9.4 验证 V-transports-1：来自 design.md Verification Criteria
- [x] 9.5 验证 V-transports-2：来自 design.md Verification Criteria

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-core-1, V-plugins-3, V-plugins-4, V-plugins-5

## 10. dev.transports:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 11. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 11.1 编写 `plugins/rtk/rtk_test.go` 单元测试：覆盖核心压缩逻辑（git status / npm install / docker logs 等 50+ 内置命令），断言压缩后 token 减少、关键信息保留
- [ ] 11.2 编写 `plugins/rtk/filterloader_test.go` 单元测试：验证 filter 加载 + ReDoS 保护 + 优先级匹配（project > global > builtin + generic-output fallback）
- [ ] 11.3 编写 `plugins/rtk/linefilter_test.go` 单元测试：覆盖 strip / keep / collapse / replace / dedup / head/tail 截断
- [ ] 11.4 编写 `plugins/rtk/hooks_test.go` 单元测试：PreLLMHook 修改 messages + PostLLMHook 重写 usage + ctx 透传
- [ ] 11.5 编写 `plugins/rtk/anthropic_test.go` 单元测试：tool_result blocks 识别 + Anthropic 适配
- [ ] 11.6 编写 `plugins/rtk/cache_control_test.go` 单元测试：cache_control 块保护（压缩后字节完全相同）

## 12. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 12.1 创建 `plugins/rtk/` 目录结构 + `go.mod`（独立 module，引用 core 子模块）
- [ ] 12.2 实现 `plugins/rtk/config.go`（Config struct + UnmarshalJSON + Validate）
- [ ] 12.3 实现 `plugins/rtk/rtk.go`（Plugin struct + Init 入口）
- [ ] 12.4 实现 `plugins/rtk/state.go`（per-request compression state，sync.Map keyed by requestID）
- [ ] 12.5 实现 `plugins/rtk/textoken.go`（token 估算，char/4 + 可选 tiktoken 接口）
- [ ] 12.6 实现 `plugins/rtk/linefilter.go`（行级规则执行：strip / keep / collapse / replace / dedup / head/tail 截断）
- [ ] 12.7 实现 `plugins/rtk/smarttruncate.go`（智能截断：head/tail 窗口 + priority pattern 保护 + char 硬限制）
- [ ] 12.8 实现 `plugins/rtk/deduplicator.go`（连续重复行合并）
- [ ] 12.9 实现 `plugins/rtk/linedetector.go`（commandDetector：50+ 内置检测器，静态正则表）
- [ ] 12.10 实现 `plugins/rtk/filterloader.go`（filter 加载 + ReDoS 保护 + 优先级匹配 + 编译缓存）
- [ ] 12.11 实现 `plugins/rtk/compression.go`（applyRtkCompression 顶层 + processRtkText 流水线）
- [ ] 12.12 实现 `plugins/rtk/anthropic.go`（Anthropic adapter：识别 `content[].type == "tool_result"` 块）
- [ ] 12.13 实现 `plugins/rtk/openai.go`（OpenAI adapter：通过 `tool_call_id` 关联链）
- [ ] 12.14 实现 `plugins/rtk/hooks.go`（PreLLMHook + PostLLMHook）
- [ ] 12.15 移植 50+ 内置 JSON 过滤器到 `plugins/rtk/filters/builtin/`（git status / npm install / docker logs / make / kubectl / ...），用 `go:embed` 打包
- [ ] 12.16 添加 `plugins/rtk` 到 `go.work` 工作空间
- [ ] 12.17 跑 `cd plugins/rtk && go build ./... && go test ./... -short` 确认全部通过

## 13. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 13.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 13.2 review agent 对 git diff feat/pg/add-rtk-compression-plugin 做静态审查
- [x] 13.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 13.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> - 是否所有 50+ 内置过滤器都打包进了 `filters/builtin/`（避免运行时找不到）
> - sync.Map 的 state 是否在 PostLLMHook 后清理（防止内存泄漏）
> - rule execution error 是否 fallback 到 passthrough（fail-open）
> - ReDoS 保护是否真的过滤了危险正则

## 14. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 14.1 执行 lint（runner 通过 modules 注入命令）
- [x] 14.2 执行测试（runner 通过 modules 注入命令）
- [x] 14.3 启动服务（如需）
- [x] 14.4 验证 V-plugins-1：来自 design.md Verification Criteria
- [x] 14.5 验证 V-plugins-2：来自 design.md Verification Criteria
- [x] 14.6 验证 V-plugins-3：来自 design.md Verification Criteria
- [x] 14.7 验证 V-plugins-4：来自 design.md Verification Criteria
- [x] 14.8 验证 V-plugins-5：来自 design.md Verification Criteria

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-plugins-1, V-plugins-2, V-core-1, V-plugins-3, V-plugins-4, V-plugins-5

## 15. dev.plugins:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 16. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [x] 16.1 确认 `.pg/changes/add-rtk-compression-plugin/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [x] 16.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [x] 16.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [x] 16.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [x] 16.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [x] 16.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [x] 16.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [x] 16.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [x] 16.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 17. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 17.1 收集所有 stage 的 Gate Assessment
- [ ] 17.2 检查跨 stage 依赖项
- [ ] 17.3 输出 Final Gate Assessment
