# redesign-providers-pages
**关联 issue**：无
**变更类型**：feature

## 背景
当前 `/workspace/providers` 页面只有 300px 左侧栏选择器 + 右侧 SPA 主面板（`?provider=xxx` URL 参数）。当管理 30+ provider 时，缺乏搜索/筛选/分组/批量/详情跳转入口，且每个 provider 的健康度、key 数、model 数、今日用量等关键信息无法在列表层看到——必须点进每个 provider 才能看到。用户从 Omniroute 的 `/dashboard/providers` 设计里发现这些问题可以通过分组卡片 + 健康度 badge + 聚合字段一次性解决。

但 Omniroute 的列表是 ~1951 行 god component + 详情页是 854 行客户端组件，已经触发 strangler-fig 拆解（Issue #3501）。我们直接复刻同样形态会重蹈覆辙，必须从一开始就组件化。

## 目标
1. 提供新的 provider 列表与详情页面，按厂商家族分组，每个 provider 卡片含健康度、key/model 数、今日请求、上次错误时间、批量启用/禁用 toggle、quick test 入口。
2. 提供新的 provider 详情页（6 个 Tabs：Overview/Keys/Models/Usage/Governance/Logs），Overview Tab 内联编辑 Network/Proxy/Performance/Governance/Beta Headers/OpenAI Config 全部配置。
3. **不删除**现有 `/workspace/providers` 页面与 ProviderConfigSheet，新旧并行运行，提供双向跳转入口。
4. 后端**仅**扩展 `ProviderResponse` 9 个聚合字段 + 新增批量端点 `POST /api/providers/{provider}/keys/batch`。

## 范围
### 包含
- 新增路由 `/workspace/providers2`（列表）和 `/workspace/providers2/:id`（详情）
- `ProviderResponse` 扩展字段：`keys_count`、`models_count`、`keys_health_status`、`today_requests`、`today_errors`、`last_used_at`、`last_error_at`、`uptime`、`avg_latency`
- 新增 `POST /api/providers/{provider}/keys/batch` 批量端点（原子事务）
- 侧边栏 Providers 菜单新增子项 `Browse Providers (New)` 入口
- 旧页面顶部"Try new view"按钮 + 新详情页"Open legacy view"按钮（双向跳转）
- 复用现有 `AddProviderDropdown` / `AddCustomProviderSheet` / `ConfirmDeleteProviderDialog` 三个组件
- 新页面独立 `data-testid` 命名空间（`providers2-*`），旧 E2E 不破坏

### 不包含
- 删除或重构现有 `/workspace/providers` 页面（并行保留）
- 删除 `ProviderConfigSheet`（保留作为高级配置后备入口）
- 引入新图标库（复用 `ui/lib/constants/icons.tsx`）
- Provider 级别 enable/disable（Toggle 仅作用于 keys）
- 历史用量趋势图表（Usage Tab 仅展示今日/近 7 日聚合数字）
- 后端重构 `configstore`、新增表、改 DB schema

## 方案概述
**后端**（`transports/bifrost-http/handlers/providers.go` + `provider_keys.go`）：
- 在 `ProviderResponse` struct 上 append 9 个聚合字段（`omitempty` 兼容旧 client）
- 在 `listProviders` / `getProvider` handler 里**聚合查询**：从 `config-db` 的 keys/models 表 JOIN 统计 + 从 `logs-db` 读当日请求/错误/最近时间戳 + 计算 uptime/avg_latency
- 新增 `batchUpdateProviderKeys` handler：接收 `{key_ids: [], enabled: bool}`，单事务内循环 `UPDATE keys SET enabled = ? WHERE id IN (...)`
- 路由注册：`r.POST("/api/providers/{provider}/keys/batch", ...)`

**前端**（`ui/app/workspace/providers2/`）：
- `layout.tsx` + `page.tsx`（列表）—— TanStack Router 路由
- `views/ProviderFamilyGroup.tsx` —— 按厂商家族分组的 section
- `views/ProviderCard.tsx` —— 卡片组件，含健康度 Badge + 聚合数字 + Toggle + Quick test
- `views/ProviderFilters.tsx` —— 搜索 + provider 多选 + 健康度 chips
- `[id]/layout.tsx` + `[id]/page.tsx`（详情）
- `[id]/views/OverviewTab.tsx` / `KeysTab.tsx` / `ModelsTab.tsx` / `UsageTab.tsx` / `GovernanceTab.tsx` / `LogsTab.tsx`
- 复用现有 `ProviderConfigSheet`（"Open legacy config sheet"按钮触发）
- 新增 RTK Query mutation hooks：`useBatchUpdateProviderKeysMutation`

**导航**：在 `ui/lib/constants/nav.ts` 的 `Providers` 菜单下新增子项 `Browse Providers (New)`，指向 `/workspace/providers2`。

## 风险和注意事项
1. **ProviderResponse wire 兼容性**：追加字段虽 `omitempty`，但下游 consumer（bifrost-cli、SDK）若用 strict struct decode，可能因未知字段 panic——需 Go unit test 覆盖。
2. **`POST /keys/batch` 半失败风险**：批量端点必须单事务，若中途 key 不存在需返回完整错误而非部分成功。
3. **N+1 查询性能**：聚合 9 个字段需对每个 provider 做多表 JOIN，30+ provider 量级需关注 latency，可加内存缓存或单查询批拉。
4. **乐观更新回滚**：Toggle 启用所有 keys 后前端立即反映，若端点返回 500 需回滚 UI 状态 + toast。
5. **God component 风险**：从一开始按 Tabs/fragment 切分新详情页，避免 OmniRoute Issue #3501 重演。
6. **迁移期认知成本**：菜单上同时存在"Providers"与"Browse Providers (New)"两个入口，需在迁移完成时决定是否彻底替换。

**约束映射**（每条风险对应 verification V-*）：
- 风险 1 → V-transports-2
- 风险 2 → V-transports-3
- 风险 3 → V-transports-2（fixture 5 provider 量级可验证）
- 风险 4 → V-transports-3
- 风险 5 → V-transports-4（Tabs 独立渲染验证组件切分）
- 风险 6 → V-transports-5（双入口并存验证）

## 未做
- 无 skipped V-*（define-summary 中全部 verifiable）
