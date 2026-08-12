# Bifrost AI Gateway 仓库扫描报告

Generated: 2026-08-12T14:35:00Z
Scanner: pg-init-project v0.1

## 技术栈

- 主构建工具: Makefile + Go modules + pnpm (UI)
- 语言: go (primary), typescript (UI), shell (scripts)
- 多模块: 是 (Go workspace + pnpm workspace)

## 模块清单

| Module id | 根目录（相对） | 语言 | 构建命令 | 测试命令 | 备注 |
|---|---|---|---|---|---|
| core | core/ | go | go build ./... | make test-core | 核心引擎 |
| framework | framework/ | go | go build ./... | make test-framework | 数据持久化/流式处理 |
| transports | transports/ | go | go build ./... | make test | HTTP 网关 |
| cli | cli/ | go | go build ./... | make test-cli | CLI 工具 |
| plugins | plugins/ | go | go build ./... | make test-plugins | 9 个 Go plugin 子模块 |
| ui | ui/ | typescript | npm run build | npm run test:unit | React + Vite 前端 |
| tests | tests/ | go | - | make run-e2e | E2E 测试 (Playwright) + Go 集成测试 |
| examples | examples/ | go | go build ./... | - | 示例代码 (webhooks, MCP servers) |
| cmd-e2eseed | cmd/e2eseed/ | go | go build ./... | - | E2E seed 工具 |
| scripts-realtime-test | scripts/realtime-test/ | go | go build ./... | - | 实时测试脚本 |

## 构建/测试入口命令

### 主构建 (项目根 Makefile)

```bash
# 构建全部 (UI + bifrost-http 二进制)
make build

# 仅 UI
make build-ui

# 本地开发环境
make dev

# 设置 Go workspace
make setup-workspace
```

### Go 模块测试

```bash
# Core 测试 (所有 provider 集成测试)
make test-core

# 指定 provider
make test-core PROVIDER=openai

# Framework 测试 (需 docker compose 启动依赖)
make test-framework

# Plugin 测试
make test-plugins

# HTTP transport 测试
make test

# MCP 测试
make test-mcp

# CLI 测试
make test-cli

# 全部测试
make test-all
```

### 前端 (ui/)

```bash
cd ui
npm install            # 或 npm ci
npm run dev            # 开发服务器 :3000
npm run build          # 构建
npm run typecheck      # 类型检查
npm run lint           # 代码检查 (oxlint)
npm run format         # 格式化
```

### E2E 测试 (Playwright)

```bash
make run-e2e                      # 全部 E2E
make run-e2e FLOW=providers        # 按 feature
make run-e2e FLOW=virtual-keys
```

## 服务端口（项目当前约定）

| 服务 | 端口 | 来源 |
|------|------|------|
| bifrost-http (API) | 8080 | main.go 默认值 |
| UI dev server | 3000 | vite.config.mts |
| UI proxy → API | 8080 | vite.config.mts proxy 配置 |

## TBD 字段（需人工复核）

- `environments.local.roles.bifrost-api.instances[0].port`: 8080 — 按 main.go 默认值推断，请确认
- `environments.local.roles.ui-dev.instances[0].port`: 3000 — 按 vite.config.mts 推断
- 是否启用 security review profile（opt-in，需手动指定 `tracks.<id>.code_review_profiles: [security, ...]`）