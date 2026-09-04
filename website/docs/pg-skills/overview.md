# 核心概念概览

pg-skills 是一套面向 AI 开发工具的软件开发工作流。它把需求、方案、环境、任务、测试、审查和运行证据保存在项目中，使下一次会话和团队成员都能从文件恢复上下文，而不只依赖聊天记录。

一句话概括：**先把变更说清楚，再在真实项目约束下构建，并留下可以复查的验收证据。**

## SEA-Driven Development

SEA 由三根支柱组成：

| 支柱 | 含义 | 在 pg-skills 中的主要载体 |
|---|---|---|
| Spec | 方案：做什么、为什么做、如何实现 | `define-summary.yaml`、`proposal.md`、`design.md`、`tasks.md` |
| Environment | 环境：代码实际在哪里构建和验证 | `.pg/project.yaml`、Hook、`env-description.yaml` |
| Acceptance | 验收：什么结果才算真正完成 | 测试、Verify、Gate、场景验证和运行报告 |

只有方案，没有环境，AI 容易根据假设编写无法运行的设计；只有单元测试，没有真实验收，结果可能只在 mock 中成立。SEA 的目标是让三者在同一个变更中相互对应。

## 一次变更是一个目录

标准工作流围绕 `.pg/changes/<change-id>/` 展开：

```text
.pg/changes/add-health-summary/
├── 0-define/
│   └── define-summary.yaml
├── proposal.md
├── design.md
├── tasks.md
├── execution-manifest.yaml
└── 2-build/
```

这个目录是变更的可追踪单元。需求调整、设计决策、任务执行和验证结果都应能够回到同一个 change。

## 标准循环

```text
pg-1-define
      ↓
pg-2-propose
      ↓
pg-3-build
      ↓
verify-and-merge
      ↓
archive
```

- `define` 调查问题、澄清需求并确定范围。
- `propose` 生成方案、设计、任务和执行 manifest。
- `build` 根据 manifest 和项目配置执行任务与质量阶段。
- `verify-and-merge` 检查最终差异并完成合并。
- `archive` 保存完成或放弃的变更记录。

对于边界清楚的小任务，可以使用 quick-build；对于已有测试失败，可以使用 regression；对于明确问题，可以使用 fix-issue。选择方法见[工作流指南](workflows.md)。

标准 build 的成功链路可能自动提交、rebase、push、合并并推送默认分支。它不是只在本地生成代码；运行前应先理解[工作流指南](workflows.md)中的 Git 行为。

## 项目配置是运行约束

`.pg/project.yaml` 描述 pg-skills 如何理解当前仓库，主要包括：

| 概念 | 作用 |
|---|---|
| Module | 一个可构建、检查或测试的代码模块 |
| Environment | 运行服务和执行真实验证所需的环境 |
| Role | 环境中的服务角色及其 start、stop、health-check 动作 |
| Track | 一组模块及其开发、测试和验证策略 |
| Stage | 按顺序组织多个 Track，并定义质量门槛 |

例如，一个包含后端服务、前端应用和共享库的仓库，可以把三者分别定义为 Module；跨模块端到端验证可以定义为 scenario Track。真实模块边界应以当前仓库的构建和测试方式为准。

## Commands、Skills、Agents 和 Runner

这四层各自承担不同职责：

- **Command** 是用户入口，例如 `/pg-3-build add-health-summary`。
- **Skill** 是可复用的工作流说明和脚本，例如 `pg-build`。
- **Agent** 是被派送的角色，例如 Test、Dev、Review、Verify。
- **Runner** 是确定性编排器，依据状态返回下一步 action，并记录结果。

Command 不等于 Skill，Agent 也不是用户需要逐个配置和手动调用的命令。适配器负责把公共工作流映射到不同工具的原生能力，Runner 保持公共执行语义。

## 事件与运行记录

pg-build 使用事件记录和 Reducer 推导状态，而不是只把进度保存在聊天上下文中。运行记录用于：

- 判断当前阶段和下一步 action。
- 在失败后定位最后一个有效状态。
- 支持 reset 或 resume。
- 保留派送、测试、修复和验证证据。

当前标准 build 将事件和 snapshot 保存在 change 的 `2-build/` 中。因此，失败时不应删除 `2-build/pipeline.events`、`2-build/pipeline.snapshot.json` 或其他阶段产物；`.pg/runs/` 在当前版本只是预留目录。

## 工具适配层

公共工作流位于 `src/core/workflows/`，开发工具差异位于 `src/integrations/<tool>/`。初始化后：

```text
.pg/             公共项目配置与运行数据
.opencode/       OpenCode 适配文件
.mobile-coder/   Mobile Coder 适配文件
.dsh/            DeepSeek Harness 适配文件
```

适配器可以改变目录、命令注册方式、Skill 加载器、Sub-agent 调用方式和模型配置形态，但不应改变 Pipeline 顺序、失败处理、结果记录和归档语义。

## 为什么这些文件值得保留

- AI 会话可以结束，变更文件仍然存在。
- 方案和代码可以在同一个 PR 中审查。
- 失败可以根据事件和报告定位，而不是重新猜测。
- 项目配置让不同开发工具共享相同的工程约束。
- 新成员可以从变更目录理解代码为什么变成现在这样。

## 下一步

- 完成第一次变更：[入门指南](getting-started.md)。
- 选择合适流程：[工作流指南](workflows.md)。
- 理解工具调用边界：[命令如何工作](how-commands-work.md)。

