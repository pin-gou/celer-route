# rtk-phase5-learn-discover 设计

## 架构概览

本次变更触及 `plugins/rtk/` 子模块，新增 2 个文件 + 2 个测试文件，不涉及 `core/` / `framework/` / `transports/` / `ui/`。

### 模块依赖图

```
plugins/rtk/
├── grouper.go          # normalizeLine（复用，不修改）
├── discover.go         # 新建：DiscoverNormalizeLine + DiscoverRepeatedNoise
├── learn.go            # 新建：SuggestFilter + CommandToId
├── discover_test.go    # 新建
└── learn_test.go       # 新建

数据流：
CommandSample[] (caller 提供)
    ↓
DiscoverRepeatedNoise ── uses ──> normalizeLine (grouper.go) + 3 步扩展
    ↓
NoiseCandidate[]
    ↓ (或者直接调用)
SuggestFilter(command, samples)
    ↓
SuggestedFilter (canonical JSON)
```

### 数据结构

```go
// discover.go
type CommandSample struct {
    Command string `json:"command"`
    Output  string `json:"output"`
}

type NoiseCandidate struct {
    Pattern string `json:"pattern"`  // regex-safe 字符串（^ 前缀锚定）
    Hits    int    `json:"hits"`     // 跨 sample 出现次数
}

// learn.go
type SuggestedFilter struct {
    ID          string                 `json:"id"`
    Label       string                 `json:"label"`
    Description string                 `json:"description"`
    Category    string                 `json:"category"`     // "generic"
    Priority    int                    `json:"priority"`     // 50
    Match       SuggestedFilterMatch   `json:"match"`
    Rules       SuggestedFilterRules   `json:"rules"`
    Preserve    SuggestedFilterPreserve `json:"preserve"`
    Tests       []FilterTest           `json:"tests"`        // 阶段五为空
    Meta        SuggestedFilterMeta    `json:"_meta"`
}

type SuggestedFilterMatch struct {
    OutputTypes []string `json:"outputTypes"`
    Commands    []string `json:"commands"`     // ["^npm\\s+install\\b"]
    Patterns    []string `json:"patterns"`
}

type SuggestedFilterRules struct {
    StripAnsi        bool     `json:"stripAnsi"`
    DropPatterns      []string `json:"dropPatterns"`
    CollapsePatterns  []string `json:"collapsePatterns"`
    IncludePatterns   []string `json:"includePatterns"`    // error + summary
    Deduplicate       bool     `json:"deduplicate"`
    MaxLines          int      `json:"maxLines"`
    HeadLines         int      `json:"headLines"`
    TailLines         int      `json:"tailLines"`
    OnEmpty           string   `json:"onEmpty"`
}

type SuggestedFilterPreserve struct {
    ErrorPatterns   []string `json:"errorPatterns"`
    SummaryPatterns []string `json:"summaryPatterns"`
}

type SuggestedFilterMeta struct {
    LearnedFromSamples int `json:"learnedFromSamples"`
    DropThreshold      int `json:"dropThreshold"` // 0.5 → 50（百分制整数便于传输）
}
```

## API 设计（如有）

本变更不暴露 HTTP 端点，不修改 `Provider` 接口，不修改 BifrostContext 字段。所有函数均为包内纯函数，未来可通过 CLI / HTTP / UI 调用方提供 `[]CommandSample`。

| 函数 | 包 | 签名 | 备注 |
|------|----|----|------|
| `DiscoverNormalizeLine` | rtk | `func(line string) string` | 复用 grouper.normalizeLine + 3 步扩展 |
| `DiscoverRepeatedNoise` | rtk | `func(samples []CommandSample) []NoiseCandidate` | 跨样本聚合 |
| `SuggestFilter` | rtk | `func(command string, samples []CommandSample) SuggestedFilter` | 学习生成过滤器骨架 |
| `CommandToId` | rtk | `func(command string) string` | 命令 slug 化 |

## 数据模型（如有）

无 DB 变更。无 `Filter` struct 字段变更。无 `Config` 字段变更。

## 组件设计（如有）

无前端组件。无新增 UI 路由。

## 关键约束与契约

### 前置条件

- Go 版本 ≥ 1.21（`regexp` 库需支持 `(?i)` 内联标志）
- `plugins/rtk/grouper.go` 已存在并实现 `normalizeLine(line string) string`，且对外可见（小写未导出函数需在本包内调用）

### 影响面

| 类别 | 影响 |
|------|------|
| 新增文件 | `plugins/rtk/discover.go`、`plugins/rtk/learn.go`、`plugins/rtk/discover_test.go`、`plugins/rtk/learn_test.go` |
| 修改文件 | 无 |
| 依赖变化 | 无（纯标准库 `regexp` / `strings` / `sort` / `encoding/json`） |
| 对外 API | 无（纯包内函数） |
| 对外契约 | 无（不读不写任何持久化数据） |
| `Filter` struct | 无字段变更 |
| `Config` struct | 无字段变更 |
| 既有 27 个 RTK 测试 | 无回归（仅新增独立测试文件） |

### 性能契约

- `DiscoverNormalizeLine`：O(L) 其中 L 是行长度（每行正则 5 次 + 1 次 trim）
- `DiscoverRepeatedNoise`：O(N × L) 其中 N = 总行数（每行 normalize 一次 + 1 次 map lookup + 1 次正则 test 冲突守卫）
- `SuggestFilter`：O(N × L) 与上面类似，外加 1 次 error/summary 正则 test 每行
- 总体对每个 sample 处理时间应 < 10ms（典型 N=1000 L=80 行样本），无内存分配热点

### 错误码与编号段

无错误码（纯函数，无错误返回路径）。所有错误通过 panic 暴露（`regexp.MustCompile` 在编译期失败时 panic），符合既有 RTK 插件风格（如 `grouper.go`、`filterloader.go`）。

### 环境限制与验证策略

依据 `.pg/changes/rtk-phase5-learn-discover/env-description.yaml` 的 `local` 环境 6 段 + relations 判断。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 discoverNormalizeLine 归一化与复用契约 | ❌ | 单元测试 | local env capabilities（rest_api_endpoint / sqlite_logs_db / vite_dev_server / sample_dataset 等）均与纯字符串处理算法不相关；验证走 `cd plugins/rtk && go test -run TestDiscoverNormalizeLine -short -count=1`，不依赖环境服务 |
| V-plugins-2 discoverRepeatedNoise 聚合与过滤 | ❌ | 单元测试 | 同上；走 `cd plugins/rtk && go test -run TestDiscoverRepeatedNoise -short -count=1` |
| V-plugins-3 suggestFilter 命令→过滤器骨架 | ❌ | 单元测试 | 同上；走 `cd plugins/rtk && go test -run TestSuggestFilter -short -count=1` |
| V-plugins-4 suggestFilter 错误/摘要识别 + 冲突守卫 | ❌ | 单元测试 | 同上；走 `cd plugins/rtk && go test -run 'TestSuggestFilterError\|TestSuggestFilterSummary\|TestSuggestFilterConflictGuard' -short -count=1` |
| V-plugins-5 commandToId slug 算法 | ❌ | 单元测试 | 同上；走 `cd plugins/rtk && go test -run TestCommandToId -short -count=1` |
| V-plugins-6 既有 RTK 测试不回归 | ❌ | 单元测试 | 同上；走 `cd plugins/rtk && go test -short -count=1 ./...` 全绿 |

> **说明**：6 个 V-* 全部标 `❌` 在 local env 列，是基于「local 环境 capabilities 不覆盖纯字符串算法验证」的判断；与之对照，单元测试是真正承担验证职责的通道。define-summary.yaml 中 6 个 V-* 的 `post_discussion_status` 均已标记为 `skipped`，proposal.md「风险和注意事项」段已列出全部 V-* id。

### 可观测性

- 关键日志点：无（纯函数，无副作用，无需日志）
- 关键指标：无
- RequestId 追踪：无（不在请求处理路径上）

## Verification Criteria

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | `DiscoverNormalizeLine` 对 ISO 时间戳、hex、版本号、独立整数、`<PKG>`、`<CODE>`、时间单位、多余空白的归一化结果与 OmniRoute 完全一致 | 无前置（纯函数） | `cd plugins/rtk && go test -run TestDiscoverNormalizeLine -short -count=1 -v` | 全部子测试 PASS |
| V-plugins-2 | `DiscoverRepeatedNoise` 对多 sample 的归一化聚合、单 sample 过滤、排序、pattern 转义符合预期 | 构造 3-5 个 fixture CommandSample（fixture 内联于测试代码） | `cd plugins/rtk && go test -run TestDiscoverRepeatedNoise -short -count=1 -v` | 全部子测试 PASS |
| V-plugins-3 | `SuggestFilter` 返回的 SuggestedFilter 结构合法：id 由 `CommandToId` 生成、match.commands 含转义后命令、rules/preserve 块存在 | 构造 3-5 个 fixture CommandSample（npm install 输出） | `cd plugins/rtk && go test -run TestSuggestFilter -short -count=1 -v` | 全部子测试 PASS |
| V-plugins-4 | `SuggestFilter` 对 ERROR_PATTERN / SUMMARY_PATTERN 行的识别正确；冲突守卫正确排除与 preserved 行匹配的 drop candidate | 构造混合样本：含 error 行 + summary 行 + 高频归一化模板 | `cd plugins/rtk && go test -run 'TestSuggestFilterError\|TestSuggestFilterSummary\|TestSuggestFilterConflictGuard' -short -count=1 -v` | 全部子测试 PASS |
| V-plugins-5 | `CommandToId` 把命令字符串规范化为 kebab-case id | 无前置（纯函数） | `cd plugins/rtk && go test -run TestCommandToId -short -count=1 -v` | 全部子测试 PASS |
| V-plugins-6 | 既有 RTK 27 个测试用例零回归 | 无前置 | `cd plugins/rtk && go test -short -count=1 ./...` | 全部 PASS（既有 27 个 + 新增若干） |

### int scr Verification Criteria

- 无（scr track 未启用，scenario track 全部 disabled）

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 不修改 `core/` 任何文件；不引入新 `Provider` 方法或 `BifrostContext` 字段 |
| framework | ❌ | 不修改 `framework/` 任何文件 |
| transports | ❌ | 不修改 `transports/` 任何文件；不新增 HTTP handler |
| plugins | ✅ | 新增 `plugins/rtk/discover.go`、`plugins/rtk/learn.go`、`plugins/rtk/discover_test.go`、`plugins/rtk/learn_test.go`；复用 `plugins/rtk/grouper.go` 的 `normalizeLine` |
| ui | ❌ | 不修改 `ui/` 任何文件 |
| scr（scenario） | ❌ | 本次为纯算法函数交付，不涉及跨 role 协作、不新增 HTTP API 端点、无跨模块联调需求——scenario track 全部 disabled |

**affected_tracks**：`[plugins]`

**scenario_tracks_decision**（per-track，SSOT 落到 `on-conditions-eval.md`）：

| scenario track | enabled | 理由 |
|---------------|---------|------|
| scr | **false** | ① 跨 role 协作验证？不——纯算法函数，无上下游服务依赖。② 新 API 端点冒烟？不——不暴露 HTTP 端点。③ 跨模块联调场景？不——单一包内函数交付。 |

**selected_stages**：`dev`（plugins track 在 dev stage 内）；`int` 阶段的 `scr` track 虽常驻但本变更下 enabled=false，不参与 scenario 编排。