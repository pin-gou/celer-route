> - **environment 选择**：dev → local，int → local

## 1. dev.framework:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 1.1 编写单元测试：timeline_events 表的 schema 结构（列名/类型/索引）与 AutoMigrate 不报错
- [ ] 1.2 编写单元测试：LogStore 新增的 timeline 事件写/读方法（write → query by log_id → 空数据）

## 2. dev.framework:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [ ] 2.1 在 `framework/logstore/tables.go` 新增 `TimelineEvent` struct（id/log_id/phase/source/plugin_name/level/message/time_offset_ms/duration_ms/timestamp，gorm tag 对齐现有 Log/MCPToolLog 风格）
- [ ] 2.2 在 SQLite + Postgres logstore 实现中把 `TimelineEvent` 加入 AutoMigrate 迁移列表
- [ ] 2.3 在 `framework/logstore` 的 LogStore 接口新增 timeline_events 读写方法，SQLite + Postgres 两个实现各自落地（按 log_id 索引查询）
- [ ] 2.4 运行 `go test ./... -short` 验证 framework 模块编译通过（degraded 前置：SQLite 本机可用）

## 3. dev.framework:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [x] 3.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 3.2 review agent 对 git diff feat/pg/add-logs-timeline-page 做静态审查
- [x] 3.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 3.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 4. dev.framework:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- [x] 4.1 执行 lint（runner 通过 modules 注入命令）
- [x] 4.2 执行测试（runner 通过 modules 注入命令）
- [x] 4.3 启动服务（如需）
- [x] 4.4 验证 V-framework-1：timeline_events 表存在且 schema 正确（检查 logs.db 表结构 + AutoMigrate 无报错）——来自 design.md「dev framework Verification Criteria」
- [x] 4.5 验证 V-framework-2：plugin 阶段点采集写入事件（查 timeline_events 按 log_id 过滤，存在 pre_llm/post_llm 各至少 1 条）——来自 design.md「dev framework Verification Criteria」

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-framework-1, V-framework-2, V-transports-1, V-transports-2, V-ui-1, V-ui-2
  - degraded: V-ui-3

## 5. dev.framework:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=framework (常驻, 无 on_conditions)
-->

- 无

## 6. dev.transports:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 6.1 编写单元测试：`GetLogTimeline` handler 在 log 存在/不存在（404）/内部错误三种路径返回正确状态码与 JSON 结构
- [ ] 6.2 编写单元测试：`GetActiveLogStream` SSE handler 建立连接后全量握手 `active_logs` 推送、断开后清理订阅

## 7. dev.transports:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 7.1 在 `transports/bifrost-http/handlers/logging.go` 新增 `GetLogTimeline` handler：按 log_id 读 Log + timeline_events + 反序列化 RoutingEngineLogs/PluginLogs/AttemptTrail，聚合为按时间排序的事件列表
- [ ] 7.2 在 `RegisterRoutes` 注册 `GET /api/logs/{id}/timeline` 路由（复用现有认证中间件）
- [ ] 7.3 在 `transports/bifrost-http/handlers/logging.go` 新增 `GetActiveLogStream` handler（SSE）：复用现有 SSE 基础设施，首连全量 processing 日志握手 + processing→success/error 增量推送，客户端断开即关闭
- [ ] 7.4 在 `RegisterRoutes` 注册 `GET /api/logs/active/stream` 路由
- [ ] 7.5 grep 确认新增方法未被外部引用冲突，编译通过

## 8. dev.transports:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [ ] 8.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 8.2 review agent 对 git diff feat/pg/add-logs-timeline-page 做静态审查
- [ ] 8.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 8.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 9. dev.transports:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- [x] 9.1 执行 lint（runner 通过 modules 注入命令）
- [x] 9.2 执行测试（runner 通过 modules 注入命令）
- [x] 9.3 启动服务（如需）
- [x] 9.4 验证 V-transports-1：`GET /api/logs/{id}/timeline` 返回结构化事件（已有带阶段事件的 log，curl 校验 events[] 含 timeline_events + RoutingEngineLogs + PluginLogs + AttemptTrail 聚合）——来自 design.md「dev transports Verification Criteria」
- [x] 9.5 验证 V-transports-2：`GET /api/logs/active/stream` SSE 推送（订阅期间发请求，收到 active_logs 握手 + processing→success 的 log_updated）——来自 design.md「dev transports Verification Criteria」

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-framework-1, V-framework-2, V-transports-1, V-transports-2, V-ui-1, V-ui-2
  - degraded: V-ui-3

## 10. dev.transports:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=transports (常驻, 无 on_conditions)
-->

- 无

## 11. dev.plugins:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [ ] 11.1 编写单元测试：plugins/logging 的 PreLLMHook/PostLLMHook 在 mock 请求下正确写入 timeline_events（按 log_id 关联，含 phase/plugin_name/level/message）
- [ ] 11.2 编写单元测试：采集失败（DB 写失败）不阻断主请求（降级为 WARN 日志）

## 12. dev.plugins:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 12.1 在 `plugins/logging` 的 PreLLMHook 内新增 timeline_events 采集：begin 时间戳 → 写入 phase=pre_llm 事件
- [x] 12.2 在 `plugins/logging` 的 PostLLMHook 内新增 timeline_events 采集：end 时间戳 + 总耗时 → 写入 phase=post_llm 事件（与 Log 主记录同一事务）
- [x] 12.3 golang 编译通过（plugins 模块）；确认采集失败降级为 WARN 不阻断主请求

## 13. dev.plugins:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 13.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [x] 13.2 review agent 对 git diff feat/pg/add-logs-timeline-page 做静态审查
- [x] 13.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [x] 13.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 14. dev.plugins:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- [x] 14.1 执行 lint（runner 通过 modules 注入命令）
- [x] 14.2 执行测试（runner 通过 modules 注入命令）
- [x] 14.3 启动服务（如需）
- [x] 14.4 验证 V-framework-2：plugin 阶段点采集（plugins/logging Pre/Post hook 写入 timeline_events，与 Log 主记录关联正确）——覆盖 plugins 采集，来自 design.md「dev framework Verification Criteria」

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-framework-1, V-framework-2, V-transports-1, V-transports-2, V-ui-1, V-ui-2
  - degraded: V-ui-3

## 15. dev.plugins:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=plugins (常驻, 无 on_conditions)
-->

- 无

## 16. dev.ui:test - dev 测试先行

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 16.1 编写单元测试：Gantt 时间轴组件的 lane 分配 / bar 渲染 / tooltip 数据计算（mock 请求列表数据）
- [ ] 16.2 编写单元测试：SSE hook 对 `active_logs`（全量握手）与 `log_updated`（增量合并）的事件处理归并
- [ ] 16.3 编写单元测试：详情面板 Timeline 标签对 `GET /api/logs/{id}/timeline` 响应的事件列表渲染

## 17. dev.ui:dev - 实现开发

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 17.1 新建 `ui/app/workspace/logs/timeline/` 顶级路由（layout.tsx + page.tsx），复用现有 logs 布局与鉴权
- [ ] 17.2 新建 `views/logsTimeline.tsx` Gantt 组件：横向时间轴 + follow/live/pan 三模式 + lane 分配（参考 omniroute RequestTimeline.tsx）
- [ ] 17.3 新建 `views/timelineToolbar.tsx` / `views/timelineLegend.tsx`：模式切换、时间窗口、状态图例
- [ ] 17.4 复用 `sheets/logDetailsSheet.tsx` + `logDetailView.tsx`，在 logDetailView 内新增 Timeline 标签页渲染阶段事件流
- [ ] 17.5 新增 RTK Query timeline 端点调用 + SSE 订阅 hook（EventSource，页面关闭自动 close）
- [ ] 17.6 为所有新增交互元素加 `data-testid`（`timeline-<element>-<qualifier>` 约定）
- [ ] 17.7 `npm run build` + `npm run lint` 通过（V-ui-3 degraded 前置）

## 18. dev.ui:review - 静态代码审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 18.1 review agent 读 design.md + tasks.md + .pg/code-review/code-review.yaml 细则
- [ ] 18.2 review agent 对 git diff feat/pg/add-logs-timeline-page 做静态审查
- [ ] 18.3 review agent 输出 review_score + p0_failures 到本 section 对应的 review 报告（路径由 dispatch 注入）
- [ ] 18.4 score < pass_threshold → escalate 至 fix-review；score ≥ pass_threshold → completed → 进入 verify

## 19. dev.ui:verify - dev 集成验证

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- [ ] 19.1 执行 lint（runner 通过 modules 注入命令）
- [ ] 19.2 执行测试（runner 通过 modules 注入命令）
- [ ] 19.3 启动服务（如需）
- [ ] 19.4 验证 V-ui-1：Gantt 总览页面可访问并渲染（浏览器访问 localhost:3008/workspace/logs/timeline，渲染 bar；点击 bar 打开 LogDetailSheet 显示 Timeline 标签页）——来自 design.md「dev ui Verification Criteria」
- [ ] 19.5 验证 V-ui-2：fallback 链展开为多 bar（构造带 fallback 请求后浏览器查看）——来自 design.md「dev ui Verification Criteria」
- [ ] 19.6 验证 V-ui-3：npm run build + npm run lint 通过（degraded：若非阻塞降级为 verify 阶段单元级校验）——来自 design.md「dev ui Verification Criteria」

  **Evidence 要求**（verify agent 在验证报告中产出，gate agent 据此评审）：
  - 每个 V-* 必须有对应的原始输出（curl 响应 / 命令行输出 / 日志片段）
  - SKIP 的 V-* 必须注明豁免理由
  - 测试结果（Tests run: N, Failures: 0, Errors: 0）必须有日志摘要

  **define-summary 对账**（自动生成）:
  - verifiable: V-framework-1, V-framework-2, V-transports-1, V-transports-2, V-ui-1, V-ui-2
  - degraded: V-ui-3

## 20. dev.ui:gate - dev 门控审查

<!-- on_conditions_eval:
     stage=dev (常驻, 无 on_conditions)
     track=ui (常驻, 无 on_conditions)
-->

- 无

## 21. int.scr:scenario-execute - 真机场景执行

<!-- on_conditions_eval:
     stage=int (常驻, 无 on_conditions)
     track=scr (常驻, 无 on_conditions)
-->

#### 步骤组 1：scenario-scr.yaml 读取

- [ ] 21.1 确认 `.pg/changes/add-logs-timeline-page/scenario-scr.yaml` 存在且每个 Scenario 含 6 段（scenario_id / critical / given / when / then / evidence；and 可选）
- [ ] 21.2 校验 scenario_id 全局唯一、critical 字段为 bool

#### 步骤组 2：执行

- [ ] 21.3 按 scenario_id 排序：先 critical=true，后 critical=false
- [ ] 21.4 串行执行每个 Scenario 的 given → when → then → and（cleanup）
- [ ] 21.5 按 when[].type 分派执行方式：
  - type=api（默认）：使用 curl 等 HTTP 工具执行 API 请求
  - type=browser：加载 `pg-browser-testing-with-devtools` SKILL，使用 Chrome DevTools MCP 工具执行浏览器交互
- [ ] 21.6 产出结构化 JSON 证据到 `2-build/<report_seq>-<scenario_id>-evidence.json`（<report_seq> 与本 phase 主报告共享同一 seq，由 dispatch_file 注入；加 seq 前缀避免多次 execute 派遣覆盖同 scenario 的历史 evidence）
- [ ] 21.7 browser 场景截图存到 `2-build/<report_seq>-<scenario_id>-screenshot.png`
- [ ] 21.8 critical=true FAIL → 立即停止后续 Scenario，全部标记 SKIPPED → record(scenario-execute, "escalate")
- [ ] 21.9 全部通过 / scenario-execute agent 写盘报告到 `2-build/<seq>-scenario-execute.md`

## 22. final-gate - 最终门控审查

<!-- on_conditions_eval:
     stage=final (常驻, 无 on_conditions)
-->

- [ ] 22.1 收集所有 stage 的 Gate Assessment
- [ ] 22.2 检查跨 stage 依赖项
- [ ] 22.3 输出 Final Gate Assessment
