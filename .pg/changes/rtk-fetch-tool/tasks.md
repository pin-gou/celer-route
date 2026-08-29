> **environment 选择**：dev → local，int → local

## 1. dev.core:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 `core/bifrost_test.go` 单元测试：验证 `Bifrost.GetMCPManager()` 返回正确的 `*mcp.MCPManager` 指针；nil bifrost 不 panic；返回的 manager 与构造时传入的 manager 同一引用
- [ ] 1.2 跑 `cd core && go test ./... -run TestGetMCPManager` 确认红（编译失败，因为方法不存在）→ 绿（任务 2.1 实现后）

## 2. dev.core:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 2.1 在 `core/bifrost.go` 的 `*Bifrost` receiver 上新增导出方法 `GetMCPManager() *mcp.MCPManager`，直接返回 `bifrost.MCPManager`（无需锁 — 字段在 NewBifrost 期间赋值后只读）
- [x] 2.2 跑 `cd core && go build ./...` 确认编译通过
- [x] 2.3 跑 `cd core && go test ./... -run TestGetMCPManager` 确认绿

## 3. dev.core:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

> **本次变更 review 关注点**：
> - `GetMCPManager()` 是否需要 nil check（mcp 字段可能是 nil if NewBifrost 收到空 config）
> - 是否在 bifrost 加锁再返回（应否，因为字段构造后只读）
> - 是否在 godoc 注释解释"为什么导出"（让 plugin 用）

## 4. dev.core:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=core (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 验证 V-core-1：来自 design.md Verification Criteria
- [x] 4.4 验证 V-core-N：来自 design.md Verification Criteria（N 由 design.md 决定）

  **Evidence 要求**：
  - 测试运行结果必须有日志摘要
  - V-core-1 (`GetMCPManager` 暴露) 对应 evidence：`go test -run TestGetMCPManager -v` 输出

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

- [ ] 6.1 编写 `transports/celer-route-http/server/plugins_test.go` 单元测试：验证 `applyRTKConfigDefaults` / `loadRTKConfig` 在 `inject_fetch_tool` 缺省 / 显式 true / 显式 false 三种 case 下正确填充 `Config.InjectFetchTool`
- [ ] 6.2 编写 `transports/celer-route-http/server/config_schema_test.go` 单元测试（若无则新建）：验证 `config.schema.json` 的 rtk 块增 `inject_fetch_tool` 字段后，schema 仍然合法（JSON Schema 自身合法 + required 字段未变）

## 7. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 7.1 在 `transports/config.schema.json` 的 rtk 块下追加 `inject_fetch_tool` 字段（type=boolean, default=true），按 JSON Schema 写法（参考既有 `enabled` 字段）
- [ ] 7.2 跑 `cd transports && go test ./server/...` 确认红→绿（schema validation 测试通过）
- [ ] 7.3 跑 `cd transports && go build ./...` 确认编译通过

## 8. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 8.1 review agent 读 design.md + tasks.md
- [x] 8.2 review agent 对 git diff 做静态审查
- [x] 8.3 review agent 输出 review_score + p0_failures
- [x] 8.4 score < pass_threshold → escalate；≥ pass_threshold → completed

> **本次变更 review 关注点**：
> - schema 字段描述是否清晰、是否与 design.md 中描述一致
> - default 值是否符合 proposal.md 承诺（true）
> - 是否破坏 schema 其它字段的 required 关系

## 9. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 9.1 执行 lint
- [x] 9.2 执行 `go test ./server/...`
- [x] 9.3 启动 gateway（如 verify agent 决定需要）；用 `curl PUT /api/context/rtk/config` 写入带 `inject_fetch_tool: false` 的 config，验证落库；再用 `inject_fetch_tool: true` / 缺省值重写，验证 default 行为
- [x] 9.4 验证 V-plugins-3 / V-plugins-4：来自 design.md Verification Criteria（schema 字段生效）

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

- [ ] 11.1 编写 `plugins/rtk/rawoutput_test.go` 单元测试：`TestRawOutputReadHandler_ValidID` — 写入 fixture `<appDir>/rtk/raw-output/1700000000000-fixture-0123456789abcdef01234567.log`，调用 `p.RawOutputReadHandler(ctx, map[string]any{"id":"0123456789abcdef01234567"})`，断言返回字符串以 `\x00RTK_RAW_OUTPUT_BEGIN\x00` 开头、以 `\x00RTK_RAW_OUTPUT_BODY_FOLLOWS\x00` 开头（V-plugins-1, V-plugins-7）
- [ ] 11.2 编写 `plugins/rtk/rawoutput_test.go` 单元测试：`TestRawOutputReadHandler_MissingFile` — 传入不存在 id，断言返回 `error` 且错误信息含 "not found or expired"
- [ ] 11.3 编写 `plugins/rtk/rawoutput_test.go` 单元测试：`TestRawOutputReadHandler_InvalidID` — 传入非 hex / 长度错的 id，断言返回 `error` 且错误信息含 "invalid id"
- [ ] 11.4 编写 `plugins/rtk/rawoutput_test.go` 单元测试：`TestRawOutputReadHandler_EmptyArgs` — 传入 `map[string]any{}`，断言返回 `error`
- [ ] 11.5 编写 `plugins/rtk/tool_schema_test.go`（新文件）单元测试：`TestRtkFetchRawOutputTool_Schema` — 断言 `RtkFetchRawOutputTool.Function.Name == "bifrostInternal-rtk_fetch_raw_output"`、`Required: ["id"]`、`id` 字段含 `pattern: ^[0-9a-f]{24}$`（V-plugins-2）
- [ ] 11.6 编写 `plugins/rtk/tool_schema_test.go` 单元测试：`TestRtkFetchRawOutputTool_StableSerialization` — 跑两次 `sonic.Marshal(RtkFetchRawOutputTool)`，断言输出字节一致（Anthropic cache 依赖）
- [ ] 11.7 编写 `plugins/rtk/hooks_test.go` 单元测试：`TestPreLLMHook_InjectsFetchToolSchema` — 构造 mock `MCPManagerLike` 返回含 `bifrostInternal-rtk_fetch_raw_output` 的工具列表，断言 `req.ChatRequest.Params.Tools` 末尾追加了 `RtkFetchRawOutputTool`（V-plugins-3）
- [ ] 11.8 编写 `plugins/rtk/hooks_test.go` 单元测试：`TestPreLLMHook_SkipsInjectWhenRTKDisabled` — `config.Enabled=false`，断言 `req.ChatRequest.Params.Tools` 不变
- [ ] 11.9 编写 `plugins/rtk/hooks_test.go` 单元测试：`TestPreLLMHook_SkipsInjectWhenInjectFetchToolFalse` — `config.InjectFetchTool=false`，断言 `req.ChatRequest.Params.Tools` 不变
- [ ] 11.10 更新 `plugins/rtk/hint_test.go` 单元测试：扩展 `TestRecoveryHint_ContainsRecoveryEndpoint` 增加断言 — hint 文本含 `bifrostInternal-rtk_fetch_raw_output` 完整 tool name + 描述 tool 调用方式（V-plugins-4）
- [ ] 11.11 编写 `plugins/rtk/rawoutput_test.go` 单元测试：`TestRawOutputReadHandler_BytesEqualHTTPEndpoint` — 同一 id，分别调用 handler 和模拟 `handlers/rtk.go:386` 的 `WrapRawOutputForHTTP`，断言两次输出字节级相等（V-plugins-7）

## 12. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 12.1 新建 `plugins/rtk/tool_schema.go`，定义 `rtkFetchRawOutputToolName` 常量、`rtkFetchRawOutputToolDescription` 字符串拼接、`RtkFetchRawOutputTool schemas.ChatTool` 变量（按 design.md §"Tool Schema" 完整实现）
- [x] 12.2 在 `plugins/rtk/rawoutput.go` 末尾追加 `RawOutputReadHandler(ctx, args) (string, error)` Go 函数（按 design.md §"MCP Tool 形态"）
- [x] 12.3 在 `plugins/rtk/config.go` 的 `Config` struct 增 `InjectFetchTool *bool` 字段（JSON tag `inject_fetch_tool,omitempty`）
- [x] 12.4 在 `plugins/rtk/config.go` 的 `applyConfigDefaults` 函数末尾追加：`if c.InjectFetchTool == nil { t := true; c.InjectFetchTool = &t }`
- [x] 12.5 在 `plugins/rtk/rtk.go` 的 `Plugin` struct 增 `bifrost *schemas.Bifrost` 字段（持有引用，用于 GetMCPManager）
- [x] 12.6 在 `plugins/rtk/rtk.go` 的 `NewPlugin` 函数签名追加 `bifrost *schemas.Bifrost` 参数；赋值给 `p.bifrost`；构造时检测 `config.Enabled && IsTruePtr(config.InjectFetchTool)` → 调 `bifrost.GetMCPManager().RegisterTool(...)` 注册
- [x] 12.7 在 `plugins/rtk/rtk.go` 新增 `rawOutputReadHandlerMCPTool(args any) (string, error)` 适配器（`MCPToolFunction[any]` 形态）
- [x] 12.8 在 `plugins/rtk/hooks.go` 的 `PreLLMHook` 末尾（既有 hint 注入后、return 前）追加：检测 `p.config.Enabled && IsTruePtr(p.config.InjectFetchTool) && p.bifrost != nil` → 调 `injectFetchToolSchemaIfAvailable(req)` 函数
- [x] 12.9 在 `plugins/rtk/hooks.go` 新增 `MCPManagerLike` interface（`GetToolPerClient(ctx) map[string][]schemas.ChatTool`）和 `mcpManagerHasFetchTool(mgr) bool` / `injectFetchToolSchemaIfAvailable(req) bool` / `hasTool(tools, name) bool` 私有函数
- [x] 12.10 改写 `plugins/rtk/hint.go` 的 `rtkRecoveryHintText` 常量（按 design.md §"Hint 文本改写"完整内容）
- [x] 12.11 跑 `cd plugins/rtk && go build ./...` 确认编译通过
- [x] 12.12 跑 `cd plugins/rtk && go test ./...` 确认所有 RTK 单测绿（含 §11 新增）

## 13. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 13.1 review agent 读 design.md + tasks.md
- [x] 13.2 review agent 对 git diff 做静态审查
- [x] 13.3 review agent 输出 review_score + p0_failures
- [x] 13.4 score < pass_threshold → escalate；≥ pass_threshold → completed

> **本次变更 review 关注点**：
> - `RtkFetchRawOutputTool` 的 function.name 是否是 byte-stable 常量（不能放在 init 函数中拼装）
> - handler 错误信息是否含 sensitive info（如文件路径泄漏）
> - PreLLMHook 注入分支是否有 race（plugin concurrent PreLLMHook + 启动期 RegisterTool）
> - hint 文本 byte 数变化后是否仍 byte-stable（应该是 — 一次性 const）
> - `IsTruePtr` 工具函数命名（参照 `BoolPtr` 已有的命名习惯）

## 14. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 14.1 执行 `cd plugins/rtk && go test ./... -race`（含 race detector）
- [x] 14.2 执行 `cd plugins/rtk && go test -run TestRawOutputReadHandler -v` 单点 verify
- [x] 14.3 启动 gateway（按 .pg/context/agent-protocol.md §2 走 hooks 协议）
- [x] 14.4 验证 V-plugins-1（来自 design.md Verification Criteria）
- [x] 14.5 验证 V-plugins-2（来自 design.md Verification Criteria）
- [x] 14.6 验证 V-plugins-3（来自 design.md Verification Criteria）
- [x] 14.7 验证 V-plugins-4（来自 design.md Verification Criteria）
- [x] 14.8 验证 V-plugins-7（来自 design.md Verification Criteria）
- [x] 14.9 验证 V-plugins-N：来自 design.md Verification Criteria（N 由 design.md 决定）

  **Evidence 要求**：
  - 单元测试日志
  - 启动日志含 "registered rtk_fetch_raw_output"（成功 case）
  - 关闭 `inject_fetch_tool=false` 后启动日志不含该 message

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

- [ ] 16.1 跑 `pg-scenario-execute` agent，按 `scenario-scr.yaml` 定义的 4 个 scenario 顺序执行
- [ ] 16.2 收集每个 scenario 的 evidence（curl 响应、log 片段、sentinel bypass 日志）

  **Scenarios**（详细见 `scenario-scr.yaml`）：
  - `S-rtk-fetch-tool-tool-call-recovery`：happy path，LLM 调 tool_call → agent loop 执行 → 原文透传
  - `S-rtk-fetch-tool-invalid-id`：handler 返回 error → LLM 收到 tool_result 含 error
  - `S-rtk-fetch-tool-opt-out`：`inject_fetch_tool=false` → 工具不注入，hint 文本降级到 GET URL

  **Evidence 要求**：
  - 每个 scenario 的 HTTP 响应体 / tool_result 内容
  - RTK plugin 日志含 `bifrostInternal-rtk_fetch_raw_output` 注册记录
  - Sentinel bypass 日志（`rtk-raw-output-bypass` technique）

- [ ] 16.3 同步插入 `tests/e2e/api/collections/provider-harness.json`（参考 `.claude/skills/harness-test-writer/SKILL.md`）：新增 `S-rtk-fetch-tool-harness` 用例，验证 chat request → LLM 调 tool_call → agent loop 自动执行 → tool_result 含 sentinel + 原文 → 下轮 chat 走 sentinel bypass → LLM 拿到完整原文
- [ ] 16.4 跑 `node tests/e2e/api/runners/augment-provider-harness.mjs` 验证 harness collection 结构合法

## 17. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final-gate (常驻, 无 on_conditions)
-->

- [ ] 17.1 gate agent 收齐 V-plugins-1 ~ 7 全部 evidence
- [ ] 17.2 gate agent 比对 design.md / proposal.md，确认所有"包含"项都有对应实现+测试
- [ ] 17.3 gate agent 给出 ship / hold 决议

> **V-* 验收表**：
> - V-plugins-1 → §11.1-11.4 + §14.2
> - V-plugins-2 → §11.5-11.6
> - V-plugins-3 → §11.7-11.9
> - V-plugins-4 → §11.10
> - V-plugins-5 → §16.3-16.4
> - V-core-1 → §1.1 + §2.1 + §4.3
> - V-plugins-7 → §11.11 + §14.4