# providercooldown-builtin-ui
**关联 issue**：无
**变更类型**：feature

## 背景

`providercooldown` 插件已编译进 bifrost binary 并列在 `builtinPluginNames` 中（`transports/bifrost-http/lib/config.go:125-135`），但 server 启动路径仅在 `loadBuiltinPlugins()` 内硬编码条件分支（`transports/bifrost-http/server/plugins.go:289-315`）——用户必须在 `config.json` 显式添加 `{name: "provider-cooldown", enabled: true}` 条目才生效。

与此同时 UI 端（`ui/app/workspace/plugins/views/pluginsView.tsx`）只暴露通用 Monaco JSON editor，没有：① default_ttl_seconds / ttl_overrides / quota_patterns 三个字段的专用表单；② cooldown state / stats 监控面板；③ 单个 provider/key 的手动解冻入口。

结果是该插件的"built-in" 标签与"启用流" / "配置界面" 完全不匹配——运维体感与 telemetry / prompts 等真正"零配置即用"的内置插件不在同一档。

## 目标

1. 改变 providercooldown 启用语义：从"显式 opt-in" 升级为"默认开启"，无 `config.json` entry 等效 `enabled=true`，显式 `enabled=false` 仍可禁用。
2. 补齐 `config.schema.json` 的 schema 校验：name 字段描述加入 `provider-cooldown`，并为该插件的 `config` 字段添加 `allOf/if/then` 专用 schema（参考 telemetry 现有做法）。
3. 在 PluginsView 内嵌入专用 fragment：顶部独立 Switch 切 enabled；下方 react-hook-form + zod 表单管理 3 个字段；下方监控面板拉取 state / stats + DELETE 解冻。
4. 新增 `docs/features/provider-cooldown.mdx` 文档。

## 范围

### 包含

- `transports/bifrost-http/server/plugins.go` 改 providercooldown 条件分支（默认开启逻辑）
- `transports/config.schema.json`：
  - `plugins.items.properties.name` 描述加 `provider-cooldown`
  - 新增 `allOf/if/then` 专用 `config` schema 校验 3 个字段类型
- `ui/app/workspace/plugins/views/pluginsView.tsx` 内嵌 providercooldown 专用 fragment
- `ui/app/workspace/plugins/fragments/` 新增 providercooldown 专用 fragment（含 form + monitoring panel）
- `ui/lib/types/plugins.ts` 复用 pluginFormSchema 共用字段
- `ui/lib/store/apis/pluginsApi.ts` 复用 3 个专用 cooldown API（state / stats / 解冻）
- `docs/features/provider-cooldown.mdx` 新建
- `plugins/providercooldown/` 单测保持通过
- `transports/bifrost-http/` 单测保持通过

### 不包含

- 其他 8 个内置插件的默认开启策略
- 其他内置插件的专用 UI 表单
- `helm-charts/bifrost/values.yaml` 字段补充
- `providercooldown` 核心逻辑改造（CooldownState / PostLLMHook / AsFilter 保持）
- RBAC 权限边界扩展
- 端到端 E2E（Playwright）冒烟

## 方案概述

**Backend**：`server/plugins.go` 把 providercooldown 从"检查 cfg != nil && cfg.Enabled" 改为"无 entry 时构造等价默认配置（Enabled=true），仍允许 entry 显式覆盖为 false"。`KeyPoolFilter` 绑定逻辑（`server.go:1928-1937`）保持——仍走 `loadBuiltinPlugins` 路径，确保 `IsBuiltinPlugin()` 仍返回 true，避免 custom 路径加载。

**Schema**：`config.schema.json` 用 `allOf: [{ if: { properties: { name: { const: "provider-cooldown" } } }, then: { properties: { config: { ...3 字段类型... } } }]` 模式。

**UI**：PluginsView 检测到 `name === "provider-cooldown"` 时切换到专用 fragment——上中下三段：① Switch 控 enabled；② form 控 3 字段；③ monitoring panel 拉取 3 个 API。`react-hook-form + zod` 复用 `ui/lib/types/plugins.ts`。

**文档**：新建 MDX，含 3 个配置的语义、默认值、UI 入口、监控 API 说明。

## 风险和注意事项

1. **KeyPoolFilter 静默失效风险（最严重）**：`loadBuiltinPlugins` 路径的 `s.KeyPoolFilter = plugin.State.AsFilter(logger)` 是 providercooldown 生效的关键。若误改 `builtinPluginNames` 或 `IsBuiltinPlugin()` 决策函数，会让 providercooldown 走 custom 路径，`KeyPoolFilter` 为 nil 但不报错——功能完全回退却无任何日志。**缓解**：本变更严格不动 `builtinPluginNames` 列表、不动 `IsBuiltinPlugin()` 决策函数；`V-transports-1` 启动验证会确认 `KeyPoolFilter` 被绑定。
2. **默认开启语义变化**：存量用户行为从"无 cooldown" 变为"自动 10 分钟 cooldown"。**约束**：用户已声明无存量用户，可接受。
3. **schema drift 风险**：UI 端 zod schema 与 `config.schema.json` 专用 schema 字段定义两处副本，可能走出 drift。**缓解**：UI 端复用 `ui/lib/types/plugins.ts` 的 `pluginFormSchema` 共用字段；config 字段类型定义集中一处。
4. **reload 链路破坏风险**：`server.go:1928-1937` 的 reload + KeyPoolFilter 重绑定逻辑依赖 plugin 仍是 builtin，本变更不动 `builtinPluginNames` 即可绕开。**缓解**：`Scenario` 覆盖 reload 链路验证（V-transports-1 包含）。
5. **fixture 副作用**：默认开启后，写入新 entry 的"禁用" 切换会创建一份新的 plugin 记录到 `config_plugins` 表，可能干扰既有 fixture。**缓解**：scenario 在干净 fixture 环境验证（V-ui-2）。
