# 在现有项目中使用 pg-skills

将 pg-skills 接入现有代码库时，不需要先把整个系统重写成规范文档。正确做法是保留现有代码、构建方式和测试体系，让 pg-skills 读取仓库现状，再从一个边界清楚的真实变更开始。

## 三十秒版本

```text
1. 把 pg-skills 安装到 .pg/skills/
2. 执行 pg init --tool <工具>
3. 让 pg-init-project 扫描并生成 .pg/project.yaml
4. 人工核对模块、环境、测试命令和生命周期脚本
5. 运行 pg doctor
6. 选择一个小而真实的变更执行 define → propose → build
```

初始化不会替代现有项目的构建系统，也不要求迁移业务代码。`.pg/` 保存的是 pg-skills 的项目配置和变更产物；标准 build 状态位于对应 change 的 `2-build/`。

## 第一步：先确认仓库现状

在初始化前，至少确认：

- 仓库是单模块、Monorepo 还是包含多个独立服务。
- 构建、Lint、单元测试和集成测试的真实命令。
- 本地依赖哪些数据库、缓存、消息系统或外部服务。
- 哪些启动和清理动作可以自动执行，哪些需要人工操作。
- 当前工作区是否干净，避免把既有修改误认为 pg-skills 产物。

## 第二步：安装并初始化工具适配

安装方式见[安装指南](installation.md)。完成安装后，在项目根目录执行：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool opencode
```

将 `opencode` 换成实际使用的工具。该命令生成工具可识别的 Commands、Skills 和 Agents，并初始化 pg-skills 运行所需的项目目录。

## 第三步：生成项目配置

在 AI 开发工具中输入：

```text
请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。
```

重点审查 `.pg/project.yaml`：

- `modules` 是否覆盖真正独立构建和测试的模块。
- 每个模块的 `build`、`lint`、`test` 是否可以直接执行。
- `environments` 是否只声明项目真实需要的环境。
- `tracks` 和 `stages` 是否符合团队的开发与验收方式。
- regression suite 是否引用了正确模块和测试键。

自动扫描只能提供初始配置。命令、模块边界或环境判断不准确时，应先修正配置，再运行工作流。

## 第四步：选择第一个变更

第一个变更应当满足：

- 有明确的用户价值或缺陷现象。
- 影响范围有限，通常集中在一个模块或一条调用链。
- 存在可执行的测试或可以补充测试。
- 不要求同时完成大规模重构、迁移和基础设施变更。

适合的例子包括新增只读接口、修复可复现 Bug、补充边界校验。不要把“理解整个系统并重构所有模块”作为第一次任务。

## 第五步：从代码事实开始定界

```text
AI CHAT  /pg-1-define
AI CHAT  <描述需求、现象、约束和非目标，并要求先调查现有实现>
```

define 阶段应引用仓库中的实际入口、模块和测试，而不是根据项目名称猜测架构。定界后再执行：

```text
AI CHAT  /pg-2-propose <change-name>
AI CHAT  /pg-3-build <change-name>
```

完整流程见[入门指南](getting-started.md)，工作流选择见[工作流指南](workflows.md)。

## 大型仓库和 Monorepo

对于大型仓库，优先按可独立构建、测试或部署的边界划分 `modules`，不要简单地把每个源码目录都当成模块。第一次接入时可以只配置当前变更需要的环境和测试，后续再增量完善。

如果一次变更跨越多个模块，应在 proposal 中明确：

- 每个模块的职责和修改边界。
- 跨模块接口或数据契约。
- 各模块独立验证与整体集成验证。
- 哪些测试可以并行，哪些必须串行。

## 团队使用建议

- 将 `.pg/project.yaml`、项目级 hooks 和有审计价值的变更产物纳入版本控制。
- 不提交密钥、个人机器路径、临时日志或本地服务凭据。
- 在 Pull Request 中同时审查业务代码与 `.pg/changes/<change>/` 中的方案和验收记录。
- 升级 pg-skills 后先运行 `pg doctor`，再开始新的变更。

首次执行标准 build 前还应确认工作树干净、当前位于预期功能分支、`.pg/project.yaml` 的 `git.default_branch` 正确，并且团队允许工作流自动 push 和合并。回归流程当前会把测试脚本修复直接推送到 `master`，默认分支不是 `master` 的仓库尤其需要先评估。

## 真实项目示例

希望查看这些原则如何落到一个真实代码库，请阅读 [celer-route 实战教程](../tutorials/celer-route.md)。

