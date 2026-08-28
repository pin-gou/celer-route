# 故障排查

本页按“现象 → 常见原因 → 处理步骤”组织。先保留错误输出和运行记录，不要通过删除 `.pg/` 来掩盖问题。

## 安装与初始化

### `ERROR: .pg/ not found` 或 `.pg/skills must be present`

**原因：** 只创建了空项目目录，尚未把 pg-skills 放入 `.pg/skills/`，或者在错误目录执行命令。

**处理：**

```powershell
cd path\to\project
dir .pg\skills\src\runtime\bin\pg
python .pg\skills\src\runtime\bin\pg init --tool opencode
```

如果文件不存在，先按[安装指南](installation.md)同步源码。

### `Unsupported tool`

**原因：** `--tool` 拼写错误，或者当前版本没有注册该适配器。

```powershell
python .pg\skills\src\runtime\bin\pg init --list-tools
```

使用列表中的标准 ID，不要根据文档猜测。

### 非交互环境提示必须指定工具

**原因：** CI 无法回答工具选择问题。

```powershell
python .pg\skills\src\runtime\bin\pg init --non-interactive --tool opencode
```

## 工作流命令没有出现

按以下顺序检查：

1. 重新运行 `pg init --tool <tool>`。
2. 检查工具目录是否生成 Commands 或 Skills。
3. 重启开发工具。
4. 确认打开的是项目根目录，不是子模块目录。
5. 检查 adapter manifest 和初始化警告。

OpenCode：

```powershell
dir .opencode\commands
dir .opencode\skills
```

DeepSeek Harness：

```powershell
dir .dsh
.dsh\run.cmd
```

### 命令名称和文档不一致

不同工具和旧版本可能使用不同前缀顺序。以当前生成文件和工具命令列表为准。重新初始化后仍不一致时，记录工具版本和实际命令列表再提交 Issue。

## `pg doctor` 报错

### `project.yaml schema invalid`

**原因：** 字段类型、层级或名称不符合当前 schema，常见于沿用旧版本配置、手工编辑 YAML，或刚执行 `pg init` 后仍在使用 placeholder。

**处理：**

1. 根据错误路径定位字段。
2. 对照 `.pg/skills/src/runtime/spec/project.schema.json`。
3. 如果刚完成 `pg init`，先在 AI 对话框加载 `pg-init-project`，生成真实项目配置。
4. 对已有配置，可让 `pg-init-project` 先只输出建议差异。
5. 修复后重新运行 Doctor。

不要用空对象或 placeholder 绕过要求。

### 缺少 `agent-protocol.md` 或 `repo-scan.md`

**原因：** 只运行了 `pg init`，尚未运行项目扫描 Skill。

```text
AI CHAT  请加载 pg-init-project skill，扫描当前仓库并补齐 pg-skills 项目上下文。
```

### Hook 公共函数缺失

**原因：** 项目 Hook 没有同步公共库，或者旧版 `common.sh` 不包含所需函数。

先查看 Doctor 给出的具体文件和函数，再从 `.pg/skills/examples/shell/hooks/lib/common.sh` 同步公共部分。项目自定义 Hook 应保留在约定的扩展区域，避免被覆盖。

## DeepSeek Harness 启动问题

### `.dsh\run.cmd` 只显示说明，没有打开界面

这是预期行为。当前入口职责为：

```text
.dsh\start-web.cmd          启动 Web 交互模式
.dsh\run-task.cmd "任务"   执行一次 Headless 任务
.dsh\run.cmd                只显示上述说明
```

### `run-task.cmd` 提示 `a task is required`

Headless 模式必须传入任务：

```powershell
.dsh\run-task.cmd "检查当前项目，只输出发现的问题"
```

它不会打开交互式 CLI，而是在当前终端输出最终结果后退出。

### DSH 页面有模型，但三级 Sub-agent 没按预期路由

检查 `.dsh/cordis.patch.yml` 中 `pg_associate`、`pg_expert`、`pg_master` 的 provider 和 model。页面右下角模型是主会话选择，不能证明三个 Sub-agent 路由已经配置正确。

### 项目移动或换机后 DSH bridge 加载失败

`.dsh/cordis.patch.yml` 中包含 `bridge/index.ts` 的绝对文件 URI。移动项目或换机后重新运行：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool deepseek-harness
```

如果你手工修改过 patch，适配器会为避免覆盖而保留它。此时请先备份三级模型映射，再更新其中的绝对 URI，或删除该受管 patch 后重新初始化并恢复模型映射。

## Pipeline 失败

### `bootstrap` 失败

bootstrap 失败时工作流按约束终止，不应让 AI 擅自修改 pg-skills Runner。检查：

- change 目录和标准产物是否完整。
- `execution-manifest.yaml` 是否通过校验。
- `.pg/project.yaml` 的 Stage、Track 和 Environment 是否存在。
- 上一轮失败状态是否要求 reset 或 resume。

### `env_drift`

**含义：** 当前真实环境与方案阶段记录的环境能力不一致。反复重跑通常无效。

**处理：** 修复环境 Hook、服务或资源配置；必要时重新执行 define/propose 的环境描述过程。

### `design_drift`

**含义：** 实现阶段发现设计或验收方案不成立。

**处理：** 更新定界和设计产物，重新运行 propose 校验，再继续 build。不要让 Dev Agent私自改变验收标准。

### 会话中断后不知道从哪里继续

保留 change 下的 `2-build/`、`pipeline.events`、`pipeline.snapshot.json` 和阶段产物，然后再次调用同一个 `/pg-3-build <change-id>`。Runner 会依据持久化状态继续或给出明确恢复要求。当前 `.pg/runs/` 是预留目录，不是恢复依据。

## 升级问题

### `remote "pg-skills" not configured`

```powershell
git remote add pg-skills https://github.com/pin-gou/pg-skills.git
git fetch pg-skills --tags
```

### 工作树不干净，升级被拒绝

先查看并提交、暂存或备份本地修改：

```powershell
git status --short
```

不要在不了解覆盖范围时直接使用 `--force`。该选项会把现有 `.pg/skills/` 备份为 `.pg/skills.backup.<timestamp>`，再复制替换源码；完成后必须检查备份目录和 Git diff。

## 仍然无法解决

提交 Issue 时提供：

```powershell
type .pg\skills\VERSION
python --version
python .pg\skills\src\runtime\bin\pg doctor
git status --short
```

同时说明操作系统、开发工具、调用位置、完整命令和未裁剪错误输出。不要提交 API Key、Token、Cookie 或私有地址。

