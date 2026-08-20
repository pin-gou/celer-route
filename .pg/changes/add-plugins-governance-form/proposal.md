# add-plugins-governance-form
**关联 issue**：无
**变更类型**：feature

## 背景

governance 插件在 `/workspace/plugins?plugin=governance` 路由下当前没有专用配置表单，只能用 JSON Editor 手动编辑配置。otel 插件已有完整 form 实现（位于 `ui/app/workspace/observability/fragments/otelFormFragment.tsx`，868 行），但没有接入 `/workspace/plugins` 路由。

参考实现是 `ui/app/workspace/plugins/fragments/rtkFragment.tsx`，它通过 react-hook-form + zod 提供了完整的产品形态（EnabledSwitch + ConfigForm + 多个 fieldset 分段 + data-testid + i18n）。governance 应当对齐这一形态。

## 目标

让 governance 插件在 `/workspace/plugins?plugin=governance` 路由下展示出完整的专用表单：
- 顶部独立的 EnabledSwitch（沿用 rtkFragment 模式）
- ConfigForm 覆盖 Config 的 4 字段：`is_vk_mandatory`、`required_headers`（TagInput 多标签输入）、`disable_auto_tool_inject`、`routing_chain_max_depth`
- 中英文双语 i18n
- 所有可交互元素加 `data-testid`

让 otel 插件在 `/workspace/plugins?plugin=otel` 路由下复用现有 observability 视图，避免重复实现。

## 范围

### 包含

1. `ui/app/workspace/plugins/fragments/governanceFragment.tsx` 新建（EnabledSwitch + ConfigForm + 4 字段 + RBAC + data-testid）
2. `ui/lib/types/plugins.ts` 添加 `governanceConfigSchema`（zod）与 `GOVERNANCE_PLUGIN` 常量
3. `ui/app/workspace/plugins/views/pluginsView.tsx` 添加 governance / otel 的 if 路由分支
4. otel 在 `/workspace/plugins?plugin=otel` 下复用 `ui/app/workspace/observability/views/plugins/otelView.tsx`
5. i18n 双语：`ui/locales/en/plugins.json` + `ui/locales/zh-CN/plugins.json` 添加 governance 键

### 不包含

- VirtualKeys / Teams / Customers / Budgets / RoutingRules 等已有 CRUD 页面
- `ui/app/workspace/governance/` 子页面扩展（当前 redirect 到 virtual-keys，保持不变）
- 其它无表单内置插件（telemetry / logging / semanticcache / maxim / prompts / compat）
- governance 后端 Config 结构变更（Go 端 4 字段已稳定）
- PluginSpanFilter（保留现有 merge 行为）

## 方案概述

技术方案要点：

- **governanceFragment.tsx**：采用与 rtkFragment 完全一致的模式（导出 `EnabledSwitch` + `ConfigForm` + `GovernanceFragment` 默认导出）。`required_headers` 使用 TagInput 多标签输入，其它三字段用 Switch / Input。
- **路由接入**：在 `pluginsView.tsx:149` 的 if-chain 中追加两个分支：
  ```tsx
  if (selectedPlugin.name === GOVERNANCE_PLUGIN) {
    return <GovernanceFragment plugin={selectedPlugin} />;
  }
  if (selectedPlugin.name === OTEL_PLUGIN) {
    return <OtelView plugin={selectedPlugin} />;
  }
  ```
- **otel 复用**：直接 import `observability/views/plugins/otelView.tsx` 现有实现，不重写。
- **i18n**：在两个 locale 文件的 `plugins.json` 中添加 `governance` 段（`enableTitle` / `enableDescription` / `settingsTitle` / `isVkMandatoryLabel` / `requiredHeadersLabel` 等）。

## 风险和注意事项

- governance Config 中 `IsVkMandatory` / `DisableAutoToolInject` 是 `*bool` 指针类型，UI 提交时必须正确处理「未设置 (nil)」与「显式 false」的区别，避免 pointer-omitempty 误删
- `RequiredHeaders` 是 `*[]string` 指针，删除最后一项时需发空数组而非 null，以保留显式「启用 required_headers 但暂未设置」的状态
- otel 复用必须保留 PluginSpanFilter 通过 merge 逻辑，避免破坏现有行为
- `ui/app/workspace/governance/page.tsx` 当前是 redirect 占位，本次保持不动以避免影响 VirtualKeys 入口

**约束验证**：design.md 的 V-* 列表必须包含验证以上每条风险的条目（V-ui-1 / V-ui-2 / V-ui-3 / V-ui-4）。
