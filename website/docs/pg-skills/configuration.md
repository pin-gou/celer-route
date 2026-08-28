# 配置指南

本页说明 pg-skills 的项目级配置由谁生成、哪些字段需要人工确认，以及 Runner 如何消费这些配置。配置的单一事实来源是 `.pg/project.yaml`，模型路由不在该文件中。

## 配置从哪里来

执行 `pg init` 后，`.pg/project.yaml` 可能仍是骨架。第一次接入项目时，在 AI 开发工具中加载 `pg-init-project`：

```text
请加载 pg-init-project skill，扫描当前仓库并初始化 pg-skills 项目配置。
```

它会根据真实构建文件、源码布局和测试入口生成初始配置，同时生成 `.pg/hooks/`、`.pg/code-review/` 和仓库扫描结果。自动扫描不能替代人工确认，尤其是端口、运行环境和服务拓扑。

配置 schema 位于：

```text
.pg/skills/src/runtime/spec/project.schema.json
```

## 五个必需字段

`.pg/project.yaml` 必须包含 `schema`、`modules`、`environments`、`tracks` 和 `stages`。下面是缩减后的结构示例，不应直接复制为真实项目配置：

```yaml
schema: spec-driven

modules:
  backend:
    root: backend
    language: go
    build: go build ./...
    lint: go vet ./...
    test:
      unit: go test ./...

environments:
  local:
    description: 本地开发环境
    roles:
      - name: backend
        instances:
          - name: backend-1
            host: localhost

tracks:
  backend:
    type: standard
    modules: [backend]
    max_fix_retries: 5

stages:
  - name: dev
    tracks: [backend]
    environment:
      required: false
    gate: all_pass
```

### Modules

`modules` 回答三个问题：代码在哪里、使用什么语言、怎样构建和测试。

- `root` 和 `language` 必填。
- `build`、`lint` 可以是命令字符串，也可以使用 `{cmd, timeout_seconds}`。
- `test` 下可以声明 `unit`、`integration`、`e2e` 等测试键。
- module 命令由 Runner 直接执行，不通过 `.pg/hooks/`。

模块应对应真实的独立构建或测试边界，不应简单地把每个源码目录都定义为 module。

### Environments

`environments` 描述运行时拓扑，而不是源码模块：

- `roles` 表示需要运行的服务角色。
- `instances` 表示角色运行在哪个 host 上。
- role 的 `actions` 可以声明 start、stop、health_check 等动作。
- environment 可以声明 `prepare_env`、`describe_env` 和 `clean_env`。
- environment 的 cross-role `actions` 可以承担 health、verify 或 setup。

没有可靠证据时，不要编造 host、端口或服务拓扑。先在 `description` 中标记待确认项，再由项目维护者补全。

### Tracks

`tracks` 定义某类工作怎样执行：

| 类型 | 用途 |
|---|---|
| `standard` | 执行 Test、Dev、Review、Verify、Gate 等标准阶段 |
| `simple` | 不派送 Sub-agent，按顺序执行 `commands` |
| `scenario` | 执行端到端用户旅程或真实场景验证 |

一个 track 可以声明可写 modules、Lint 覆盖、重试次数，以及是否启用 Review、Verify 和 Gate。`on_conditions` 用于让 pg-propose 判断当前变更是否需要该 track。

### Stages

`stages` 决定 track 的执行顺序。Runner 按 stage 顺序执行，同一 stage 中的 track 也按列表顺序执行。

- `environment.required: false` 适合纯编译、单元测试或模拟测试。
- `environment.required: true` 表示该 stage 需要真实运行环境。
- `selection_rules` 帮助 pg-propose 为当前变更选择具体 environment。
- `gate` 支持 `all_pass`、`any_pass` 和 `no_gate`。

## Hooks 配置

`.pg/hooks/` 只服务于 environment 生命周期，不承载 module 的 build、lint 和 test。

```text
.pg/hooks/
├── backend-start.sh
├── backend-stop.sh
├── backend-health-check.sh
├── prepare_env.sh
├── clean_env.sh
└── lib/common.sh
```

对应的 `script` 路径在 `environments` 中声明。修改服务启动方式时，需要同时确认 YAML 引用和脚本内容。不要为 module test 额外建立同名 Hook。

## Code Review 配置

`pg-init-project` 根据 module language 生成 `.pg/code-review/`：

- Java 类项目使用 `java-spring` profile。
- Go 项目使用 `go` profile。
- TypeScript 和 Vue 项目使用 `vue3` profile。
- 其他语言回退到 `default`。
- `security` 是显式启用的可选 profile，不会仅凭语言自动启用。

已有 `.pg/code-review/` 时，初始化 Skill 不应覆盖用户调整过的规则。

## Regression 配置

`regression.suite` 与 tracks、stages 相互独立。每个 suite 必须明确声明：

```yaml
regression:
  suite:
    backend:
      module: backend
      test_keys: [unit]
      environment:
        name: local
        required_roles: []
```

`module` 必须存在于 `modules`，每个 `test_key` 必须存在于对应 module 的 `test`，environment 和 required role 也必须真实存在。

## 其他顶层配置

按项目需要还可以声明：

- `git.default_branch`：验证和合并的目标分支。
- `verify_merge`：stale branch、rebase 和 diff gate 策略。
- `flyway.migration_path`：数据库迁移目录。
- `propose.injections`、`build.injections`：向指定流程注入团队规则。

这些字段不是所有项目都必须配置。不要为了“配置完整”而加入项目并不使用的能力。

## 哪些内容不在 project.yaml

- OpenCode、Mobile Coder 和 DSH 的模型路由。
- 某一次变更的 proposal、design、tasks 和 manifest。
- 某一次运行的事件、日志和 snapshot。
- 开发工具自己的全局配置和 API 密钥。

模型设置见[模型路由指南](model-routing.md)，目录与产物见[项目目录与产物](project-structure.md)。

## 修改后的检查

```powershell
python .pg\skills\src\runtime\bin\pg doctor
git diff -- .pg\project.yaml .pg\hooks .pg\code-review
```

`pg doctor` 通过只代表结构和基础引用有效，不代表端口、外部服务或测试命令一定符合业务环境。首次运行标准变更前，应手动执行关键 build 和 test 命令。

## 相关文档

- 第一次生成配置：[入门指南](getting-started.md)。
- 已有大型仓库接入：[在现有项目中使用 pg-skills](existing-projects.md)。
- 查看每个目录由谁读取：[项目目录与产物](project-structure.md)。

