# add-plugins-governance-form 设计

## 架构概览

```
┌─ /workspace/plugins?plugin=<name> (page.tsx) ─────────────────────────�
│                                                                       │
│  ├─ 左侧 sidebar: allPlugins[] 列表                                    │
│  │   └─ 点击 → setSelectedPluginId(name)                              │
│  │                                                                       │
│  └─ PluginsView (pluginsView.tsx)                                     │
│       ├─ PROVIDER_COOLDOWN_PLUGIN → ProvidercooldownFragment           │
│       ├─ RTK_PLUGIN → RtkFragment                                      │
│       ├─ GOVERNANCE_PLUGIN → GovernanceFragment (本次新增)              │
│       ├─ OTEL_PLUGIN → OtelView (复用 observability)                  │
│       └─ 其它 → 通用 JSON Editor 表单                                   │
└──────────────────────────────────────────────────────────────────────┘

┌─ GovernanceFragment (governanceFragment.tsx, 本次新增) ────────────────┐
│                                                                       │
│  ├─ EnabledSwitch                                                      │
│  │   ├─ useUpdatePluginMutation(GOVERNANCE_PLUGIN, { enabled })        │
│  │   └─ RBAC: Plugins:Update                                         │
│  │                                                                       │
│  └─ ConfigForm                                                         │
│      ├─ useForm<GovernanceFormValues>(zodResolver(governanceConfigSchema))│
│      ├─ Fieldset: "访问控制"                                            │
│      │   ├─ isVkMandatory (Switch, *bool)                             │
│      │   └─ requiredHeaders (TagInput, *[]string)                     │
│      ├─ Fieldset: "行为"                                               │
│      │   ├─ disableAutoToolInject (Switch, *bool)                     │
│      │   └─ routingChainMaxDepth (Input number, *int)                 │
│      └─ Submit: useUpdatePluginMutation({ config: values })            │
└──────────────────────────────────────────────────────────────────────┘

┌─ OtelView (复用 observability/views/plugins/otelView.tsx) ────────────┐
│  └─ OtelFormFragment (868 行已有实现)                                  │
│      ├─ Profile 列表 (useFieldArray)                                   │
│      ├─ Trace / Metrics Tab                                            │
│      ├─ SecretVarInput for collector_url / metrics_endpoint            │
│      └─ HeadersTable for 3 组 headers                                  │
└──────────────────────────────────────────────────────────────────────┘
```

数据流（governance ConfigForm 提交）：

```
用户改表单字段
  → react-hook-form onSubmit
    → useUpdatePluginMutation({ name: GOVERNANCE_PLUGIN, data: { enabled, config } })
      → PUT /api/plugins/governance
        → plugins.go handler (transports/pg-gateway-http/handlers/plugins.go:96-104)
          → maps.Copy merge 现有 config + 新 config
          → 持久化到 config.db (configstore)
            → 返回 200
      → UI: toast.success + form.reset(values)
```

## API 设计（如有）

无新增 API。复用现有 `PUT /api/plugins/{name}` 端点：

### PUT /api/plugins/governance - Request Body
```json
{
  "enabled": true,
  "path": null,
  "config": {
    "is_vk_mandatory": true,
    "required_headers": ["x-org-id", "x-team-id"],
    "disable_auto_tool_inject": false,
    "routing_chain_max_depth": 5
  }
}
```

### PUT /api/plugins/governance - Response Body (200)
```json
{
  "name": "governance",
  "enabled": true,
  "path": null,
  "config": {
    "is_vk_mandatory": true,
    "required_headers": ["x-org-id", "x-team-id"],
    "disable_auto_tool_inject": false,
    "routing_chain_max_depth": 5
  },
  "is_custom": false,
  "status": {
    "status": "active",
    "types": ["llm", "http"]
  }
}
```

### PUT /api/plugins/governance - Response Body (4xx 失败)
```json
{
  "error": {
    "code": "permission_denied",
    "message": "User lacks RBAC permission for Plugins:Update",
    "data": null
  }
}
```

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | 成功 | 0 |
| 403 | 用户无 Plugins:Update 权限 | permission_denied |
| 422 | config JSON schema 校验失败 | config_invalid |

### GET /api/plugins/{name}
无变化，沿用现有端点。

## 数据模型（如有）

无新增数据模型。governance Config 结构（Go 端 `plugins/governance/main.go:37`）保持不变：

```go
type Config struct {
    IsVkMandatory         *bool     `json:"is_vk_mandatory"`
    RequiredHeaders       *[]string `json:"required_headers"`
    DisableAutoToolInject *bool     `json:"disable_auto_tool_inject"`
    RoutingChainMaxDepth  *int      `json:"routing_chain_max_depth"`
}
```

前端 zod schema（`ui/lib/types/plugins.ts`）：

```typescript
export const governanceConfigSchema = z.object({
  is_vk_mandatory: z.boolean().optional(),
  required_headers: z.array(z.string()).optional(),
  disable_auto_tool_inject: z.boolean().optional(),
  routing_chain_max_depth: z.number().int().min(1).max(100).optional(),
});

export const GOVERNANCE_PLUGIN = "governance";
export const OTEL_PLUGIN = "otel";
```

## 组件设计（如有）

### governanceFragment.tsx（新建）

| 子组件 | 职责 | 主要 hook |
|--------|------|----------|
| `EnabledSwitch` | 顶部独立 enabled toggle | useUpdatePluginMutation |
| `ConfigForm` | 4 字段表单 + 提交 | useForm + useUpdatePluginMutation |
| `GovernanceFragment`（默认导出） | 组合 EnabledSwitch + ConfigForm | — |

```tsx
// 组件结构（与 rtkFragment.tsx 完全一致的模式）
export function EnabledSwitch({ plugin }: { plugin: Plugin }) { ... }
export function ConfigForm({ plugin }: { plugin: Plugin }) {
  const form = useForm<GovernanceFormValues>({
    resolver: zodResolver(governanceConfigSchema),
    defaultValues: {
      is_vk_mandatory: pluginConfig.is_vk_mandatory ?? false,
      required_headers: pluginConfig.required_headers ?? [],
      disable_auto_tool_inject: pluginConfig.disable_auto_tool_inject ?? false,
      routing_chain_max_depth: pluginConfig.routing_chain_max_depth ?? 5,
    },
  });
  // 2 个 fieldset: "访问控制" + "行为"
  // 提交: useUpdatePluginMutation({ config: values })
}
export default function GovernanceFragment({ plugin }: { plugin: Plugin }) {
  return <div>
    <EnabledSwitch plugin={plugin} />
    <ConfigForm plugin={plugin} />
  </div>;
}
```

### TagInput 组件选择

需求：`required_headers` 是 `*[]string`，UI 要支持逐项添加/删除 header 名称。

候选方案：
- (A) 用 Badge + Input 组合，每项渲染 Badge + 关闭按钮，底部 Input 添加新项
- (B) 用 react-select 的多选模式（项目已有依赖？）
- (C) 用项目已有的 TagInput 组件（检查 `ui/components/ui/`）

本次采用 **(A)**：直接用项目已有的 Badge + Input 组合，简单可控。如果发现 ui/components 已有 TagInput，则切换。

### pluginsView.tsx 路由接入

在 `pluginsView.tsx:163` 后插入：

```tsx
if (selectedPlugin.name === GOVERNANCE_PLUGIN) {
  return (
    <div className="ml-4 w-full">
      <GovernanceFragment plugin={selectedPlugin} />
    </div>
  );
}
if (selectedPlugin.name === OTEL_PLUGIN) {
  return (
    <div className="ml-4 w-full">
      <OtelView plugin={selectedPlugin} />
    </div>
  );
}
```

### OtelView 复用

`ui/app/workspace/observability/views/plugins/otelView.tsx` 当前签名接收 plugin 参数。需要在 pluginsView.tsx 中 import 它并传入 `selectedPlugin`。

## 关键约束与契约

### 前置条件
- 无新依赖、无后端变更、无 schema 变更
- 现有 `useUpdatePluginMutation` 已支持任意 plugin name 的 PUT
- 项目已有 i18n（en + zh-CN）+ react-hook-form + zod，无需新增依赖

### 影响面

| 影响面 | 详情 |
|--------|------|
| 新增文件 | `ui/app/workspace/plugins/fragments/governanceFragment.tsx` |
| 修改文件 | `ui/app/workspace/plugins/views/pluginsView.tsx`（+2 个 if 分支） |
| 修改文件 | `ui/lib/types/plugins.ts`（+governanceConfigSchema + GOVERNANCE_PLUGIN + OTEL_PLUGIN） |
| 修改文件 | `ui/locales/en/plugins.json`（+governance 段） |
| 修改文件 | `ui/locales/zh-CN/plugins.json`（+governance 段） |
| 不影响 | governance 后端 Go 代码 |
| 不影响 | VirtualKeys / Routing Rules 等其它工作区页面 |
| 不影响 | PluginSpanFilter（otel 复用保留 merge 行为） |

### 性能契约
- 无新增 HTTP 请求（沿用现有 PUT /api/plugins/{name}）
- 无新增数据库查询
- 无新增 SSE/WebSocket 连接

### 错误码与编号段
- 无新增错误码（沿用现有 permission_denied / config_invalid）

### 环境限制与验证策略

依据 `.pg/changes/add-plugins-governance-form/env-description.yaml`（pg-propose 1.6 产出）：

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-ui-1 governance 表单渲染 | ✅ | scenario（browser type=browser）+ vitest jsdom 单元测试 | n/a |
| V-ui-2 governance 表单提交与持久化 | ✅ | scenario（api type=api，PUT /api/plugins/governance）+ vitest mock 单元测试 | n/a |
| V-ui-3 otel 复用渲染 | ✅ | scenario（browser type=browser） | n/a |
| V-ui-4 i18n 双语切换 | ✅ | scenario（browser type=browser）+ vitest jsdom 翻译键存在性测试 | n/a |

资源引用：
- `{env.business_systems[name=ui-dev].endpoints[name=dev].url}` → `http://localhost:3008`
- `{env.business_systems[name=pg-gateway-api].endpoints[name=api].url}` → `http://localhost:9080`
- `{env.data_resources[name=fixture-plugins]}` → 2 行 seed plugin（含 governance）

### 可观测性
- 沿用现有 sonner toast（success/error）
- 无需新增日志点或指标

## Verification Criteria

### dev ui Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | governance 表单渲染：在 /workspace/plugins?plugin=governance 下选中 governance，右侧渲染 EnabledSwitch + ConfigForm，不再展示 JSON Editor | fixture-plugins 含 governance；ui-dev 启动 | Chrome DevTools MCP 访问 `http://localhost:3008/workspace/plugins?plugin=governance`，断言 `data-testid="governance-fragment"` 存在 + `data-testid="json-editor"` 不存在 | 表单渲染正确 |
| V-ui-2 | governance 表单提交：修改 is_vk_mandatory 后点 Save，触发 PUT /api/plugins/governance 并返回 200，UI 收到 success toast | fixture-plugins 含 governance；pg-gateway-api 启动；admin 登录态 | Chrome DevTools MCP 切换 Switch + click `data-testid="governance-save-button"` + 监听 network PUT /api/plugins/governance + 断言 `data-testid="governance-toast-success"` 出现 | PUT 200 + toast |
| V-ui-3 | otel 复用渲染：在 /workspace/plugins?plugin=otel 下渲染出 otelFormFragment 内容（Profile 列表 + 字段） | fixture-plugins 含 otel；ui-dev 启动 | Chrome DevTools MCP 访问 `http://localhost:3008/workspace/plugins?plugin=otel`，断言 `data-testid="otel-fragment"` 存在 | otel 表单渲染正确 |
| V-ui-4 | i18n 双语切换：governance fragment 在 zh-CN 下 label/description 显示中文，无「令牌」误用 | fixture-plugins 含 governance；ui-dev 启动 | Chrome DevTools MCP 设置 localStorage.pg-gateway.locale = "zh-CN" + reload + 断言 `data-testid="governance-field-is-vk-mandatory-label"]` innerText 含 "强制要求虚拟密钥" | 翻译正确 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 无后端引擎改动 |
| framework | ❌ | 无 framework 改动 |
| transports | ❌ | 无 transports handler 改动（沿用现有 PUT /api/plugins/{name}） |
| plugins | ❌ | 无 Go plugin 代码改动 |
| ui | ✅ | 新建 governanceFragment.tsx、修改 pluginsView.tsx / types/plugins.ts / locale 文件 |
| scr | ❌ | 单模块（ui）UI 任务，无需跨模块联调 scenario |

**affected_tracks**：`[ui]`

**scenario_tracks_decision**：
- `scr: enabled=false`
  - 跨 role 协作验证？否（纯 UI track，无后端改动）
  - 新 API 端点？否（沿用现有 PUT /api/plugins/{name}）
  - 跨模块联调？否（仅 ui 模块）

> **注意**：本任务仅触发 scenario-ui.yaml 中的 scenario 章节（受 ui track 影响），无需独立 scenario-scr.yaml。

### V-* track 归属说明

本任务中所有 V-* 均为 `V-ui-*`（受影响 track 是 ui），统一归属到 ui Verification Criteria 章节，与 affected_tracks `[ui]` 一致。
