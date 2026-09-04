# 工作流指南

本指南帮助你根据任务类型选择 pg-skills 工作流。工作流是“动作组合”，不是要求所有任务都走同一条重型流程。

## 工作流概览

| 场景 | 推荐入口 | 主要特点 |
|---|---|---|
| 新功能或重要行为变更 | `define → propose → build` | 方案、环境和验收完整 |
| 边界明确的小任务 | `quick-build` | 跳过 proposal/design/tasks |
| 已有测试套件回归 | `regression` | 执行测试并分类修复失败 |
| 明确的缺陷或 Issue | `fix-issue` | 围绕问题诊断、修复和验证 |
| 方案仍然模糊 | `define` | 先调查和定界，不立即编码 |
| 失败后或主动放弃 | `archive` | 手动保存变更记录 |

## 标准功能开发

适用于新能力、跨模块改动、接口变化、环境相关功能和需要团队审查的任务。

```text
/1-pg-define
      ↓
/2-pg-propose <change-id>
      ↓
人工审查 proposal/design/tasks/manifest
      ↓
/3-pg-build <change-id>
      ↓
archive → verify-and-merge
```

> **Git 副作用：** 标准 build 的成功链路会调用 `pg-verify-and-merge`。它可能提交功能分支、rebase、使用 `--force-with-lease` 推送功能分支、切换到默认分支、squash merge、提交并推送默认分支。运行前必须确认工作树干净、分支和 `git.default_branch` 正确，并且远端写入符合团队流程。

选择标准流的信号：

- 需求包含多个验收条件。
- 需要修改公共 API、数据格式或兼容行为。
- 影响当前项目的多个模块。
- 需要真实环境或端到端场景验证。
- 错误实现的代价较高。

## 先定界再决定

如果你知道问题但不知道最佳方案，只运行 `/1-pg-define`。它是探索和定界过程，不要求立即进入 propose。

```text
AI CHAT  /1-pg-define 调查当前服务的请求 fallback 流程，比较两个实现方向，先不要修改代码。
```

定界后的环境验证通过后，工具会先询问是否继续细化验证方法，再询问下一步流向。选择推荐的 propose 路径会直接加载 `pg-propose`，不需要再次输入命令；如果希望手动进入该阶段，也可以输入 `/2-pg-propose <change-id>`。范围很小时可以转入 quick-build，也可以决定暂不实施。

## 快速构建

适用于范围小、边界清楚、可以通过现有测试直接验收的任务：

```text
AI CHAT  /2b-pg-quick-build
AI CHAT  修正健康检查响应中的错误字段名，并补充单元测试。
```

quick-build 不生成标准的 `proposal.md`、`design.md` 和 `tasks.md`。不要用于以下情况：

- 需求仍有歧义。
- 涉及公共协议或迁移。
- 需要多个 Track 协调。
- 需要复杂真实环境验收。
- 团队要求先审方案再写代码。

quick-build 会为完成的任务创建本地 Git commit，但按当前工作流不会执行 push 或创建 PR。

## 回归测试

适用于项目已有 regression suite，需要执行并处理失败：

```text
AI CHAT  /4-pg-regression framework
```

回归流程会根据 `.pg/project.yaml` 中的 `regression.suite` 找到模块、测试键、环境和输出格式。失败一般分为：

- 测试脚本本身可以安全修复。
- 生产代码问题需要单独修复。
- 超出自动修复边界，需要人工判断。

没有在 `project.yaml` 声明的 suite 不应靠猜测运行。

> **当前回归流程的 Git 行为需要特别注意：** 它会提交测试脚本修复并直接推送到名为 `master` 的分支；生产代码修复则走独立分支和 PR。只应在分支策略与远端权限明确允许时运行。若项目默认分支不是 `master`，先评估该限制，不要假定 `git.default_branch` 会改写这里的目标。

## 问题修复

适用于已有 Issue、明确报错或可复现缺陷：

```text
AI CHAT  /5-pg-fix-issue
AI CHAT  修复 provider fallback 超时问题；现象、期望和复现步骤如下……
```

fix-issue 会围绕问题证据、复现、根因、修复和验证开展工作。它不应被用作没有范围的“顺便重构”。运行日志通常位于 `.pg/fix-issue/`，与标准 change 的 build 记录分开。当前 Skill 不自动 commit 或 push，完成后由用户审查 diff 并决定如何提交。

## 手动归档

标准 pg-build 成功时会自动进入归档流程。以下情况使用手动归档：

- 构建失败后决定终止该变更。
- 需求取消。
- 已通过其他方式完成，需要整理残留变更目录。

```text
AI CHAT  /6-pg-archive <change-id>
```

归档只处理变更记录，不替你执行 Git commit、push 或 PR。

## 如何选择

```text
需求是否清楚？
├─ 否 → 1-pg-define
└─ 是
   ├─ 是否只是小而明确的局部任务？
   │  ├─ 是 → 2b-pg-quick-build
   │  └─ 否 → 1-pg-define → 2-pg-propose → 3-pg-build
   ├─ 是否已有明确缺陷？ → 5-pg-fix-issue
   └─ 是否在运行既有测试套件？ → 4-pg-regression
```

## 最佳实践

- 一个 change 只承载一个可以用一句话说明的目标。
- propose 后先审查产物，再运行 build。
- 不要为了“流程完整”给一行文案修改套用复杂标准流。
- 不要为了“速度”把跨模块、环境相关改动塞进 quick-build。
- 失败后保留事件、日志和报告，先判断是代码、环境还是设计漂移。

## 下一步

- 查看可复制场景：[示例与教程](examples.md)。
- 查询命令参数：[命令参考](commands.md)。
- 理解变更产物：[核心概念概览](overview.md)。
