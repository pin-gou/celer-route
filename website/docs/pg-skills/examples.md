# 示例与教程

本页提供可以照着使用的通用场景，并链接到完整的真实项目教程。`TERMINAL` 表示在终端输入，`AI CHAT` 表示在 AI 开发工具对话框输入。

下面的需求名称和业务背景均为虚构示例，目的是说明工作流选择，不依赖某个特定仓库。正式实施前，仍应让 define 阶段调查当前代码。

## 完整真实项目教程

如果你希望在一个已经接入 pg-skills 的真实代码库中，从接入检查一路执行到构建和验收，请阅读 [celer-route 实战教程](../tutorials/celer-route.md)。该教程单独说明仓库结构、现有配置、变更过程和检查点，不把真实项目细节混进入门指南。

## 示例 1：新增一个小功能

**适用场景：** 需求相对清楚，但会影响接口、实现和测试。

```text
AI CHAT  /pg-1-define
AI CHAT  为当前服务增加只读健康状态端点。调查现有路由、健康检查和测试方式，确认范围与验收标准，不要立即编码。
```

定界完成后确认 change-id，例如 `add-health-endpoint`：

```text
AI CHAT  /pg-2-propose add-health-endpoint
```

打开并审查：

```text
.pg/changes/add-health-endpoint/proposal.md
.pg/changes/add-health-endpoint/design.md
.pg/changes/add-health-endpoint/tasks.md
.pg/changes/add-health-endpoint/execution-manifest.yaml
```

确认方案后：

```text
AI CHAT  /pg-3-build add-health-endpoint
```

**完成标准：** 接口行为、测试、Review、Verify 和必要的环境验证均有记录，而不仅是代码可以编译。

## 示例 2：修复一个 Bug

**适用场景：** 有明确的错误现象、日志或 Issue。

```text
AI CHAT  /pg-5-fix-issue

现象：会话仍有活动请求时被提前判定为超时。
请先复现并定位根因，再进行最小修复；补充能够失败于旧代码、通过于新代码的测试。
```

一个好的 Bug 输入应包含：

- 观察到的行为。
- 期望行为。
- 复现条件。
- 日志、错误消息或相关测试。
- 明确的非目标。

不要只写“超时逻辑有问题，帮我修一下”。这会迫使 Agent 猜测范围。

## 示例 3：快速完成局部任务

**适用场景：** 变更局部、边界明确、不需要独立方案评审。

```text
AI CHAT  /pg-2b-quick-build

修正文档中过期的命令示例，并运行文档链接检查。不要修改运行时逻辑。
```

quick-build 会直接进入实现和验证。执行前问自己：如果 AI 理解错了，是否需要先通过 proposal 发现？如果答案是“是”，应改用标准流。

## 示例 4：执行回归测试

项目的 `.pg/project.yaml` 可以声明多个 regression suite。假设存在 `unit`：

```text
AI CHAT  /pg-4-regression unit
```

预期流程：

1. 读取 suite 对应的 module、test_keys 和 environment。
2. 准备环境并执行测试。
3. 收集失败并区分测试问题与生产代码问题。
4. 在允许的边界内修复测试。
5. 输出问题清单，并在需要时派送生产代码修复。

运行后检查 `.pg/regression/` 中的 session 日志和汇总，不要只看聊天中的最后一句话。

## 示例 5：需求不清楚，先探索

```text
AI CHAT  /pg-1-define

当前服务的启动速度变慢了，但我还不知道瓶颈在哪里。
请读取启动路径和初始化代码，列出可能原因、证据和可选改进方向；这次先不创建实现方案。
```

define 可以只完成调查和定界。没有足够证据时，不必强行进入 propose。

## 示例 6：中断后继续构建

如果 `/pg-3-build` 因工具会话关闭或可恢复错误中断，不要新建同名 change，也不要删除 `2-build/`，尤其是其中的 `pipeline.events` 和 `pipeline.snapshot.json`。

先检查状态和错误，再重新调用：

```text
AI CHAT  /pg-3-build add-health-endpoint
```

Runner 会根据事件和 snapshot 判断是否可以 resume。若报告 `env_drift` 或 `design_drift`，应先修复环境或重新生成设计，而不是反复重跑。

## 示例 7：检查项目而不修改代码

```text
AI CHAT  请加载 pg-init-project skill，只检查当前 .pg/project.yaml 与仓库结构是否一致，列出差异，不要修改文件。
```

这种自然语言调用适合 Skill 本身没有独立 Slash Command 的情况。用户目标和“不要修改”的边界应写清楚。

## 从终端检查工作结果

```powershell
python .pg\skills\src\runtime\bin\pg doctor
git status --short
git diff --stat
```

标准变更还应查看：

```powershell
dir .pg\changes
dir .pg\changes\add-health-endpoint\2-build
```

## 下一步

- 需要真实仓库的完整演练：[celer-route 实战教程](../tutorials/celer-route.md)。
- 正在接入已有大型代码库：[在现有项目中使用 pg-skills](existing-projects.md)。
- 不确定应该选哪个示例：[工作流指南](workflows.md)。
- 需要完整命令说明：[命令参考](commands.md)。
- 遇到失败：[故障排查](troubleshooting.md)。
