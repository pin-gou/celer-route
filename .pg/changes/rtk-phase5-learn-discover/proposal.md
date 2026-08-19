# rtk-phase5-learn-discover
**关联 issue**：无
**变更类型**：feature

## 背景

`plugins/rtk/` 已实现 RTK 核心压缩管线（ANSI 剥离 → 命令检测 → 过滤器匹配 → 行过滤 → 去重 → 智能截断 → 字符硬限），并支持 OpenAI Chat（`role=tool`）/ Anthropic（`tool_result` block）/ Responses API（`function_call_output`）三路压缩。当前 52 个内置 JSON 过滤器通过 embed 注入，零外部维护。

参考仓库 `../OmniRoute` 的 RTK 引擎除了上述核心管线外，还提供了「Learn / Discover 过滤器自学习」能力——`discover.ts` 从历史 `CommandSample` 集合中自动发现跨样本重复出现的归一化噪音模板，`learn.ts` 根据样本集自动建议一份 `SuggestedFilter`（含 dropPatterns / errorPatterns / summaryPatterns）。两者结合能显著降低用户手工维护 `.rtk/filters.json` 的成本，并跟上实际项目命令输出演化。

oc2-gateway 的 RTK 插件目前缺失这两个能力。用户只能手工编写过滤器，无法利用历史样本反哺。

## 目标

在 `plugins/rtk/` 下新增两个纯函数文件，对齐 OmniRoute `discover.ts` / `learn.ts` 的语义与算法：

1. `discover.go` —— 实现 `DiscoverNormalizeLine` / `DiscoverRepeatedNoise`，导出 `CommandSample` / `NoiseCandidate` 类型
2. `learn.go` —— 实现 `SuggestFilter` / `CommandToId`，导出 `SuggestedFilter` 类型（canonical JSON 形态）
3. `discover_test.go` / `learn_test.go` —— 表驱动单测覆盖归一化、阈值过滤、冲突守卫等关键路径

阶段五不暴露 CLI/HTTP 入口，不接入 hooks，不读 raw-output 目录，不写 canonical → legacy 适配层——纯算法交付。运行时消费与运维面（CLI/HTTP/UI）由后续阶段承接。

## 范围

### 包含

- 新建 `plugins/rtk/discover.go`（约 150 行）：纯算法，无 I/O
- 新建 `plugins/rtk/learn.go`（约 200 行）：纯算法
- 新建 `plugins/rtk/discover_test.go`（约 200 行）
- 新建 `plugins/rtk/learn_test.go`（约 300 行）
- 复用 `plugins/rtk/grouper.go` 的 `normalizeLine`（不修改原函数）
- 输出 canonical JSON 形态的 `SuggestedFilter`（与 OmniRoute 一致）

### 不包含

- `SuggestedFilter → Filter.Rules[]` 的 legacy 适配层（运行时消费问题，留待后续阶段）
- CLI / HTTP 入口暴露 learn/discover（后续阶段）
- 自动从 `plugins/rtk/rawoutput.go` 的 raw-output 目录构造 samples（样本 I/O 由调用方提供）
- UI 配置面板（阶段六）
- 引擎堆叠 / 主动触发（阶段六）
- `plugins/rtk/config.go` 新增字段（learn/discover 是离线工具，无运行时开关需求）
- 修改 `grouper.normalizeLine` 既有实现与既有 27 个 RTK 测试用例
- 修改 `applyLineFilter` / `Filter.UnmarshalJSON`（结构性问题，超出阶段五范围）

## 方案概述

### discover.go

复用 `grouper.normalizeLine` 的 7 步归一化（ISO 时间戳 → 括号时间 → Hex → 语义版本 → 独立整数 → 折叠空白 → trim），在此基础上追加 3 步：

1. `npm install left-pad@1.2.3` / `pip install requests==1.0` → `<PKG>@<N>`
2. `Error: E404` / `code: ENOENT` → `<CODE>`
3. `5s` / `120ms` / `4kb` → `<N>`

`DiscoverRepeatedNoise(samples []CommandSample)` 跨样本聚合归一化模板，过滤单样本命中，通过 `normalizedToPattern` 把 `<N>`/`<PKG>`/`<CODE>` 转 regex 安全字符串（转义特殊字符 + 加 `^` 前缀锚定），按 `hits desc + pattern asc` 排序返回 `[]NoiseCandidate`。

### learn.go

定义 `SuggestedFilter`（canonical JSON 形态，含 match/rules/preserve/_meta 块）。`SuggestFilter(command string, samples []CommandSample)` 算法：

1. 对样本每行原始文本（非归一化）匹配 `ERROR_PATTERN`（`ERR!` / `error[:/]` / `failed` / `exception` / `traceback` / `panic` / `fatal` / `critical`）→ 收集 `preservedRawLines` 与 `errorNorms`
2. 对样本每行原始文本匹配 `SUMMARY_PATTERN`（`success` / `done` / `complete` / `built` / `installed`）→ 收集 `summaryNorms`
3. 对每个归一化模板，统计其跨样本出现率：≥ `DROP_THRESHOLD_RATIO = 0.5` 才进入候选
4. **冲突守卫**：drop candidate 若匹配任一 `preservedRawLines` 行 → 跳过
5. 组装 `SuggestedFilter`：`id = "suggested-" + CommandToId(command)`，`priority = 50`，`category = "generic"`，`match.commands = ["^" + regexp.QuoteMeta(command) + "\\b"]`

`CommandToId(command)`：`strings.TrimSpace` → `strings.ToLower` → 非 `[a-z0-9]` 字符替换为 `-` → 去首尾 `-`。例：`"npm install"` → `"npm-install"`。

### discover_test.go / learn_test.go

沿用 `grouper_test.go` 的表驱动风格（`testing.T` + `t.Run()` + 子测试 + `t.TempDir()`）。覆盖：

- `TestDiscoverNormalizeLine`：时间戳变体、hex 变体、版本号、整数、`<PKG>`、`<CODE>`、时间单位、空白折叠
- `TestDiscoverRepeatedNoise`：单样本过滤、hits 聚合、pattern 排序、转义
- `TestSuggestFilter`：命令 slug、错误/摘要识别、阈值、空样本骨架、冲突守卫
- `TestCommandToId`：trim / lower / 字符替换 / 首尾去 `-`
- `TestNoRegression`：跑既有 27 个 RTK 测试用例，零失败

## 风险和注意事项

1. **Go `(?i)` 内联标志**：OmniRoute `ERROR_PATTERN` 用 `(?i)` 内联大小写不敏感。Go 1.21+ 标准库 `regexp.MustCompile` 支持 `(?i)`，但要确保编译选项位置正确（`(?i)` 必须在模式最前段，否则范围错位）。Go 版本需 ≥ 1.21。
2. **`<PKG>` 包名字符集范围**：OmniRoute 同时支持 `left-pad` 和 `@scope/name`，阶段五先实现前者，scoped package 是否纳入留待 dev 阶段拍板（详见 `define-summary.yaml` 的 `open_questions`）
3. **`SuggestedFilter` 运行时消费问题**：阶段五仅保证算法正确性。oc2-gateway 现有 `applyLineFilter` 只读 `Filter.Rules[]`，canonical 扁平字段（`dropPatterns` / `keepPatterns` 等）被 struct 接收但运行时不消费。运行时消费属于后续阶段（CLI/HTTP/UI）的工作
4. **`DROP_THRESHOLD_RATIO = 0.5` 的样本量依赖**：当 samples < 2 时，所有 drop candidate 因只跨 1 个样本被过滤，返回空骨架。这是 OmniRoute 既有行为，阶段五沿用，不做特殊处理
5. **冲突守卫保守策略**：OmniRoute 用「drop candidate 匹配任一 preserved 行则跳过」——可能产生空 `dropPatterns`。这是设计决策，阶段五保持一致
6. **既有 27 个 RTK 测试用例零回归**：discover.go / learn.go 不修改 `grouper.normalizeLine`、`applyLineFilter`、`Filter.UnmarshalJSON`、`config.go` 字段；新增仅是独立文件 + 表驱动测试
7. **未做的验证项**（来自 `define-summary.yaml` 的 V-*，全部 `skipped` 状态）：
   - **V-plugins-1**：discoverNormalizeLine 归一化与复用契约
   - **V-plugins-2**：discoverRepeatedNoise 聚合与过滤
   - **V-plugins-3**：suggestFilter 命令→过滤器骨架
   - **V-plugins-4**：suggestFilter 错误/摘要识别 + 冲突守卫
   - **V-plugins-5**：commandToId slug 算法
   - **V-plugins-6**：既有 RTK 测试不回归

   全部 V-* 标 `skipped` 是因为 target environment（local）的 capabilities（rest_api_endpoint / sqlite_logs_db / vite_dev_server 等）与 RTK 算法函数验证不相关，验证通过 `cd plugins/rtk && go test -short -count=1` 完成。这些 V-* 在 proposal.md 「未做」段之外另有遗漏需另列，详见 `define-summary.yaml`。