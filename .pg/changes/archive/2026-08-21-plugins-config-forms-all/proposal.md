# plugins-config-forms-all
**关联 issue**：无
**变更类型**：feature

## 背景

`/plugins` 目录下 12 个 Go 插件中，仅 5 个（governance、provider-cooldown、otel、telemetry、maxim）拥有 UI 配置表单；其余 7 个（logging、semantic_cache、mocker、prompts、compat、modelcatalogresolver、jsonparser）只能通过通用 JSON 编辑器或直接修改 `config.json` 进行配置，运维体验差且易出错。

与此同时 `transports/config.schema.json` 存在 schema drift：
- `governance` schema 段缺少 `disable_auto_tool_inject` 和 `routing_chain_max_depth` 两个字段
- `mocker` 整个 schema 段缺失
- `logging` / `semantic_cache` / `compat` 字段名与 Go config struct 需人工校对

## 目标

1. 为 7 个无 UI 表单的插件提供配置入口：4 个真实表单 + 3 个占位说明卡
2. 同步 `transports/config.schema.json` 与 Go config struct，固化 schema 字段定义
3. 完善中英文 i18n 键值对（LLM token 译"词元"、auth token 译"令牌"）
4. 不改后端插件注册路径、不新增 REST 端点，沿用现有 `PUT /api/plugins/{name}` 契约

## 范围

### 包含

- 4 个真实表单片段（react-hook-form + Zod）：
  - `loggingFragment`（4 字段：disable_content_logging、retain_content_in_object_storage、allow_per_request_content_storage_override、logging_headers）
  - `semanticCacheFragment`（11 字段，含 `provider` / `embedding_model` / `dimension` 的 `allOf` 条件联动）
  - `compatFragment`（4 个 toggle：convert_text_to_chat、convert_chat_to_responses、should_drop_params、should_convert_params）
  - `mockerFragment`（Monaco JSON 编辑器 + Zod 实时校验）
- 3 个占位说明卡：
  - `promptsFragment`（说明"无配置项；通过 Prompts CRUD 管理"）
  - `modelcatalogresolverFragment`（说明"无配置项；自动启用"）
  - `jsonparserFragment`（说明"未注册到 plugin 加载系统"）
- `pluginsView.tsx` 散转逻辑中接入 7 个新插件，按配置类型分别渲染
- `transports/config.schema.json` 同步：
  - 补齐 `governance` 缺失的 2 个字段
  - 新增 `mocker` schema 段（含 `allOf` 条件）
  - 校对 `logging` / `semantic_cache` / `compat` 与 Go config struct
- `ui/lib/types/plugins.ts` 增补 4 个 Zod schema + 1 个 i18n 标签映射
- `ui/locales/en/plugins.json` 和 `ui/locales/zh-CN/plugins.json` 增补 49 个键值
- `ui/app/workspace/plugins/fragments/` 目录下新增 7 个 fragment 文件

### 不包含

- rtk 插件（已有 8 字段组表单）
- governance / provider-cooldown / otel / telemetry / maxim（已有表单）
- jsonparser 接入 plugin 注册系统（不改 `loadBuiltinPlugins`）
- 新增 REST 端点（沿用 `PUT /api/plugins/{name}` 契约）
- compat schema 从 `client_config` 迁出到 `plugins[]`（仅 UI 表面化）
- 后端插件行为变更（仅同步 schema 字段，不改 Go 行为）

## 方案概述

### 前端

- 在 `ui/app/workspace/plugins/fragments/` 下新增 7 个片段，沿用 `governanceFragment` / `providercooldownFragment` 的 react-hook-form + Zod 模式
- `mocker` 因 Go config 嵌套层级深（5 级）+ 无现成 schema，使用 Monaco JSON 编辑器配 Zod 实时校验
- `pluginsView.tsx` 散转逻辑扩展 7 个 case：`logging` / `semantic_cache` / `mocker` / `compat` → 对应 fragment；`prompts` / `modelcatalogresolver` / `jsonparser` → 占位卡
- i18n 键遵循 `pluginNames.*`（display name）+ `*Config.fields.*`（字段标签）+ `*Config.sections.*`（章节标题）三层结构

### 后端

- 不改 Go 代码逻辑，仅校对 `config.schema.json`：
  - `governance` schema 增补 `disable_auto_tool_inject`（boolean）和 `routing_chain_max_depth`（integer, 1-100）
  - `mocker` 新增 schema 段（含 `global_latency` / `rules[]` / `default_behavior` 枚举）
  - `logging` / `semantic_cache` / `compat` 字段名、enum、allOf 条件与 Go config struct 字段对齐
- `compat` 配置仍从 `client_config` 读取（不改 schema 位置），UI 通过 `useGetPluginsQuery` 兼容层桥接

### 验证

- dev 阶段：Go 单元测试（`plugins/logging` / `plugins/semanticcache` / `plugins/mocker` / `plugins/compat`）、`cd ui && npm run format`、`cd ui && npm run build`
- int 阶段：scenarios 跨模块联调（浏览器跑 UI + 调 PUT /api/plugins/{name} + 验证后端写入）

## 风险和注意事项

| # | 风险 | 验证方式 |
|---|------|---------|
| 1 | compat 表面化仅从 client_config 读取的字段，不改变后端存储位置，将来若 client_config schema 调整需同步联改 | V-plugins-1 + V-transports-1 单元测试与 schema 校验 |
| 2 | semantic_cache 表单条件规则需与 schema allOf 保持同步，任何一处变动都可能引发校验不吻合 | V-ui-2 浏览器采证 + dev stage V-ui-2 condition branches |
| 3 | schema 字段同步可能使 config.json 示例文件需更新（同一阶段补一个 fixture） | V-transports-1 ajv 校验 |
| 4 | prompts / modelcatalogresolver / jsonparser 三个占位卡的文案、何时移除占位需要后续产品讨论 | V-ui-5 占位卡片渲染验证 |
| 5 | mocker Monaco JSON 编辑器大 JSON 卡顿 | V-ui-3 Monaco 渲染 + Zod 校验压测 |

每条风险均可被至少 1 个 V-* 验证（见 design.md Verification Criteria）。
