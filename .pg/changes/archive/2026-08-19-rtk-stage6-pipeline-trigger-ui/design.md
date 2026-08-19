# rtk-stage6-pipeline-trigger-ui 设计

## 架构概览

本次变更跨 3 个模块：

```
plugins/rtk/                          # 后端 Go 插件
├── engine.go         [新]            # CompressionEngine 接口 + EngineCatalog + Pipeline runner
├── engine_test.go    [新]            # 接口契约 + 堆叠执行测试
├── config.go         [改]            # 新增 Pipeline []EngineSpec + MinTokensToCompress int 字段
├── config_test.go    [改]            # 新增字段默认值/校验测试
├── hooks.go          [改]            # PreLLMHook 入口新增 token 阈值判断
├── hooks_test.go     [改]            # 阈值边界测试
└── compression.go    [不改内容]      # estimateTokens 复用

transports/
├── config.schema.json [改]           # rtk 块新增 pipeline + min_tokens_to_compress 字段定义
└── pg-gateway-http/handlers/         # 无改动（plugin CRUD 已支持任意字段透传）

ui/                                   # 前端 TypeScript + React
├── app/workspace/plugins/
│   └── fragments/
│       └── rtkFragment.tsx [新]      # RTK 配置 fragment
├── lib/types/plugins.ts  [改]        # 新增 rtkConfigSchema (zod)
└── locales/{en,zh-CN}/plugins.json  [改]   # 新增 plugins.rtk.* i18n key

tests/e2e/                            # Playwright e2e
└── features/plugins/rtk-config.spec.ts [新]
```

数据流：
1. 用户在前端 RTK fragment 编辑配置 → submit
2. `useUpdatePluginMutation({name:"rtk", data:{...}})` → `PATCH /api/plugins/rtk`
3. transport 层 plugins handler → 持久化到 config-db 的 plugins 表
4. pg-gateway 重启或下次 PreLLMHook 调用时，Config 字段被 RTK 插件的 `Init` 加载
5. PreLLMHook 走 EngineCatalog + Pipeline runner 顺序执行

## API 设计（如有）

无新增独立 HTTP endpoint。复用现有 `/api/plugins/rtk` PATCH 端点。

但 config.schema.json 中 RTK 块新增两个字段：

```json
{
  "type": "object",
  "title": "RTK Plugin Config",
  "properties": {
    "pipeline": {
      "type": "array",
      "description": "Compression engine pipeline (ordered list). Default: [{id:\"rtk\"}]",
      "items": {
        "type": "object",
        "required": ["id"],
        "properties": {
          "id": {
            "type": "string",
            "enum": ["rtk"],
            "description": "Engine id. Unknown ids fail-soft (skip + warn)."
          },
          "config": {
            "type": "object",
            "description": "Engine-specific config override (currently unused for id=rtk; reserved for future engines)"
          }
        }
      },
      "default": [{"id": "rtk"}]
    },
    "min_tokens_to_compress": {
      "type": "integer",
      "minimum": 0,
      "description": "Skip entire compression when estimated request tokens < threshold. 0 = never skip.",
      "default": 0
    }
  }
}
```

## 数据模型（如有）

无新增数据库表/迁移。`plugins` 表的 `config` 字段为 JSON blob，新字段通过 PATCH 自动持久化。

## 组件设计（如有）

### 后端 — engine.go

```go
// plugins/rtk/engine.go
package rtk

type CompressionEngine interface {
    Id() string                                             // e.g. "rtk"
    Apply(ctx *schemas.BifrostContext, text string, cfg EngineConfig) (EngineResult, error)
    HealthCheck() error
    IsEnabled() bool
    Schema() json.RawMessage
}

type EngineConfig struct {
    Enabled  bool            `json:"enabled"`
    Settings json.RawMessage `json:"settings,omitempty"`  // engine-specific
}

type EngineResult struct {
    Text         string  `json:"text"`
    InputBytes   int     `json:"input_bytes"`
    OutputBytes  int     `json:"output_bytes"`
    CompressedBy float64 `json:"compressed_by"`
    Skipped      bool    `json:"skipped,omitempty"`
    Reason       string  `json:"reason,omitempty"`
}

type EngineBreakdown struct {
    Id           string  `json:"id"`
    InputBytes   int     `json:"input_bytes"`
    OutputBytes  int     `json:"output_bytes"`
    CompressedBy float64 `json:"compressed_by"`
}

type PipelineResult struct {
    FinalText       string           `json:"text"`
    EngineBreakdown []EngineBreakdown `json:"engine_breakdown"`
}

// 全局注册表（id → engine）
var globalCatalog = map[string]CompressionEngine{}

func RegisterEngine(e CompressionEngine) { globalCatalog[e.Id()] = e }

// Pipeline runner：顺序执行 Config.Pipeline，输出累加
func RunPipeline(ctx *schemas.BifrostContext, text string, pipeline []EngineSpec, defaultConfig EngineConfig) (PipelineResult, error) {
    result := PipelineResult{FinalText: text}
    for _, step := range pipeline {
        e, ok := globalCatalog[step.Id]
        if !ok {
            log.Warnf("rtk: unknown engine id %q, skipping", step.Id)
            continue
        }
        cfg := defaultConfig
        if step.Config != nil { cfg.Settings = step.Config }
        if !e.IsEnabled() { continue }
        out, err := e.Apply(ctx, text, cfg)
        if err != nil {
            return result, err
        }
        if out.Skipped { continue }
        result.EngineBreakdown = append(result.EngineBreakdown, EngineBreakdown{
            Id: e.Id(), InputBytes: out.InputBytes, OutputBytes: out.OutputBytes, CompressedBy: out.CompressedBy,
        })
        text = out.Text
    }
    result.FinalText = text
    return result, nil
}
```

### 前端 — rtkFragment.tsx

复用 `providercooldownFragment.tsx` 模板：
- `useGetPluginQuery("rtk")` 读取配置
- `useUpdatePluginMutation()` 提交
- `useForm<RTKConfigFormValues>` + `zodResolver(rtkConfigSchema)` 校验
- `EnabledSwitch` 复用 providercooldown 模式
- 表单字段按 rtkConfigSchema 渲染（Intensity Select、max_lines/max_chars Number input、dedup_threshold Number、raw_output_retention Select、apply_to_* Checkbox 组、custom_filters_enabled/trust_project_filters Switch、enable_grouping Switch）

字段分组：

| 分组 | 字段 |
|------|------|
| 启用与强度 | `enabled` (Switch), `intensity` (Select: minimal/standard/aggressive) |
| 行/字符上限 | `max_lines_per_result`, `max_chars_per_result`, `dedup_threshold` |
| 作用范围 | `apply_to_tool_results`, `apply_to_code_blocks`, `apply_to_assistant_messages` |
| 分组 | `enable_grouping`, `grouping_threshold` |
| 过滤器 | `custom_filters_enabled`, `trust_project_filters` |
| 原始输出 | `raw_output_retention`, `raw_output_max_bytes` |
| 高级（新增） | `pipeline` (textarea JSON 编辑), `min_tokens_to_compress` |

> 注意：`pipeline` 字段作为高级配置，UI 暂以 JSON textarea 渲染（避免引入复杂的 array 嵌套表单），后续 iteration 可替换为可视化 engine picker。

## 关键约束与契约

### 前置条件
- plugins/rtk 现有的 Config 字段全部已落地（✅ 阶段一～五已完成）
- 已存在的 52 个内置过滤器仍有效（engine.go 不动 linefilter/filterloader）
- 前端 plugins 页侧边栏已存在 RTK 入口位（✅ 现有 PluginsView 渲染所有已存在 plugin）

### 影响面
- **后端改动文件**：`plugins/rtk/engine.go`（新增 ~150 行）、`plugins/rtk/engine_test.go`（新增 ~200 行）、`plugins/rtk/config.go`（+2 字段）、`plugins/rtk/hooks.go`（PreLLMHook 头部 +5 行）、`plugins/rtk/config_test.go`（+2 用例）、`plugins/rtk/hooks_test.go`（+3 用例）、`transports/config.schema.json`（+30 行）
- **前端改动文件**：`ui/app/workspace/plugins/fragments/rtkFragment.tsx`（新增 ~250 行）、`ui/lib/types/plugins.ts`（+1 schema）、`ui/locales/en/plugins.json`（+15 key）、`ui/locales/zh-CN/plugins.json`（+15 key）、`tests/e2e/features/plugins/rtk-config.spec.ts`（新增 ~80 行）
- **不破坏对外 API**：插件 CRUD 接口契约不变（config blob 自动透传）

### 性能契约
- `engine.go` 的 RunPipeline 顺序执行，整体延迟 = 各引擎 Apply 延迟之和。RTK 自身 Apply 是 O(text length)，符合现状。
- `estimateTokens` 在 PreLLMHook 头部调用一次：O(text length)，可忽略（与压缩本身同阶）。
- `Pipeline` 默认 1 个引擎（[{id:"rtk"}]），不影响现状。

### 错误码与编号段
- `engine.go` 引入 `errors.New("rtk: unknown engine id %q")` 用于 fail-soft warning，不暴露给调用方
- `hooks.go` 在 MinTokensToCompress 跳过路径不返回错误（保持现状语义）

### 环境限制与验证策略

> 依据 `.pg/changes/rtk-stage6-pipeline-trigger-ui/env-description.yaml`（目标 env=local）

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-plugins-1 压缩引擎管线堆叠行为 | ✅ | Go 单测（engine_test.go）+ pg-gateway 集成 | n/a |
| V-plugins-2 主动触发 token 阈值跳过 | ✅ | Go 单测（hooks_test.go）+ pg-gateway 集成 + logs-db 比对 | n/a |
| V-ui-1 RTK fragment 渲染与表单提交 | ✅ | Playwright e2e + 手动截屏 | 多浏览器兼容留待 CI |

**env-description 资源引用**（per `define-summary.yaml`）：
- V-plugins-1：`{env.business_systems[name=pg-gateway-api]}` + `{env.config_resources[name=bifrost-binary]}`
- V-plugins-2：`{env.business_systems[name=pg-gateway-api]}` + `{env.data_resources[name=logs-db]}`
- V-ui-1：`{env.business_systems[name=ui-dev]}` + `{env.business_systems[name=pg-gateway-api]}`

### 可观测性
- **关键日志点**：
  - `engine.go:RunPipeline` 对每个 engine 执行输出 INFO 日志，含 id/inputBytes/outputBytes/compressedBy
  - 未知 engine id → WARN（含 step.Id）
  - `hooks.go:PreLLMHook` 阈值跳过路径输出 DEBUG 日志，含 estimated tokens 与 threshold
- **关键指标**：plugin 内部 metrics 不暴露 Prometheus（沿用现状）
- **RequestId 追踪**：无需新增埋点（plugin 已有 RequestId 透传）

## Verification Criteria

按 stages 顺序遍历，stages = [{name:"dev",tracks:[plugins,transports,ui]},{name:"int",tracks:[scr]}]。

> 说明：受影响的 standard tracks 是 plugins + transports + ui 三条；scr 是 scenario track（int stage），由 phase-2 scenario 覆盖。

### dev plugins Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | CompressionEngine 接口注册与堆叠行为 | `plugins/rtk/engine.go` 已实现并 RegisterEngine | 跑 `cd plugins/rtk && go test ./... -run TestEngine` | EngineCatalog 中含 `id="rtk"`；Pipeline runner 顺序执行并累加 engineBreakdown；未知 id warn+skip 而非 panic |
| V-plugins-2 | PreLLMHook 主动触发 token 阈值跳过 | `plugins/rtk/hooks.go` 入口加阈值判断 + Config.MinTokensToCompress 默认 0 | 跑 `cd plugins/rtk && go test ./... -run TestHooksMinTokens` | MinTokens=0 → 全压（保持现状）；MinTokens=1000000 & req tokens=10 → 跳过压缩（输出字节与输入一致） |
| V-plugins-3 | Config 默认值零值安全 | `plugins/rtk/config.go` 新增字段 | 跑 `cd plugins/rtk && go test ./... -run TestConfigDefaults` | `applyConfigDefaults` 不引入 panic；空 Pipeline 自动补 `[{id:"rtk"}]` |
| V-plugins-4 | config.schema.json 增量更新 | `transports/config.schema.json` 3130-3225 行 rtk 块 | 跑 `python3 .pg/skills/src/core/workflows/scripts/pg-parse-config.py --key transports.config.schema.json.rtk` | schema 校验通过；新字段含默认 |

### dev transports Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | plugin PATCH 透传新增字段 | 已 seed fixture-plugins（含 rtk） | `curl -X PATCH /api/plugins/rtk -d '{"config":{"pipeline":[{"id":"rtk"}],"min_tokens_to_compress":500}}'` | 返回 200 + 更新后 plugin；再 `GET /api/plugins/rtk` 见新字段持久化 |

### dev ui Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | RTK fragment 渲染与字段出现 | ui-dev:3008 已启动；seed 已含 rtk plugin | Playwright e2e `tests/e2e/features/plugins/rtk-config.spec.ts` | 进入 `/workspace/plugins` → 选中 rtk → fragment 渲染 enabled/intensity/max_lines_per_result/max_chars_per_result/dedup_threshold/raw_output_retention 等字段；enabled 开关切换后 submit → API 返回 200 |
| V-ui-2 | i18n 中英文渲染 | ui-dev:3008 | Playwright e2e 切换 zh-CN 后断言 `插件.rtk.启用` 等 key 命中 | 字段标签中文正确显示 |

### int scr Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-scr-1 | 跨模块端到端：UI 改配置 → 后端持久化 → 重启生效 | pg-gateway 已 seed rtk plugin；prepare_env 启动成功 | 浏览器 e2e：打开 /workspace/plugins → 改 enabled=true → 提交 → 重启 pg-gateway → 日志中可见 RTK 插件加载 | 整链路无报错 |
| V-scr-2 | 跨模块端到端：MinTokensToCompress 阈值生效 | 同上一行前置 | 通过插件 PATCH 设置 MinTokensToCompress=1000000，发送一个 token<1M 的 chat request | 响应 tool result 内容未被修改） |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 不改 core/bifrost.go、core/schemas/plugin.go、core/inference.go |
| framework | ❌ | 不改 framework/* |
| transports | ✅ | `transports/config.schema.json` 新增 rtk 字段定义 |
| plugins | ✅ | `plugins/rtk/engine.go`（新）+ `config.go`（改）+ `hooks.go`（改）+ 测试增量 |
| ui | ✅ | `ui/app/workspace/plugins/fragments/rtkFragment.tsx`（新）+ zod schema + i18n + Playwright e2e |
| scr | ✅（int stage） | 跨 plugins + transports + ui 三模块端到端联调 |

**affected_tracks**：`[plugins, transports, ui, scr]`

**scenario_tracks_decision**（per-track，结构化）：
- `scenario-scr=true`：跨 plugins + transports + ui 三模块协作验证（满足"跨多个 role / service 协作验证？"为是）；同时 V-scr-1/2 覆盖跨模块联调场景。

**scenario_reason**：跨多个 role 协作验证=是（plugins 接口 + transports schema + ui 表单三模块联动）；新 API 端点=否（复用现有 PATCH /api/plugins/rtk）；跨模块联调=是（int.scr Verification Criteria 中的两条 scr V-* 验证配置→持久化→生效全链路）。因此 scenario-scr 启用。