# 项目目录与产物

pg-skills 接入业务项目后，会同时存在公共运行目录和开发工具专用目录。本页说明它们由谁生成、谁读取，以及是否适合提交到版本控制。

## 总体结构

```text
your-project/
├── .pg/                    pg-skills 公共源码、配置与运行数据
│   ├── skills/             嵌入的 pg-skills 仓库源码
│   ├── project.yaml        项目级 SSOT 配置
│   ├── context/            仓库扫描和 Agent 协议
│   ├── hooks/              环境生命周期脚本
│   ├── code-review/        Review profile
│   ├── changes/            变更定义、方案和构建状态
│   └── runs/               当前版本预留目录
├── .opencode/              OpenCode 适配目录，可选
├── .mobile-coder/          Mobile Coder 适配目录，可选
├── .dsh/                   DeepSeek Harness 适配目录，可选
├── pg-run                  Unix Hook 包装器
└── pg-run.cmd              Windows Hook 包装器
```

项目通常选择一个默认开发工具。多个适配目录可以共存，但 `.pg/tool-integration.json` 只记录最后一次成功选择，升级自动刷新以它为准。

## `.pg/skills/`

这里存放完整 pg-skills 源码，通常通过 Git subtree 引入。CLI、Runner、schema、公共工作流和适配器都来自这里。

- 业务需求不应直接修改其中的源码。
- 团队应固定可复现的 tag 或 commit。
- 升级应使用 Git subtree 流程，而不是覆盖复制。

## `.pg/project.yaml`

这是当前项目的配置单一事实来源，描述 modules、environments、tracks、stages 和 regression suite。它由 `pg-init-project` 生成初值，再由项目维护者确认。

应提交该文件，但不得包含密钥或仅适用于个人电脑的秘密配置。字段说明见[配置指南](configuration.md)。

## `.pg/context/`、`.pg/hooks/` 和 `.pg/code-review/`

- `context/` 保存仓库扫描结果、Agent 协议和迁移提示。
- `hooks/` 保存服务启动、停止、健康检查和环境准备脚本。
- `code-review/` 保存语言相关的 Review profile 和规则。

这些文件会影响所有工作流，通常应提交并接受代码审查。运行日志、PID 和临时探测结果应按项目 `.gitignore` 规则处理。

## `.pg/changes/`

每个标准变更有独立目录：

```text
.pg/changes/<change-id>/
├── 0-define/
│   └── define-summary.yaml
├── proposal.md
├── design.md
├── tasks.md
├── execution-manifest.yaml
├── scenario-<track>.yaml        # 需要场景时
└── 2-build/                     # 构建事件、snapshot 和阶段产物
```

- `proposal.md` 解释为什么做、做什么和不做什么。
- `design.md` 解释接口、数据流、兼容性和技术决策。
- `tasks.md` 是可执行任务清单。
- `execution-manifest.yaml` 把任务映射到 Stage、Track、Module、Agent 和验证阶段。
- 每个 Stage 使用的环境写在 `execution-manifest.yaml` 的 `stages[].environment` 中；当前工作流不生成独立的 `environment.yaml`。
- `2-build/` 是恢复执行的重要状态，构建中断时不要删除。

成功流程可能把变更移动到 `.pg/changes/archive/`。是否提交活动变更和归档记录由团队策略决定，但用于审计和 PR 评审的方案通常值得保留。

## `.pg/runs/`

`pg init` 会创建该目录，但当前标准 `pg-build` 不在这里保存事件或 snapshot。它是预留运行目录，并被 `.pg/.gitignore` 默认忽略。恢复构建时应保留对应 change 的 `2-build/pipeline.events`、`2-build/pipeline.snapshot.json` 和其他阶段产物。

## 工具专用目录

### `.opencode/`

`pg init --tool opencode` 渲染 Commands、Skills 和 Agents，并生成 `.pg-adapter-manifest.json`。OpenCode 直接从这里加载项目能力。

### `.mobile-coder/`

`pg init --tool mobile-coder` 渲染 Commands、Skills、Agents、辅助 runtime，以及适配后的 pg-skills runtime。适配器不会修改 `mobile-coder.json`。

### `.dsh/`

`pg init --tool deepseek-harness` 生成原生 Skills、Commands、Agents、Cordis command bridge、模型路由 patch，以及 Web/Headless 启动脚本。当前适配不再使用 `.deepseek-harness/`。

三个目录中的 `.pg-adapter-manifest.json` 记录适配器管理的文件和 hash。重新初始化时：

- 未修改的旧文件可以安全刷新或移除。
- 用户修改过的受管文件会被保留并产生 warning。
- 不在 manifest 中的自定义文件不会被当作 pg-skills 受管文件覆盖。

## 哪些文件应该提交

通常建议提交：

- `.pg/skills/` 的 subtree 内容。
- `.pg/project.yaml`。
- 稳定的 context、hooks 和 code-review 配置。
- 团队选择的工具适配目录。
- proposal、design、tasks 和需要保留的验收记录。

通常不应提交：

- API 密钥、Token 和个人凭据。
- PID、临时缓存和无审计价值的大型日志。
- 个人机器专用路径。
- 与团队默认工具无关且未维护的旧适配目录。

最终规则应写入项目 `.gitignore` 和贡献指南，而不是只依赖本页。

DeepSeek Harness 的 `.dsh/cordis.patch.yml` 包含指向本项目 `.dsh/bridge/index.ts` 的绝对文件 URI，因此它具有机器和检出路径相关性。项目移动或换机后应重新运行 `pg init --tool deepseek-harness`；若该文件被手工修改而被适配器保留，还需更新 URI，或重建该文件后重新应用模型路由。

## 相关文档

- 理解项目配置：[配置指南](configuration.md)。
- 初始化工具目录：[安装指南](installation.md)。
- 查看具体工具目录：[OpenCode](tutorials/opencode.md)、[Mobile Coder](tutorials/mobile-coder.md)、[DeepSeek Harness](tutorials/deepseek-harness.md)。

