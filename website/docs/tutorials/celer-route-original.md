---
title: celer-route 原始教程（归档）
description: 迁入 celer-route 文档站前的原始 pg-skills 实战教程
sidebar_label: celer-route 原始教程
unlisted: true
---

# celer-route 真实项目逐步教程

本教程使用真实的 [pin-gou/celer-route](https://github.com/pin-gou/celer-route) 仓库，带你完成一次可复现的 pg-skills 标准工作流。你会从一个干净的 Fork 开始，接入 pg-skills，校正项目配置，然后依次执行 `define → propose → build`，最后用 Git、测试命令和 Pipeline 产物判断结果是否真的成功。

> **教程基线：** 本文依据 celer-route 提交 `c2cd181b0b4842d48911ec71e4ee92d77181a1af` 编写。该提交中，健康检查位于 `transports/celer-route-http/handlers/health.go`，注册 `GET /health`，但没有独立的 `health_test.go`。如果新版仓库已经补上这些测试，请检出本教程基线，或者把练习调整为补充仍缺少的分支。

## 你将完成什么

完成本教程后，你应该能够独立完成以下事情：

1. 把 pg-skills 接入一个真实的多模块仓库。
2. 分清 `pg init` 与 `pg-init-project` 的职责。
3. 发现并修正自动扫描结果中的目录、测试和环境假设。
4. 用 define 调查真实代码，而不是让 AI 猜测需求位置。
5. 审查 proposal、design、tasks 和 execution manifest。
6. 在允许自动 Git 操作的隔离分支中执行 build。
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

本文以 Windows PowerShell 和 DeepSeek Harness 为主路径。需要：

- Git。
- Python 3。
- Go。教程基线的 `core/go.mod`、`framework/go.mod` 和 `transports/go.mod` 声明 Go `1.26.5`。
- Node.js。仓库 `.nvmrc` 声明 `22.12.0`。
- 官方 DeepSeek Harness CLI `dsh`，并已配置可用 Provider。
- 一个你有写权限的 celer-route Fork。

先检查：

```powershell
git --version
python --version
go version
node --version
dsh --help
```

本练习只运行 Go handler 测试，不要求构建 UI。Node.js 主要用于 DSH；如果你计划运行 celer-route 的完整构建，还需要满足仓库 Makefile 中的其他依赖。

> celer-route 的 Makefile 明确使用 Bash。Windows 下运行 `make build` 或 `make dev` 时，应使用 Git Bash、WSL 或其他兼容 Bash 的环境。本文的聚焦测试直接调用 `go test`，不依赖 Makefile。

## 2. 克隆你自己的 Fork

不要直接把 `origin` 指向 `pin-gou/celer-route` 后执行本教程。标准 `pg-build` 的成功链路可能推送功能分支，并把结果 squash merge 到 `.pg/project.yaml` 配置的默认分支。

先在 GitHub Fork `pin-gou/celer-route`，然后克隆自己的 Fork：

```powershell
git clone https://github.com/<你的账号>/celer-route.git
cd celer-route
git remote add upstream https://github.com/pin-gou/celer-route.git
git remote -v
```

你应该看到：

```text
origin    https://github.com/<你的账号>/celer-route.git
upstream  https://github.com/pin-gou/celer-route.git
```

如果 `origin` 仍然是 `https://github.com/pin-gou/celer-route.git`，先停下。没有官方仓库写权限时，build 最终会在 push 阶段失败；即使你有写权限，也不应把教程练习直接推向官方默认分支。

## 3. 固定教程基线

拉取官方提交并创建独立的教程基线分支：

```powershell
git fetch upstream
git switch --detach 2d7ebb7886034cab145684ca23495537b8e028ec
git switch -c tutorial/pg-skills-base
git status --short
```

最后一条命令应没有输出。再确认当前提交：

```powershell
git rev-parse HEAD
```

预期：

```text
2d7ebb7886034cab145684ca23495537b8e028ec
```

教程使用独立的 `tutorial/pg-skills-base`，而不是 `main`。后面即使 build 自动合并，影响的也只是你 Fork 中的教程分支。

## 4. 先认识真实仓库

在接入 pg-skills 前，先查看仓库事实：

```powershell
Get-ChildItem -Directory
Get-Content .nvmrc
Get-Content transports\go.mod -TotalCount 8
Select-String -Path transports\celer-route-http\handlers\health.go -Pattern 'RegisterRoutes|/health|DisableDBPingsInHealth'
```

在教程基线中，主要边界包括：

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

## 5. 验证代码基线能编译

先只编译 handler 测试包，不运行测试：

```powershell
Push-Location transports\celer-route-http
go test ./handlers -run '^$' -count=1
Pop-Location
```

预期命令退出码为 0。首次执行可能下载 Go 依赖。

如果此时失败，先处理 Go 版本、网络、私有模块或仓库基线问题。不要把“原项目无法编译”带入 pg-skills 工作流，否则后面无法区分是适配问题还是项目环境问题。

## 6. 安装 pg-skills

### 已发布版本

当包含 DeepSeek Harness 适配器的版本已经发布时，按照[安装指南](https://github.com/pin-gou/pg-skills/blob/main/docs/installation.md)使用固定 tag 或 commit，通过 Git subtree 安装到：

```text
.pg/skills/
```

不要在教程中使用浮动的 `main`，否则同一套步骤可能在不同日期得到不同结果。

### 验证尚未合并的本地适配器

如果你正在验证本机 `D:\sx\pg-skills-deepseek-harness` 中尚未发布的分支，可在 celer-route 根目录执行：

```powershell
git remote add pg-skills-local D:\sx\pg-skills-deepseek-harness
git fetch pg-skills-local feature/deepseek-harness-adapter
git subtree add --prefix=.pg/skills pg-skills-local feature/deepseek-harness-adapter --squash
```

如果 remote 已存在，先检查而不是重复添加：

```powershell
git remote -v
```

安装后确认：

```powershell
Get-Content .pg\skills\VERSION
Test-Path .pg\skills\src\runtime\bin\pg
```

`Test-Path` 应返回 `True`。

## 7. 生成 DeepSeek Harness 适配目录

在 celer-route 根目录执行：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool deepseek-harness
```

这一步生成公共项目骨架和 DSH 适配层，但不会替你正确理解 celer-route 的所有模块和运行环境。

检查生成结果：

```powershell
Get-ChildItem .dsh
Get-ChildItem .dsh\commands
Get-ChildItem .dsh\skills
```

当前适配器应把 DSH 文件统一放在一个目录：

```text
.dsh/
├── commands/
├── agents/
├── skills/
├── bridge/
├── cordis.patch.yml
├── start-web.cmd
├── run-task.cmd
├── run.cmd
└── README.md
```

- `start-web.cmd` 启动交互式 Web 页面。
- `run-task.cmd "任务"` 执行一次 Headless 任务并在终端返回结果。
- `run.cmd` 只显示两种入口的说明，不启动 DSH。

如果没有 `.dsh/`，先执行：

```powershell
Get-Location
Test-Path .pg\skills\src\integrations\deepseek_harness\adapter.py
python .pg\skills\src\runtime\bin\pg init --list-tools
```

最常见原因是终端不在 celer-route 根目录，或 `.pg/skills/` 并不是包含 DSH 适配器的版本。

## 8. 启动 DSH 并初始化项目配置

启动交互式 Web 模式：

```powershell
.dsh\start-web.cmd
```

在 Web 页面确认当前工作区是 celer-route，然后发送：

```text
请加载 pg-init-project skill，扫描当前 celer-route 仓库并初始化 pg-skills 项目配置。

要求：
1. 识别真实的 Go modules、UI 和插件边界。
2. 所有 build、lint、test 命令都说明从哪个目录执行。
3. 不凭空编造端口、数据库和外部服务。
4. 完成后列出 modules、environments、tracks、stages、regression suite，以及仍需人工确认的项目。
```

这一步会生成或更新：

```text
.pg/project.yaml
.pg/hooks/
.pg/context/
.pg/code-review/
```

`pg init` 与 `pg-init-project` 不相同：

| 动作 | 负责什么 |
|---|---|
| 终端中的 `pg init --tool deepseek-harness` | 建立 `.pg/` 骨架并渲染 `.dsh/` 适配文件 |
| DSH 对话中的 `pg-init-project` Skill | 阅读真实仓库并生成项目级 modules、environments、tracks 和 stages |

## 9. 人工审查 `.pg/project.yaml`

不要因为 YAML 已生成就直接运行 build。打开 `.pg/project.yaml`，先检查以下内容。

### 9.1 模块边界

本教程基线至少应识别 `core`、`framework`、`transports`、`plugins` 和 `ui`。本次练习只属于 `transports`。

### 9.2 命令工作目录

Runner 从项目上下文执行 module 命令。命令必须能准确进入对应 module，不能假设仓库根目录存在 `go.mod` 或 `package.json`。

可在终端逐条验证聚焦命令：

```powershell
Push-Location core
go test ./... -short -count=1
Pop-Location

Push-Location framework
go test ./... -short -count=1
Pop-Location

Push-Location transports\celer-route-http
go test ./handlers -run '^$' -count=1
Pop-Location
```

对本次练习，`transports` 的测试配置至少要能够表达等价于下面的命令：

```text
cd transports/celer-route-http && go test ./handlers -count=1
```

如果自动生成的 `core.build` 是仓库根目录执行的 `go build ./...`，或 `ui.build` 是仓库根目录执行的 `npm run build`，应先修正。celer-route 根目录没有统一的 `go.mod` 或 `package.json`。

### 9.3 环境要求

本练习只测试 `fasthttp.RequestCtx` 和内存中的 `lib.Config`，不需要启动 API、UI 或数据库。因此负责单元测试的 Stage 应允许：

```yaml
environment:
  required: false
```

不要为了让教程“看起来完整”而编造端口。celer-route Makefile 的默认 API 和 UI 端口分别是 `8080` 和 `3000`，但真实运行时可以被环境变量覆盖。只有实际启动命令和团队配置才能决定 `.pg/hooks/` 应使用什么端口。

### 9.4 Git 默认分支

本教程故意不向 `main` 合并。把配置改为：

```yaml
git:
  default_branch: tutorial/pg-skills-base
```

然后运行结构检查：

```powershell
python .pg\skills\src\runtime\bin\pg doctor
```

`pg doctor` 通过只说明结构和引用基本有效，不代表每条项目命令已经在本机成功执行。

## 10. 提交并推送教程基线

build 要求可追踪的 Git 基线。先审查初始化产生的文件：

```powershell
git status --short
git diff -- .pg\project.yaml .pg\hooks .pg\code-review .dsh
```

特别检查：

- 没有 API 密钥、Token 或个人凭据。
- `.dsh/cordis.patch.yml` 的三级模型配置符合你的环境。
- patch 中的 bridge URI 指向当前项目。
- `.pg/project.yaml` 的默认分支是 `tutorial/pg-skills-base`。
- Hooks 中没有未经验证的生产地址。

确认后提交并推送到你的 Fork：

```powershell
git add .pg .dsh pg-run pg-run.cmd
git commit -m "chore: add pg-skills tutorial baseline"
git push -u origin tutorial/pg-skills-base
```

不要手工创建练习功能分支。后面的 define 和 propose 都在 `tutorial/pg-skills-base` 上进行；启动 build 时，Runner 会自动创建固定名称的功能分支：

```text
feat/pg/test-health-handler-contract
```

先确认仍在教程基线分支，并且工作树干净：

```powershell
git branch --show-current
git status --short
```

预期当前分支是 `tutorial/pg-skills-base`，最后一条命令没有输出。

## 11. 用 define 调查真实代码

回到 DSH Web 页面，输入：

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
- 本次验证不需要运行环境。

如果 define 建议新增生产接口、启动数据库或修改 UI，先追问其代码证据，不要继续 propose。

### 检查 define 产物

在终端查看：

```powershell
Get-Content .pg\changes\test-health-handler-contract\0-define\define-summary.yaml
```

检查 change-id、目标、非目标、验收条件和环境要求是否与刚才确认的一致。

如果 define 请求执行 `describe_env`，本次纯单元测试不应依赖在线服务。要求它说明为什么需要环境；没有真实需要时，应把相关验收标记为不依赖环境，而不是伪造一个“已验证”的服务状态。

## 12. 用 propose 生成执行方案

在 DSH Web 页面输入：

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
- 单元测试 Stage 不应要求 local API/UI 环境。
- Review profile 应适用于 Go。

发现错误时，要求 AI 修订 proposal 产物；不要靠 build 阶段“自动猜对”。

## 13. 提交方案并通过 build 安全闸

define 和 propose 会在默认分支工作区生成 change 产物，但 `pg-build` 的分支守卫要求从干净的默认分支启动。先只审查并提交本次 change：

```powershell
git status --short
git diff -- .pg/changes/test-health-handler-contract
git add .pg/changes/test-health-handler-contract
git commit -m "docs(pg): add health handler contract test proposal"
git push origin tutorial/pg-skills-base
```

如果 `git status --short` 还显示 change 目录之外的修改，不要把它们顺手加入这次提交。先判断来源并单独处理。

标准 build 可能自动 commit、push、rebase，并把功能分支 squash merge 到配置的默认分支。执行前逐条确认：

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
当前分支               tutorial/pg-skills-base
origin                 你的 Fork
教程基线 tracking      origin/tutorial/pg-skills-base
git.default_branch     tutorial/pg-skills-base
```

任一项不符合就先停下。尤其不要在 `origin` 指向官方仓库或默认分支仍为 `main` 时继续。

build 启动后，Runner 才会从当前默认分支创建 `feat/pg/test-health-handler-contract`，并自动提交 Pipeline 初始化和后续阶段产物。不要提前创建其他名称的功能分支，否则分支守卫会拒绝启动。

## 14. 执行 build

在 DSH Web 页面输入：

```text
/pg-3-build test-health-handler-contract
```

运行过程中，pg-build 会根据 manifest 派送任务，并把事件和 snapshot 写入该 change 的 `2-build/`。不要在中途删除这个目录，也不要手工修改 Pipeline 状态来“制造成功”。

如果 DSH 要求批准文件修改、测试或 Git 操作，请根据刚才审查的范围决定。超出 `health_test.go`、聚焦测试和教程分支 Git 操作的请求，应先询问原因。

## 15. 用证据判断是否成功

不要只看 Web 页面中的“完成”两个字。按下面四组证据检查。

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
tutorial/pg-skills-base
```

并且 `origin/tutorial/pg-skills-base` 包含本次 squash merge。若 build 在合并前失败，工作区可能保留在失败现场，这是诊断证据，不要擅自切分支后宣称成功。

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
外部环境               不需要
聚焦测试               通过且确实执行了 TestHealth*
handlers 包测试         通过
合并目标               你的 Fork 中 tutorial/pg-skills-base
Pipeline 状态           done，或有明确可解释的失败记录
```

如果 AI 实现与文件名略有不同，但满足相同边界和验收条件，可以接受。教程评价的是工程证据，不是文本完全一致。

## 17. 常见失败及处理

### `.dsh/` 没生成

确认当前目录、pg-skills 版本和工具列表：

```powershell
Get-Location
Test-Path .pg\skills\src\integrations\deepseek_harness\adapter.py
python .pg\skills\src\runtime\bin\pg init --list-tools
```

### `start-web.cmd` 不存在

这通常说明 `.dsh/` 来自旧适配器。升级 `.pg/skills/` 后重新运行：

```powershell
python .pg\skills\src\runtime\bin\pg init --tool deepseek-harness
```

如果适配器提示保留了用户修改文件，阅读 warning，并核对 `.dsh/.pg-adapter-manifest.json`。

### DSH 页面里命令无法识别

检查：

```powershell
Test-Path .dsh\bridge\index.ts
Test-Path .dsh\commands\pg-1-define.md
Get-Content .dsh\cordis.patch.yml
```

项目移动后，patch 中指向 bridge 的绝对 URI 可能失效。重新初始化，并从项目根目录使用 `.dsh\start-web.cmd` 启动。

### `pg doctor` 报 schema 错误

不要长期使用 `pg init` 创建的 placeholder。先运行 `pg-init-project` Skill，再检查 `.pg/project.yaml` 的五个必需部分：`modules`、`environments`、`tracks`、`stages` 和 `schema`。

### Go 测试在 define 前就失败

这是项目基线或本机环境问题，不是本次健康测试变更造成的。记录完整命令和错误，先修复 Go 版本、依赖下载或仓库状态，再重新开始练习。

### build 报工作树不干净

运行：

```powershell
git status --short
```

不要盲目 `git add -A`。启动 build 前应位于 `tutorial/pg-skills-base`，并已提交、推送 define/propose 产物。逐项判断剩余内容是本次 change、用户自己的修改还是临时文件；只提交确认应该进入仓库的内容。

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

修复明确问题后，在 DSH Web 页面再次输入：

```text
/pg-3-build test-health-handler-contract
```

Runner 会依据现有事件和 snapshot 判断是否能继续。不要新建同名 change，也不要先删除 `2-build/`。

## 18. 练习结束后的复盘

请不要只问“代码写出来了吗”。用下面的问题复盘：

1. `pg init` 和 `pg-init-project` 分别生成了什么？
2. 自动项目扫描中有哪些内容必须由人确认？
3. define 找到了哪些真实文件和调用关系？
4. propose 为什么只选择 `transports`？
5. execution manifest 实际运行了哪条测试命令？
6. Review 和 Verify 的证据分别在哪里？
7. 为什么本教程使用独立 default branch，而不是 `main`？
8. 如果运行中断，哪些文件保证可以诊断或恢复？

能够根据实际文件回答这些问题，才说明你学会了 pg-skills 的工作方式，而不只是照着输入了三条 Slash Command。

## 下一步

- 把相同方法用于一个小型生产缺陷，但仍先固定基线和验收证据。
- 阅读[在现有项目中使用 pg-skills](https://github.com/pin-gou/pg-skills/blob/main/docs/existing-projects.md)，处理更复杂的 Monorepo 边界。
- 阅读[项目目录与产物](https://github.com/pin-gou/pg-skills/blob/main/docs/project-structure.md)，理解 change、archive 和 `2-build/`。
- 阅读[DeepSeek Harness 完整教程](https://github.com/pin-gou/pg-skills/blob/main/docs/tutorials/deepseek-harness.md)，验证 Commands、Skills、Sub-agent 和三级模型路由。
- 阅读[故障排查](https://github.com/pin-gou/pg-skills/blob/main/docs/troubleshooting.md)，按现象定位初始化、适配、测试和恢复问题。
