# DeepSeek Harness 完整教程

本教程说明如何生成项目级 `.dsh/` 适配目录，通过 Web 交互模式或 Headless 单任务模式启动 DSH，并验证 Commands、Skills、原生 Sub-agent 和三级模型路由。

## 前置条件

- 已安装 Git、Python 3、Node.js 和官方 DeepSeek Harness CLI `dsh`。
- DSH 已配置可用 provider 和凭据。
- `dsh` 能从终端执行。

```powershell
dsh --help
```

## 1. 安装 pg-skills

按照[安装指南](../installation.md)把 pg-skills 放入 `.pg/skills/`，然后确认 CLI 入口存在。

## 2. 生成 `.dsh/`

```powershell
python .pg\skills\src\runtime\bin\pg init --tool deepseek-harness
```

预期生成：

```text
.dsh/
├── commands/
├── agents/
├── skills/
├── bridge/
│   └── index.ts
├── cordis.patch.yml
├── start-web.cmd
├── start-web.sh
├── run-task.cmd
├── run-task.sh
├── run.cmd
├── run.sh
├── README.md
└── .pg-adapter-manifest.json
```

当前适配器统一使用 `.dsh/`，不会另外生成 `.deepseek-harness/`。

## 3. 理解这些文件

- `skills/` 由 DSH 原生 Skill loader 加载。
- `commands/` 和 `agents/` 保存渲染后的工作流与角色文档。
- `bridge/index.ts` 读取 Command 文档并通过 Cordis 注册 Slash Commands。
- `cordis.patch.yml` 加载 command bridge，并注册 `pg_associate`、`pg_expert`、`pg_master` 三个原生 Sub-agent 工具。
- `start-web.*` 启动交互式 Web 页面。
- `run-task.*` 执行一次 Headless 任务并直接在终端返回结果。
- `run.*` 只打印上述两种入口的使用说明，不启动 DSH。

## 4. 配置三级模型路由

打开 `.dsh/cordis.patch.yml`，找到三个 `toolName`：

```text
pg_associate
pg_expert
pg_master
```

分别设置它们的 `agentOptions.provider` 和 `agentOptions.model`。默认模板把三档都设置为：

```text
deepseek-official/deepseek-v4-flash
```

只有修改为不同的实际模型，才实现真正的三档模型分层。Web 页面右下角选择的模型属于主会话，不会替代这些 Sub-agent 配置。

配置细节见[模型路由指南](../model-routing.md)。不要把 API 密钥直接写进 patch 文件。

该 patch 还包含指向 `.dsh/bridge/index.ts` 的绝对文件 URI。项目移动或换机后应重新运行初始化；如果你手工修改过 patch，适配器可能保留旧文件，此时必须同时检查 URI 和三级模型映射。

## 5. 启动 Web 交互模式

Windows：

```powershell
.dsh\start-web.cmd
```

Linux 或 macOS：

```bash
.dsh/start-web.sh
```

脚本会从项目根执行：

```text
dsh --profile web --patch .dsh/cordis.patch.yml
```

浏览器页面打开后，在当前工作区新建会话。不同 DSH Web 版本不一定提供完整的 Slash Command 列表或自动补全，因此不能只依赖界面菜单判断适配是否成功。

## 6. 使用 Headless 单任务

Headless 不是持续交互式 TUI。它接收一个任务，执行完成后直接在当前终端输出结果并退出。

Windows：

```powershell
.dsh\run-task.cmd "检查当前项目，只回复 DSH_READY，不要修改文件"
```

Linux 或 macOS：

```bash
.dsh/run-task.sh "检查当前项目，只回复 DSH_READY，不要修改文件"
```

不传任务会提示 `a task is required` 或打印 usage，这是入口参数校验，不是适配器加载失败。

## 7. 初始化项目配置

在 Web 会话中发送：

```text
请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。完成后列出识别到的 modules、environments、tracks、stages 和测试命令。
```

审查 `.pg/project.yaml`、Hooks 和 Review profile，再运行 `pg doctor`。Web 页面负责交互，项目配置和 Runner 状态仍保存在 `.pg/`。

## 8. 验证四条链路

### Command

先从文件系统确认 bridge 要注册的命令：

```powershell
dir .dsh\commands
```

然后在 Web 页面实际调用 `/pg-1-define`，并给出“只调查、不要修改文件”的任务。命令能够被识别并进入 define，才证明 `cordis.patch.yml → bridge/index.ts → Command` 链路正常。若当前 Web 版本提供 `/` 自动补全，可以把它作为辅助检查，但不是必需条件。

### Skill

发送：

```text
请加载 pg-define skill，只读取当前项目并回答项目名称和主要语言，不要修改文件。
```

这证明 `.dsh/skills/` 被原生 Skill loader 发现。

### Sub-agent

要求主 Agent 派送一个简单只读调查，并确认能得到返回结果。这证明 Cordis 注册的原生 subagent tool 可以调用。

### 模型路由

分别派送 associate、expert 和 master 路由，并查看 DSH 日志或 provider 记录中的实际模型。只让 Sub-agent 回复路由名称，不能单独证明真实模型映射正确。

## 9. 运行标准变更

> 标准 build 成功后可能自动提交、rebase、push、合并并推送默认分支。先确认工作树干净、当前为预期功能分支、`git.default_branch` 正确，并且允许远端写入。

Web 模式中输入：

```text
/pg-1-define

为当前服务增加一个只读健康状态端点。请先调查已有实现和测试，确认范围与验收方式，不要立即编码。
```

确认 change-id 后依次执行：

```text
/pg-2-propose add-health-endpoint
/pg-3-build add-health-endpoint
```

在执行 build 前审查 proposal、design、tasks 和 execution manifest。完成后检查业务 diff、测试结果和对应 change 的 `2-build/` 事件、snapshot 与阶段报告。

## 10. 升级与自定义文件

升级 `.pg/skills/` 后重新运行 `pg init --tool deepseek-harness`。适配器通过 manifest 更新受管文件；检测到手动修改的 `cordis.patch.yml` 等文件时会保留并产生 warning。

应人工比较新版模板，避免长期保留的自定义 patch 错过 bridge 或 DSH API 更新。

## 常见问题

- `profile "tui" does not exist`：当前安装没有 TUI profile，使用 `start-web` 或 `run-task`。
- `run.cmd` 没有打开页面：它只显示入口说明；交互模式使用 `start-web.cmd`。
- Web 页面能选模型但三级路由没变化：主会话模型和 Sub-agent 路由是两套配置。
- 命令目录存在但 Slash Command 不出现：检查 Cordis patch 是否加载 `bridge/index.ts`，并从项目级启动脚本进入 DSH。

## 相关文档

- [支持的开发工具](../supported-tools.md)
- [项目目录与产物](../project-structure.md)
- [故障排查](../troubleshooting.md)

