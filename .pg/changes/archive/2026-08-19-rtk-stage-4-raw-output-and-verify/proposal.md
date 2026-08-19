# rtk-stage-4-raw-output-and-verify

**关联 issue**：无
**变更类型**：feature

## 背景

oc2-gateway 的 RTK 插件 (plugins/rtk/) 在阶段一/二/三已经落地核心压缩管线
（文档保护 + 截断/去重标记 + 模糊分组 + assistant 消息压缩 + 双格式 + 三级 +
信任模型），但运维/调试面仍有三个明显缺口：

1. **原始 tool output 无回查路径**——当压缩管线把原始输出截短/丢弃后，调试
   失败场景时无法回查"压缩前实际是什么"。OmniRoute 的 RTK 引擎通过 raw
   output 保留机制（`maybePersistRtkRawOutput`）解决了这个问题，oc2-gateway
   完全缺失。
2. **52 个 builtin filter + 用户自定义 filter 无内联测试机制**——任何对
   `lineFilter`/`deduplicator`/`smartTruncate` 的改动都会在 CI 阶段被现有
   集成测试覆盖，但**无法定位到具体哪个 filter 的行为变了**。OmniRoute 的
   `runRtkFilterTests` 机制把每个 filter 的 `tests` 字段作为内联用例，
   verify 跑通即知道哪个 filter 出问题。
3. **secret 泄漏风险**——当 secret（OpenAI key、Slack token、AWS key、
   Authorization header、key=value 凭据）落在 tool output 并被压缩管线保留
   在日志/缓存时，无脱敏机制会导致凭据泄漏。OmniRoute 的 `redactRtkRawOutput`
   用 5 条正则做了覆盖。

## 目标

把 RTK 插件从"压缩管线"升级为"压缩管线 + 运维/调试面"，与 OmniRoute RTK
引擎对齐到阶段四：

- **可观测性**：实际发生压缩的 tool output 留底到 `<appDir>/rtk/raw-output/`，
  retention 策略可配置（never / failures / always），sidecar `.meta.json`
  写明 command/timestamp/failure/redacted/bytes。
- **可验证性**：52 个 builtin filter 全部带内联 tests，`runRtkFilterTests`
  在 CI 阶段跑通后输出 passed/outcomes/benchmark/filtersWithoutTests 报告。
- **可安全**：落盘前对原始输出做 5 类密钥脱敏（OpenAI/Slack/AWS/
  Authorization/credential field），`redacted` 标志位记入 sidecar。

## 范围

### 包含

- `plugins/rtk/rawoutput.go`（新文件，~250 行）：
  - `RtkRawOutputRetention` 枚举：`never`/`failures`/`always`
  - `RtkRawOutputPointer` 结构：`{ID, Path, Bytes, SHA256, Redacted}`
  - `redactRtkRawOutput(text)` → `{Text, Redacted}`：5 条正则替换
  - `isLikelyFailureOutput(text)` → `bool`：9 关键词正则
  - `MaybePersistRtkRawOutput(raw, opts)` → `*RtkRawOutputPointer`：落盘
    + sidecar，best-effort 磁盘错误降级
  - `ReadRtkRawOutput(id)` → `string`：阶段五 learn/discover 用，预先暴露
- `plugins/rtk/verify.go`（新文件，~150 行）：
  - `FilterTest`/`FilterTestOutcome`/`FilterBenchmarkRow`/`VerifyResult` 类型
  - `RunRtkFilterTests(opts?)` `VerifyResult`：遍历 `loader.cachedFilters`，
    跑 `applyLineFilter` 比对 `tests[].expected`，按 category 聚合 benchmark
- `Filter` struct 新增 `Tests []FilterTest` 顶层字段（兼容 legacy + canonical
  双格式 JSON 解析）
- `Config` 新增 2 字段：
  - `RawOutputRetention string`（枚举校验，默认 `never`）
  - `RawOutputMaxBytes int`（最小 1024，默认 1048576）
- `compression.go` 主流程接入 raw output 保留：`processRtkTextWithCommand`
  在 `CompressedTokens < OriginalTokens` 时（D1 决策：严格对齐 OmniRoute，
  不用 oc2-gateway 现有 5% 阈值）调 `MaybePersistRtkRawOutput`
- `CompressionState` 新增 `RawOutputPointers []*RtkRawOutputPointer` 字段
- 52 个 builtin filter JSON 手工补充 `tests` 字段（每个至少 1 个最小用例）
- `plugins/rtk/rawoutput_test.go`（~300 行）+ `plugins/rtk/verify_test.go`
  （~200 行）+ `plugins/rtk/rtk_test.go` 补充 raw output 集成场景
- `transports/config.schema.json` rtk 配置块同步新增 2 字段

### 不包含

- `listRtkCommandSamples` 函数（阶段五 learn/discover 用，仅 `ReadRtkRawOutput`
  暴露即可）
- 新增 env var `RTK_RAW_OUTPUT_DIR`（D2 决策：用 `appDir` 作为根目录，与
  现有 filterloader 全局路径同根）
- HTTP admin 端点 `/admin/rtk/verify`（D3 决策：纯函数 + Go test 入口）
- PostLLMHook 异步跑 verify（性能抖动风险 + 与 OmniRoute 偏离）
- CLI 入口（与 D3 设计原则不符）
- 压缩主流程的 5% 阈值改造（D1 已决策严格对齐 OmniRoute，不沿用现有阈值；
  行为变更不在本阶段范围）
- 阶段五 `discover.go`/`learn.go` 自学习
- 阶段六 跨引擎堆叠 / 主动触发 / UI

## 方案概述

按 OmniRoute rawOutput.ts + verify.ts 的 TS 实现 1:1 移植到 Go，关键差异：

| 项 | OmniRoute (TS) | oc2-gateway (Go) |
|---|---|---|
| DATA_DIR | `process.env.DATA_DIR \|\| ~/.omniroute` | `appDir`（transports/server/plugins.go:122 `os.Getwd()`） |
| 落盘根目录 | `<DATA_DIR>/rtk/raw-output/` | `<appDir>/rtk/raw-output/`（与 filterloader 同根） |
| 触发条件 | `compressedTokens < originalTokens` | 同上（D1 决策严格对齐） |
| 文件名模板 | `<ts_ms>-<slug>-<id24>.log` + `<same>.meta.json` | 同上 |
| 5 条脱敏正则 | RegExp (5 条) | regexp.Regexp (5 条预编译) |
| verify 入口 | CLI / 测试入口 | Go test（不挂 PostLLMHook） |
| builtin tests | 每个 builtin JSON 含 tests | 手工补齐 52 个 |

**关键技术决策**：

- D1：触发条件严格对齐 OmniRoute（`compressed < original`），不用 oc2-gateway
  现有 5% 阈值
- D2：落盘根目录用 `appDir`（与现有 filterloader 全局路径对称）
- D3：verify 是纯函数，仅 Go test 入口，不暴露 HTTP / CLI
- D4/D5：52 个 builtin tests 手工设计（D5 决策）
- D6：`tests` 字段加在 `Filter` struct 顶层（双格式 JSON 自动接管）

**降级策略**：
- 磁盘错误（ENOSPC/EACCES）→ `MaybePersistRtkRawOutput` 返回 `nil`，不抛
  error/panic，调用方视为落盘失败，不影响 result/stats
- sidecar `.meta.json` 写失败 → 主 `.log` 已写入的 pointer 仍返回（best-
  effort）
- retention=`never` → 完全跳过落盘，保持现有零副作用行为

## 风险和注意事项

1. **手工补齐 52 个 builtin tests 与 oc2-gateway 行为一致性的回归风险**
   （D4/D5 决策）—— oc2-gateway 的 `lineFilter`/`deduplicator`/
   `smartTruncate` 与 OmniRoute 的实现可能有细微差异（如 head/tail 边界、
   collapsePattern 顺序、truncate 标记行位置），手工写的 `expected` 可能
   不匹配 → V-plugins-7 短期不通过，需 build 阶段循环修复。设计 mitigation：
   `RunRtkFilterTests` 跑通后 `filtersWithoutTests=[]` 但 `passed=false`
   的 filter 列表会作为修复 backlog 输出。
2. **D1 阈值变更的全局行为影响**——把"5% 阈值"换成"压缩即触发"后，原本
   0-5% 压缩率的 filter 现在会拿到 `Compressed=true` 标记，stats.Techniques
   也会变长。Mitigation：在 `rtk_test.go` 增加对比测试，验证现有 5% 阈值
   场景不被破坏（如 stats.Techniques 仍正确标记）。
3. **落盘目录无清理机制**——`<appDir>/rtk/raw-output/` 持续增长，进程
   崩溃或磁盘满时旧文件不会自动清理。阶段六前不会引入 LRU/TTL，用户需
   自行清理或挂载 tmpfs/logrotate。Mitigation：sidecar 写磁盘字节数
   （`bytes`）便于运维做容量监控。
4. **5 条脱敏正则覆盖不完整**——base64 JWT、自定义 vendor key 等形态无法
   覆盖，仅保证 OpenAI/Slack/AWS + 标准凭据字段名 + 标准 Authorization
   header。用户自定义 filter 不走 redact 路径，自行负责。
5. **compression.go 改造面较大**——`processRtkTextWithCommand` 是 hot path，
   新增 raw output 落盘调用会增加每次实际压缩的延迟（写磁盘 + 计算 sha256
   + 路径生成）。Mitigation：retention=`never`（默认值）时完全跳过，
   retention=`failures`/`always` 才付出成本。

**约束映射**：上述风险 1 对应 V-plugins-7（builtin tests 完整性），
风险 2 对应 V-plugins-9（主流程接入触发条件），风险 3-5 是已知运维约束
（无对应 V-*，由 operations 文档/告警覆盖）。
