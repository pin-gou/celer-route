# 安装指南

本指南说明如何把 pg-skills 以项目级依赖安装到现有代码仓库，并为所选 AI 开发工具生成适配文件。

> pg-skills 当前采用项目级接入：源码位于项目的 `.pg/skills/`。`pg init` 负责初始化项目数据和生成工具适配层，但不会替你下载 pg-skills 源码。

## 前置条件

- Git。
- Python 3。
- 一个受支持的 AI 开发工具，例如 OpenCode、Mobile Coder 或 DeepSeek Harness。
- 一个已纳入 Git 管理的目标项目。

先检查基础环境：

```powershell
git --version
python --version
```

Linux 或 macOS 如果没有 `python` 命令，请使用 `python3 --version`。

## 安装到项目

推荐使用 Git subtree。这样 pg-skills 与业务代码一起版本化，但仍可从上游升级。

```powershell
cd path\to\your-project
git remote add pg-skills https://github.com/pin-gou/pg-skills.git
git fetch pg-skills --tags
git subtree add --prefix=.pg/skills pg-skills v0.9.2 --squash
```

如果已经存在名为 `pg-skills` 的 remote，不要重复添加；用下面的命令检查：

```powershell
git remote -v
```

也可以手动复制仓库到 `.pg/skills/`，但手动副本不能直接使用标准 subtree 升级流程。

## 初始化工具适配层

查看当前版本支持的工具：

```powershell
python .pg\skills\src\runtime\bin\pg init --list-tools
```

显式选择工具是最清楚、最可复现的方式：

```powershell
# OpenCode
python .pg\skills\src\runtime\bin\pg init --tool opencode

# Mobile Coder
python .pg\skills\src\runtime\bin\pg init --tool mobile-coder

# DeepSeek Harness
python .pg\skills\src\runtime\bin\pg init --tool deepseek-harness
```

不传 `--tool` 时，交互式终端会根据项目目录、环境变量和本机命令进行探测并让你确认。CI 或其他非交互环境必须显式指定：

```powershell
python .pg\skills\src\runtime\bin\pg init --non-interactive --tool opencode
```

## 初始化会创建什么

所有适配器都会创建或补齐：

```text
.pg/
├── project.yaml
├── hooks/
├── context/
├── scripts/
├── changes/
└── runs/                 # 当前版本预留目录，不保存标准 build 状态
```

然后根据工具生成对应目录：

```text
OpenCode          .opencode/
Mobile Coder      .mobile-coder/
DeepSeek Harness  .dsh/
```

这些工具目录承载渲染后的 Commands、Skills、Agent 角色或桥接配置；`.pg/` 保存与工具无关的项目配置和变更产物。标准 build 的事件与 snapshot 位于相应 change 的 `2-build/`，不是 `.pg/runs/`。初始化还会在项目根目录生成 `pg-run` 和 Windows 用的 `pg-run.cmd` Hook 包装器。详细差异见[支持的开发工具](supported-tools.md)。

## 初始化项目配置

`pg init` 只会生成项目骨架。此时的 `.pg/project.yaml` 可能仍是 placeholder，直接运行 `pg doctor` 可能因 placeholder 不符合完整 schema 而失败。先在 AI 开发工具中加载 `pg-init-project` Skill，让它扫描仓库并生成真实配置：

```text
AI CHAT  请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。
```

不要长期使用初始化时的 placeholder 配置。生成后应人工检查模块、环境、Track、Stage、Git 默认分支和测试命令。

## 验证安装

完成项目配置后运行结构和配置检查：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

然后确认以下条件：

- `.pg/skills/VERSION` 存在。
- `.pg/tool-integration.json` 记录了所选工具。
- 所选工具目录中可以看到 pg 工作流文件。
- 重启开发工具后，能够看到 `pg-1-define`、`pg-2-propose`、`pg-3-build` 等命令或 Skills。

`pg doctor` 的警告不一定阻止使用，但错误应在开始工作流前解决。具体处理见[故障排查](troubleshooting.md)。

## 升级

查看可用版本：

```powershell
python .pg\skills\src\runtime\bin\pg upgrade --list
```

升级到指定版本：

```powershell
python .pg\skills\src\runtime\bin\pg upgrade 0.9.2
```

升级会同步 `.pg/skills/`，并刷新已经选择的工具适配层。升级前应提交或备份业务代码，升级后重新运行 `pg doctor` 并查看 Git diff。`--force` 不是普通重试：它会先把现有 `.pg/skills/` 备份为带时间戳的 `.pg/skills.backup.*`，再复制替换源码；仅在确认常规 subtree 升级不适用时使用。

## 移除

pg-skills 的项目数据、源码和工具适配文件可能与用户自定义文件共存，因此不要直接盲删整个工具目录。安全做法是：

1. 先提交当前工作或创建备份分支。
2. 查看 `.pg/skills/`、`.pg/tool-integration.json` 和工具目录中的 `.pg-adapter-manifest.json`。
3. 通过 Git diff 确认哪些文件由 pg-skills 生成。
4. 只删除确认属于 pg-skills 的文件，再运行项目自身测试。

## 下一步

- 第一次使用：阅读[入门指南](getting-started.md)。
- 不知道命令在哪里输入：阅读[命令如何工作](how-commands-work.md)。
- 需要切换或配置客户端：阅读[支持的开发工具](supported-tools.md)。

