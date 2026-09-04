# 支持的开发工具

pg-skills 的核心工作流与开发工具无关。每个适配器负责把公共 Commands、Skills、Agents 和能力占位符转换为目标工具能够加载的项目文件。

当前分支包含以下适配器：

| 工具 | `--tool` 值 | 项目目录 | 安装形态 |
|---|---|---|---|
| OpenCode | `opencode` | `.opencode/` | 渲染 Commands、Skills、Agents |
| Mobile Coder | `mobile-coder` | `.mobile-coder/` | 渲染 Commands、Skills、Agents |
| DeepSeek Harness | `deepseek-harness` | `.dsh/` | 原生 Skills、Cordis Commands 和模型路由桥 |

使用下面的命令获取当前代码实际注册的工具，而不是依赖静态文档：

```powershell
python .pg\skills\src\runtime\bin\pg init --list-tools
```

## 公共边界

无论选择哪个工具，以下内容保持一致：

- `.pg/project.yaml`。
- `.pg/changes/` 和工作流产物；标准 build 状态位于 change 的 `2-build/`。
- Python Runner、Event、Reducer、Hook 和 Doctor。
- Pipeline 阶段、失败处理、结果记录、验证和归档语义。

工具适配器只负责“如何加载和调用”，不应重写 pg-build 等公共工作流逻辑。

## OpenCode

初始化：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool opencode
```

生成：

```text
.opencode/
├── commands/
├── skills/
├── agents/
└── .pg-adapter-manifest.json
```

OpenCode 从该目录加载项目级 Commands、Skills 和 Agents。适配器使用 OpenCode 的 Skill、Task、question 和 TodoWrite 等能力语义。

模型路由使用：

```text
pg-router/pg-associate
pg-router/pg-expert
pg-router/pg-master
```

适配器不会替用户修改 `opencode.json` 或安装模型 provider。用户需要在 OpenCode 配置中保证这些路由可解析，或根据团队模型配置调整生成模板。

初始化后重启 OpenCode，再检查命令列表。

## Mobile Coder

初始化：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool mobile-coder
```

生成：

```text
.mobile-coder/
├── commands/
├── skills/
├── agents/
└── .pg-adapter-manifest.json
```

适配器把公共 Skill 加载、Sub-agent 派送、用户提问和任务跟踪语义转换为 Mobile Coder 的原生能力。当前模板把 associate、expert、master 三档都映射为 `current`，因此它保留工作流角色分层，但不会自动提供三个不同的真实模型。

适配器不会修改用户的 `mobile-coder.json`。

## DeepSeek Harness

初始化：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool deepseek-harness
```

生成：

```text
.dsh/
├── skills/
├── commands/
├── agents/
├── bridge/index.ts
├── cordis.patch.yml
├── start-web.cmd
├── run-task.cmd
└── run.cmd
```

主要机制：

- `.dsh/skills/` 由 Harness 原生 Skill loader 加载。
- `bridge/index.ts` 通过 Cordis 注册项目级 pg Commands。
- `cordis.patch.yml` 加载命令桥和三个原生 Sub-agent 工具。
- `pg_associate`、`pg_expert`、`pg_master` 对应三级工作流路由。

Windows 交互使用：

```powershell
.dsh\start-web.cmd
```

一次 Headless 任务：

```powershell
.dsh\run-task.cmd "检查当前项目配置，只输出问题清单"
```

查看使用说明：

```powershell
.dsh\run.cmd
```

三个路由的 provider 和 model 在 `.dsh/cordis.patch.yml` 中配置。默认值可以相同，但用户可以改为三个不同模型。Web 页面选择的主会话模型不等于 Sub-agent 的三级路由；Sub-agent 派送仍按 Cordis 配置执行。

`cordis.patch.yml` 还包含 `bridge/index.ts` 的绝对文件 URI。项目移动、换机或改变检出路径后需要重新初始化；手工改过 patch 时还应确认适配器是否因保护用户修改而保留了旧 URI。

## 能否同时初始化多个工具

可以先后运行不同 `--tool`，各自的项目目录可以共存。但 `.pg/tool-integration.json` 只记录最后一次成功选择，`pg upgrade` 自动刷新时会以该记录为准。

团队仓库最好明确一个默认工具，其他适配目录是否提交由团队约定决定。

## 切换工具

```powershell
python .pg\skills\src\runtime\bin\pg init --tool <new-tool>
python .pg\skills\src\runtime\bin\pg doctor
```

切换只生成新适配层，不会迁移或删除旧工具中的用户自定义文件。确认新工具可用后，再通过 Git diff 清理不需要的旧适配文件。

## 相关文档

- 完整操作教程：[OpenCode](tutorials/opencode.md)、[Mobile Coder](tutorials/mobile-coder.md)、[DeepSeek Harness](tutorials/deepseek-harness.md)。
- 三级模型映射：[模型路由指南](model-routing.md)。
- 生成目录和文件所有权：[项目目录与产物](project-structure.md)。
- 不清楚在哪输入命令：[命令如何工作](how-commands-work.md)。
- 初始化失败：[故障排查](troubleshooting.md)。
- 适配器开发边界：[tool-integrations.md](tool-integrations.md)。

