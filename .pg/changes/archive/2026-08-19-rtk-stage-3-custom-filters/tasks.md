> - **environment 选择**：dev → local，int → local

## 1. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写 Filter 双格式解析测试（对应 V-plugins-1）：在 `plugins/rtk/filterloader_test.go` 新增 `TestUnmarshalDualFormat`，构造 legacy + canonical 各 1 个 JSON 文件（t.TempDir + os.WriteFile），断言 `Filter` struct 字段填充正确；同时新增 `TestBuiltinFiltersUnchanged` 跑 53 个内置 embed 文件验证零迁移。
- [ ] 1.2 编写三级优先级 + 白/黑名单测试（对应 V-plugins-2）：新增 `TestLoaderPriority` + `TestLoaderEnabledDisabled`，t.TempDir 造 project/global/builtin 三级 fixture，断言 sourceRank+formatRank+priority+id 排序；Config 设 EnabledFilters + DisabledFilters 断言过滤生效。
- [ ] 1.3 编写 trust.json 4 场景 + env var 旁路测试（对应 V-plugins-3）：新增 `TestTrustJSON4Scenarios` + `TestEnvVarBypass`，t.TempDir 造 trust.json 4 种状态（pass/fail/missing/legacy_field）+ `os.Setenv("OMNIROUTE_RTK_TRUST_PROJECT_FILTERS","1")` 旁路测试；断言 loader.Diagnostics() 包含对应 WARN/INFO。
- [ ] 1.4 编写 Plugin 持有 Loader 回归测试（修隐式 bug 验证）：新增 `TestPluginInitHoldsLoader`，构造 `Plugin` 通过 `Init(ctx, config, logger, appDir)`，断言 `p.loader != nil` 且 `p.loader.Match("shell","git status")` 返回 builtin filter；构造不同 config 调用两次 Init 验证 config 字段真正生效。
- [ ] 1.5 编写 Diagnostics 接口测试：新增 `TestLoaderDiagnostics`，故意构造 ReDoS-prone JSON 触发校验失败，断言 `loader.Diagnostics()` 返回结构化记录（Source/Format/Path/Level/Message）。
- [ ] 1.6 跑通现有 27+ 测试不回归：`cd plugins/rtk && go test ./... -count=1`，特别是 `filterloader_test.go` 现有 `TestFilterLoaderBuiltin` / `TestFilterLoaderMatch` / `TestFilterLoaderPriority` / `TestFilterLoaderReDoSProtection` / `TestFilterLoaderReDoSRejectsBadFilter` 等不失败。

## 2. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 2.1 重写 `plugins/rtk/linefilter.go` 的 `Filter` struct：扩展为兼容 legacy 7 字段 + canonical 27 字段的胖 struct（id/label/category/priority/commandPatterns/matchPatterns/outputTypes/stripPatterns/keepPatterns/collapsePatterns/stripAnsi/replace/matchOutput/truncateLineAt/onEmpty/filterStderr/deduplicate/head_lines/tail_lines/maxLines/errorPatterns/summaryPatterns/tests），所有字段带 `omitempty` JSON tag；新增 `ReplaceRule` / `MatchOutputRule` / `FilterTest` struct。
- [ ] 2.2 在 `Filter` 上实现 `UnmarshalJSON(data []byte) error`：先 `json.Unmarshal` 进 alias struct（防递归），然后仲裁 head/tail/max_lines（canonical head_lines > 0 时覆盖 head，反之亦然）+ ID/Name（双向 fallback）。
- [ ] 2.3 重写 `plugins/rtk/filterloader.go`：保留现有 `FilterLoader` struct 字段，新增 `cachedFilters []*Filter` / `diagnostics []FilterLoadDiagnostic` / `appDir string` / `config *Config`；新增 `Load(appDir string) error` 方法实现三级 source 收集 + 双格式解析 + 排序 + 缓存；新增 `Diagnostics() []FilterLoadDiagnostic` 公开接口。
- [ ] 2.4 在 `filterloader.go` 新增 `projectFiltersTrusted(filtersPath string, trustProjectFilters bool) (bool, string)`：返回 `(trusted bool, reason string)`，env var 双名（OMNIROUTE_RTK_TRUST_PROJECT_FILTERS / BIFROST_RTK_TRUST_PROJECT_FILTERS）任一为 1 → trusted=true + reason="env bypass"；否则读 trust.json 校验 SHA256（filtersSha256 优先，trustedFiltersSha256 兜底兼容）。
- [ ] 2.5 新增 `plugins/rtk/diagnostics.go`：定义 `FilterLoadDiagnostic` struct（Source/Format/Path/Level/Message），提供 `(*FilterLoader).Diagnostics()` 方法。
- [ ] 2.6 修改 `plugins/rtk/config.go`：新增 `CustomFiltersEnabled bool` / `TrustProjectFilters bool` / `EnabledFilters []string` / `DisabledFilters []string` 4 字段；扩展 `Validate()` 接受新字段（仅校验类型，不强制非空）。
- [ ] 2.7 修改 `plugins/rtk/rtk.go`：`Init(ctx, config, logger, appDir string) (*Plugin, error)` 多收 appDir；`Plugin` struct 新增 `loader *FilterLoader` 字段；Init 末尾调 `loader.Load(appDir)`，失败仅 WARN 不中断（保持现有 fail-open 策略）。
- [ ] 2.8 修改 `plugins/rtk/compression.go`：删除 `globalLoader` / `loaderOnce` / `defaultConfig` / `getFilterLoader()` 全部包级变量与函数；改 `applyRtkCompression` 与 `applyRtkCompressionResponses` 接收 `*Plugin` 参数（签名变更），内部通过 `plugin.loader.Match(...)` 取 filter。
- [ ] 2.9 修改 `plugins/rtk/hooks.go`：`PreLLMHook` / `PostLLMHook` 调 `applyRtkCompression(req, p)` 与 `applyRtkCompressionResponses(req, p)`，透传 `p` 实例。
- [ ] 2.10 修改 `transports/config.schema.json` rtk 配置块（行 3127-3208）：在 `additionalProperties: false` 下新增 4 字段定义（custom_filters_enabled / trust_project_filters / enabled_filters / disabled_filters），每个含 description + type + default（如适用）。
- [ ] 2.11 编写 diagnostics_test.go 单元测试（与 1.5 配对）；扩展 `config_test.go` 覆盖 4 个新字段的 Validate 行为（空值/非空值/互斥场景）。
- [ ] 2.12 自检：阶段一/二测试不回归（`TestApplyRtkCompression` / `TestCacheControlPreservation` / `TestGrouperNormalizeLine` / `TestSmartTruncate` / `TestDeduplicator` 等 27+ 测试全部 PASS）。

## 3. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/rtk-stage-3-custom-filters 做静态审查
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
  - verifiable: V-plugins-1, V-plugins-2, V-plugins-3
  - degraded: V-plugins-4

  **4.4 子任务**：
  - 4.4.1 V-plugins-1：跑 `TestUnmarshalDualFormat` + `TestBuiltinFiltersUnchanged`，断言 PASS；Evidence = go test 输出片段。
  - 4.4.2 V-plugins-2：跑 `TestLoaderPriority` + `TestLoaderEnabledDisabled`，断言 PASS；Evidence = go test 输出片段。
  - 4.4.3 V-plugins-3：跑 `TestTrustJSON4Scenarios` + `TestEnvVarBypass`，断言 PASS；Evidence = go test 输出片段。
  - 4.4.4 V-plugins-4 (degraded)：豁免理由「阶段三纯单测覆盖；live E2E 留阶段六或独立 track」；记录到 verify report 的 SKIP 段。

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
