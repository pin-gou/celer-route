# 入门指南

本指南帮助你在任意代码项目中完成第一次 pg-skills 标准变更。示例需求“增加健康状态端点”是一个虚构场景，只用于说明通用流程，不依赖特定仓库或技术栈。

> **命令输入在两个地方。** `pg init`、`pg doctor` 在终端运行；`/pg-1-define`、`/pg-2-propose`、`/pg-3-build` 在 AI 开发工具的对话框中运行。

## 前五分钟

整个标准流程如下：

```text
TERMINAL  将 pg-skills 安装到项目的 .pg/skills/
TERMINAL  python .pg/skills/src/runtime/bin/pg init --tool opencode
AI CHAT   加载 pg-init-project，初始化项目配置
TERMINAL  python .pg/skills/src/runtime/bin/pg doctor
AI CHAT   /pg-1-define
AI CHAT   /pg-2-propose add-health-endpoint
AI CHAT   /pg-3-build add-health-endpoint
```

前两步建立项目级能力，后续工作主要发生在 AI 开发工具的对话框中。

## 1. 进入目标项目

从项目根目录开始。这里应该能够看到项目的 Git 仓库以及构建文件，例如 `go.mod`、`package.json`、`pom.xml` 或 `pyproject.toml`。

```powershell
cd path\to\your-project
git status --short
```

如果尚未安装 pg-skills，请先完成[安装指南](installation.md)。安装后的源码入口应为：

```text
.pg/skills/src/runtime/bin/pg
```

## 2. 初始化开发工具适配层

下面以 OpenCode 为例：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool opencode
```

你也可以把 `opencode` 替换为 `pg init --list-tools` 返回的其他工具 ID。

初始化后，公共项目数据位于 `.pg/`，工具专用文件位于对应工具目录：

```text
your-project/
├── .pg/
│   ├── skills/
│   ├── project.yaml
│   └── changes/
└── .opencode/              # 本例选择 OpenCode
```

## 3. 生成真实项目配置

`pg init` 首次创建的 `.pg/project.yaml` 可能只是骨架。打开 AI 开发工具中的项目，在对话框发送：

```text
请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。
```

Skill 会根据当前项目的源码、构建文件和测试入口生成或更新：

```text
.pg/project.yaml
.pg/context/repo-scan.md
.pg/context/agent-protocol.md
.pg/code-review/
```

人工检查 modules、build/lint/test 命令、Tracks、Stages，以及需要真实服务时的 Environment 和 Hook。然后重新运行：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

## 4. 定界第一个变更

下面使用虚构需求“增加健康状态端点”演示标准流程。在 AI 对话框输入：

```text
/pg-1-define

我希望为当前服务增加一个只读健康状态端点。
请先调查现有路由、健康检查、错误响应和测试方式，向我确认范围和验收条件，不要立即写代码。
```

define 应先读取当前代码、提出澄清问题并确定边界。定界完成后会创建：

```text
.pg/changes/<change-id>/0-define/define-summary.yaml
```

此时检查 change-id、目标与非目标、验收条件和环境依赖，而不是只看 AI 的文字总结。

## 5. 生成并审查提案

假设定界后的 change-id 为 `add-health-endpoint`：

```text
AI CHAT  /pg-2-propose add-health-endpoint
```

标准产物包括：

```text
.pg/changes/add-health-endpoint/
├── 0-define/define-summary.yaml
├── proposal.md
├── design.md
├── tasks.md
└── execution-manifest.yaml
```

需要环境或场景验证时，还可能生成 `env-description.yaml` 和 `scenario-<track>.yaml`。

不要立即开始构建。按以下顺序审查：

1. `proposal.md`：问题、范围和非目标是否正确。
2. `design.md`：接口、数据流、兼容性和失败路径是否清楚。
3. `tasks.md`：任务是否可执行，并包含测试工作。
4. `execution-manifest.yaml`：Stage、Track 和环境是否与 `.pg/project.yaml` 对应。

发现问题时，可以直接修改文件，或者要求 AI 修订指定部分。产物不是不可回退的阶段门。

## 6. 构建并验证

> **这一步可能修改远端 Git。** 标准 build 成功后会进入 `pg-verify-and-merge`，可能自动提交功能分支、rebase、push、合并并推送 `.pg/project.yaml` 中的默认分支。继续前请确认工作树干净、当前是正确的功能分支、默认分支配置正确，并且你有意授权这些 Git 操作。

方案确认后执行：

```text
AI CHAT  /pg-3-build add-health-endpoint
```

pg-build 会读取 tasks 和 manifest，通过 Runner 派送 Test、Dev、Review、Verify、Gate 等角色。实际阶段由项目配置和 Track 类型决定，用户不需要手动逐个调用内部 Agent。

运行过程中可以查看：

```text
.pg/changes/add-health-endpoint/2-build/
.pg/changes/add-health-endpoint/2-build/pipeline.events
.pg/changes/add-health-endpoint/2-build/pipeline.snapshot.json
```

成功后，工作流会进入最终验证、合并和归档。失败时保留事件、日志和报告，根据最后一个失败阶段处理，参见[故障排查](troubleshooting.md)。

## 7. 检查结果

完成后至少确认代码与设计一致、测试覆盖验收条件、必需验证真实执行，并且 Git diff 没有意外修改 pg-skills 源码。

```powershell
python .pg\skills\src\runtime\bin\pg doctor
git status --short
git diff --stat
```

## 下一步

- 在真实 celer-route 中走完整流程：[celer-route 实战教程](../tutorials/celer-route.md)。
- 查看更多任务模式：[示例与教程](examples.md)。
- 接入已有大型代码库：[在现有项目中使用](existing-projects.md)。
- 选择标准流、快速流或回归流：[工作流指南](workflows.md)。

