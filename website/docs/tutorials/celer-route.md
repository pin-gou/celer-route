---
title: 使用 pg-skills 完成一次真实变更
description: 在已经接入 pg-skills 的 celer-route 项目中完成定界、提案、构建和结果验证
sidebar_label: pg-skills 实战
---

# celer-route 真实项目逐步教程

本教程使用真实的 [pin-gou/celer-route](https://github.com/pin-gou/celer-route) 项目，带你完成一次可复现的 pg-skills 标准工作流。你会检查项目中已有的 pg-skills 配置，理解任务边界，然后依次执行 `define → propose → build`，最后用 Git、测试命令和 Pipeline 产物判断结果是否真的成功。


## 你将完成什么

完成本教程后，你应该能够独立完成以下事情：

1. 确认真实项目中的 pg-skills、OpenCode 命令和项目配置已经就绪。
2. 根据现有模块、测试和环境配置判断任务应该落在哪个 Track。
3. 在执行前发现范围、测试命令和环境假设中的问题。
4. 用 define 调查真实代码，而不是让 AI 猜测需求位置。
5. 审查 proposal、design、tasks 和 execution manifest。
6. 在理解自动 Git 操作的前提下安全执行 build。
7. 根据测试、Git 历史和 `2-build/` 产物判断成功或失败。

本教程预计需要 45 到 90 分钟，不包括首次下载 Go 和 Node.js 依赖的时间。

## 本次练习

练习的 change-id 固定为：

```text
test-health-handler-contract
```

练习目标是为现有健康检查补充单元测试，至少验证：

- `DisableDBPingsInHealth=true` 时返回 HTTP 200。
- 响应 JSON 中 `status` 为 `ok`，`components.db_pings` 为 `disabled`。
- DB Ping 未禁用、三个 Store 都为空时返回 HTTP 200。
- 此时 `components.db_pings` 为 `ok`。
- 响应类型是 JSON。
- 不修改 `/health` 的生产行为，不启动真实数据库，不引入外部服务。

预期主要新增文件是：

```text
transports/celer-route-http/handlers/health_test.go
```

这不是要求 AI 照抄某段测试代码。它必须先阅读当前实现和仓库测试惯例，再决定测试结构。

## 1. 准备环境

本文以 Windows PowerShell 和 OpenCode 为主路径。需要：

- Git。
- Python 3。
- Go。当前项目的 `core/go.mod`、`framework/go.mod` 和 `transports/go.mod` 声明 Go `1.26.5`。
- Node.js。仓库 `.nvmrc` 声明 `22.12.0`。
- 已安装并能够正常使用的 OpenCode。

先检查：

```powershell
git --version
python --version
go version
node --version
opencode --version
```

第 5 步的手工预检只编译 Go handler 测试，不要求构建 UI；但第 14 步的 pg-build 会遵守项目现有 Stage 配置。当前 `dev.environment.required` 为 `true`，因此 Runner 会准备 `local` 环境，相关 Hook 可能安装 UI 依赖、构建 UI 嵌入资源和 celer-route HTTP 服务。要完成端到端 build，Git Bash、Go、Node.js、npm 和项目依赖都必须可用。OpenCode 使用的模型和 Provider 也应提前配置完成。

> celer-route 的 Makefile 明确使用 Bash。Windows 下运行 `make build` 或 `make dev` 时，应使用 Git Bash、WSL 或其他兼容 Bash 的环境。本文的聚焦测试直接调用 `go test`，不依赖 Makefile。

## 2. 获取项目代码

如果本机还没有 celer-route，可以直接克隆项目：

```powershell
git clone https://github.com/pin-gou/celer-route.git
cd celer-route
```

本教程假设你使用的是已经接入 pg-skills 的项目版本。此时不需要再安装 pg-skills，也不需要重新生成 OpenCode 适配目录。后续步骤会先检查这些文件是否齐全。

如果你已经有 celer-route 工作目录，直接进入项目根目录即可，但应先确认没有未完成的个人修改：

```powershell
git status --short
```

## 3. 确认 pg-skills 已经就绪

在项目根目录执行：

```powershell
Test-Path .pg\skills\src\runtime\bin\pg
Test-Path .pg\project.yaml
Test-Path .opencode\commands\pg-1-define.md
Test-Path .opencode\skills\pg-define\SKILL.md
Test-Path .opencode\agents\pg-manager.md
Get-Content .pg\tool-integration.json
```

前五项都应返回 `True`，工具记录应显示：

```json
{
  "schema_version": 1,
  "tool": "opencode"
}
```

再运行结构检查：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

如果上述文件缺失，不要按本文重新安装或初始化。先确认自己是否位于 celer-route 项目根目录、是否使用包含 pg-skills 的项目版本，并查看[故障排查](../pg-skills/troubleshooting.md)。

最后确认本次练习目标尚未实现：

```powershell
Test-Path transports\celer-route-http\handlers\health_test.go
```

预期返回 `False`。如果已经存在该文件，请不要重复练习同一个任务。

## 4. 先认识真实仓库

开始工作流前，先查看仓库事实：

```powershell
Get-ChildItem -Directory
Get-Content .nvmrc
Get-Content transports\go.mod -TotalCount 8
Select-String -Path transports\celer-route-http\handlers\health.go -Pattern 'RegisterRoutes|/health|DisableDBPingsInHealth'
```

当前项目的主要边界包括：

| 目录 | 作用 | 主要技术 |
|---|---|---|
| `core/` | 网关核心引擎和 Schema | Go module |
| `framework/` | 配置、存储、Tracing 等基础设施 | Go module |
| `transports/` | HTTP 传输与兼容接口 | Go module |
| `plugins/` | 多个独立 Go Plugin module | Go |
| `ui/` | 管理界面 | TypeScript、Vite |

健康检查的真实调用关系是：

```text
server/server.go
  → NewHealthHandler(...)
  → HealthHandler.RegisterRoutes(...)
  → GET /health
  → HealthHandler.getHealth(...)
```

先记住这些事实。后面如果 AI 把任务分配给 `core` 或 `ui`，你就有依据判断它的范围可能错了。

## 5. 验证项目能够编译

先只编译 handler 测试包，不运行测试：

```powershell
Push-Location transports\celer-route-http
go test ./handlers -run '^$' -count=1
Pop-Location
```

预期命令退出码为 0。首次执行可能下载 Go 依赖。

如果此时失败，先处理 Go 版本、网络、私有模块或项目自身问题。不要把“原项目无法编译”带入 pg-skills 工作流，否则后面无法区分是适配问题还是项目环境问题。

## 6. 认识现有 pg-skills 配置

celer-route 已经提交了以下内容：

```text
.pg/
├── skills/                 pg-skills 源码
├── project.yaml            项目模块、环境、Track 和 Stage 配置
├── hooks/                  本地环境生命周期脚本
├── code-review/            代码审查规则
└── changes/                变更产物和归档记录

.opencode/
├── commands/               OpenCode Slash Commands
├── skills/                 OpenCode 可加载的 Skills
├── agents/                 测试、开发、审查和验证角色
└── .pg-adapter-manifest.json
```

这些文件是项目的一部分，不需要每位用户重复生成。你在本教程中主要使用：

- `.pg/project.yaml`：确认本次任务属于 `transports`。
- `.opencode/commands/`：调用 define、propose 和 build。
- `.pg/changes/<change-id>/`：查看需求、方案和运行证据。
- `.pg/skills/`：提供工作流脚本和运行时。

完整目录说明见[项目目录与产物](../pg-skills/project-structure.md)。

## 7. 检查 OpenCode 命令和模型路由

查看项目已经提供的命令：

```powershell
Get-ChildItem .opencode\commands
```

至少应包含：

```text
pg-1-define.md
pg-2-propose.md
pg-3-build.md
```

生成的 Agent 使用以下三级路由：

```text
pg-router/pg-associate
pg-router/pg-expert
pg-router/pg-master
```

项目中的 `opencode.json` 已声明这些名称，但你的 OpenCode 环境仍需能够把它们解析到实际模型。开始工作流前先确认 Provider 或路由插件可用，具体机制见[模型路由指南](../pg-skills/model-routing.md)。

## 8. 启动 OpenCode

从 celer-route 项目根目录启动：

```powershell
opencode
```

在命令列表中确认存在：

```text
/pg-1-define
/pg-2-propose
/pg-3-build
```

如果命令文件存在但界面中没有显示，请确认 OpenCode 打开的是当前项目根目录，然后重新加载项目。不要再次运行安装流程。命令加载原理见[命令如何工作](../pg-skills/how-commands-work.md)。

## 9. 阅读现有项目配置

打开 `.pg/project.yaml`，先理解项目已经定义的执行边界，不要为了本次练习随意重写配置。

### 9.1 模块

项目包含 `core`、`framework`、`transports`、`plugins` 和 `ui`。本次健康检查测试属于 `transports`，不应修改其他模块。

### 9.2 测试命令

`transports.test.unit` 对应：

```text
cd transports/celer-route-http && go test ./... -short -count=1
```

本教程还会手工运行更聚焦的 handlers 测试，用来证明新增用例确实执行。

### 9.3 环境与 Stage

现有配置包含 `dev` 和 `int` Stage，以及 local 环境生命周期 Hooks。当前两个 Stage 的 `environment.required` 都是 `true`。对于本次只影响 `transports` 的变更，manifest 应启用 `dev.transports`，并禁用不相关的 Track；即便如此，Runner 仍会为启用的 `dev` Stage 准备 `local` 环境。执行前应阅读配置和 Hook 脚本，确认 Git Bash、Go、Node.js 和 npm 可用，不要把环境准备失败误判为测试代码失败。

### 9.4 Git

正常项目配置的默认分支是 `main`。不要为了教程改成内部练习分支：

```powershell
Select-String -Path .pg\project.yaml -Pattern 'default_branch'
```

预期看到 `default_branch: main`。配置字段的详细说明见[配置指南](../pg-skills/configuration.md)。

## 10. 做一次只读加载验证

在 OpenCode 对话框发送：

```text
请加载 pg-define skill，只读取当前项目并回答：
1. 项目名称；
2. 主要语言；
3. .pg/project.yaml 中定义了哪些 modules；
4. 本次健康检查测试应属于哪个 module。

不要修改任何文件。
```

回答应识别 celer-route、Go/TypeScript、多模块结构，并把任务归入 `transports`。这一步通过只说明 Skill 能被加载，不代表完整工作流已经成功。

## 11. 用 define 调查真实代码

回到 OpenCode 对话框，输入：

```text
/pg-1-define
```

然后发送完整任务：

```text
change-id 使用 test-health-handler-contract。

请为 celer-route 现有 GET /health 处理器补充契约测试。先调查当前实现、路由注册、响应工具函数和 handlers 目录中的测试惯例，不要立即修改业务代码。

验收要求：
1. DisableDBPingsInHealth=true 时返回 HTTP 200、JSON status=ok、components.db_pings=disabled。
2. DB Ping 未禁用且 ConfigStore、LogsStore、VectorStore 都为空时返回 HTTP 200、status=ok、components.db_pings=ok。
3. 响应 Content-Type 是 application/json。
4. 新测试不连接数据库、不启动网关、不需要 API 密钥。
5. 不改变 /health 生产行为；除测试文件外不修改业务源码。
6. 使用聚焦 Go 测试命令验证。

请明确列出目标文件、非目标、风险、验证命令和为什么该任务属于 transports 模块。
```

### define 阶段应该发现什么

至少应提到：

- `transports/celer-route-http/handlers/health.go`。
- 路由是 `GET /health`，不是新建一个“健康摘要”路由。
- `getHealth` 是同 package 的未导出方法，测试可以沿用 handlers 包的现有风格。
- `SendJSON` 负责设置 JSON Content-Type。
- 本次变更不需要 `core`、`framework`、`plugins` 或 `ui`。
- 新测试自身不依赖数据库或在线服务；但当前项目的 Pipeline 配置仍会在 build 时准备 `local` 环境。

如果 define 建议新增生产接口、启动数据库或修改 UI，先追问其代码证据，不要继续 propose。

### 检查 define 产物

在终端查看：

```powershell
Get-Content .pg\changes\test-health-handler-contract\0-define\define-summary.yaml
```

检查 change-id、目标、非目标、验收条件和环境要求是否与刚才确认的一致。

如果 define 请求执行 `describe_env`，本次纯单元测试不应依赖在线服务。要求它说明为什么需要环境；没有真实需要时，应把相关验收标记为不依赖环境，而不是伪造一个“已验证”的服务状态。

## 12. 用 propose 生成执行方案

在 OpenCode 对话框输入：

```text
/pg-2-propose test-health-handler-contract
```

生成后，不要直接 build。逐个打开：

```powershell
Get-Content .pg\changes\test-health-handler-contract\proposal.md
Get-Content .pg\changes\test-health-handler-contract\design.md
Get-Content .pg\changes\test-health-handler-contract\tasks.md
Get-Content .pg\changes\test-health-handler-contract\execution-manifest.yaml
```

### proposal 审查清单

- 问题是现有健康契约缺少专门测试，而不是缺少健康接口。
- 范围只包含 handler 测试。
- 非目标明确排除生产逻辑、数据库、UI 和 Provider 配置。
- 验收条件包含具体状态码和 JSON 字段。

### design 审查清单

- 说明测试如何构造 `fasthttp.RequestCtx`。
- 说明如何创建最小 `lib.Config`。
- 使用 `json.Unmarshal` 或等价结构化断言，不依赖 JSON Map 的字段顺序。
- 不为了测试而导出 `getHealth`。
- 不创建真实 ConfigStore、LogsStore 或 VectorStore。

### tasks 审查清单

合理的任务通常只有：

1. 阅读 handler 和同目录测试惯例。
2. 新增 `health_test.go` 并覆盖两个成功分支。
3. 运行聚焦测试。
4. 进行 Go Review 和最终验证。

如果 tasks 出现“重构全部健康系统”“新增前端页面”或“配置真实数据库”，说明范围已经漂移。

### execution manifest 审查清单

- module 应是 `transports`。
- 选择的 Track 应只覆盖 `transports`。
- 不应派发 `core`、`framework`、`plugins` 或 `ui` 的开发任务。
- 测试命令应进入 `transports/celer-route-http`。
- `dev.transports` 应启用并选择项目已有的 `local` 环境，因为当前配置明确要求环境准备。
- `dev` 下的 `core`、`framework`、`plugins`、`ui` Track 应禁用；`int.scr` 也应禁用，因为本次没有跨模块场景变更。
- 新增测试仍应是纯单元测试；Runner 准备 local 环境是项目级编排要求，不代表测试可以依赖该服务。
- Review profile 应适用于 Go。

发现错误时，要求 AI 修订 proposal 产物；不要靠 build 阶段“自动猜对”。

## 13. 执行 build 前的安全检查

标准 build 成功后会自动提交、推送功能分支，并把结果合并、推送到 `.pg/project.yaml` 指定的 `main`。因此，完整执行 build 前必须使用自己有写权限的 celer-route Fork。

如果当前 `origin` 指向官方仓库，先在 GitHub 创建 Fork，然后执行：

```powershell
git remote rename origin upstream
git remote add origin https://github.com/<你的账号>/celer-route.git
git push -u origin main
```

如果一开始克隆的就是自己的 Fork，只需确认 `origin` 正确。

提交 define 和 propose 产生的变更资料：

```powershell
git status --short
git diff -- .pg/changes/test-health-handler-contract
git add .pg/changes/test-health-handler-contract
git commit -m "docs(pg): define health handler contract tests"
git push origin main
```

然后逐项检查：

```powershell
git status --short
git branch --show-current
git remote get-url origin
git branch -vv
Select-String -Path .pg\project.yaml -Pattern 'default_branch'
```

预期：

```text
工作树                 无未提交修改
当前分支               main
origin                 你的 Fork
main tracking          origin/main
git.default_branch     main
```

任一项不符合都先停止。不要手工创建功能分支；Runner 会从 `main` 创建 `feat/pg/test-health-handler-contract`。

## 14. 执行 build

在 OpenCode 对话框输入：

```text
/pg-3-build test-health-handler-contract
```

运行过程中，pg-build 会根据 manifest 派送任务，并把事件和 snapshot 写入该 change 的 `2-build/`。不要在中途删除这个目录，也不要手工修改 Pipeline 状态来“制造成功”。

如果 OpenCode 要求批准文件修改、测试或 Git 操作，请根据刚才审查的范围决定。超出 `health_test.go`、聚焦测试和本次变更所需 Git 操作的请求，应先询问原因。

## 15. 用证据判断是否成功

不要只看 OpenCode 回复中的“完成”两个字。按下面四组证据检查。

### 15.1 代码证据

```powershell
Test-Path transports\celer-route-http\handlers\health_test.go
git show --stat --oneline HEAD
git show --name-only --format='' HEAD
```

应看到测试文件。除 pg-skills 运行产物外，不应出现无关业务模块修改。

### 15.2 测试证据

亲自在终端运行聚焦测试：

```powershell
Push-Location transports\celer-route-http
go test ./handlers -run '^TestHealth' -count=1
Pop-Location
```

预期退出码为 0，并且输出不是 `[no tests to run]`。如果实际测试函数使用了不同名称，先打开 `health_test.go`，再把 `-run` 改为真实名称。

然后运行整个 handlers 包：

```powershell
Push-Location transports\celer-route-http
go test ./handlers -count=1
Pop-Location
```

聚焦测试证明新用例执行；整个 package 测试用于发现同包回归。二者不能互相替代。

### 15.3 Git 证据

build 运行中会使用 `feat/pg/test-health-handler-contract`。成功链路结束后，pg-verify-and-merge 通常会让工作区停留在配置的默认分支：

```powershell
git branch --show-current
git log --oneline --decorate -5
git status --short
```

本教程预期当前分支是：

```text
main
```

并且 `origin/main` 包含本次 squash merge。若 build 在合并前失败，工作区可能保留在失败现场，这是诊断证据，不要擅自切分支后宣称成功。

### 15.4 Pipeline 证据

成功后 change 可能已移动到 `.pg/changes/archive/`。查找运行状态：

```powershell
Get-ChildItem .pg\changes -Recurse -Filter pipeline.snapshot.json
Get-ChildItem .pg\changes -Recurse -Filter pipeline.events
```

打开对应 `test-health-handler-contract` 的 `2-build/`，确认：

- Test 阶段实际执行了哪条命令。
- Dev 阶段只修改了约定范围。
- Review 是否有发现以及如何处理。
- Verify 和 Gate 的最终状态。
- 失败重试是否被完整记录。

对话总结、代码 diff、测试输出、Git 结果和 Pipeline 状态同时一致，才能判定端到端成功。

## 16. 你应该得到的最终结果

完成本教程后，结果应满足：

```text
业务行为               GET /health 语义不变
主要代码变化           新增 health_test.go
受影响模块             transports
测试外部依赖           不需要数据库或在线服务
Pipeline 环境          当前配置会准备 local 环境
聚焦测试               通过且确实执行了 TestHealth*
handlers 包测试         通过
合并目标               你的 Fork 中 main
Pipeline 状态           done，或有明确可解释的失败记录
```

如果 AI 实现与文件名略有不同，但满足相同边界和验收条件，可以接受。教程评价的是工程证据，不是文本完全一致。

## 17. 常见失败及处理

### pg-skills 文件不存在

如果 `.pg/skills/`、`.pg/project.yaml` 或 `.opencode/` 不存在，先检查：

```powershell
Get-Location
git branch --show-current
git status --short
```

本文面向已经接入 pg-skills 的 celer-route 项目版本，不包含重新安装步骤。请切换到正确项目版本或恢复被误删的受管文件，再继续操作。

### OpenCode 中没有 pg 命令

检查：

```powershell
Test-Path .opencode\commands\pg-1-define.md
Test-Path .opencode\skills\pg-define\SKILL.md
Test-Path .opencode\agents\pg-manager.md
```

确认文件存在后，从项目根目录重新启动或重新加载 OpenCode。目录存在只证明项目已经提供适配文件，命令能出现在 OpenCode 中才证明宿主已经加载。

### Agent 模型无法解析

检查 `opencode.json` 和当前 OpenCode Provider 配置，确认 `pg-router/pg-associate`、`pg-router/pg-expert`、`pg-router/pg-master` 都能映射到真实模型。模型路由属于 OpenCode 配置，不应写进 `.pg/project.yaml`。

### `pg doctor` 报 schema 错误

celer-route 已经提交了完整的 `.pg/project.yaml`，不要通过重新初始化来掩盖错误。先阅读 `pg doctor` 的具体报错，再用 `git diff -- .pg/project.yaml` 检查文件是否被误改；如果文件与仓库版本一致，则根据报错核对 pg-skills 版本、Schema 引用和项目文件是否完整。

### Go 测试在 define 前就失败

这是项目自身或本机环境问题，不是本次健康测试变更造成的。记录完整命令和错误，先修复 Go 版本、依赖下载或仓库状态，再重新开始练习。

### build 报工作树不干净

运行：

```powershell
git status --short
```

不要盲目 `git add -A`。启动 build 前应位于 `main`，并已提交、推送 define/propose 产物。逐项判断剩余内容是本次 change、用户自己的修改还是临时文件；只提交确认应该进入仓库的内容。

### build 在 push 阶段失败

检查：

```powershell
git remote -v
git branch -vv
Select-String -Path .pg\project.yaml -Pattern 'default_branch'
```

最常见原因是 `origin` 指向官方仓库、功能分支没有 upstream，或默认分支没有先推到 Fork。

### AI 说测试通过，但终端显示没有测试运行

以终端为准。打开 `health_test.go` 查找真实测试名，并用 `go test -run` 精确执行。`[no tests to run]` 不是成功证据。

### build 中断后如何继续

保留：

```text
.pg/changes/test-health-handler-contract/2-build/pipeline.events
.pg/changes/test-health-handler-contract/2-build/pipeline.snapshot.json
```

修复明确问题后，在 OpenCode 对话框再次输入：

```text
/pg-3-build test-health-handler-contract
```

Runner 会依据现有事件和 snapshot 判断是否能继续。不要新建同名 change，也不要先删除 `2-build/`。

## 18. 练习结束后的复盘

请不要只问“代码写出来了吗”。用下面的问题复盘：

1. celer-route 已经提交了哪些 pg-skills 和 OpenCode 文件，它们分别由谁读取？
2. 为什么本次任务属于 `transports`，而不是 `core` 或 `ui`？
3. define 找到了哪些真实文件和调用关系？
4. propose 为什么只选择 `transports`？
5. execution manifest 实际运行了哪条测试命令？
6. Review 和 Verify 的证据分别在哪里？
7. 为什么完整 build 前必须把 `origin` 指向自己有写权限的 Fork？
8. 如果运行中断，哪些文件保证可以诊断或恢复？

能够根据实际文件回答这些问题，才说明你学会了 pg-skills 的工作方式，而不只是照着输入了三条 Slash Command。

## 下一步

- 把相同方法用于一个小型生产缺陷，但仍先确认项目状态和验收证据。
- 阅读 [pg-skills 在现有项目中的使用指南](../pg-skills/existing-projects.md)，处理更复杂的 Monorepo 边界。
- 阅读 [pg-skills 项目目录与产物](../pg-skills/project-structure.md)，理解 change、archive 和 `2-build/`。
- 阅读 [OpenCode 完整教程](../pg-skills/tutorials/opencode.md)，进一步了解 Commands、Skills、Agents 和三级模型路由。
- 阅读 [pg-skills 故障排查](../pg-skills/troubleshooting.md)，按现象定位初始化、适配、测试和恢复问题。
