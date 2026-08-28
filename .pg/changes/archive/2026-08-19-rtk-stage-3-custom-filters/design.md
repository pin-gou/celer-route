# rtk-stage-3-custom-filters 设计
## 架构概览

### 模块归属
- **Track**：`plugins` (`.pg/project.yaml` tracks.plugins, type=standard, modules=[plugins])
- **目标模块**：`plugins/rtk/`（独立 go module `github.com/pin-gou/celer-route/plugins/rtk`）
- **不涉及**：core / framework / transports / ui

### 数据流

```
<app-dir>/.rtk/filters.{json,toml}   ← project source (rank=3, format=rtk-toml-v1:2 / omniroute-json:1)
        │ (trust.json SHA256 校验)
        ▼
┌────────────────────────────────────────────────────────────────────┐
│  Plugin.Init(ctx, config, logger, appDir)                         │
│  ├─ validate config (Config.Validate)                              │
│  ├─ applyConfigDefaults                                            │
│  ├─ NewFilterLoader(config).Load(appDir) ─┐                        │
│  └─ p.loader = loader                    │                        │
│                                           ▼                        │
│  FilterLoader.Load:                                                 │
│   1. collectSources(appDir, config):                               │
│      - project  : <app-dir>/.rtk/filters.{json,toml} (trust)       │
│      - global   : <app-dir>/rtk/filters.{json,toml} (auto-trust)   │
│      - builtin  : embed.FS plugins/rtk/filters/builtin/*.json      │
│   2. parseFilterFile(source) → []*Filter (双格式 + UnmarshalJSON)  │
│   3. applyEnabledDisabled(filters, config) → 最终加载集              │
│   4. sort: sourceRank desc, formatRank desc, priority desc, id asc │
│   5. cache to loader.cachedFilters                                 │
└────────────────────────────────────────────────────────────────────┘
        │
        ▼ (PreLLMHook / PostLLMHook 调)
   p.loader.Match(commandType, command) → *Filter
        │
        ▼
   applyLineFilter(text, filter) → 现有逻辑（linefilter.go 不变）
```

### 与 OmniRoute 对齐度
| OmniRoute 文件 | oc2-gateway 文件 | 阶段三对齐 |
|---|---|---|
| `filterLoader.ts` (332 行) | `plugins/rtk/filterloader.go` 重写 | ✅ 三级 source + trust.json + 双格式 |
| `filterSchema.ts` (213 行) | `plugins/rtk/linefilter.go` 扩展 Filter struct | ✅ canonical 字段 + UnmarshalJSON |
| `tomlCompatibility.ts` (334 行) | TOML 识别 warn 跳过 | ❌ 阶段四补 |
| `commandDetector.ts` | `plugins/rtk/linedetector.go` | ✅ 阶段一已对齐 |
| `lineFilter.ts` | `plugins/rtk/linefilter.go` | ✅ 阶段一已对齐 |

## 数据模型（如有）

无 DB schema 变更。本变更仅修改 Go struct 与 JSON 加载逻辑，不引入新表 / 索引 / 字段。

新增 Config 字段（JSON 配置层，非数据库）：
```go
type Config struct {
    // ... 现有 11 字段 ...
    CustomFiltersEnabled bool     `json:"custom_filters_enabled"`  // 默认 true
    TrustProjectFilters  bool     `json:"trust_project_filters"`   // 默认 false
    EnabledFilters       []string `json:"enabled_filters"`         // 空=全部启用
    DisabledFilters      []string `json:"disabled_filters"`        // 空=无禁用
}
```

新增 Loader 内部状态：
```go
type FilterLoader struct {
    // ... 现有 fields ...
    cachedFilters []*Filter              // Load 后填充, Match 使用
    diagnostics   []FilterLoadDiagnostic // Load 期间收集 (warning/error)
    appDir        string                 // Init 注入
    config        *Config                // 持有用于 enabled/disabled 过滤
}

type FilterLoadDiagnostic struct {
    Source  string // "project" | "global" | "builtin"
    Format  string // "omniroute-json" | "rtk-toml-v1"
    Path    string
    Level   string // "warning" | "error"
    Message string
}
```

Filter struct 扩展（plugins/rtk/linefilter.go）：
```go
type Filter struct {
    // === Legacy 字段 (保留, 现有 53 个内置 JSON 零迁移) ===
    Name             string     `json:"name,omitempty"`
    Command          string     `json:"command,omitempty"`
    Rules            []LineRule `json:"rules,omitempty"`
    Head             int        `json:"head,omitempty"`
    Tail             int        `json:"tail,omitempty"`
    MaxLines         int        `json:"max_lines,omitempty"`
    PriorityPatterns []string   `json:"priority_patterns,omitempty"`

    // === Canonical 字段 (新增, 与 OmniRoute RtkFilterDefinition 对齐) ===
    ID          string   `json:"id,omitempty"`
    Label       string   `json:"label,omitempty"`
    Description string   `json:"description,omitempty"`
    Category    string   `json:"category,omitempty"` // git|test|build|shell|docker|package|infra|cloud|generic
    Priority    int      `json:"priority,omitempty"` // 0-100, 默认 50
    Tests       []FilterTest `json:"tests,omitempty"`

    // Canonical match 块
    CommandPatterns []string `json:"commandPatterns,omitempty"` // regex
    MatchPatterns   []string `json:"matchPatterns,omitempty"`   // content regex
    OutputTypes     []string `json:"outputTypes,omitempty"`     // shell|api|doc-read

    // Canonical rules 块
    StripPatterns    []string             `json:"stripPatterns,omitempty"`     // legacy alias of Rules[strip]
    KeepPatterns     []string             `json:"keepPatterns,omitempty"`      // legacy alias of Rules[keep]
    CollapsePatterns []string             `json:"collapsePatterns,omitempty"`  // legacy alias of Rules[collapse]
    StripAnsi        bool                 `json:"stripAnsi,omitempty"`
    Replace          []ReplaceRule        `json:"replace,omitempty"`
    MatchOutput      []MatchOutputRule    `json:"matchOutput,omitempty"`
    TruncateLineAt   int                  `json:"truncateLineAt,omitempty"`
    OnEmpty          string               `json:"onEmpty,omitempty"`
    FilterStderr     bool                 `json:"filterStderr,omitempty"`
    Deduplicate      bool                 `json:"deduplicate,omitempty"`
    HeadLines        int                  `json:"head_lines,omitempty"`  // 仲裁 head
    TailLines        int                  `json:"tail_lines,omitempty"`  // 仲裁 tail

    // Canonical preserve 块
    ErrorPatterns   []string `json:"errorPatterns,omitempty"`
    SummaryPatterns []string `json:"summaryPatterns,omitempty"`
}

type FilterTest struct {
    Name     string `json:"name"`
    Input    string `json:"input"`
    Expected string `json:"expected"`
    Command  string `json:"command,omitempty"`
}

type ReplaceRule struct {
    Pattern     string `json:"pattern"`
    Replacement string `json:"replacement"`
}

type MatchOutputRule struct {
    Pattern string `json:"pattern"`
    Message string `json:"message"`
    Unless  string `json:"unless,omitempty"`
}
```

### UnmarshalJSON 仲裁逻辑
```go
func (f *Filter) UnmarshalJSON(data []byte) error {
    // Step 1: 标准 unmarshal 进所有字段
    type filterAlias Filter // 防递归
    if err := json.Unmarshal(data, (*filterAlias)(f)); err != nil {
        return err
    }
    // Step 2: 仲裁 head/tail/max_lines 双向 (canonical 优先)
    if f.HeadLines > 0 { f.Head = f.HeadLines } else if f.Head > 0 { f.HeadLines = f.Head }
    if f.TailLines > 0 { f.Tail = f.TailLines } else if f.Tail > 0 { f.TailLines = f.Tail }
    if f.MaxLines > 0 { /* keep */ } else { f.MaxLines = f.MaxLines }
    // Step 3: 仲裁 Name/ID (canonical ID 优先)
    if f.ID == "" && f.Name != "" { f.ID = f.Name }
    if f.Name == "" && f.ID != "" { f.Name = f.ID }
    // Step 4: 仲裁 Command 优先级 (Q9 = A: canonical commandPatterns 优先, legacy Command 兜底)
    // 注: Match() 内部判断, UnmarshalJSON 不动
    return nil
}
```

## 关键约束与契约

### 前置条件
- Go 1.26.6（与 go.work 一致）
- `plugins/rtk/go.mod` 现有依赖不变（无需新增 toml 库，TOML 阶段四补）
- 阶段一/二已实现的 `compression.go` / `linefilter.go` / `smarttruncate.go` / `deduplicator.go` / `grouper.go` 不破坏

### 影响面
- **修改文件**（5 个）：
  - `plugins/rtk/filterloader.go` — 重写
  - `plugins/rtk/linefilter.go` — 扩展 Filter struct + UnmarshalJSON
  - `plugins/rtk/config.go` — 新增 4 字段
  - `plugins/rtk/rtk.go` — Init 多收 appDir + 新增 loader 字段
  - `plugins/rtk/compression.go` — 删 globalLoader + getFilterLoader() + defaultConfig 硬编码
  - `plugins/rtk/hooks.go` — PreLLMHook/PostLLMHook 改用 p.loader.Match
  - `transports/config.schema.json` — rtk 配置块新增 4 字段
- **新增文件**（1 个）：`plugins/rtk/diagnostics.go`（独立 FilterLoadDiagnostic struct + Loader.Diagnostics() 方法）
- **测试文件**（3 个）：`filterloader_test.go` / `config_test.go` 重写扩展 + `diagnostics_test.go` 新增
- **API 签名变更**：`rtk.Init(ctx, config, logger, appDir string) (*Plugin, error)` — 多收 1 参数
- **API 新增**：`(*FilterLoader).Diagnostics() []FilterLoadDiagnostic`
- **是否破坏任何对外 API**：仅 `rtk.Init` 签名变更；调用方（plugin 注册器）在本仓库内 grep 仅有 `rtk.Plugin{}` 字面构造，无外部显式调用 `rtk.Init`

### 性能契约
- FilterLoader.Load **仅在 Plugin.Init 时跑一次**，缓存到 `loader.cachedFilters`；每次 Match 调用 O(N) 遍历（与现状一致），无 hot-path 性能回退
- Trust SHA256 校验仅在 load 时跑一次；后续 Match 无 trust 校验开销
- `enabled_filters` / `disabled_filters` 过滤为 O(N) 单次扫描，结果缓存在 `loader.cachedFilters`（不每次 Match 重新过滤）

### 错误码与编号段
- 不引入新错误码。错误经 `Plugin.logger` 写 WARN/ERROR，载荷为 `FilterLoadDiagnostic{Source, Format, Path, Level, Message}`
- 53 个内置 JSON 解析失败 → 跳过单文件 + WARN（与现状 `fail-open` 策略一致）
- project / global filter 解析失败 → 跳过单文件 + WARN（与 OmniRoute `collectFilterSources` 行为一致）
- trust.json 缺失/不匹配 → 跳过整个 project source + WARN
- TOML 文件 → 跳过 + WARN "TOML support planned for stage 4"

### 环境限制与验证策略

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 双格式 Filter JSON 解析（legacy + canonical） | ✅ | 单元测试 | n/a |
| V-plugins-2 project > global > builtin 三级优先级 + 白/黑名单 | ✅ | 单元测试（t.TempDir 造三级 fixture） | n/a |
| V-plugins-3 trust.json SHA256 4 场景 + env var 旁路 | ✅ | 单元测试（t.TempDir 造 4 种 trust.json + os.Setenv） | n/a |
| V-plugins-4 服务端 live 端到端验证（启动 pg-gateway-api + 发 chat 请求） | ❌ | n/a | 阶段三单元测试已覆盖 Loader.Match/Load 行为；live E2E 留阶段六（UI/API 同步时一并验证）；本 V-* 不进入 scenario track covers |

**不可验证部分（V-plugins-4）降级策略**：在 `define-summary.yaml` 标 `degraded` + `env_resource_refs: []`，tasks.md 对应 verify 章节写明"非本阶段交付范围，留阶段六或独立 track"。`pg-validate-proposal.py` 阶段 3 会按 PR-B2 校验：本 V-* 在 design.md「环境限制与验证策略」段已被列出，符合 degraded 契约。

### 可观测性
- **关键日志点**：
  - `Plugin.Init` 完成时 INFO: `rtk: filter loader initialized, total=N (project=N global=N builtin=N), diagnostics=N`
  - 单条 source 失败 WARN: `rtk: filter skipped source=project path=<path> reason=<error>`
  - trust.json 不匹配 WARN: `rtk: project filters SHA256 mismatch, skipping path=<path>`
  - TOML 占位 WARN: `rtk: TOML support planned for stage 4, skipping path=<path>`
- **关键指标**：暂不引入 Prometheus counter（与现有 RTK 插件一致，metrics 留阶段六）
- **RequestId 追踪**：无变更（filter loader 与 request 无关）
- **暴露面**：`(*FilterLoader).Diagnostics()` 返回所有 WARN/ERROR 记录，阶段六 UI 可直接 GET

## Verification Criteria

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | 双格式 Filter JSON 解析（legacy + canonical） | 1) legacy JSON `{name, command, rules, head, tail}` <br> 2) canonical JSON `{id, label, category, priority, match:{commands}, rules:{...}, preserve:{...}, tests}` <br> 3) 内置 53 个 JSON 全部走默认 Load | 单元测试 `TestUnmarshalDualFormat` + `TestBuiltinFiltersUnchanged` | (a) legacy 字段全部填充到 Filter struct <br> (b) canonical 字段全部填充 <br> (c) 53 个内置 JSON 在新 struct 下零修改可继续工作 <br> (d) UnmarshalJSON 仲裁: head_lines 覆盖 head, 反之亦然 |
| V-plugins-2 | project > global > builtin 三级优先级 + 白/黑名单 | t.TempDir 造 3 个 source: <br> - `project/.rtk/filters.json` 含 git-status-proj <br> - `global/rtk/filters.json` 含 git-status-glob <br> - builtin embed.FS 已含 git-status <br> + Config.EnabledFilters = ["git-status-proj", "git-status"] <br> + Config.DisabledFilters = ["git-status"] | 单元测试 `TestLoaderPriority` + `TestLoaderEnabledDisabled` | (a) Match("shell","git status") 返回 git-status-proj (rank=3) <br> (b) EnabledFilters=["git-status-proj","git-status"] → 仅这两个被加载 <br> (c) DisabledFilters=["git-status"] 在启用集合内剔除后, 只剩 git-status-proj <br> (d) 排序: sourceRank desc, formatRank desc, priority desc, id asc |
| V-plugins-3 | trust.json SHA256 4 场景 + env var 旁路 | t.TempDir 造 project dir 4 个场景: <br> (a) trust.json filtersSha256 = sha256(filters.json) <br> (b) trust.json filtersSha256 = "wrong_hash" <br> (c) trust.json 缺失 <br> (d) trust.json 存在但字段为 trustedFiltersSha256 (兼容旧字段) <br> + os.Setenv("OMNIROUTE_RTK_TRUST_PROJECT_FILTERS","1") 后再跑一次 | 单元测试 `TestTrustJSON4Scenarios` + `TestEnvVarBypass` | (a) 加载成功 <br> (b) 跳过 + warn "SHA256 mismatch" <br> (c) 跳过 + warn "untrusted" <br> (d) 加载成功 (兼容旧字段) <br> (e) env var=1 → 任意 trust 状态都加载, diagnostics 含 info "trust bypassed by env var" |
| V-plugins-4 | 服务端 live 端到端验证 | 启动 pg-gateway-api + 加载自定义 filter + 真实 chat 请求触发 tool result 压缩 | **degraded**: 留阶段六或独立 track | n/a |

## 变更类型判定

- **变更类型**：feature
- **affected_tracks**：`["plugins"]`
- **scenario track 启用决策**：`scr=false`
- **scenario 决策依据**：
  - 跨 role 协作验证？**否**——仅 `plugins/rtk/` 单模块内部，hook 不跨 role
  - 新 API 端点？**否**——`Diagnostics()` 是 Loader 方法，非 HTTP endpoint；`Init` 签名变更仅内部调用
  - 跨模块联调？**否**——filter loader 与 `core/framework/transports/ui` 无运行时依赖
  - **结论**：纯单模块改动，`scr=false`，scenario 不写
