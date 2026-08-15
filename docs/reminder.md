# 侧边栏菜单隐藏记录

隐藏原因：部分功能当前不需要使用，隐藏菜单项以减少干扰。仅前端隐藏，路由仍可访问。

## 隐藏清单

| 目录 | 隐藏项 | 方式 |
|---|---|---|
| 可观测性 | MCP 日志、连接器 | 子项 `hasAccess: false` |
| 模型 | 预算与限制、定价覆盖 | 子项 `hasAccess: false` |
| MCP网关 | 整个目录 | 父级 + 5 子项 `hasAccess: false` |
| 治理 | 整个目录 | 父级 + 虚拟密钥子项 `hasAccess: false` |
| — | Webhooks | 独立项 `hasAccess: false` |
| — | 提示词仓库 | 独立项 `hasAccess: false` |
| — | 技能仓库 | 独立项 `hasAccess: false` |
| — | 评估 (Evals) | 独立项 `hasAccess: false` |
| 设置 | 功能开关 | 子项 `hasAccess: false` |

## 恢复方法

在 `ui/components/sidebar.tsx` 中将对应项的 `hasAccess: false, // hidden` 改回原始值（如 `hasAccess: hasSettingsAccess,`），并补充对应的 `useRbac` 变量声明和依赖数组条目即可。

## 清理掉的变量

以下 `useRbac` 变量因所有引用项被隐藏而移除，如需恢复特定菜单项需重新声明：

- `hasObservabilityAccess`
- `hasMCPGatewayAccess`
- `hasMCPLogsAccess`
- `hasVirtualKeysAccess`
- `hasGovernanceLegacyAccess`
- `hasFeatureFlagsAccess`
- `hasPromptRepositoryAccess`
- `hasSkillsRepositoryAccess`
- `hasAnyGovernanceAccess`（派生自 `hasVirtualKeysAccess`）

## 变更文件

`ui/components/sidebar.tsx`