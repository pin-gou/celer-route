# home-free-tier-recommendation 设计

## 架构概览

本次变更涉及 2 个模块：transports（后端 2 个新端点 + config 新段）+ ui（home 页重做）。新增 2 个 REST 端点（沿用现有 Provider/Key 的创建契约，不改后端核心注册路径）。

```
┌──────────────────────────────────────────────────────────────────────────┐
│  ui/app/workspace/home/                                                  │
│  ├── views/homePage.tsx ← 5 个 card 重排：                                │
│  │     保留: endpointCard / systemHealthCard /                            │
│  │           setupStatusBar / providerTopologyCard                        │
│  │     替换: quickStartCard → FreeTierRecommendationCard                  │
│  ├── components/                                                          │
│  │   ├── freeTierRecommendationCard.tsx       (新增 - 推荐主卡)          │
│  │   ├── freeTierOneKeyConfigDialog.tsx       (新增 - 一键填 key 弹窗)    │
│  │   └── bundleApplyCard.tsx                  (新增 - 单个 bundle 卡片)   │
│  ├── hooks/                                                              │
│  │   └── useRecentRoutingRulesQuery.ts        (新增 - 最近路由查询)      │
│  ├── lib/store/apis/                                                     │
│  │   └── catalogApi.ts                         (新增 - /api/catalog RTK) │
│  └── locales/{en,zh-CN}/home.json             (新增 - 25+ 键值)          │
├──────────────────────────────────────────────────────────────────────────┤
│  transports/celer-route-http/                                            │
│  ├── handlers/                                                           │
│  │   ├── catalog.go                            (新增 - /api/catalog/bundles)│
│  │   ├── logs.go                               (新增 - /api/logs/recent-routing-rules)│
│  └── lib/config.go                             (修改 - 解析 remote_catalog 段) │
├──────────────────────────────────────────────────────────────────────────┤
│  transports/config.schema.json                                            │
│  └── 新增 remote_catalog 段: url_template (string),                      │
│      refresh_interval_seconds (int, default 3600),                       │
│      max_bundles (int, default 100),                                      │
│      max_bundle_size_bytes (int, default 1MB),                           │
│      max_provider_models (int, default 50)                                │
├──────────────────────────────────────────────────────────────────────────┤
│  framework/configstore/  (无修改)                                        │
│  plugins/logging/         (无修改)                                         │
└──────────────────────────────────────────────────────────────────────────┘
```

**数据流**：

```
[运营] → 每日 push {base}/bundles/zh-CN.json + en.json (静态 CDN)
        ↓
[后端 goroutine] (每 3600s 一次，ETag 协商) → 缓存到内存 catalogStore
        ↓
[UI 浏览器] GET /api/catalog/bundles?lang=zh-CN (带 If-None-Match)
        ↓
[UI] 渲染 FreeTierRecommendationCard：每个 bundle 一个子卡
        ↓
[用户] 点击「一键配置」→ 弹窗填 key → 串行调
     POST /api/providers → POST /api/providers/{provider}/keys
        ↓
[后端] providers.go + provider_keys.go (现有逻辑，无修改)
        ↓
[UI] 刷新 ProviderTopologyCard + RTK cache invalidate

[并发] UI 调 GET /api/logs/recent-routing-rules?limit=100
     → 后端查 logs.db 最近 100 条 → 按 routing_rule_id 去重 → 返回 (id, name, last_used_at)
     → 每个 bundle 卡片底部展示前 3 条，点击跳 /workspace/routing-rules/$id
```

## API 设计（如有）

### GET /api/catalog/bundles - Request

无 Request Body。Query 参数：

| 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|
| lang | string | 否 | 语种代码（zh-CN / en），缺省回退 en |

请求头：
- `If-None-Match: "<etag>"`（可选，客户端缓存协商）

### GET /api/catalog/bundles - Response Body (200)

```json
{
  "bundles": [
    {
      "id": "coding",
      "title": "编程开发",
      "description": "代码补全与调试首选",
      "providers": [
        {
          "provider": "openai",
          "models": ["gpt-4o-mini", "gpt-4.1"],
          "apply_url": "https://platform.openai.com/signup",
          "apply_steps": ["注册账号", "申请 API Key", "回到此处填入"],
          "is_keyless": false,
          "notes": "新用户首月 $5 免费额度"
        },
        {
          "provider": "opencode",
          "models": ["default"],
          "apply_url": "",
          "apply_steps": [],
          "is_keyless": true,
          "notes": "免 Key, 直接添加"
        }
      ]
    }
  ],
  "updated_at": "2026-08-28T08:00:00Z",
  "version": "2026-08-28"
}
```

响应头：
- `ETag: "abc123..."`（基于内存快照 hash 派生）

### GET /api/catalog/bundles - Response Body (200, 空)

```json
{
  "bundles": [],
  "updated_at": null,
  "version": null
}
```

### GET /api/catalog/bundles - Response Body (304)

无 Body，仅 `ETag` 头。

### GET /api/catalog/bundles - Response Body (5xx 不返回)

任何上游 / 解析失败一律返回 200 + 空 bundles，**不返回 5xx**。前端靠 `bundles.length == 0` 渲染空状态。

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | 任意上游/解析结果，含空 bundles | 0 |
| 304 | 客户端带 `If-None-Match` 且匹配当前 ETag | n/a |

---

### GET /api/logs/recent-routing-rules - Request

Query 参数：

| 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 取最近多少条日志（1-1000），默认 100 |

### GET /api/logs/recent-routing-rules - Response Body (200)

```json
{
  "rules": [
    {
      "id": "rr-uuid-1",
      "name": "pg-master",
      "last_used_at": "2026-08-28T07:45:12Z",
      "use_count": 42
    },
    {
      "id": "rr-uuid-2",
      "name": "hermes-default",
      "last_used_at": "2026-08-28T07:30:00Z",
      "use_count": 18
    }
  ]
}
```

### GET /api/logs/recent-routing-rules - Response Body (200, 无规则)

```json
{
  "rules": []
}
```

### GET /api/logs/recent-routing-rules - Response Body (4xx 失败)

```json
{
  "error": {
    "code": "INVALID_LIMIT",
    "message": "limit must be between 1 and 1000",
    "extra": {
      "field": "limit",
      "given": 99999
    }
  }
}
```

| 状态码 | 触发条件 | 错误码 |
|--------|----------|--------|
| 200 | 任意查询结果 | 0 |
| 400 | `limit` 超出 [1, 1000] | INVALID_LIMIT |
| 500 | logs.db 查询失败 | LOGS_QUERY_FAILED |

## 数据模型（如有）

**不新增数据库表 / 字段**。沿用现有：
- `logs.db` 中的请求日志表（已有 `routing_rule_id` / `routing_rule_name` 字段需确认；如缺则日志行记 `null`，聚合返回空数组）
- `config.db` 中的 providers / keys / routing_rules 表（无修改）

新增 **内存数据结构**（在 transports 模块内）：

```go
// transports/celer-route-http/handlers/catalog.go
type bundleCatalog struct {
    mu       sync.RWMutex
    bundles  map[string]map[string]*bundleSnapshot  // [lang][version]
    etags    map[string]string
    fetchedAt map[string]time.Time
}

type bundleSnapshot struct {
    Version   string                  `json:"version"`
    UpdatedAt string                  `json:"updated_at"`
    Bundles   []*bundleEntry          `json:"bundles"`
}

type bundleEntry struct {
    ID          string                 `json:"id"`
    Title       string                 `json:"title"`
    Description string                 `json:"description"`
    Providers   []*bundleProviderEntry `json:"providers"`
}

type bundleProviderEntry struct {
    Provider   string   `json:"provider"`
    Models     []string `json:"models"`
    ApplyURL   string   `json:"apply_url"`
    ApplySteps []string `json:"apply_steps"`
    IsKeyless  bool     `json:"is_keyless"`
    Notes      string   `json:"notes"`
}
```

## 组件设计（如有）

### 1. homePage.tsx 改造

```typescript
import FreeTierRecommendationCard from "../components/freeTierRecommendationCard";
// ...

return (
  <div className="mx-auto w-full max-w-7xl space-y-4">
    <header>...</header>
    <EndpointCard endpointUrl={endpoint} />
    <SystemHealthCard />
    <SetupStatusBar />
    <FreeTierRecommendationCard />     {/* 替换 quickStartCard */}
    <ProviderTopologyCard />
  </div>
);
```

### 2. freeTierRecommendationCard.tsx 主卡

```typescript
const { data, error, refetch } = useGetBundlesQuery({ lang: i18n.language });
const recent = useRecentRoutingRulesQuery({ limit: 100 });

if (data?.bundles.length === 0) {
  return <EmptyStateCard onRetry={refetch} />;
}

return (
  <Card data-testid="home-free-tier-card">
    <CardHeader>
      <CardTitle>{t('freeTier.title')}</CardTitle>
      {data.updated_at && <CardDescription>{t('freeTier.updatedAt', { at: data.updated_at })}</CardDescription>}
    </CardHeader>
    <CardContent>
      {data.bundles.map(b => (
        <BundleApplyCard key={b.id} bundle={b} recentRules={recent.data?.rules.slice(0, 3)} />
      ))}
    </CardContent>
  </Card>
);
```

### 3. bundleApplyCard.tsx 单个 bundle

```typescript
<Card data-testid={`home-free-tier-bundle-${bundle.id}`}>
  <CardHeader>
    <CardTitle>{bundle.title}</CardTitle>
    <CardDescription>{bundle.description}</CardDescription>
  </CardHeader>
  <CardContent>
    {bundle.providers.map(p => (
      <ProviderRow
        key={p.provider}
        provider={p}
        onConfigure={() => setDialogState({ open: true, provider: p })}
      />
    ))}
  </CardContent>
  <CardFooter>
    <RecentRulesFooter rules={recentRules} />   {/* 最近路由 */}
  </CardFooter>
</Card>
```

### 4. freeTierOneKeyConfigDialog.tsx 弹窗

```typescript
const [createProvider] = useCreateProviderMutation();
const [createKey] = useCreateProviderKeyMutation();

async function onSubmit(apiKey: string) {
  try {
    await createProvider({ provider: dialog.provider.provider }).unwrap();
  } catch (e) {
    if (e.status === 409) {
      // 已存在，继续创建 key
    } else throw e;
  }
  if (!dialog.provider.is_keyless && apiKey) {
    await createKey({ provider: dialog.provider.provider, key: apiKey }).unwrap();
  }
  toast.success(t('freeTier.configSuccess'));
  catalogApi.util.invalidateTags(['CatalogBundles']);
}
```

### 5. useRecentRoutingRulesQuery.ts

```typescript
export const useRecentRoutingRulesQuery = (args: { limit?: number }) =>
  useGetRecentRoutingRulesQuery({ limit: args.limit ?? 100 });
```

## 关键约束与契约

### 前置条件

- `transports/config.schema.json` 必须先定义 `remote_catalog` 段
- 运营需先在 CDN 上传 `bundles/zh-CN.json` + `bundles/en.json`，并在 config.json 中配置 `remote_catalog.url_template`（默认空，整模块隐藏）
- `logs.db` 日志表必须含 `routing_rule_id` / `routing_rule_name` 字段（如缺则聚合返回空数组，单测覆盖）

### 影响面

- **新增文件**：
  - `transports/celer-route-http/handlers/catalog.go`（约 200 行）
  - `transports/celer-route-http/handlers/logs.go` 中追加 `recentRoutingRulesHandler`（约 80 行）
  - `transports/celer-route-http/handlers/catalog_test.go`（单元测试）
  - `transports/celer-route-http/handlers/logs_recent_test.go`（单元测试）
  - `ui/app/workspace/home/components/freeTierRecommendationCard.tsx`
  - `ui/app/workspace/home/components/freeTierOneKeyConfigDialog.tsx`
  - `ui/app/workspace/home/components/bundleApplyCard.tsx`
  - `ui/app/workspace/home/hooks/useRecentRoutingRulesQuery.ts`
  - `ui/lib/store/apis/catalogApi.ts`
  - `ui/locales/en/home.json`（新增约 25 键）
  - `ui/locales/zh-CN/home.json`（新增约 25 键）
  - `examples/configs/remote-catalog-bundles.example.json`（schema 示例）
- **修改文件**：
  - `ui/app/workspace/home/views/homePage.tsx`（移除 quickStartCard，替换为 FreeTierRecommendationCard）
  - `ui/app/workspace/home/components/providerTopologyCard.tsx`（命名改为 Configured Providers，可选）
  - `transports/celer-route-http/server/server.go`（注册新路由 2 条）
  - `transports/celer-route-http/handlers/middlewares.go`（路由权限）
  - `transports/celer-route-http/lib/config.go`（解析 remote_catalog 段）
  - `transports/config.schema.json`（新增 remote_catalog 段定义）
- **不破坏任何对外 API**：仅新增，不改既有契约

### 性能契约

- `GET /api/catalog/bundles`：内存读 + ETag 比对，p99 < 5ms
- `GET /api/logs/recent-routing-rules`：单 SQL 查询（`SELECT routing_rule_id, routing_rule_name, MAX(timestamp) ... GROUP BY routing_rule_id ORDER BY last_used_at DESC LIMIT N`），p99 < 50ms（即使 100 万行日志）
- 后端 goroutine 拉取 JSON：超时 5s，连续 3 次失败后停止拉取（避免雪崩），下次启动重试
- 内存上限：`max_bundles=100` × `max_bundle_size_bytes=1MB` ≈ 100MB 上限

### 错误码与编号段

- `INVALID_LIMIT`（400）— `limit` 越界
- `LOGS_QUERY_FAILED`（500）— logs.db 查询失败
- `REMOTE_CATALOG_FETCH_FAILED`（仅后端 WARN 日志，不对外返回）
- 沿用既有：`INVALID_PROVIDER_KEY`（provider_keys.go 已有）

### 可观测性

- 关键日志点：
  - INFO：catalog 首次拉取成功 / ETag 命中 304 / 周期刷新触发
  - WARN：上游 4xx/5xx / JSON 解析失败 / ETag 协商失败
  - ERROR：连续 3 次拉取失败后停止 goroutine
- 关键指标：
  - `catalog_fetch_total{lang,status}` Counter（status=ok|http_error|parse_error）
  - `catalog_fetch_duration_seconds` Histogram
  - `recent_routing_rules_query_duration_seconds` Histogram
- RequestId 追踪：复用 `BifrostContextKeyRequestID`

### 环境限制与验证策略

> **必填**（v0.9.0 新增）。依据 `.pg/changes/home-free-tier-recommendation/env-description.yaml` 中目标 env `local` 的 6 段判断。

| 功能契约 (V-*) | local 可验证 | 验证方式 | 不可验证部分的处理 |
|---------------|:---:|------|------|
| V-transports-1 后端代理拉取 bundle JSON + ETag + `/api/catalog/bundles` | ⚠️ | dev 单元测试 + int scenario | 需 prepare_env 阶段用 python -m http.server 起本地 mock 服务模拟 CDN；缺 mock 时 scenario 标 @skip |
| V-transports-2 最近 100 条日志 routing rule 聚合 | ✅ | dev 单元测试 + int scenario | n/a |
| V-transports-3 拉取失败/解析失败/空 bundle 时端点降级 | ✅ | dev 单元测试 | 注入 mock 失败路径 |
| V-ui-1 home 页推荐卡渲染 + 弹窗填 key 流程 | ✅ | dev vitest + int scenario | n/a |
| V-ui-2 套餐卡片底部展示「最近路由规则」+ 跳转 | ✅ | dev vitest + int scenario | n/a |
| V-ui-3 i18n 按浏览器语言选语种拉取 | ⚠️ | dev vitest + int scenario | 需 prepare_env 起本地 mock；缺 mock 时 scenario 标 @skip |
| V-ui-4 拉取失败/空 bundle 时隐藏卡片 + 重试按钮 | ✅ | dev vitest | n/a |
| V-ui-5 E2E 走完整路径（mock JSON + 填 key + 看到「最近路由」） | ⚠️ | int scenario Playwright | 需 MSW + 本地 mock；scenario 标 degraded |

**V-* 命名规范**：
- 编号格式：`V-{track_id}-{seq}`（如 `V-transports-1`、`V-ui-1`）
- 同一 design.md 中不允许混用连字符和下划线
- 每个 V-* 必须在 `## Verification Criteria` 表中出现且仅出现一次
- 本节引用的 V-* 与 "Verification Criteria" 表中的 ID **完全一致**

## Verification Criteria

### dev transports Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-transports-1 | 后端代理拉取 bundle JSON + ETag 缓存 + 透传端点 | `remote_catalog.url_template` 配置为本地 mock URL；prepare_env 起 `python -m http.server` 模拟 | curl 启动后端 → 等 1 个周期拉取 → `curl -i GET /api/catalog/bundles?lang=zh-CN` 含 ETag 头；带 `If-None-Match` 第二次 → 304 | 200 返回正确 bundles + ETag；304 无 body |
| V-transports-2 | 最近 100 条日志 routing rule 聚合端点 | logs.db 已 seeded（fixture-routing-rules），调任意 chat 接口产生 100+ 条日志 | curl `GET /api/logs/recent-routing-rules?limit=100` | 200 返回非空 rules 数组，按 last_used_at 倒序 |
| V-transports-3 | 拉取失败/解析失败/空 bundle 时端点降级 | mock server 返回 500 / 无效 JSON / 空 bundles 文件 | curl `GET /api/catalog/bundles?lang=zh-CN` | 200 返回 `{bundles:[], updated_at:null}`，不返 5xx |

### dev ui Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-1 | home 页推荐卡渲染 + 弹窗填 key 流程 | mock 远程 JSON 返回含 coding bundle | Playwright：访问 /workspace/home → 看到 FreeTierRecommendationCard → 点「一键配置」填 fake key → 验证 POST /api/providers + POST /api/providers/{provider}/keys 被调 | 卡片渲染 + 弹窗成功提交 + Redux cache 刷新 |
| V-ui-2 | 套餐卡片底部展示「最近路由规则」+ 跳转 | fixture-routing-rules 已 seeded | Playwright：访问 /workspace/home → 看到 bundle 卡片底部展示前 3 条路由 → 点击其中一条 | 跳转到 `/workspace/routing-rules/$id` |
| V-ui-3 | i18n 按浏览器语言选语种拉取 | mock 远程 zh-CN.json / en.json 两文件 | Playwright：切 UI locale 到 en → 刷新 → 验证 `/bundles/en.json` 被拉取 | URL 命中对应语种文件 |
| V-ui-4 | 拉取失败/空 bundle 时隐藏卡片 + 重试按钮 | mock 返回空 bundles | Playwright：访问 /workspace/home → 看到空状态卡 + 重试按钮 → 点击后重新调 /api/catalog/bundles | 空状态渲染正确 + 重试按钮可点 |

### int scr Verification Criteria

| ID | 验证项 | 前置/数据准备 | 方法 | 预期结果 |
|-----|--------|---------------|------|---------|
| V-ui-5 | E2E 走完整路径（mock JSON + 填 key + 看到「最近路由」） | local env 启动 + prepare_env 起 mock JSON server + fixture-routing-rules 已 seeded | Playwright + DevTools MCP：访问 /workspace/home → 看到 coding bundle 卡片 → 点「一键配置」填 fake key → 验证 POST /api/providers + POST /api/providers/{provider}/keys 被调 → 卡片底部展示 fixture-routing-rules 数据 | 完整链路无报错，providerTopologyCard 出现新 provider |

## 变更类型判定

| track | 是否影响 | 理由 |
|-------|---------|------|
| core | ❌ | 核心引擎无改动 |
| framework | ❌ | 框架层无改动 |
| transports | ✅ | 新增 catalog.go + 路由注册 + config schema 段 + remote_catalog 配置解析 |
| plugins | ❌ | 插件无改动 |
| ui | ✅ | home 页重做 + 新增 5 个组件/hook + RTK Query + i18n 25 键 |
| scr | ✅ | scenario 跨模块联调（浏览器 + 后端代理 + DB） |

**affected_tracks**：`[transports, ui]`

**scenario_tracks_decision**：
- `scr` track：`enabled = true`
- 依据：
  - 跨 role 协作验证？✅ (ui-dev + celer-route-api + logs-db + routing_rules)
  - 新 API 端点？✅ (新增 GET /api/catalog/bundles, GET /api/logs/recent-routing-rules)
  - 跨模块联调？✅ (ui ↔ transports 跨 track 联动)