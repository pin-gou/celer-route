# rtk-stage1-markers-and-doc-protection 设计

## 架构概览

本次变更只触及 `plugins/rtk/` 内部压缩管线，**不涉及任何外部 API、不改 hooks / config / openai / anthropic 集成**。改动围绕 `processRtkTextWithCommand`（compression.go:338）这一核心入口展开，在现有 9 步管线中插入 isDocumentLikeRead 分支与三处 marker 注入。

```
plugins/rtk/compression.go:338  processRtkTextWithCommand
  │
  ├─ 1. stripANSI
  ├─ 2. isShortErrorMessage (early-exit)
  ├─ 3. defaultDetector.detect → {Type:"shell", Command:""} 兜底
  ├─ 4. detection.Type != "shell" → 直接返回原文本
  │
  ├─ ★ NEW: isDocumentLikeRead 判定
  │    = detection.Type=="shell" && detection.Command=="" && !hasGenericErrorMarkers(text)
  │    │
  │    ├─ true  → 跳过 step 5/6/8/9，保留 step 7 dedup
  │    │         (文档读取：仅 ANSI 剥离 + dedup，无 filter / 截断 / char limit)
  │    └─ false → 继续正常管线
  │
  ├─ 5. loader.Match(cmd)            (generic filter when cmd=="")
  ├─ 6. applyLineFilter(text, filter)
  ├─ 7. applyDedup(stripped, threshold) → 返回 (text, collapsed)
  ├─ 8. applySmartTruncate(deduped, effectiveFilter) → 返回 (text, dropped)
  ├─ 9. truncateToCharLimit + [rtk:truncated by chars] 追加
  │
  └─ estimateTokens(result) → ProcessStats.CompressedTokens
```

**与 OmniRoute 语义差异**：

| 维度 | OmniRoute | oc2 本实现 |
|------|-----------|-----------|
| isDocumentLikeRead 的 Type 判定 | `type === "unknown"` | `Type === "shell"` |
| 兜底 detector 返回 | `{type:"unknown"}` | `{Type:"shell", Command:""}` |

oc2 的 `defaultDetector.detect()` 在没有规则匹配时**总是**返回 `Type:"shell"`（linedetector.go:212），没有 `"unknown"` 类型。因此本实现判定条件 `Type=="shell" && Command==""` 比 OmniRoute 的 `type=="unknown" && !command` **更宽**：任何未识别命令的 shell 输出（即便被识别为某种 shell 子类型）只要 `Command==""` 且无错误标记就进入保护路径。这是与 OmniRoute 的有意语义差，符合 `temp/rtk.md` 阶段一规格。

**涉及模块**：

| 模块 | 路径 | 改动 |
|------|------|------|
| plugins/rtk | `plugins/rtk/linedetector.go` | 新增 `hasGenericErrorMarkers(text) bool` 辅助函数 |
| plugins/rtk | `plugins/rtk/compression.go` | `processRtkTextWithCommand` 加 isDocumentLikeRead 分支；`truncateToCharLimit` 调用后追加 char marker |
| plugins/rtk | `plugins/rtk/smarttruncate.go` | `applySmartTruncate` 改返回 `(string, int)`；插入 `[rtk:truncated N lines]` |
| plugins/rtk | `plugins/rtk/deduplicator.go` | `applyDedup` 改返回 `(string, int)`；追加 `[line repeated Nx]` + `[rtk:dropped N repeated lines]` |
| plugins/rtk | `plugins/rtk/linefilter_test.go` | 更新 3 处现有测试断言；新增 5 个测试 |

## API 设计（如有）

无新增 HTTP API。

## 数据模型（如有）

无新增数据模型。所有改动是函数实现细节（marker 文本、签名变更）。

## 组件设计（如有）

不适用。本次为压缩管线内部行为调整。

## 关键约束与契约

### 前置条件

- Go toolchain ≥1.26.6（与 `go.work` 一致）
- `plugins/rtk/` 模块独立可编译（现状已满足）
- 不引入新 config 字段、env vars、配置 schema 变更
- 不修改 `filterloader.go` / `config.go` / `hooks.go` / `openai.go` / `anthropic.go`

### 影响面

| 函数 | 文件 | 签名变更 |
|------|------|---------|
| `applySmartTruncate(input, filter)` | smarttruncate.go | `string` → `(string, int)` |
| `applyDedup(input, threshold)` | deduplicator.go | `string` → `(string, int)` |
| `processRtkTextWithCommand` | compression.go | 内部签名不变（返回仍为 `(string, *ProcessStats)`）；调用 step 7/8 时解构第二个返回值 |

**blast radius**：
- `applySmartTruncate` 调用方：仅 `compression.go:397`（step 8）
- `applyDedup` 调用方：仅 `compression.go:390`（step 7）
- 直接调用这两个函数的现有测试：`linefilter_test.go`（14 个测试中需更新 3 个：`TestLineFilterDedup` / `TestLineFilterHeadAndTail` / `TestLineFilterMaxLines`）

**是否破坏任何对外 API**：否。所有 marker 文本仅出现在 `PreLLMHook` 修改后的 tool 消息内容中，输出给 LLM provider 的格式不变。

### 性能契约

- 每条 tool 消息处理仍由 step 1-9 顺序执行，marker 字符串拼接 + 一行 `strings.Join` 增量；性能开销 < 1µs/条（与 OmniRoute 一致）
- `hasGenericErrorMarkers` 用预编译 `*regexp.Regexp` 复用（与 `isShortErrorMessage` 一致风格）
- `applySmartTruncate` / `applyDedup` 内部循环不变，仅末尾多一次 `strings.Join` 与 marker 构造
- 不引入新对象分配热点（marker 字符串为常量或 `fmt.Sprintf` 单次）

### 错误码与编号段

不适用。无新增错误码。

### 环境限制与验证策略

依据 `.pg/changes/rtk-stage1-markers-and-doc-protection/env-description.yaml`（阶段 1.6 产出）的 6 段判断：local 环境**未声明任何 Go 工具链 / RTK plugin 专用能力**，但 `config_resources[name=bifrost-binary]`（编译产物）的存在隐含 Go toolchain + 源码树可访问。所有 V-* 通过本地 Go 工具链直接验证。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 isDocumentLikeRead 文档读取不被截断 | ✅ | 单元测试 | n/a |
| V-plugins-2 截断 marker 插入正确 | ✅ | 单元测试 | n/a |
| V-plugins-3 去重 marker 追加正确 | ✅ | 单元测试 | n/a |
| V-plugins-4 字符截断 marker 追加正确 | ✅ | 单元测试 | n/a |
| V-plugins-5 plugins/rtk 包 `go build ./...` 通过 | ✅ | 单元测试 | n/a |
| V-plugins-6 plugins/rtk 包 `go vet ./...` 无错 | ✅ | 单元测试 | n/a |
| V-plugins-7 plugins/rtk 单元测试全部通过 | ✅ | 单元测试 | n/a |

**降级策略**：所有 V-* 均 `post_discussion_status=verifiable`，**无** degraded / skipped。design.md 无需"环境限制与降级路径"扩展段。

### 可观测性

- marker 文本（`[rtk:truncated N lines]` / `[line repeated Nx]` / `[rtk:dropped N repeated lines]` / `[rtk:truncated by chars]`）即"压缩可观测性"——LLM 可直接看到丢失信息量
- `ProcessStats.Techniques` 仍记录 `"dedup"` / `"smarttruncate"` / `"charlimit"`（compression.go:382-407 不变），用于 logs-db metadata
- 不新增 `WARN` / `ERROR` 日志（marker 已自解释）

## Verification Criteria

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | isDocumentLikeRead 文档读取不被截断 | 无 | 调用 `processRtkTextWithCommand`，input 为 ~147 行无错误标记的代码/prose，detection 兜底为 `{Type:"shell", Command:""}` | 输出保留全文（仅 ANSI 剥离 + dedup），无 `[rtk:truncated N lines]` marker |
| V-plugins-2 | 截断 marker 插入正确 | 无 | 调用 `applySmartTruncate`，filter.Head=3 + Tail=2 + 10 行输入 | 输出 head(3) + `[rtk:truncated N lines]`（N=5）+ tail(2) |
| V-plugins-3 | 去重 marker 追加正确 | 无 | 调用 `applyDedup`，threshold=3 + 5 个相同连续行 + 1 个不同行 | 输出 1 个原始行 + `[line repeated 4x]` + `[rtk:dropped 4 repeated lines]` + 1 个不同行 |
| V-plugins-4 | 字符截断 marker 追加正确 | 无 | 调用 `processRtkTextWithCommand`，input 超 `config.MaxCharsPerResult` | 输出末尾追加 `[rtk:truncated by chars]` |
| V-plugins-5 | plugins/rtk 包 `go build ./...` 通过 | 无 | 在 `plugins/rtk/` 目录执行 `go build ./...` | 编译成功无错 |
| V-plugins-6 | plugins/rtk 包 `go vet ./...` 无错 | 无 | 在 `plugins/rtk/` 目录执行 `go vet ./...` | 无 vet 警告 |
| V-plugins-7 | plugins/rtk 单元测试全部通过 | 无 | 在 `plugins/rtk/` 目录执行 `go test ./... -short -count=1` | 全部测试通过（含 3 个更新断言 + 5 个新增测试） |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 不涉及 core/ 任何文件 |
| framework | ❌ | 不涉及 framework/ 任何文件 |
| transports | ❌ | 不涉及 transports/ 任何文件 |
| plugins | ✅ | 唯一改动模块；`plugins/rtk/{compression,linedetector,smarttruncate,deduplicator,linefilter_test}.go` |
| ui | ❌ | 不涉及前端任何文件 |

**affected_tracks**：`[plugins]`

**scenario track(s) 决策**：
- 跨 role 协作验证？否（仅 plugins/rtk 内部函数签名 + 行为变更）
- 新 API 端点？否（无 HTTP / IPC / SDK 接口变更）
- 跨模块联调？否（仅 Go 单元测试覆盖函数行为）

**结论**：所有 scenario track **禁用**（`scr=false`）。本变更纯单模块内部行为，纯 Go 单元测试覆盖，无需 E2E 场景验证。

> **注意**：scenario track 禁用时，tasks.md / execution-manifest.yaml / scenario-scr.yaml 三产物均不包含 scr 章节——避免冗余。V-* 仍通过 tasks.md verify 章节的 `go test ./...` 命令覆盖。