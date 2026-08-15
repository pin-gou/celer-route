# redesign-providers-pages 设计

## 架构概览

### 涉及模块
- **后端**（`transports` track）：`transports/bifrost-http/handlers/providers.go` 扩展 `ProviderResponse`，新增聚合计算逻辑；`transports/bifrost-http/handlers/provider_keys.go` 新增批量 handler
- **数据层**：复用现有 `configstore`（SQLite config.db）—— 读 keys/models 数量；复用 `logstore`（SQLite logs.db）—— 读当日请求/错误/最近时间戳
- **前端**（`ui` track）：新建 `ui/app/workspace/providers2/` 路由目录，复用 `ui/lib/constants/icons.tsx` 图标，复用 `ui/lib/store/apis/providersApi.ts` RTK Query（扩展 9 个聚合字段类型 + 新增 `useBatchUpdateProviderKeysMutation`），复用 `ProviderConfigSheet` 作为后备配置入口

### 数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│ Browser (Vite dev :3008)                                            │
│   /workspace/providers2  ─┐                                        │
│   /workspace/providers2/:id ─┤ TanStack Router                     │
└──────┬──────────────────────┴───────────────────────────────┬─────┘
       │ useGetProvidersQuery()                                │
       │ useGetProviderQuery(name)                              │
       │ useBatchUpdateProviderKeysMutation({provider, keys, enabled})
       ▼                                                        │
┌─────────────────────────────────────────────────────────────────────┐
│ Bifrost HTTP API (:9080)                                            │
│   GET /api/providers        ─┐                                     │
│   GET /api/providers/:name  ─┤  listProviders/getProvider         │
│   POST .../keys/batch       ─┘  batchUpdateProviderKeys (NEW)     │
└──────┬─────────────────────────────┬───────────────────────────────┘
       │                             │
       ▼                             ▼
┌──────────────────┐          ┌──────────────────┐
│ config.db        │          │ logs.db          │
│  - providers     │          │  - request_logs  │
│  - keys (COUNT)  │          │  - SUM today     │
│  - models (COUNT)│          │  - MAX(ts)       │
└──────────────────┘          └──────────────────┘
```

### 前端组件层级

```
ui/app/workspace/providers2/
├── layout.tsx                       # createFileRoute 路由
├── page.tsx                         # 列表页主组件
├── views/
│   ├── ProviderFamilyGroup.tsx      # 单个厂商家族的 section
│   ├── ProviderCard.tsx             # 单张 provider 卡片
│   ├── ProviderFilters.tsx          # 搜索 + provider 多选 + 健康度筛选
│   └── useProviders2Data.ts         # 数据聚合 hook
├── dialogs/
│   └── TryLegacyViewButton.tsx      # "Try legacy view" 跳转按钮
└── [id]/
    ├── layout.tsx                   # /workspace/providers2/:id 路由
    ├── page.tsx                     # 详情页主组件（Tabs 容器）
    └── views/
        ├── OverviewTab.tsx          # Network/Proxy/Performance/... 全部内联编辑
        ├── KeysTab.tsx              # 复用 modelProviderKeysTableView
        ├── ModelsTab.tsx            # 复用 + sync 按钮
        ├── UsageTab.tsx             # 今日/近 7 日聚合数字
        ├── GovernanceTab.tsx        # 复用 providerGovernanceTable
        └── LogsTab.tsx              # 仅跳转入口
```

## API 设计

### GET /api/providers（修改：扩展响应字段）

### GET /api/providers - Response Body (200)
```json
{
  "providers": [
    {
      "name": "openai",
      "network_config": { "...": "..." },
      "concurrency_and_buffer_size": { "...": "..." },
      "proxy_config": null,
      "send_back_raw_request": false,
      "send_back_raw_response": false,
      "store_raw_request_response": false,
      "custom_provider_config": null,
      "openai_config": null,
      "provider_status": "active",
      "status": "",
      "description": "",
      "config_hash": "",
      "keys_count": 3,
      "models_count": 47,
      "keys_health_status": "healthy",
      "today_requests": 1284,
      "today_errors": 3,
      "last_used_at": "2026-08-15T01:42:00Z",
      "last_error_at": "2026-08-15T00:15:22Z",
      "uptime": 0.998,
      "avg_latency_ms": 312
    }
  ],
  "total": 5
}
```

新增字段说明（追加，不破坏现有契约）：

| 字段 | 类型 | 含义 | 数据来源 |
|------|------|------|----------|
| `keys_count` | int | 该 provider 下 key 总数 | `config-db.keys WHERE provider=?` |
| `models_count` | int | 该 provider 模型总数 | `config-db.models WHERE provider=?` |
| `keys_health_status` | string | `"healthy"` / `"degraded"` / `"unknown"` | 聚合所有 key 的 `list_models_failed` 标志 |
| `today_requests` | int | 当日请求数（UTC 日界） | `logs-db.request_logs WHERE provider=? AND ts>=<today_start>` |
| `today_errors` | int | 当日错误数 | 同上 + status>=400 |
| `last_used_at` | string RFC3339 | 最近一次成功请求时间 | `MAX(ts) WHERE provider=? AND status<400` |
| `last_error_at` | string RFC3339 | 最近一次错误请求时间 | `MAX(ts) WHERE provider=? AND status>=400` |
| `uptime` | float (0~1) | 近 24h 健康比例 | `1 - today_errors/today_requests`（空数据=1） |
| `avg_latency_ms` | int | 近 24h 平均延迟 | `AVG(latency_ms) WHERE provider=? AND ts>=<24h_ago>` |

### POST /api/providers/{provider}/keys/batch（新增）

### POST /api/providers/{provider}/keys/batch - Request Body
```json
{
  "key_ids": ["key-1", "key-2", "key-3"],
  "enabled": true
}
```

### POST /api/providers/{provider}/keys/batch - Response Body (200)
```json
{
  "updated": 3,
  "key_ids": ["key-1", "key-2", "key-3"]
}
```

### POST /api/providers/{provider}/keys/batch - Response Body (4xx)
```json
{
  "error": "batch_update_failed",
  "message": "key_ids [key-2] not found for provider openai",
  "missing_key_ids": ["key-2"]
}
```

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | 全部 key 成功更新 | 0 |
| 400 | 任何 key_id 不属于该 provider | `batch_update_failed`（事务回滚） |
| 404 | provider 不存在 | `provider_not_found` |
| 500 | DB 错误 | `internal_error` |

## 数据模型

无 schema 变更。聚合字段全部为只读计算，不落库。

## 组件设计

### 列表页交互

```
┌─ Toolbar ──────────────────────────────────────────────┐
│ [Search...]  [Provider ▾]  [Health: All|Active|Error] │
│ [+ Add Provider ▾]                       [Compact Mode]│
├─ OpenAI Family ────────────────────────────────────────┤
│ ┌─ openai ─────────────────┐  ┌─ openai-custom ───────┐│
│ │ ● active  3 keys 47 mods │  │ ● active  1 key 5 mods ││
│ │ 1284 reqs  3 errs        │  │  0 reqs                ││
│ │ last err: 2h ago         │  │  [Test]  [Toggle ●/○] ││
│ │ [Test]  [Toggle ●/○]     │  └────────────────────────┘│
│ └──────────────────────────┘                            │
├─ Anthropic Family ─────────────────────────────────────┤
│ ...                                                    │
└─ [Try legacy view →] ──────────────────────────────────┘
```

### 详情页结构

```
┌─ Breadcrumb: Providers (New) > openai ─────────────────┐
│ ● active  openai                              [Legacy ↗]│
├─ Tabs: [Overview] [Keys] [Models] [Usage] [Gov.] [Logs]─┤
├─ Overview Tab (default) ────────────────────────────────┤
│  ┌─ Network ─────────┐  ┌─ Proxy ──────────┐            │
│  │ Base URL          │  │ HTTP Proxy       │            │
│  │ Max Conn Per Host │  │ ...              │            │
│  │ [Edit]            │  │ [Edit]           │            │
│  └───────────────────┘  └──────────────────┘            │
│  ┌─ Performance ─────────────────────────────────┐      │
│  │ Concurrency / Buffer Size  [Edit]             │      │
│  └───────────────────────────────────────────────�      │
│  ┌─ Governance (if hasGovernanceAccess) ─────────┐      │
│  │ Budget / Rate Limits  [Edit]                  │      │
│  └───────────────────────────────────────────────┘      │
│  ┌─ Beta Headers (anthropic family) ─────────────┐      │
│  └───────────────────────────────────────────────┘      │
│  ┌─ OpenAI Config (openai only) ─────────────────┐      │
│  └───────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────┘
```

### 厂商家族分组映射

| Family | Providers |
|--------|-----------|
| OpenAI Family | openai, openai-custom |
| Anthropic Family | anthropic |
| Google Family | gemini, vertex |
| Meta-Llama Family | groq, cerebras, ollama, perplexity, openrouter, parasail, nebius, xai, sgl |
| AWS Family | bedrock |
| Custom | 所有 `custom_provider_config != null` 的 provider |
| Other | minimax, mistral, cohere, huggingface, replicate, elevenlabs 等 |

## 关键约束与契约

### 前置条件
- bifrost-api 必须在 `:9080` 端口可访问
- config-db / logs-db 必须已 seed（`fixture-providers` / `fixture-keys` / `logs.db` 三个 fixture 必须就绪）
- ui-dev 必须在 `:3008` 端口可访问
- 不需要 DB schema migration（聚合字段为只读计算）

### 影响面
- **后端表/索引/字段变更**：**无 DB 变更**
- **后端 service / handler 签名变更**：
  - `ProviderResponse` struct：append 9 个只读字段（`omitempty`）
  - 新增 handler：`batchUpdateProviderKeys(ctx, provider)`
  - 新增路由：`POST /api/providers/{provider}/keys/batch`
- **后端聚合计算引入的查询**：每个 provider 多 4 次 DB 查询（keys count、models count、today 统计、24h 统计）。30+ provider 时 ~120 次查询/列表请求，可优化为单批查询（GROUP BY provider）
- **是否破坏任何对外 API**：**否**。`ProviderResponse` 字段 append，`omitempty` 兼容；现有 consumer（CLI/SDK）若 strict decode 需更新 struct（已在 V-transports-2 覆盖）

### 性能契约
- `GET /api/providers` 响应时间目标：30 provider 量级 ≤ 200ms
- 聚合查询必须**单查询批拉**（GROUP BY provider），禁止循环 N+1
- 批量 keys 端点单事务执行，禁止 partial success
- `POST /keys/batch` 涉及 keys 数量上限 500

### 错误码与编号段
- 新增错误码：
  - `batch_update_failed` — 批量 keys 更新失败（任何 key_id 不存在）
  - 沿用现有 `provider_not_found` / `internal_error`

### 环境限制与验证策略

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-transports-1 新列表页可访问并列出 provider | ✅ | scenario（type=browser） | n/a |
| V-transports-2 ProviderResponse 聚合字段正确返回 | ✅ | scenario（type=api）+ 单元测试 | logs.db 空时 today_* 默认 0 / last_* 默认 null，前端渲染"无数据" |
| V-transports-3 批量 keys 端点原子工作 | ✅ | scenario（type=api） | n/a |
| V-transports-4 详情页 6 个 Tabs 都能渲染 | ✅ | scenario（type=browser） | Usage Tab 在 logs.db 空时显示占位 |
| V-transports-5 旧路由 /workspace/providers 不受影响 | ✅ | scenario（type=browser）跑旧 E2E | n/a |
| V-transports-6 新旧路由双向跳转按钮可用 | ✅ | scenario（type=browser） | n/a |

### 可观测性
- 关键日志点：`batchUpdateProviderKeys` 入口 INFO log（含 provider + 批量 keys 数量），失败 ERROR log（含 missing_key_ids）
- 关键指标：暂不引入新 Counter/Gauge（聚合字段本身即指标）
- RequestId 追踪：复用现有 BifrostContext，沿链路传递

## Verification Criteria

### dev transports Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | 新列表页可访问并渲染 5 个 fixture provider 的分组卡片 | bifrost-api 在 :9080 启动 + fixture 已 seed | curl GET /api/providers + curl GET /workspace/providers2 HTML | API 返回 5 provider；HTML 包含 "OpenAI Family" 等分组文案 |
| V-transports-2 | ProviderResponse 9 个聚合字段全部返回 | fixture 5 provider 已 seed | curl GET /api/providers | 响应 JSON 每个 provider 含 keys_count / models_count / keys_health_status / today_requests / today_errors / last_used_at / last_error_at / uptime / avg_latency 共 9 字段 |
| V-transports-3 | POST /keys/batch 原子更新 | 选 1 个 fixture provider，临时给其加 3 个 test keys | curl POST .../keys/batch body={"key_ids":[...], "enabled":true} | 返回 200 + updated:3；DB 内 3 个 keys 全部 enabled=true；二次请求 {"enabled":false} 全部回滚 |
| V-transports-4 | 批量端点错误路径：传不存在的 key_id | 同上 | curl POST .../keys/batch body 含不存在 key_id | 返回 400 + missing_key_ids 字段；DB 内事务回滚（其他 key 未受影响） |

### dev ui Verification Criteria
| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-5 | 详情页 6 个 Tabs 都能渲染（Overview 默认） | bifrost-api 在 :9080 + ui-dev 在 :3008 + fixture 已 seed | Playwright 访问 /workspace/providers2/openai 并依次点击 6 个 Tab | 每个 Tab 切换后对应内容区出现，无 console.error；Overview Tab 默认展开 |
| V-transports-6 | 新旧路由双向跳转按钮可用 + 旧路由 E2E 不破坏 | 同上 | Playwright (1) /workspace/providers 顶部点击"Try new view" → URL 变 /workspace/providers2；(2) 新详情页点 "Open legacy view" → URL 变 /workspace/providers?provider=openai；(3) 跑现有 providers E2E | 全部跳转 URL 正确；旧 E2E 100% 通过 |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 仅修改 transports 层 handler + ui 层新页面；core 引擎 / Provider 接口不变 |
| framework | ❌ | 不涉及 configstore / logstore / streaming |
| transports | ✅ | `ProviderResponse` 扩展 9 字段；新增 `POST /api/providers/{provider}/keys/batch` handler |
| cli | ❌ | CLI 不消费聚合字段（若 strict decode 需更新但不在本次范围） |
| plugins | ❌ | 无 plugin 改动 |
| ui | ✅ | 新增 `ui/app/workspace/providers2/` 目录；扩展 `providersApi.ts` 类型与 hooks；导航菜单新增子项 |

**affected_tracks**：`[transports, ui]`

**scenario_tracks_decision**：
- `scr: true` —— 跨 transports + ui 跨模块联调验证（API + 浏览器冒烟 + 旧路由兼容），需端到端 scenario 覆盖
- 启用理由：① 跨 role 协作（transports 后端 + ui 前端）→ 是；② 新 API 端点 `/keys/batch` 需端到端冒烟 → 是；③ 跨模块联调（新页面 + 旧路由并存兼容性）→ 是
