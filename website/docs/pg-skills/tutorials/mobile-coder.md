# Mobile Coder 完整教程

本教程说明如何在项目中生成 Mobile Coder 原生可发现的 Commands、Skills、Agents 和运行辅助文件，并验证 pg-skills 标准工作流。

## 前置条件

- 已安装 Git、Python 3 和 Mobile Coder。
- Mobile Coder 已能正常打开目标项目并执行基本任务。
- 已了解当前适配器使用主会话模型，不提供三档独立真实模型映射。

## 1. 安装 pg-skills

按照[安装指南](../installation.md)把 pg-skills 放入 `.pg/skills/`。确认 CLI 存在：

```powershell
Test-Path .pg\skills\src\runtime\bin\pg
```

## 2. 生成 Mobile Coder 适配目录

```powershell
python .pg\skills\src\runtime\bin\pg init --tool mobile-coder
```

预期生成：

```text
.mobile-coder/
├── commands/
├── agents/
├── skills/
├── runtime/
│   └── scripts/
├── pg-skills/
│   └── src/runtime/
├── README.md
└── .pg-adapter-manifest.json
```

`.pg/skills/` 仍是上游源码；`.mobile-coder/` 是面向 Mobile Coder 的渲染结果和适配 runtime。适配器不会创建或修改 `mobile-coder.json`。

## 3. 理解模型行为

当前 Mobile Coder 模板把 associate、expert 和 master 都渲染为 `current`：

```text
associate → current
expert    → current
master    → current
```

因此角色职责仍然不同，但都使用当前会话模型。不要把它描述成已经配置了三个独立模型。完整边界见[模型路由指南](../model-routing.md)。

## 4. 让 Mobile Coder 重新加载

重启 Mobile Coder 并重新打开项目。打开 `/skills`，确认至少能看到：

```text
pg-init-project
pg-define
pg-propose
pg-build
```

再确认 Commands 和 Agents 已生成：

```powershell
dir .mobile-coder\commands
dir .mobile-coder\skills
dir .mobile-coder\agents
```

如果文件存在但工具界面没有显示，问题位于 Mobile Coder 的项目发现或缓存，而不是 pg-skills Runner。

## 5. 初始化项目配置

在 Mobile Coder 对话框中输入：

```text
请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。完成后列出识别到的 modules、environments、tracks、stages 和测试命令。
```

人工审查生成结果并运行：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

不要因为 `.mobile-coder/` 已经生成，就跳过 `.pg/project.yaml` 的项目级配置。

## 6. 做最小加载验证

先发送只读任务：

```text
请加载 pg-define skill，只读取当前项目并回答项目名称、主要语言以及是否存在 .pg/project.yaml。不要修改文件。
```

随后让 Mobile Coder 调用一个 Sub-agent 完成简单只读调查，确认工具的原生 subagent dispatcher 能够返回结果。由于三档均为 `current`，此验证只证明角色派送正常，不证明存在独立模型路由。

## 7. 运行标准变更

> 标准 build 成功后可能自动提交、rebase、push、合并并推送默认分支。先确认工作树干净、当前为预期功能分支、`git.default_branch` 正确，并且允许远端写入。

```text
/pg-1-define

为当前服务增加一个只读健康状态端点。请先调查已有实现和测试，确认范围与验收方式，不要立即编码。
```

确认 change-id 后执行：

```text
/pg-2-propose add-health-endpoint
```

审查 `.pg/changes/add-health-endpoint/` 中的 proposal、design、tasks 和 manifest，再执行：

```text
/pg-3-build add-health-endpoint
```

Mobile Coder 适配器在 `pg-3-build` Command 和 `pg-manager` Agent 中加入完成契约，要求 Runner 返回 `done` 后继续执行 verify-and-merge，不能提前宣称构建完成。

## 8. 检查与升级

```powershell
git status --short
git diff --stat
python .pg\skills\src\runtime\bin\pg doctor
```

升级 `.pg/skills/` 后重新执行 `pg init --tool mobile-coder`。适配器会刷新未修改的受管文件，并保留用户修改和不在 manifest 中的自定义文件。

## 常见问题

- `/skills` 没有 pg-skills：重启 Mobile Coder，并确认项目根存在 `.mobile-coder/skills/`。
- 找不到 `mobile-coder.json`：这是正常现象，当前适配器明确不生成该文件。
- 三个角色仍使用同一模型：这是当前模板的设计现状，不是初始化失败。
- 构建提前结束：检查生成的 `pg-3-build.md` 和 `pg-manager.md` 是否包含 Mobile Coder completion contract。

## 相关文档

- [配置指南](../configuration.md)
- [项目目录与产物](../project-structure.md)
- [故障排查](../troubleshooting.md)

