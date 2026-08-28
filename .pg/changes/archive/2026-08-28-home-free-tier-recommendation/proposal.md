# home-free-tier-recommendation
**关联 issue**：无
**变更类型**：feature

## 背景

个人用户接入 LLM 网关后最迫切的需求是省钱。当下 `/workspace/home` 展示的是运维级视图（endpoint、systemHealth、setupStatus、providerTopology）+ 通用 onboarding 步骤，没有引导用户发现当下可用的免费 LLM provider —— 用户得自己搜索、申请、配 provider key。

我们计划让运营侧每日产出一份 JSON 文件（按语种分片，URL 模板 `{base}/bundles/{lang}.json`），把当前推荐的免费 provider、模型、申请链接、典型使用场景 (coding / content_creation 等) 都列在 JSON 里。后端负责代为拉取并缓存，前端登录后 home 页展示"免费 provider 推荐"卡片，按运营推送的套餐 (bundle) 渲染，引导用户申请 key 并一键配置。

## 目标

1. 用户登录后立即看到当下可用的免费 provider 套餐，无需自己搜索
2. 用户点击 provider 卡片上的申请链接直达外部申请页（外链 `target=_blank`）
3. 用户回填 key 后通过弹窗一键 POST providers + POST provider keys 完成接入（keyless provider 跳过 POST keys）
4. 套餐卡片底部展示"最近 100 条日志用过的路由规则"，帮助用户关联到自己的实际使用
5. 拉取失败/解析失败/空 bundle 时静默降级，home 页其他 4 个 card 不受影响

## 范围

### 包含

- 后端代理：新增 `remote_catalog` 配置段 + ETag 缓存 + `GET /api/catalog/bundles?lang=<lang>` 端点
- 后端新增：最近 100 条日志的 routing rule 聚合端点 `GET /api/logs/recent-routing-rules?limit=100`
- UI 重做 `/workspace/home`：保留 endpoint/systemHealth/setupStatus/providerTopology 四个 card，把 quickStartCard 替换为 FreeTierRecommendationCard
- FreeTierRecommendationCard：bundle 列表 + 模型列表 + 申请链接外链 + 一键填 key 弹窗（POST `/api/providers` + POST `/api/providers/{provider}/keys` 串行；keyless provider 自动跳过 keys）
- 每个 bundle 卡片底部展示"最近路由规则"列表，点击跳转到 `/workspace/routing-rules/$id`
- 拉取失败/解析失败/空 bundle → 卡片渲染空状态 + 重试按钮
- i18n：URL 模板 `{base}/bundles/{lang}.json`，按浏览器语言选语种；无 base 配置时整个推荐模块隐藏
- 中英文 i18n 键值补齐（含词元/令牌正确翻译）

### 不包含

- 自动创建路由规则（仅展示用户最近用过的路由作为引导，不自动写 routing_rules）
- 多租户隔离的 bundle 推送（v1 只支持全局运营 JSON）
- 多副本部署的运营 JSON 一致性（进程内定时刷新即可，多副本各自拉取）
- bundle JSON 的版本号 / 灰度发布机制
- 运营 JSON 的鉴权（HMAC / API key）
- bundle JSON 内 model 列表的运行时可达性过滤（v1 只展示运营标注）
- provider 卡片"已被 X 个虚拟 key 引用"社交信号
- 已有 4 个 home card 的字段重写（仅保留 + 替换 quickStartCard）

## 方案概述

### 数据层（运营 JSON schema）

```json
{
  "version": "2026-08-28",
  "updated_at": "2026-08-28T08:00:00Z",
  "base_url": "https://cdn.example.com",
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
        }
      ]
    }
  ]
}
```

不同语言对应不同文件 `/{base}/bundles/zh-CN.json`、`/en/{base}/bundles/en.json`。

### 后端

- 在 `transports/config.schema.json` 新增 `remote_catalog.url_template` 配置段（默认 `""`，空时整个推荐模块隐藏）
- 新增 `transports/celer-route-http/handlers/catalog.go`：进程内 goroutine 每 N 秒（默认 3600）拉一次，按 ETag 协商；暴露 `GET /api/catalog/bundles?lang=zh-CN` 返回内存快照 + `ETag` 响应头；客户端带 `If-None-Match` 时返回 304
- 新增 `transports/celer-route-http/handlers/logs.go` 端点 `GET /api/logs/recent-routing-rules?limit=100`：查 logs.db 中最近 N 条日志，按 `routing_rule_id` 去重，返回 `(id, name, last_used_at)` 倒序列表
- 拉取失败/解析失败/空 bundle → 端点始终返回 `200 + { bundles: [], updated_at: null }`，不返回 5xx
- 复用 `network.SSRFSafeDialContext` 防止 SSRF

### 前端

- 改造 `ui/app/workspace/home/views/homePage.tsx`：5 个 card 改为 5 个 card，保留 endpoint/systemHealth/setupStatus/providerTopology，替换 quickStartCard 为 FreeTierRecommendationCard
- 新增 `ui/app/workspace/home/components/freeTierRecommendationCard.tsx`：
  - 用 RTK Query 调 `GET /api/catalog/bundles?lang=<currentLocale>`
  - 拉取失败/空数组时渲染空状态卡 + 重试按钮
  - 每个 bundle 一个子卡：provider 列表 + models 列表 + 申请链接外链 (`target="_blank"`) + "一键配置"按钮
- 新增 `ui/app/workspace/home/components/freeTierOneKeyConfigDialog.tsx`：弹窗收集 API Key，调 `POST /api/providers` + `POST /api/providers/{provider}/keys`（keyless provider 跳过 keys），409 翻译为"已配置"
- 新增 `ui/app/workspace/home/hooks/useRecentRoutingRulesQuery.ts`：`/api/logs/recent-routing-rules` RTK Query hook（带 ETag）
- `FreeTierRecommendationCard` 每个 bundle 卡片底部展示"最近路由"（最多 3 条），点击跳到 `/workspace/routing-rules/$id`
- 中英文 i18n 补齐 25+ 键值（bundleTitle / applyNow / configureNow / noBundles / retry / recentRoutingRules 等），严格遵循"词元/令牌"区分

### 验证

- dev 阶段：Go 单元测试（`transports/celer-route-http/handlers/catalog_test.go`、`logs_test.go`）、前端 vitest 单测（hook reducer）、`cd ui && npm run format` + `cd ui && npm run build`
- int 阶段：scenario 跨模块联调（浏览器 + 后端代理 + DB）

## 风险和注意事项

| # | 风险 | 验证方式 |
|---|------|---------|
| 1 | bundle JSON 体积较大时进程内存占用上升 | V-transports-1 单元测试覆盖 `max_bundles` / `max_bundle_size` 截断 |
| 2 | `remote_catalog.url_template` 误指向内网地址触发 SSRF | V-transports-1 单测验证 `network.SSRFSafeDialContext` 拦截 |
| 3 | quickStartCard 被移除会破坏依赖其 data-testid 的现有 E2E | V-ui-1 grep `tests/e2e/features/` 防御性再扫（前期扫描已确认无引用，回归阶段需复扫） |
| 4 | 运营 JSON 错配 base URL 时整个推荐模块静默消失，用户无感 | V-ui-4 空状态 + 重试按钮 + SystemHealthCard 卡片补充"运营推荐:不可用"指示 |
| 5 | POST `/api/providers` 已存在时返回 409，弹窗需把 409 翻译为"该 provider 已配置，请直接使用" | V-ui-1 弹窗文案 + scenario 走 409 路径 |
| 6 | "最近 100 条日志"无 routing_rule_id 时聚合返回空数组 | V-transports-2 单测覆盖空路径 |
| 7 | bundle JSON 内 provider 字段是运营手填，可能拼写错误或漏字段 | V-transports-1 schema 校验 + 单测覆盖缺失字段 |
| 8 | i18n key 漏翻导致 UI 显示空白 | V-ui-3 切语种 + 静态扫描中英文键值对齐 |

每条风险均可被至少 1 个 V-* 验证（见 design.md Verification Criteria）。