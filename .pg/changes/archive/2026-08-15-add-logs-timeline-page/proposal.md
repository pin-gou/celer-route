# add-logs-timeline-page
**关联 issue**：无
**变更类型**：feature

## 背景

Bifrost 现有的 `logs` 列表页是表格视图（`ui/app/workspace/logs/page.tsx`），每行展示一条请求日志的主要结构化字段。这种形态难以直观回答以下排查问题：

- 某个时间窗口内多个请求的**并发关系与耗时分布**如何？（哪些请求并行、各自持续多久）
- 此刻**正在处理中的请求**有哪些？（活跃请求）
- 一次带 fallback 的请求，fallback 链上的每次尝试**如何串联**？
- 单个请求内部经历哪些**阶段**（PreLLM hooks / key selection / 上游调用 / PostLLM hooks / 序列化），各自耗时多少？

用户希望引入类似 omniroute `/dashboard/logs/timeline` 的 Request Timeline 视图，以横向 Gantt 时间轴 + 单请求阶段时间线这两种形态补足上述缺口。

## 目标

- 新增独立顶级路由 `ui/app/workspace/logs/timeline/`：横向 Gantt 时间轴总览，按时间并排展示请求 bar，支持 omniroute 全量交互（follow/live/pan 三种模式、自动 lane 分配避免重叠、滚轮缩放、hover tooltip、点击展开详情）。
- 新增后端结构化阶段数据：`timeline_events` 表（`framework/logstore`，SQLite + Postgres 两种后端）记录 plugin pipeline 阶段点；`GET /api/logs/{id}/timeline` 端点聚合阶段事件；`GET /api/logs/active/stream` SSE 端点推送活跃请求。
- 点击 Gantt bar 后弹出 `LogDetailSheet`，详情面板新增 Timeline 标签页展示该请求的阶段事件流。

## 范围
### 包含
- `framework/logstore`：新增 `timeline_events` 表 + schema 迁移（SQLite + Postgres 实现）。
- `plugins/logging`：在 PreLLMHook / PostLLMHook 采集 plugin pipeline 阶段点并写入 `timeline_events`。
- `transports/bifrost-http/handlers/logging.go`：新增 `GetLogTimeline` 方法 + `GET /api/logs/active/stream` SSE 端点（复用现有 SSE 基础设施）。
- `ui`：新增 `/workspace/logs/timeline` 顶级路由、Gantt 时间轴组件、详情面板 Timeline 标签页、SSE 活跃请求订阅。
- 复用现有认证/鉴权中间件（virtual key 隔离仍生效）。

### 不包含
- core 引擎的细分阶段埋点（key selection / retry / fallback / 上游网络 / streaming chunk）。
- ClickHouse logstore 后端的 `timeline_events` 实现。
- 写交互（重发请求、跳转到 playground、删除）。
- `timeline_events` 历史数据回填。
- MCP tool execution 事件纳入 timeline（MVP 暂不纳入，后续扩展）。

## 方案概述

采用「后端结构化事件 + 前端 Gantt 渲染」的分层方案：

1. **数据层**：新增 `timeline_events` 表，与日志主记录 `Log` 通过 `log_id` 关联。`plugins/logging` 在执行 PreLLMHook / PostLLMHook 时记录阶段事件（阶段名、plugin 名、时间戳、延迟、消息）。
2. **API 层**：在现有 `LoggingHandler` 新增 `GET /api/logs/{id}/timeline`，后端聚合 `timeline_events` + `RoutingEngineLogs` + `PluginLogs` + `AttemptTrail` 为结构化时间线事件列表；新增 `GET /api/logs/active/stream` SSE 端点推送 processing→success/error 状态的活跃请求更新。
3. **UI 层**：独立路由渲染横向 Gantt 时间轴（复用现有 list + detail API），点击 bar 打开 `LogDetailSheet`（新增 Timeline 标签页），通过 SSE 实时合并活跃请求。

## 风险和注意事项

- `timeline_events` 表随请求量增长会放大 SQLite 日志库体积，MVP 阶段不引入归档/卸载，需在 PR 说明中记录为后续优化项（V-framework-1 验证表可读可写）。
- SSE 长连接在 bifrost-api 高并发下可能成为连接池压力源；MVP 限定「logs 页面订阅 + 关闭页面自动断连」，不全局常驻（V-transports-2 验证断开即收）。
- 移植 omniroute Gantt UI 需遵守项目既有 UI 规范（entitySelectors、data-testid、Radix 体系），可能引入 UI 适配成本（V-ui-1 / V-ui-2 验证页面可用 + fallback 链展开）。
- plugin pipeline 阶段点采集依赖 plugins/logging 在 Pre/Post hook 的执行路径，需确认其注册位置下都生效（V-framework-2 验证真实请求产生阶段事件）。

> **未做（skipped）项**：无。本变更不存在 `post_discussion_status=skipped` 的 V-*；全部 V-* 为 `verifiable`（6 项）或 `degraded`（1 项，V-ui-3 详见 design.md「环境限制与验证策略」）。
