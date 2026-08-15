# providercooldown-builtin-ui 设计

## 架构概览

涉及 3 个代码模块 + 1 个文档：

| 模块 | 改动 |
|------|------|
| `transports/bifrost-http/server/plugins.go` | providercooldown 条件分支从"cfg != nil && cfg.Enabled" 改为"无 entry 等效 enabled=true" |
| `transports/config.schema.json` | name 描述补 `provider-cooldown`；新增 allOf/if/then 专用 config schema |
| `ui/app/workspace/plugins/views/pluginsView.tsx` + 新建 fragment | PluginsView 内嵌 providercooldown 专用 fragment（Switch + form + monitoring panel） |
| `docs/features/provider-cooldown.mdx` | 新建 |

数据流：

```
Server 启动
  loadBuiltinPlugins()
    → providercooldown case: 若无 entry → 构造等价默认 config (enabled=true)
                            若 entry.enabled=true → 加载
                            若 entry.enabled=false → 跳过
    → providercooldown.NewPlugin(logger).Init(cfg)
    → s.KeyPoolFilter = plugin.State.AsFilter(logger)  ← 关键绑定

UI 配置启用
  fragment Switch onChange
    → RTK Query mutation: PUT /api/plugins/provider-cooldown
      → handler.updatePlugin (复用，handler/plugins.go:380)
    → configstore.UpsertPlugin (DB config_plugins 表)

UI 表单提交
  fragment form submit
    → react-hook-form + zod 校验
    → RTK Query mutation: PUT /api/plugins/provider-cooldown
      → body.config 含 default_ttl_seconds / ttl_overrides / quota_patterns
    → configstore.UpsertPlugin → reload plugin → KeyPoolFilter rebind

UI 监控面板
  useGetCooldownStateQuery() → GET /api/plugins/provider-cooldown/state
  useGetCooldownStatsQuery() → GET /api/plugins/provider-cooldown/stats
  解冻按钮 → DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId}
```

## API 设计

本变更不新增 API。复用现有 3 个 cooldown 专用 endpoint + 通用 plugin CRUD：

### PUT /api/plugins/provider-cooldown - Request Body

```json
{
  "name": "provider-cooldown",
  "enabled": true,
  "config": {
    "default_ttl_seconds": 600,
    "ttl_overrides": {
      "openai": 300
    },
    "quota_patterns": [
      "insufficient_quota",
      "quota exceeded",
      "billing",
      "usage limit"
    ]
  }
}
```

### PUT /api/plugins/provider-cooldown - Response Body (200)

```json
{
  "name": "provider-cooldown",
  "actualName": "provider-cooldown",
  "enabled": true,
  "config": {
    "default_ttl_seconds": 600,
    "ttl_overrides": {
      "openai": 300
    },
    "quota_patterns": [
      "insufficient_quota",
      "quota exceeded",
      "billing",
      "usage limit"
    ]
  },
  "isCustom": false,
  "path": null,
  "status": {
    "name": "provider-cooldown",
    "status": "active",
    "logs": [],
    "types": ["llm"]
  }
}
```

### PUT /api/plugins/provider-cooldown - Response Body (4xx 失败)

```json
{
  "error": {
    "code": "INVALID_CONFIG",
    "message": "default_ttl_seconds must be positive integer",
    "data": {
      "field": "config.default_ttl_seconds"
    }
  }
}
```

### GET /api/plugins/provider-cooldown/state - Response Body (200)

```json
{
  "plugin": "provider-cooldown",
  "count": 0,
  "entries": [
    {
      "provider": "openai",
      "key_id": "key-abc-123",
      "expires_at": "2026-08-15T18:00:00Z",
      "remaining": 300000000000
    }
  ]
}
```

### GET /api/plugins/provider-cooldown/stats - Response Body (200)

```json
{
  "plugin": "provider-cooldown",
  "mark_count": 12,
  "suppressed_count": 8,
  "current_active_count": 3
}
```

### DELETE /api/plugins/provider-cooldown/state/{provider}/{keyId} - Response Body (200)

```json
{
  "message": "Cooldown cleared",
  "provider": "openai",
  "key_id": "key-abc-123"
}
```

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | 成功 | 0 |
| 400 | config schema 校验失败（default_ttl_seconds 非正数等） | INVALID_CONFIG |
| 404 | DELETE 时指定 provider/keyId 不存在（`No active cooldown for this (provider, key)`） | NOT_FOUND |
| 403 | 启用 auth bypass 时建 custom plugin | AUTH_BYPASS_REQUIRED |

## 数据模型

### config_plugins 表（DB schema 不变）

```
TablePlugin { name string PK; enabled bool; config JSON; isCustom bool; ... }
```

字段在不变 schema 的前提下写入：

| 字段 | 写入值 |
|------|--------|
| `name` | `provider-cooldown` |
| `enabled` | 来自 Request Body |
| `config` | JSON 序列化后的 3 字段对象 |
| `isCustom` | `false`（handler 写入 `!isBuiltin`） |

**schema 演进**：本变更不引入 schema 迁移，仅 schema 校验层（`config.schema.json` 的 allOf/if/then）变化。

## 组件设计

### 结构

```
PluginsView (ui/app/workspace/plugins/views/pluginsView.tsx)
  └─ pluginsList: PluginRow[]
       └─ PluginRow[name="provider-cooldown"]
            └─ <ProvidercooldownFragment />  ← 新增
                 ├─ <EnabledSwitch />          ← 顶部 Switch
                 ├─ <ConfigForm />             ← react-hook-form + zod
                 │    ├─ <NumberInput name="default_ttl_seconds" />
                 │    ├─ <KeyValueEditor name="ttl_overrides" />
                 │    └─ <ListEditor name="quota_patterns" />
                 └─ <MonitoringPanel />        ← 下方监控
                      ├─ <StatsCard />         ← 拉 /stats
                      ├─ <StateList />         ← 拉 /state
                      └─ <UnfreezeButton />    ← DELETE /state/{p}/{k}
```

### EnabledSwitch

- 复用 `ui/components/ui/switch.tsx`
- value：`isPluginEnabled(plugin)`
- onChange：`useUpdatePluginMutation({ enabled: newValue })`
- RBAC：`useRbac(RbacResource.Plugins, RbacOperation.Update)`
- 表单状态：受控
- 视觉：fragment 顶部独立 banner 行（"Enable Provider Cooldown" + Switch）

### ConfigForm

- 库：`react-hook-form` + `zod`
- schema 复用：`ui/lib/types/plugins.ts` 已有 `pluginFormSchema`（扩展 providercooldown 专用 fragment）
- 字段：
  - `default_ttl_seconds: number.min(1).max(86400)`（秒上限 1 天）
  - `ttl_overrides: Record<string, number.min(1)>`（keyValue 编辑器）
  - `quota_patterns: string.min(1)` 数组（行内 + 删除）
- 保存按钮：触发 `useUpdatePluginMutation({ config: formData })`
- 错误反馈：inline field error（项目约定）

### MonitoringPanel

- 拉取：`useGetCooldownStateQuery()` + `useGetCooldownStatsQuery()`（新增 RTK Query endpoint）
- 轮询：5s 间隔（参考 telemetry 监控面板）
- 操作：unfreeze 按钮触发 `useUnfreezeCooldownMutation()`（新增 RTK Query endpoint 包装 DELETE）

### 复用清单

| 复用项 | 位置 |
|--------|------|
| react-hook-form + zod 模板 | `ui/lib/types/plugins.ts` pluginFormSchema |
| RTK Query 通用 plugin CRUD | `ui/lib/store/apis/pluginsApi.ts` |
| Switch / Input / Button | `ui/components/ui/*` |
| Sheet / Drawer | `ui/components/ui/sheet.tsx` |
| RBAC hook | `useRbac(RbacResource.Plugins, X)` |
| 表单组件 | `ui/components/ui/form.tsx` |

## 关键约束与契约

### 前置条件

- `transports/bifrost-http/lib/config.go:builtinPluginNames` 已含 `providercooldown.PluginName`（不改）
- `transports/bifrost-http/server/plugins.go` 中 `loadBuiltinPlugins` 路径的 providercooldown case 保持硬编码实例化（仅条件分支调整）
- `server.go:1928-1937` 的 reload + KeyPoolFilter rebind 路径保持不变
- 不动 `IsBuiltinPlugin()` 决策函数
- configstore DB schema 不变（无迁移）

### 影响面

| 类型 | 路径 | 变更 |
|------|------|------|
| Go 条件分支 | `transports/bifrost-http/server/plugins.go:289-315` | 默认开启逻辑 |
| JSON Schema | `transports/config.schema.json:2725-2799` | name 描述 + 专用 config schema |
| TS 组件 | `ui/app/workspace/plugins/views/pluginsView.tsx` | 嵌入分支 |
| TS fragment | `ui/app/workspace/plugins/fragments/providercooldownFragment.tsx` | 新建 |
| TS types | `ui/lib/types/plugins.ts` | 扩展 providercooldown 字段 schema |
| TS RTK Query | `ui/lib/store/apis/pluginsApi.ts` | 扩展 3 个 endpoint |
| 文档 | `docs/features/provider-cooldown.mdx` | 新建 |
| 单测 | `plugins/providercooldown/cooldown_test.go` | 保持（可能补充） |
| 单测 | `transports/bifrost-http/server/plugins_test.go` | 补充默认开启 case |

**是否破坏对外 API**：否（仅扩展默认行为，复用现有 endpoint）

### 性能契约

- `loadBuiltinPlugins` 不引入新 IO（仅内存条件判断）
- `quotes` 拉取 / 监控面板轮询：5s 间隔，单次 GET < 100ms
- Providercooldown 监控列表项最多 100 行（超过分页，不在本变更范围）

### 错误码与编号段

不新增错误码。沿用 `INVALID_CONFIG` / `NOT_FOUND` / `AUTH_BYPASS_REQUIRED`。

### 环境限制与验证策略

> **依据** `.pg/changes/providercooldown-builtin-ui/env-description.yaml`（local 环境的 6 段声明），并对齐 `0-define/define-summary.yaml` 中已确认的 V-* 状态。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-transports-1 providercooldown 默认开启 + KeyPoolFilter 绑定 | ✅ | scenario + 单元测试 | n/a |
| V-transports-2 真实 quota 错误触发 cooldown 全链路 | ❌ | n/a | local 无真实可触发 quota 错误的 LLM key；情景在 production mock 中验证 |
| V-transports-3 config.schema.json 拒绝非法配置 | ❌ | n/a | local 无自动注入非法 fixture 路径；schema 校验由单元测试覆盖 |
| V-transports-4 transports/bifrost-http 单测通过 | ✅ | 单元测试 | n/a |
| V-plugins-1 providercooldown 单测全绿 | ✅ | 单元测试 | n/a |
| V-ui-1 PluginsView 列表显示 providercooldown 启用状态 | ✅ | scenario + Vitest | n/a |
| V-ui-2 专用 fragment 表单提交 3 字段 | ✅ | scenario + Vitest | n/a |
| V-ui-3 监控面板拉取 state / stats / 触发解冻 | ✅ | scenario + Vitest | n/a |
| V-ui-4 UI Vitest 单元测试覆盖 fragment | ✅ | Vitest | n/a |

### 可观测性

- **关键日志**：
  - `providercooldown: enabled by default`（INFO，启动时若未显式 entry）
  - `providercooldown: key marked for cooldown, provider=... keyId=...`（WARN，已有）
  - `providercooldown: key unfrozen by user action`（INFO，新增）
- **关键指标**：
  - `providercooldown_mark_total`（Counter）
  - `providercooldown_suppressed_total`（Counter）
  - `providercooldown_active_count`（Gauge）
- **RequestId 追踪**：沿用 bifrost 默认 request id 链路（在 plugin 拦截入口加 `requestId` 字段便于排错）

## Verification Criteria

### dev transports Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | 不在 config.json 写 provider-cooldown entry，server 启动后 providercooldown 加载为 active 状态 | local env 已就绪（bifrost-api + ui-dev） | scenario：调用 GET /api/plugins/{name} | response.status == "active"，response.enabled == true |
| V-transports-2 | 真实 quota 错误触发 cooldown 全链路 | 需真实可触发 429 insufficient_quota 的 LLM key | scenario（生产 mock） | mark 触发、KeyPoolFilter 剔除该 key |
| V-transports-3 | config.schema.json 拒绝非法配置（ttl_overrides 负数） | 构造非法 config fixture | 单元测试 | schema validation 失败 |
| V-transports-4 | transports/bifrost-http 单测通过 (loadBuiltinPlugins default-on case) | plugins 模块构建产物存在 | `cd transports/bifrost-http && go test ./... -short -count=1` | all tests pass |

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | providercooldown 单测全绿（标记 GC TTL 过滤） | plugins/providercooldown 模块编译通过 | `cd plugins/providercooldown && go test ./... -short -count=1` | all tests pass |

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | PluginsView 列表项显示 providercooldown enabled 状态 | 启动 ui-dev + bifrost-api | Vitest mock scenario | list 渲染 enabled=true |
| V-ui-2 | 专用 fragment 表单提交 3 字段后 DB 落盘 | RTK Query mock PUT | Vitest scenario | form submit 触发 mutation，DB mock 收到 3 字段 |
| V-ui-3 | 监控面板拉取 state / stats 后渲染 + 解冻按钮触发 DELETE | RTK Query mock 3 个 API | Vitest scenario | panel 渲染列表 + stats + DELETE 触发 |
| V-ui-4 | UI Vitest 单元测试覆盖 fragment（zod schema + form 校验） | n/a | `cd ui && npx vitest run` | all tests pass |

### int scr Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1-int | 端到端：bifrost-api 启动 + 调 GET /api/plugins/provider-cooldown 确认 active | bifrost-api 运行中 | scenario | 200 + status=active |
| V-ui-1-int | 端到端：浏览器访问 /workspace/plugins 看到 providercooldown 行 | ui-dev + bifrost-api 都运行 | scenario | DOM 渲染 plugin row |
| V-ui-2-int | 端到端：浏览器修改 3 字段后提交，刷新页面验证新 config | ui-dev + bifrost-api | scenario | DB 中 config 字段更新 |
| V-ui-3-int | 端到端：浏览器监控面板拉取 state / stats | ui-dev + bifrost-api | scenario | GET /state + /stats 触发 + 列表渲染 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 核心引擎未改 |
| framework | ❌ | 框架基础设施未改 |
| transports | ✅ | server/plugins.go 默认开启逻辑 + config.schema.json 专用 schema |
| cli | ❌ | CLI 工具未改 |
| plugins | ✅ | providercooldown 单测保持通过（核心逻辑不动） |
| ui | ✅ | PluginsView 内嵌专用 fragment + 3 字段表单 + 监控面板 |
| scr | ✅ | e2e 跨模块（transports + plugins + ui）联调 |

**affected_tracks**：`transports`, `plugins`, `ui`

**scenario track 启用决策**（`scr`）：

- 跨 role 协作验证？✅（transports 后端 + plugins 运行时 + ui 前端）
- 新 API 端点？❌（沿用现有 3 个 cooldown API）
- 跨模块联调？✅（UI 提交 → REST → bifrost 内部插件 → KeyPoolFilter 绑定）

**`scr` enabled = true**

**selected-stages**：`dev`
**environment**：dev → local
