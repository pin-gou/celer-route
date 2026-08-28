# 命令参考

本页是 pg-skills AI 工作流命令的参考。它们在 AI 开发工具对话框中调用，不在 PowerShell、CMD 或 Bash 中执行。

> 本文使用当前适配器生成的 `/pg-*` 形式。不同工具或旧版本可能显示不同拼写，请以工具命令列表和生成文件为准。

## 快速参考

| 命令 | 用途 | 主要产物 |
|---|---|---|
| `/pg-1-define` | 探索、澄清和定界 | `0-define/define-summary.yaml` |
| `/pg-1-grill` | 对需求或方案进行追问与压力测试 | 澄清结论或修订建议 |
| `/pg-2-propose` | 一次性生成标准变更产物 | proposal、design、tasks、manifest |
| `/pg-2b-quick-build` | 跳过标准提案直接实现 | 代码、测试和验证结果 |
| `/pg-3-build` | 执行标准 change | `2-build/`、事件、报告 |
| `/pg-4-regression` | 运行回归 suite 并处理失败 | `.pg/regression/` |
| `/pg-5-fix-issue` | 修复明确问题 | `.pg/fix-issue/`、代码和测试 |
| `/pg-6-archive` | 手动归档失败或放弃的 change | `.pg/changes/archive/` |

## `/pg-1-define`

进入探索、设计和定界模式。适合需求模糊、需要调查代码或需要明确验收边界的任务。

```text
/pg-1-define
```

重新定界已有 change：

```text
/pg-1-define --redefine <change-id>
```

define 的唯一标准落盘环节是定界后环境验证和 `define-summary.yaml`。它不应直接编写业务代码。

## `/pg-1-grill`

用于对需求、假设或方案进行集中追问，暴露遗漏、冲突和不可验证条件。适合在 propose 前提高输入质量。

```text
/pg-1-grill
请针对这个需求的兼容性、失败路径和验收边界连续追问。
```

## `/pg-2-propose <change-id>`

根据定界结果生成标准变更产物：

```text
/pg-2-propose add-health-summary
```

主要产物：

```text
proposal.md
design.md
tasks.md
execution-manifest.yaml
scenario-<track>.yaml      # 仅 scenario Track
env-description.yaml      # 仅需要环境时
```

propose 不负责实现代码。生成后应人工审查，再执行 build。

## `/pg-2b-quick-build`

直接处理边界清楚的小任务，不生成标准 proposal、design 和 tasks。

```text
/pg-2b-quick-build
修复一个局部空指针问题并补充单元测试。
```

如果任务涉及公共行为、跨模块设计或真实环境验收，应使用标准流。quick-build 会为完成的任务创建本地 commit，但不会 push 或创建 PR。

## `/pg-3-build <change-id>`

根据 `tasks.md` 和 `execution-manifest.yaml` 启动 Pipeline：

```text
/pg-3-build add-health-summary
```

未提供 change-id 时，支持用户问答的工具会列出待处理 change。build 会加载 `pg-build` Skill，运行 bootstrap，并循环执行 Runner 的 `next` 和 `record`，直到 `done` 或 `failed`。

> 成功链路会调用 `pg-verify-and-merge`，可能自动提交、rebase、推送功能分支、合并并推送默认分支。执行前请确认工作树干净、分支和 `git.default_branch` 正确，并且允许远端写入。

## `/pg-4-regression <suite>`

执行 `.pg/project.yaml` 中声明的回归测试套件：

```text
/pg-4-regression framework
```

suite 必须能够映射到 module、test_keys、environment 和 output_format。该流程不会凭空创造项目测试命令。

当前回归 Skill 会把测试脚本修复提交并直接推送到 `master`；生产代码修复使用独立分支和 PR。若仓库不允许直接推送 `master`，或默认分支不是 `master`，不要直接运行该流程。

## `/pg-5-fix-issue`

围绕明确问题执行诊断、修复和验证：

```text
/pg-5-fix-issue
修复 provider fallback 超时问题。现象、期望、复现方法和证据如下……
```

该 Command 没有必需的位置参数，问题 ID 可以写在自然语言描述中。输入最好同时包含现象、期望、复现方法和证据。日志位于 `.pg/fix-issue/`，不要与标准 change 的运行目录混淆；当前流程不会自动 commit 或 push。

## `/pg-6-archive <change-id>`

手动归档失败后、取消或主动放弃的变更：

```text
/pg-6-archive add-health-summary
```

pg-build 成功时通常会自动归档，因此不要把此命令作为每次构建后的固定步骤。它不执行 Git commit、push 或 PR。

## 自然语言调用 Skills

不是所有 Skill 都有用户 Slash Command。可以直接要求工具加载：

```text
请加载 pg-init-project skill，扫描当前项目并初始化 .pg/project.yaml。
```

其他内部 Skill，例如 `pg-verify-and-merge`，通常由主工作流触发，不建议绕过上游步骤手动调用。

## 相关文档

- 如何选择命令：[工作流指南](workflows.md)。
- 终端命令：[CLI 参考](cli.md)。
- 命令没有显示：[命令如何工作](how-commands-work.md)。

