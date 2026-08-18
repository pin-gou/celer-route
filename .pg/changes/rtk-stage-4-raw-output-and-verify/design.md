# rtk-stage-4-raw-output-and-verify 设计

## 架构概览

本变更在 plugins/rtk/ 子模块下新增 2 个独立 Go 文件 + 改造 4 个现有文件，
全部在 `plugins` track 内完成，不涉及跨模块改动。

```
plugins/rtk/
├── rawoutput.go         [NEW]   ~250 行: RtkRawOutputRetention/RtkRawOutputPointer/redactRtkRawOutput/isLikelyFailureOutput/MaybePersistRtkRawOutput/ReadRtkRawOutput
├── verify.go            [NEW]   ~150 行: FilterTest/FilterTestOutcome/FilterBenchmarkRow/VerifyResult/RunRtkFilterTests/trimComparable
├── filterloader.go      [MOD]   +1 字段: Filter.Tests []FilterTest (双格式 JSON 自动接管)
├── config.go            [MOD]   +2 字段: RawOutputRetention/RawOutputMaxBytes + Validate 校验 + applyConfigDefaults 默认值
├── compression.go       [MOD]   processRtkTextWithCommand 在 stats.CompressedTokens < stats.OriginalTokens 时调 MaybePersistRtkRawOutput
├── state.go             [MOD]   CompressionState.RawOutputPointers []*RtkRawOutputPointer (累加点)
├── filters/builtin/     [MOD]   52 个 JSON 各加 "tests": [{name, input, expected}] 字段
├── rawoutput_test.go    [NEW]   ~300 行
├── verify_test.go       [NEW]   ~200 行
├── rtk_test.go          [MOD]   补充 raw output 集成场景
└── filterloader_test.go [MOD]   补充 Tests 字段解析测试
```

**数据流**：

```
PreLLMHook (compression.go)
  → processRtkTextWithCommand(text, config, loader, command)
    → stats.CompressedTokens < stats.OriginalTokens ?
        YES → MaybePersistRtkRawOutput(raw, {retention, command: cmd, maxBytes})
              → 写 <appDir>/rtk/raw-output/<ts>-<slug>-<id24>.log
              → 写 <same>.meta.json (best-effort)
              → 失败 → 返回 nil (不阻塞)
              → 成功 → pointer 累加进 ProcessStats.RawOutputPointers
        NO  → 跳过
    → pointer 进一步累加到 CompressionState.RawOutputPointers
  → return state
PostLLMHook (hooks.go)
  → state.RawOutputPointers 可被读 (本阶段不消费, 留作日志/UI)

Verify (verify.go, Go test 入口)
  → RunRtkFilterTests()
    → loader.cachedFilters 遍历
      → filter.tests 逐条跑 applyLineFilter(test.input, filter)
      → 与 trimComparable(test.expected) 比对
      → 按 category 聚合 benchmark
    → return VerifyResult{Passed, Outcomes, FiltersWithoutTests, Benchmark, Diagnostics}
```

## API 设计

本变更**不引入 HTTP/REST API**（D3 决策：verify 是纯函数，仅 Go test 入口）。
新增的 Go 公共 API 如下：

### Raw Output API (rawoutput.go)

```go
// 保留策略枚举
type RtkRawOutputRetention string
const (
    RawOutputRetentionNever     RtkRawOutputRetention = "never"
    RawOutputRetentionFailures  RtkRawOutputRetention = "failures"
    RawOutputRetentionAlways    RtkRawOutputRetention = "always"
)

// 落盘结果指针
type RtkRawOutputPointer struct {
    ID       string  // sha256(`${now}:${slug}:${raw.length}:${redacted.text}`)[:24]
    Path     string  // <appDir>/rtk/raw-output/<ts_ms>-<slug>-<id24>.log
    Bytes    int     // utf8 字节数
    SHA256   string  // hex(sha256(redacted.text)) 完整 64 字符
    Redacted bool    // 任一脱敏正则是否命中
}

// 5 条脱敏正则 (ReDoS-safe)
func RedactRtkRawOutput(value string) (text string, redacted bool)

// 失败检测 (9 关键词)
func IsLikelyFailureOutput(value string) bool

// 落盘 (best-effort 磁盘错误降级)
func MaybePersistRtkRawOutput(raw string, opts PersistOptions) *RtkRawOutputPointer
type PersistOptions struct {
    Retention RtkRawOutputRetention  // never | failures | always
    Command   string                 // 用于生成 commandSlug
    MaxBytes  int                    // 默认 1048576, 最小 1024
    Failure   *bool                  // 显式 override isLikelyFailureOutput
}

// 读取已落盘的 raw output
func ReadRtkRawOutput(pointerID string) string
```

### Verify API (verify.go)

```go
// Filter 内联测试用例
type FilterTest struct {
    Name     string  `json:"name"`
    Command  string  `json:"command,omitempty"`  // 可选
    Input    string  `json:"input"`
    Expected string  `json:"expected"`
}

// Filter 顶层新增字段 (filterloader.go)
type Filter struct {
    // ... 现有 26 个 legacy + canonical 字段 ...
    Tests []FilterTest `json:"tests,omitempty"`  // [NEW] 兼容双格式
}

// 单条 test 结果
type FilterTestOutcome struct {
    FilterID string
    TestName string
    Passed   bool
    Actual   string
    Expected string
}

// benchmark 聚合行
type FilterBenchmarkRow struct {
    Category             string
    Filters              int
    Tests                int
    AverageSavingsPercent float64
}

// verify 返回值
type VerifyResult struct {
    Passed              bool
    Outcomes            []FilterTestOutcome
    FiltersWithoutTests []string
    Benchmark           []FilterBenchmarkRow
    Diagnostics         []FilterLoadDiagnostic
}

// 主入口
func RunRtkFilterTests(opts *VerifyOptions) VerifyResult
type VerifyOptions struct {
    RequireAll            bool  // 要求所有 filter 都含 tests
    CustomFiltersEnabled  bool  // 透传给 loader.Load
    TrustProjectFilters   bool  // 透传给 loader.Load
    AppDir                string  // [NEW] loader.Load 入口
}
```

### Config 新增字段 (config.go)

```go
type Config struct {
    // ... 现有 16 个字段 ...
    RawOutputRetention string `json:"raw_output_retention"`  // [NEW] never|failures|always, 默认 "never"
    RawOutputMaxBytes  int    `json:"raw_output_max_bytes"`   // [NEW] 默认 1048576, 最小 1024
}

// Validate 新增校验:
func (c *Config) Validate() error {
    // ... 现有校验 ...
    if c.RawOutputRetention != "" {
        switch c.RawOutputRetention {
        case "never", "failures", "always":
        default:
            return fmt.Errorf("rtk: invalid raw_output_retention %q: must be one of never, failures, always", c.RawOutputRetention)
        }
    }
    if c.RawOutputMaxBytes < 0 {
        return fmt.Errorf("rtk: raw_output_max_bytes must be >= 0, got %d", c.RawOutputMaxBytes)
    }
    if c.RawOutputMaxBytes > 0 && c.RawOutputMaxBytes < 1024 {
        return fmt.Errorf("rtk: raw_output_max_bytes must be >= 1024 when set, got %d", c.RawOutputMaxBytes)
    }
    return nil
}

// applyConfigDefaults 新增:
if c.RawOutputRetention == "" {
    c.RawOutputRetention = "never"
}
if c.RawOutputMaxBytes == 0 {
    c.RawOutputMaxBytes = 1048576
}
```

### CompressionState 新增字段 (state.go)

```go
type CompressionState struct {
    // ... 现有字段 ...
    RawOutputPointers []*RtkRawOutputPointer  // [NEW] 累加自各次压缩
}
```

## 数据模型

无数据库 schema 变更。本变更仅在 `plugins/rtk/` 子模块的 Go 代码 + JSON 配置内做
改动，**不涉及任何 SQL/DDL**。

## 关键约束与契约

### 前置条件

- Go 1.26.6+（与现有 go.work 一致）
- `appDir` 由 `transports/server/plugins.go:122` 注入为 `os.Getwd()`，本变更
  复用同一 `appDir` 作为 raw-output 根目录（D2 决策：与 filterloader
  同根）
- 阶段三产出的 `Filter.Tests` 字段已通过 UnmarshalJSON 仲裁兼容 legacy 与
  canonical 双格式 JSON，本变更**无需修改 filterloader.go 的解析逻辑**
- 阶段一/二/三产出的所有现有单元测试（27+ 个 filter loader 测试 +
  compression/hooks/smarttruncate/deduplicator/grouper 测试）必须继续
  通过（回归保护）

### 影响面

**改动文件清单**（按行数排序）：

| 文件 | 类型 | 预估行数 | 备注 |
|------|------|---------|------|
| `plugins/rtk/rawoutput.go` | NEW | ~250 | 6 个公开 API + 5 条正则预编译 |
| `plugins/rtk/rawoutput_test.go` | NEW | ~300 | 脱敏/落盘/sidecar/失败检测/磁盘错误降级 5 大类 |
| `plugins/rtk/filters/builtin/*.json` | MOD | +52-208 (52 × 1-4 行) | 每个加 tests 字段 |
| `plugins/rtk/verify.go` | NEW | ~150 | 4 类型 + 主入口 + trimComparable |
| `plugins/rtk/verify_test.go` | NEW | ~200 | passed/outcomes/benchmark/filtersWithoutTests |
| `plugins/rtk/config.go` | MOD | +20 | 2 字段 + Validate + applyConfigDefaults |
| `plugins/rtk/rtk_test.go` | MOD | +80 | raw output 集成场景 |
| `plugins/rtk/filterloader_test.go` | MOD | +20 | Tests 字段解析测试 |
| `plugins/rtk/compression.go` | MOD | +25 | 主流程接入 raw output |
| `plugins/rtk/filterloader.go` | MOD | +3 | Filter.Tests 字段 |
| `plugins/rtk/state.go` | MOD | +2 | CompressionState.RawOutputPointers |
| `plugins/rtk/hooks.go` | MOD | 0 | 本阶段不消费 (留作日志/UI) |
| `transports/config.schema.json` | MOD | +20 | rtk 配置块加 2 字段 |

**对外 API 破坏性**：无（Go 公共 API 仅新增，不修改/删除现有方法）

**性能影响**：retention=`never`（默认）时 hot path 无任何额外调用；
retention=`failures`/`always` 时每次实际压缩增加 1 次正则扫描（5 条预编译）
+ 1 次 SHA-256 + 1 次 `os.MkdirAll`（首次）+ 1 次 `os.WriteFile` +
1 次 sidecar `os.WriteFile`。在 SSD 上单次落盘 ~50µs，相对压缩管线
~100µs 量级可接受。

### 性能契约

- retention=`never` 必须 zero overhead（不能有 regex 编译开销，每次
  `MaybePersistRtkRawOutput` 调用前置 nil-check 后 return）
- 5 条脱敏正则**必须**在 init 时预编译（`var reSecretXXX = regexp.MustCompile`），
  避免 hot path 重编译
- sidecar 写失败**不能**阻塞主 `.log` 已成功的 pointer 返回
- main `.log` 写失败**不能**抛 panic/error，必须返回 `nil` 让调用方降级

### 错误码与编号段

不涉及 HTTP 错误码（无新 API）。Go 层错误仅在 Validate 阶段 fail-fast：
- `rtk: invalid raw_output_retention %q: must be one of never, failures, always`
- `rtk: raw_output_max_bytes must be >= 0, got %d`
- `rtk: raw_output_max_bytes must be >= 1024 when set, got %d`

### 环境限制与验证策略

> **SSOT**：本段从 `.pg/changes/rtk-stage-4-raw-output-and-verify/env-description.yaml`
> 派生。所有 V-* 在目标 env（local）均可验证（纯 Go 库代码 + t.TempDir()）。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 Raw Output 保留策略 never/failures/always 按配置生效 | ✅ | 单元测试 | n/a |
| V-plugins-2 5 条密钥脱敏正则正确替换为占位符 | ✅ | 单元测试 | n/a |
| V-plugins-3 isLikelyFailureOutput 9 关键词正确识别 | ✅ | 单元测试 | n/a |
| V-plugins-4 sidecar .meta.json 写入正确 | ✅ | 单元测试 | n/a |
| V-plugins-5 磁盘错误 (EACCES) best-effort 降级不阻塞压缩管线 | ✅ | 单元测试 (chmod 0 临时目录) | macOS root 用户下 EACCES 模拟失效 → 仅在 Linux CI 验证, macOS 标 degraded |
| V-plugins-6 runRtkFilterTests passed / outcomes / benchmark 正确 | ✅ | 单元测试 | n/a |
| V-plugins-7 52 个 builtin 都含 tests 后 filtersWithoutTests=[] | ✅ | 单元测试 | n/a |
| V-plugins-8 benchmark 按 category 聚合正确 | ✅ | 单元测试 | n/a |
| V-plugins-9 compression 主流程接入 raw output 保留 | ✅ | 单元测试 | n/a |
| V-plugins-10 RawOutputPointers 透传到 CompressionState | ✅ | 单元测试 | n/a |

**结论**：所有 V-* 在 local 环境均可用纯 Go test 验证，无需启动 pg-gateway-api
或 UI dev server。V-plugins-5 在 macOS root 下 EACCES 模拟失效属于次要风险，
build 阶段如发现 macOS runner 失败可降级为 Linux-only 验证（不改 design）。

### 可观测性

**关键日志点**：

| 级别 | 来源 | 字段 | 触发 |
|------|------|------|------|
| INFO | `rtk.go Init` | `total=NN (project=X global=Y builtin=Z), diagnostics=N` | 插件初始化（现有） |
| WARN | `rawoutput.go` | `rtk: raw-output persistence failed: %v` | `os.MkdirAll`/`os.WriteFile` 失败 |
| WARN | `rawoutput.go` | `rtk: raw-output sidecar write failed: %v` | sidecar `.meta.json` 写失败 |
| WARN | `rawoutput.go` | `rtk: raw-output truncated to %d bytes` | 超过 max_bytes 截断 |

**关键指标**：本阶段**不新增 metrics**（与 OmniRoute 对齐：raw output 是
诊断资产，不进入 Prometheus 指标）。PostLLMHook 未来如消费
`state.RawOutputPointers` 做日志落库，可读 pointer 数量。

**RequestId 追踪**：本变更不引入新 ctx key。raw output 落盘的文件名不含
RequestID（与 OmniRoute 一致，command+timestamp+sha256 即可定位）。

## Verification Criteria

### dev plugins Verification Criteria

> dev 阶段的 plugins track 包含本变更的全部功能验证。所有 V-plugins-N
> 编号已在 `0-define/define-summary.yaml` 锁定为 `verifiable` 状态。

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | Raw Output retention=never/failures/always 按配置生效 | t.TempDir() + fixture (command + raw 文本) | Go test `TestRetentionPolicies` 跑 3 个子场景；断言 retention=never → 0 个文件, failures → 仅失败输出写盘, always → 全写盘；文件名严格遵循 `<ts>-<slug>-<id24>.log` | 三种策略行为正确, 文件名格式一致 |
| V-plugins-2 | 5 条密钥脱敏正则正确替换为占位符 | fixture 含各类型密钥（sk-/xox[A-Z]-/AKIA/key=value/Authorization: Bearer|Basic） | Go test `TestRedactRtkRawOutput` 5 个子场景；断言 text 替换 + redacted=true | 5 条正则全命中, redacted=true |
| V-plugins-3 | isLikelyFailureOutput 9 关键词正确识别 | fixture 含 error/failed/exception/traceback/panic/fatal/critical/TS\d{4}/FAIL 各 1 + 正常输出 1 | Go test `TestIsLikelyFailureOutput`；断言失败关键词 → true, 正常输出 → false | 9 关键词全 true, 正常输出 false |
| V-plugins-4 | sidecar .meta.json 写入正确 | t.TempDir() + fixture | Go test `TestSidecarMetadata`；写盘后 read sidecar + JSON parse；断言 command/timestamp/failure/redacted/bytes 五字段 | 五字段全正确 |
| V-plugins-5 | 磁盘错误 (EACCES) best-effort 降级不阻塞 | t.TempDir() + `chmod 000` 触发 EACCES | Go test `TestDiskErrorGracefulDegradation`；断言 `MaybePersistRtkRawOutput` 返回 nil 且不 panic | 返回 nil, 无 panic, 调用方拿到 stats 不变 |
| V-plugins-6 | runRtkFilterTests passed / outcomes / benchmark 正确 | fixture 含 2 个 filter (带 tests) | Go test `TestRunRtkFilterTests`；断言 Passed/Outcomes/Benchmark 三段 | Passed 反映测试结果, benchmark 按 category 聚合 |
| V-plugins-7 | 52 个 builtin 都含 tests 后 filtersWithoutTests=[] | 无需 fixture（直接遍历 embed.FS） | Go test `TestBuiltinFiltersHaveTests`；遍历 `plugins/rtk/filters/builtin/*.json`；断言每个 JSON parse 成功且 `tests` 字段非空 | FiltersWithoutTests 长度为 0 |
| V-plugins-8 | benchmark 按 category 聚合正确 | fixture 含 2 个不同 category filter, 各 2 tests | Go test `TestBenchmarkAggregation`；断言 benchmark rows 按 category 升序, 同 category 下 filters/tests/averageSavingsPercent 数学正确 | 聚合数学正确 |
| V-plugins-9 | compression 主流程接入 raw output 保留 | fixture 含真实 tool output (command + 输出文本) + retention=always | Go test `TestCompressionTriggersRawOutput`；跑 `processRtkTextWithCommand`；断言 stats.RawOutputPointers 至少 1 个 pointer 且 ID 非空 | pointer 存在, ID 非空 |
| V-plugins-10 | RawOutputPointers 透传到 CompressionState | 同上 fixture + applyRtkCompression 全路径 | Go test `TestStateRawOutputPointersPropagation`；断言 state.RawOutputPointers 包含 ProcessStats 累加的指针 | state.RawOutputPointers 非空, ID 一致 |

### int scr Verification Criteria

scr track 未启用（见 §变更类型判定），不生成 V-*。

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 无 core/ 改动 |
| framework | ❌ | 无 framework/ 改动 |
| transports | ❌ | 仅 `transports/config.schema.json` 同步新增 2 字段（配置 schema 而非 transport 代码），归入 plugins track 的连带改动 |
| **plugins** | ✅ | `plugins/rtk/` 下新增 2 文件 + 改造 6 文件 + 改 52 个 builtin JSON；transport schema 同步改动 |
| ui | ❌ | 无 ui/ 改动 |
| **scr** (scenario) | ❌ | 本变更纯 Go 库代码 + Go test 验证，**无 HTTP API/UI 改动**，**无跨模块联调场景**，无需 scenario track 验证。V-plugins-1 到 V-plugins-10 全部由 plugins track 的单元测试覆盖（dev stage）。 |

**affected_tracks**：`plugins`

**scenario track 启用决策**：

- 跨 role 协作验证？**否**（纯单模块 Go 库代码）
- 新 API 端点？**否**（无 HTTP/REST 端点新增）
- 跨模块联调？**否**（compression/hooks/state 在同一 plugins/rtk 包内）

→ **scr track 禁用**（`scenario-scr=false`）

> scenario 阶段（2.6）将 no-op，不生成 `scenario-scr.yaml`，tasks.md / manifest
> 均不含 scr 章节。

**common decisions**（由 `pg-gen-tasks-skeleton.py` 常量块固化）：

- error_response_strategy: A（无新 HTTP API）
- auth_scope: project（无新 HTTP API）
- data_migration_strategy: C（无 schema 变更）
- transaction_boundary: A（无 service 层事务）
- frontend_interaction_style: A（无前端交互）
