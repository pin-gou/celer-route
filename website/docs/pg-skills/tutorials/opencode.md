# OpenCode 完整教程

本教程说明如何在一个项目中安装 pg-skills、生成 OpenCode 适配目录、验证 Commands、Skills 和 Agents，并运行一次标准变更。

## 前置条件

- 已安装 Git、Python 3 和 OpenCode。
- OpenCode 已能在目标项目中正常对话和读取文件。
- 如果使用三级独立模型，已准备能够解析 `pg-router/*` 的 provider 或路由插件。

## 1. 安装 pg-skills

在项目根目录通过 Git subtree 把仓库放入 `.pg/skills/`。完整命令和升级方式见[安装指南](../installation.md)。确认入口存在：

```powershell
Test-Path .pg\skills\src\runtime\bin\pg
```

## 2. 生成 OpenCode 适配目录

```powershell
python .pg\skills\src\runtime\bin\pg init --tool opencode
```

预期生成：

```text
.opencode/
├── commands/
├── skills/
├── agents/
└── .pg-adapter-manifest.json
```

这些是根据公共工作流渲染出的真实项目文件，不依赖 `.opencode/` 到 `.pg/skills/` 的 symlink。

## 3. 配置模型路由

生成的 Agent 使用：

```text
pg-router/pg-associate
pg-router/pg-expert
pg-router/pg-master
```

适配器不会修改 `opencode.json`。请在 OpenCode 的 provider 或路由插件中让三个 id 能解析到实际模型。不同插件使用不同配置 schema，不要直接套用其他路由插件的 JSON。

如果三个路由尚未配置，先解决模型解析问题，再验证工作流。详细说明见[模型路由指南](../model-routing.md)。

## 4. 让 OpenCode 重新加载项目

关闭并重新打开当前 OpenCode 项目，使其重新读取 `.opencode/`。在命令列表中确认至少存在：

```text
/pg-1-define
/pg-2-propose
/pg-3-build
```

同时检查生成目录：

```powershell
dir .opencode\commands
dir .opencode\skills
dir .opencode\agents
```

目录存在只证明适配文件已生成；命令能在 OpenCode 中出现，才证明工具已加载它们。

## 5. 初始化项目配置

在 OpenCode 对话框中输入：

```text
请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。完成后列出识别到的 modules、environments、tracks、stages 和测试命令。
```

人工审查 `.pg/project.yaml`、`.pg/hooks/` 和 `.pg/code-review/`，然后再次运行：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

配置字段见[配置指南](../configuration.md)。

## 6. 做最小加载验证

先验证 Skill 能被调用，不修改文件：

```text
请加载 pg-define skill，只读取当前项目并回答项目名称、主要语言以及是否存在 .pg/project.yaml。不要修改任何文件。
```

再验证角色派送和模型路由。要求 OpenCode 分别调用简单只读角色，并从 OpenCode 日志或 provider 记录确认实际模型。Agent 自报的路由名称只能作为辅助证据。

## 7. 运行标准变更

> 标准 build 成功后可能自动提交、rebase、push、合并并推送默认分支。先确认工作树干净、当前为预期功能分支、`git.default_branch` 正确，并且允许远端写入。

在对话框输入：

```text
/pg-1-define

为当前服务增加一个只读健康状态端点。请先调查已有路由、健康检查和测试，确认范围与验收条件，不要立即编码。
```

确认 define 产物和 change-id。假设为 `add-health-endpoint`：

```text
/pg-2-propose add-health-endpoint
```

审查 proposal、design、tasks 和 execution manifest 后：

```text
/pg-3-build add-health-endpoint
```

不要跳过提案审查，也不要仅凭聊天回复判断成功。检查业务 diff、测试结果和对应 change 的 `2-build/` 事件、snapshot 与阶段报告。

## 8. 升级后刷新

更新 `.pg/skills/` 后重新执行：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool opencode
python .pg\skills\src\runtime\bin\pg doctor
```

适配器依据 `.pg-adapter-manifest.json` 更新受管文件，并保留检测到的用户修改或未跟踪自定义文件。出现 warning 时必须查看 Git diff。

## 常见问题

- 命令不出现：确认从项目根打开 OpenCode，并在初始化后重启。
- 模型找不到：检查 `pg-router` provider 或路由插件，不要修改 `.pg/project.yaml`。
- 重新初始化没有覆盖文件：文件可能被用户修改，适配器会保留并给出 warning。
- 工作流中断：保留对应 change 的 `2-build/`，按[故障排查](../troubleshooting.md)处理。

## 相关文档

- [OpenCode、Mobile Coder 和 DSH 对比](../supported-tools.md)
- [项目目录与产物](../project-structure.md)
- [示例与教程](../examples.md)

