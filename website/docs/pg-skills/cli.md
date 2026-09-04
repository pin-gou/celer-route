# CLI 参考

pg-skills CLI 负责项目初始化、结构检查和版本升级。入口位于：

```text
.pg/skills/src/runtime/bin/pg
```

本文以 Windows 的 `python` 为例；Linux 和 macOS 通常使用 `python3`。

## 命令概览

```text
pg init       初始化 .pg/ 并安装开发工具适配层
pg doctor     检查项目结构和 project.yaml
pg upgrade    升级 .pg/skills/ 并刷新适配层
```

查看总帮助：

```powershell
python .pg\skills\src\runtime\bin\pg --help
```

## `pg init`

初始化公共项目骨架并安装指定工具适配层。该操作是幂等的，可以重复执行以刷新适配文件。

```powershell
python .pg\skills\src\runtime\bin\pg init [options]
```

### init 参数

| 参数 | 说明 |
|---|---|
| `--tool TOOL` | 显式选择开发工具适配器 |
| `--list-tools` | 列出当前版本支持的适配器并退出 |
| `--no-tool-config` | 只创建 `.pg/` 骨架，不生成工具目录 |
| `--no-symlinks` | `--no-tool-config` 的历史兼容别名 |
| `-y`, `--yes` | 自动接受唯一检测到的工具 |
| `--non-interactive` | 禁止交互；必须同时显式传入 `--tool` |

### init 示例

```powershell
# 交互检测
python .pg\skills\src\runtime\bin\pg init

# 明确选择 OpenCode
python .pg\skills\src\runtime\bin\pg init --tool opencode

# CI 中初始化
python .pg\skills\src\runtime\bin\pg init --non-interactive --tool opencode

# 只建立公共骨架
python .pg\skills\src\runtime\bin\pg init --no-tool-config
```

`pg init` 不下载 pg-skills，也不会替用户配置模型密钥。它保留已经存在的项目配置和用户文件，并通过 adapter manifest 管理生成文件。

## `pg doctor`

检查当前项目的 pg-skills 状态：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

主要检查：

- pg-skills 版本和目录是否存在。
- `.pg/project.yaml` 是否符合 schema。
- 项目骨架和关键上下文文件是否存在。
- Hook 公共能力是否完整。
- 工具适配选择是否有效。

Doctor 输出分为错误和警告。错误通常会返回非零退出码；警告可能允许继续，但应判断是否会影响当前工作流。

## `pg upgrade`

升级通过 Git subtree 接入的 pg-skills，并刷新项目骨架和已配置适配层。

```powershell
python .pg\skills\src\runtime\bin\pg upgrade [version] [options]
```

### upgrade 参数

| 参数 | 说明 |
|---|---|
| `version` | 目标版本，例如 `0.9.2`；省略时使用 master |
| `--list` | 获取远程 tag 并列出可用版本 |
| `--interactive` | 升级前列出冲突文件并让用户选择 |
| `--force` | 备份现有 `.pg/skills/` 后，以复制方式替换源码并绕过常规 subtree 流程 |

### upgrade 示例

```powershell
python .pg\skills\src\runtime\bin\pg upgrade --list
python .pg\skills\src\runtime\bin\pg upgrade 0.9.2
python .pg\skills\src\runtime\bin\pg doctor
```

稳定项目应升级到明确 tag，而不是默认跟随 master。升级前提交或备份本地修改，升级后查看 Git diff。

`--force` 会创建 `.pg/skills.backup.<timestamp>` 后再替换 `.pg/skills/`，不是“自动 stash 后继续”。它可能留下较大的备份目录；仅在了解覆盖范围并确认常规升级不可用时使用。

## 项目根目录解析

CLI 从当前目录向上查找 `.pg`、`package.json`、`pom.xml`、`go.mod` 或 `pyproject.toml`。为避免操作错项目，推荐始终在目标项目根目录执行命令。

## 退出码

- `0`：命令成功。
- 非 `0`：参数、结构、配置、Git 或适配器操作失败。

在 CI 中应检查退出码，不要只搜索输出中的“ERROR”。

## 相关文档

- 完整安装过程：[安装指南](installation.md)。
- AI 对话命令：[命令参考](commands.md)。
- CLI 报错：[故障排查](troubleshooting.md)。

