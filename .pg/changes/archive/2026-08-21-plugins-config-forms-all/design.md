# plugins-config-forms-all 设计

## 架构概览

本次变更涉及 3 个模块、跨前后端，无新增 REST 端点，沿用现有 `PUT /api/plugins/{name}` 契约。

```
┌──────────────────────────────────────────────────────────────────────────┐
│  ui/app/workspace/plugins/                                               │
│  ├── page.tsx                                                            │
│  ├── views/pluginsView.tsx ← 散转逻辑（+ 7 个新 plugin 名称 → 片段）     │
│  └── fragments/                                                          │
│      ├── governanceFragment.tsx         (已有)                           │
│      ├── providercooldownFragment.tsx   (已有)                           │
│      ├── rtkFragment.tsx                (已有)                           │
│      ├── loggingFragment.tsx            (新增)                           │
│      ├── semanticCacheFragment.tsx      (新增)                           │
│      ├── mockerFragment.tsx             (新增)                           │
│      ├── compatFragment.tsx             (新增)                           │
│      ├── promptsFragment.tsx            (新增 - 占位)                    │
│      ├── modelcatalogresolverFragment.tsx (新增 - 占位)                  │
│      └── jsonparserFragment.tsx         (新增 - 占位)                    │
├──────────────────────────────────────────────────────────────────────────┤
│  ui/lib/types/plugins.ts ← 4 个 Zod schema + 1 个 i18n 标签映射          │
│  ui/locales/{en,zh-CN}/plugins.json ← 49 个键值对                        │
├──────────────────────────────────────────────────────────────────────────┤
│  transports/config.schema.json ← schema 同步                              │
│  ├── governance: 补齐 disable_auto_tool_inject / routing_chain_max_depth │
│  ├── mocker: 新增 schema 段（含 allOf 条件）                             │
│  ├── logging / semantic_cache / compat: 校对字段名 / enum / allOf        │
├──────────────────────────────────────────────────────────────────────────┤
│  plugins/{logging,semanticcache,mocker,compat}/                          │
│  └── 单元测试覆盖（UnmarshalJSON 默认值、Init、validateXxxConfig）        │
└──────────────────────────────────────────────────────────────────────────┘
```

**数据流**：

```
[User] → 浏览器 /workspace/plugins → 点击插件 → 加载 fragment
       → react-hook-form 渲染表单 / Monaco JSON 渲染
       → 用户编辑 → 提交
       → useUpdatePluginMutation({name, data: {enabled, config}})
       → PUT /api/plugins/{name}  (复用)
       → plugins.go updatePlugin (现有逻辑，merge config)
       → SQLite configstore
       → Response → Redux invalidate getPlugins → 重新拉取
```

## API 设计（如有）

本变更**沿用现有 API**，无新增端点。涉及的 PUT 端点：

### PUT /api/plugins/{name} - Request Body

```json
{
  "enabled": true,
  "config": {
    "disable_content_logging": false,
    "retain_content_in_object_storage": false,
    "allow_per_request_content_storage_override": false,
    "logging_headers": ["x-bf-vk", "x-bf-request-id"]
  }
}
```

### PUT /api/plugins/{name} - Response Body (200)

```json
{
  "message": "Plugin updated successfully",
  "plugin": {
    "name": "logging",
    "actualName": "logging",
    "enabled": true,
    "config": {
      "disable_content_logging": false,
      "retain_content_in_object_storage": false,
      "allow_per_request_content_storage_override": false,
      "logging_headers": ["x-bf-vk", "x-bf-request-id"]
    },
    "isCustom": false,
    "path": null,
    "status": {
      "name": "logging",
      "status": "active",
      "logs": [],
      "types": ["preLLMHook", "postLLMHook"]
    }
  }
}
```

### PUT /api/plugins/{name} - Response Body (4xx 失败)

```json
{
  "error": {
    "code": "INVALID_PLUGIN_CONFIG",
    "message": "validation failed: dimension must be >= 2 when provider is set",
    "extra": {
      "field": "dimension",
      "reason": "provider_set_requires_dimension_2"
    }
  }
}
```

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | 更新成功 | 0 |
| 400 | 字段校验失败（如 semantic_cache dimension 不满足 allOf 条件） | INVALID_PLUGIN_CONFIG |
| 404 | 插件不存在 | PLUGIN_NOT_FOUND |
| 500 | 后端 storage 写入失败 | STORAGE_ERROR |

## 数据模型（如有）

**不新增数据库表 / 字段**。沿用 `table_plugin` 表（已有结构）：

```sql
-- 现有 schema, 无变更
CREATE TABLE plugins (
  name TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL DEFAULT 0,
  config TEXT NOT NULL DEFAULT '{}',  -- JSON 字符串
  placement TEXT,
  orders INTEGER,
  is_custom INTEGER DEFAULT 0,
  path TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## 组件设计（如有）

### 1. loggingFragment.tsx

```typescript
useForm<LoggingConfig>({
  resolver: zodResolver(loggingConfigSchema),
  defaultValues: {
    disable_content_logging: plugin.config.disable_content_logging ?? false,
    retain_content_in_object_storage: plugin.config.retain_content_in_object_storage ?? false,
    allow_per_request_content_storage_override: plugin.config.allow_per_request_content_storage_override ?? false,
    logging_headers: plugin.config.logging_headers ?? [],
  },
});
```

字段：
- `disable_content_logging` (Switch)
- `retain_content_in_object_storage` (Switch，依赖 `LogsStore.ObjectStorage` 配置)
- `allow_per_request_content_storage_override` (Switch)
- `logging_headers` (TagInput，多字符串)

### 2. semanticCacheFragment.tsx（条件联动）

```typescript
const provider = form.watch("provider");
const hasProvider = !!provider && provider.length > 0;

form.setValue("embedding_model", "", { shouldValidate: false });
form.setValue("dimension", hasProvider ? 2 : 1, { shouldValidate: true });
```

字段：
- `provider` (Select，下拉 21 个 provider enum)
- `embedding_model` (Input，hasProvider 时必填)
- `dimension` (Number，hasProvider 时 ≥2，否则锁定 1)
- `ttl` (Input，支持 "5m" / 300 两种格式)
- `threshold` (Slider, 0-1)
- `vector_store_namespace` (Input)
- `default_cache_key` (Input)
- `conversation_history_threshold` (Number, ≥0)
- `cache_by_model` (Switch, 默认 true)
- `cache_by_provider` (Switch, 默认 true)
- `exclude_system_prompt` (Switch, 默认 false)

### 3. compatFragment.tsx（4 toggle）

```typescript
useForm<CompatConfig>({
  resolver: zodResolver(compatConfigSchema),
  defaultValues: {
    convert_text_to_chat: plugin.config.convert_text_to_chat ?? true,
    convert_chat_to_responses: plugin.config.convert_chat_to_responses ?? true,
    should_drop_params: plugin.config.should_drop_params ?? true,
    should_convert_params: plugin.config.should_convert_params ?? false,
  },
});
```

字段（4 个 Switch）：
- `convert_text_to_chat` (默认 true)
- `convert_chat_to_responses` (默认 true)
- `should_drop_params` (默认 true)
- `should_convert_params` (默认 false)

> 注意：compat 配置从 `client_config.compat` 读取，需在 `useGetPluginsQuery` 兼容层解析后映射到 fragment。

### 4. mockerFragment.tsx（Monaco JSON 编辑器）

```typescript
const [jsonText, setJsonText] = useState(JSON.stringify(plugin.config ?? {}, null, 2));
const [parsedError, setParsedError] = useState<string | null>(null);

function validate(text: string): { ok: boolean; data?: MockerConfig } {
  try {
    const data = JSON.parse(text);
    const result = mockerConfigSchema.safeParse(data);
    if (!result.success) {
      setParsedError(result.error.errors[0].message);
      return { ok: false };
    }
    setParsedError(null);
    return { ok: true, data: result.data };
  } catch (e) {
    setParsedError("Invalid JSON: " + e.message);
    return { ok: false };
  }
}
```

字段：单一 Monaco 编辑器（JSON 模式），实时校验 Zod 错误提示。

### 5. prompts / modelcatalogresolver / jsonparser Fragment（占位卡）

```typescript
export function PromptsFragment() {
  return (
    <Card>
      <Title>Prompts</Title>
      <Description>此插件无配置项。所有提示词管理通过 Prompts CRUD 页面完成：/workspace/prompts</Description>
      <Button onClick={() => navigate('/workspace/prompts')}>前往 Prompts 页面</Button>
    </Card>
  );
}
```

### 6. pluginsView.tsx 散转逻辑扩展

```typescript
const pluginFragmentMap: Record<string, React.FC<...>> = {
  // existing
  governance: GovernanceFragment,
  'provider-cooldown': ProvidercooldownFragment,
  rtk: RtkFragment,
  // new
  logging: LoggingFragment,
  semantic_cache: SemanticCacheFragment,
  mocker: MockerFragment,
  compat: CompatFragment,
  // placeholder
  prompts: PromptsFragment,
  modelcatalogresolver: ModelcatalogresolverFragment,
  jsonparser: JsonparserFragment,
};
```

## 关键约束与契约

### 前置条件

- `transports/config.schema.json` 同步更新（包括 governance 字段补齐 + mocker schema 新增）
- `ui/lib/types/plugins.ts` 中 4 个 Zod schema + 1 个 i18n 标签映射必须先就绪
- `ui/locales/en/plugins.json` 和 `ui/locales/zh-CN/plugins.json` 49 个键值对必须同步就绪
- `pluginsView.tsx` 散转逻辑必须先扩展，否则 fragment 不会被加载

### 影响面

- **新增文件**：
  - `ui/app/workspace/plugins/fragments/loggingFragment.tsx`
  - `ui/app/workspace/plugins/fragments/semanticCacheFragment.tsx`
  - `ui/app/workspace/plugins/fragments/mockerFragment.tsx`
  - `ui/app/workspace/plugins/fragments/compatFragment.tsx`
  - `ui/app/workspace/plugins/fragments/promptsFragment.tsx`
  - `ui/app/workspace/plugins/fragments/modelcatalogresolverFragment.tsx`
  - `ui/app/workspace/plugins/fragments/jsonparserFragment.tsx`
- **修改文件**：
  - `ui/app/workspace/plugins/views/pluginsView.tsx`（散转逻辑 + 7 个 import）
  - `ui/lib/types/plugins.ts`（4 个 Zod schema + 1 个标签映射）
  - `ui/locales/en/plugins.json`（49 个键）
  - `ui/locales/zh-CN/plugins.json`（49 个键）
  - `transports/config.schema.json`（schema 同步）
- **不破坏任何对外 API**：沿用 `PUT /api/plugins/{name}` 契约

### 性能契约

- Monaco 渲染 mocker JSON 时需虚拟化（>1000 行不卡顿）
- i18n 加载：49 个键值对通过 locale JSON 静态加载，无运行时开销
- API 调用：每次 save 1 次 PUT /api/plugins/{name}，无 N+1

### 错误码与编号段

- `INVALID_PLUGIN_CONFIG`（400）— config 字段校验失败
- `PLUGIN_NOT_FOUND`（404）— 插件不存在
- `STORAGE_ERROR`（500）— 后端 storage 写入失败

### 可观测性

- 关键日志点：fragment 加载成功 / Zod 校验失败 / PUT 请求超时
- 关键指标：fragment 加载耗时、save 成功率
- RequestId 追踪：复用 `BifrostContextKeyRequestID`

### 环境限制与验证策略

> **必填**（v0.9.0 新增）。依据 `.pg/changes/plugins-config-forms-all/env-description.yaml` 中目标 env `local` 的 6 段判断。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-ui-1 logging 表单字段渲染与保存 | ✅ | scenario + dev 单元测试 | n/a |
| V-ui-2 semantic_cache 条件字段联动 | ✅ | scenario 浏览器 + jsdom 单测 | n/a |
| V-ui-3 mocker JSON 编辑器 + Zod 校验 | ✅ | scenario + dev 单元测试 | n/a |
| V-ui-4 compat 4 toggle 开关 | ✅ | scenario + dev 单元测试 | n/a |
| V-ui-5 3 个占位卡片可访问 | ✅ | scenario 浏览器 | n/a |
| V-ui-6 中英文 i18n 键值对齐 | ✅ | dev 静态扫描 + scenario | n/a |
| V-ui-7 浏览器采证：5 个表单 dev server 可加载 | ✅ | scenario 浏览器 + DevTools | n/a |
| V-transports-1 schema 与 Go config struct 同步 | ✅ | dev 单元测试（ajv 校验） | n/a |
| V-plugins-1 Go 单元测试覆盖 | ✅ | dev Go 单元测试 | n/a |

**V-* 命名规范**：
- 编号格式：`V-{track_id}-{seq}`（如 `V-ui-1`）
- 同一 design.md 中不允许混用连字符和下划线
- 每个 V-* 必须在 `## Verification Criteria` 表中出现且仅出现一次
- 本节引用的 V-* 与 "Verification Criteria" 表中的 ID **完全一致**

## Verification Criteria

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | logging 表单字段渲染与 PUT 保存 | `useGetPluginsQuery` 返回 `logging` 配置 | 浏览器访问 /workspace/plugins/logging，点击 Edit → 切换每个 Switch → 提交 → 验证 PUT /api/plugins/logging 返回 200 + Redux cache 刷新 | 4 字段值与提交一致；reload 后字段不变 |
| V-ui-2 | semantic_cache 条件字段联动 | `useGetPluginsQuery` 返回 `semantic_cache` 配置 | 浏览器 /workspace/plugins/semantic_cache：选 provider → embedding_model 变必填、dimension 最小值 2；清空 provider → embedding_model 禁用、dimension 锁定 1 | 3 个字段联动行为符合 allOf 规则 |
| V-ui-3 | mocker JSON 编辑器 + Zod 校验 | `useGetPluginsQuery` 返回 `mocker` 配置（若有） | 浏览器 /workspace/plugins/mocker：输入合法 JSON → 实时校验通过；输入 invalid JSON → 错误提示，不允许提交 | 编辑器渲染 + Zod 校验流程正确 |
| V-ui-4 | compat 4 toggle 开关 | client_config.compat 配置 | 浏览器 /workspace/plugins/compat：切换 4 个 Switch → 提交 → 验证 PUT /api/plugins/compat | 4 字段值与提交一致 |
| V-ui-5 | 3 个占位卡片可访问 | 无 | 浏览器 /workspace/plugins，分别点击 prompts / modelcatalogresolver / jsonparser | 看到对应占位说明卡 + 跳转按钮 |
| V-ui-6 | 中英文 i18n 键值对齐 | 无 | 切换 UI 运行时 locale 到 zh-CN，刷新页面 | 49 个新增键全部正常显示 |
| V-ui-7 | 浏览器采证：5 个表单 dev server 可加载 | ui-dev 服务运行 | Chrome DevTools 访问 /workspace/plugins | 侧边栏出现 7 个新插件入口，点击后能加载对应 fragment |

### dev transports Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | config.schema.json 与 Go config struct 同步 | fixture 配置样例 | ajv 校验 `config.schema.json` 接受 fixture 配置 | 无 validation error；governance 缺字段错误消失 |

### dev plugins Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-plugins-1 | Go 单元测试覆盖 | 无 | `make test-plugins` | logging / semanticcache / mocker / compat 4 个 module 单元测试全绿 |

### int scr Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-scr-1 | 端到端：编辑 logging 表单 → configstore 持久化 | local env 启动 + SQLite configstore | Playwright 跑完整流程：访问 /workspace/plugins/logging → 切换 Switch → 提交 → 验证 GET /api/plugins/logging 返回新值 → 验证 SQLite config.db 中 plugin.config 字段已更新 | 状态一致、reload 页面字段不变 |
| V-scr-2 | 端到端：编辑 semantic_cache 条件规则 | local env 启动 | Playwright：选 provider → 验证 embedding_model 必填 → 提交 → 验证后端 schema 校验通过 | 联动行为 + 后端 schema 校验一致 |
| V-scr-3 | 端到端：mocker JSON 编辑器保存 | local env 启动 | Playwright：输入合法 JSON → 提交 → 验证后端 SQLite 存了新 JSON | 编辑器 → 后端 → DB 一致 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 核心引擎无改动 |
| framework | ❌ | 框架层无改动 |
| transports | ✅ | config.schema.json 同步（governance 字段补齐 + mocker schema 新增） |
| plugins | ✅ | packages/logging, semanticcache, mocker, compat 单元测试覆盖 |
| ui | ✅ | 7 个 fragment + 4 个 Zod schema + 49 个 i18n 键 + pluginsView 散转 |
| scr | ✅ | 跨模块联调（浏览器 + dev server + REST API） |

**affected_tracks**：`[ui, transports, plugins]`

**scenario_tracks_decision**：
- `scr` track：`enabled = true`
- 依据：
  - 跨 role 协作验证？✅ (ui-dev + pg-gateway-api + configstore)
  - 新 API 端点？❌ (沿用现有 PUT /api/plugins/{name})
  - 跨模块联调？✅ (ui ↔ transports ↔ plugins 三 track 联动)
