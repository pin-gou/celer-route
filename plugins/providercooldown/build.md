# providercooldown 构建与部署

## 目录结构

```
plugins/providercooldown/
├── cooldown.go          # 核心逻辑: CooldownState, KeyPoolFilter, LLMPlugin
├── config.go            # Config 解析 (default_ttl_seconds, ttl_overrides)
├── cooldown_test.go     # 核心测试 (19 用例)
├── config_test.go       # Config 解析测试 (10 用例)
├── Dockerfile           # 多阶段构建 Dockerfile
├── Makefile             # image / buildx / run / push
├── README.md            # 完整文档
├── go.mod / go.sum      # Go 模块依赖
├── version              # 版本号
└── build.md             # 本文件
```

## 构建镜像

### 前置条件

- Docker ≥ 24
- 构建上下文为**仓库根目录**（`/home/Admin/workspaces/bifrost`）

### 国内镜像源加速

国内网络环境下，建议使用以下镜像源加速构建：

| 参数 | 国内推荐值 | 说明 |
|---|---|---|
| `APK_REPOS` | `https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.23/main,https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.23/community` | Alpine apk 镜像 |
| `NPM_REGISTRY` | `https://registry.npmmirror.com` | npm 镜像 |
| `GOPROXY` | `https://goproxy.cn,direct` | Go 模块镜像 |

### 本地构建（amd64 单架构）

```bash
# 从仓库根执行
make -C plugins/providercooldown image \
  IMAGE=myco/bifrost \
  TAG=v1.0.0-cooldown
```

等价于直接 docker build：

```bash
docker build -f plugins/providercooldown/Dockerfile \
  --build-arg VERSION=v1.0.0-cooldown \
  --build-arg APK_REPOS=https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.23/main,https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.23/community \
  --build-arg NPM_REGISTRY=https://registry.npmmirror.com \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  -t myco/bifrost:v1.0.0-cooldown \
  -t myco/bifrost:latest \
  .
```

### 多架构构建（arm64 + amd64，推送到 registry）

```bash
make -C plugins/providercooldown buildx-image \
  IMAGE=myco/bifrost TAG=v1.0.0-cooldown \
  PLATFORMS=linux/arm64,linux/amd64
```

> 需要 Docker buildx 和 QEMU binfmt 支持。  
> 多架构构建直接 `--push`，不会在本地 docker 中保留镜像。

### Dockerfile 构建阶段说明

```
Stage 1: ui-builder (node:25-alpine3.23)
  ├── apk upgrade / npm ci / npm run build-enterprise
  └── 输出: /app/out (UI 静态文件)

Stage 2: builder (golang:1.26.5-alpine3.23)
  ├── apk add gcc musl-dev sqlite-dev binutils binutils-gold
  ├── go work init → use core/framework/plugins/transports
  ├── go build -tags sqlite_static -o /out/bifrost-http
  └── 输出: 静态链接 bifrost-http 二进制 (~197MB)

Stage 3: runtime (alpine:3.23.4)
  ├── apk add musl libgcc ca-certificates zlib
  ├── COPY binary + docker-entrypoint.sh
  └── 最终镜像: ~138MB
```

d providercooldown 是**普通 Go 包**（不是 .so plugin），编译时通过 `transports/go.mod` 的 `replace` 指令直接链接进 bifrost-http 二进制，无需动态加载。

## 运行

### 准备数据目录

```bash
mkdir -p ./data
chmod 777 ./data
```

> `chmod 777` 是因为容器内以 `nobody`（UID 65534）用户运行，需要挂载目录可写。  
> 生产环境可以用 `--user 0` 或配置 PodSecurityContext `fsGroup: 0` 替代。

### 准备配置文件

创建 `./data/config.json`，必须包含 `plugins[]` 块启用 providercooldown：

```json
{
  "$schema": "https://www.getbifrost.ai/schema",
  "providers": {
    "openai": {
      "keys": [
        { "name": "k1", "value": "env.OPENAI_API_KEY", "weight": 1 },
        { "name": "k2", "value": "env.OPENAI_API_KEY_2", "weight": 1 }
      ]
    }
  },
  "plugins": [
    {
      "enabled": true,
      "name": "provider-cooldown",
      "config": {
        "default_ttl_seconds": 600,
        "ttl_overrides": { "openai": 30 }
      }
    }
  ],
  "client": {
    "drop_excess_requests": false,
    "initial_pool_size": 50
  }
}
```

### 启动容器

```bash
# 使用 Makefile
make -C plugins/providercooldown run IMAGE=myco/bifrost TAG=v1.0.0-cooldown

# 或直接 docker
docker run -d --name bifrost \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e OPENAI_API_KEY=sk-... \
  myco/bifrost:latest

# 查看启动日志
docker logs -f bifrost

# 等待健康检查通过（通常需 10-15 秒）
curl http://localhost:8080/health
```

### 推送到 Registry

```bash
make -C plugins/providercooldown push IMAGE=myco/bifrost TAG=v1.0.0-cooldown
```

## 验证

### 检查插件状态

```bash
curl http://localhost:8080/api/plugins | python3 -m json.tool
```

预期输出片段：

```json
{
    "name": "provider-cooldown",
    "enabled": true,
    "status": "active",
    "types": ["llm"],
    "config": {
        "default_ttl_seconds": 600,
        "ttl_overrides": { "openai": 30 }
    }
}
```

- `status: "active"` — 插件初始化成功
- `types: ["llm"]` — 已被识别为 LLMPlugin
- `config` — 配置已正确解析

### 验证配额冷却行为

1. 用一把配额已耗尽的 API key 发送请求
2. 观察日志中 `provider-cooldown` 标记了 `(provider, key_id)` 冷却
3. 立即再发同请求 —— Bifrost 应跳过该 key（LB 选另一把 key，或走 fallback 链）
4. 等待 TTL 超时后该 key 恢复可用

## 常见问题

### 容器启动失败：`permission denied`

```
failed to initialize default config store: open /app/data/config.db: permission denied
```

**原因**：宿主机挂载的 `/app/data` 目录对容器内 `nobody` 用户不可写。  
**解决**：

```bash
chmod 777 ./data   # 或
docker run --user 0 ...   # 以 root 运行
```

### 镜像构建失败：`adduser: /etc/passwd: Operation not permitted`

**原因**：Docker 守护进程的底层文件系统（如特定 NFS 或 overlay 配置）不支持 `adduser`。  
**解决**：镜像已改用 `nobody` 用户 + `chmod 777`，无需 `adduser`。如果遇到此错误，确保使用最新版本 Dockerfile。

### apk 仓库超时

```
WARNING: fetching https://dl-cdn.alpinelinux.org/alpine/v3.23/main/APKINDEX.tar.gz: Operation timed out
```

**原因**：国内网络无法访问 `dl-cdn.alpinelinux.org`。  
**解决**：构建时传入 `--build-arg APK_REPOS=...` 使用国内镜像源（见上文"国内镜像源加速"）。

### Go 模块下载超时

```
go: module github.com/xxx: Get "https://proxy.golang.org/...": dial tcp: i/o timeout
```

**解决**：构建时传入 `--build-arg GOPROXY=https://goproxy.cn,direct`。

## 架构说明

### plugin 集成方式

providercooldown **不是** Go 动态插件（`.so`），而是**普通 Go 包**，通过以下方式集成：

1. `transports/go.mod` 添加 `require` + `replace` 指向 `../plugins/providercooldown`
2. `transports/bifrost-http/server/plugins.go` 在 `loadBuiltinPlugins` 中注册为内置插件
3. `transports/bifrost-http/server/server.go` 新增 `KeyPoolFilter` 字段，在 `bifrost.Init` 时注入
4. `transports/bifrost-http/lib/config.go` 加入 `builtinPluginNames`

这样 providercooldown 在编译时直接链接进 `bifrost-http` 二进制，**无需动态加载**，没有 Go plugin ABI 兼容风险。

### 数据流

```
请求 → bifrost core
  → KeyPoolFilter (CooldownState.IsCoolingDown): 跳过冷却中的 key
  → 选定 key → provider API 调用
  → 失败 (429 + quota 消息 / 402)
  → PostLLMHook (CooldownPlugin.PostLLMHook): 标记 (provider, key_id) 冷却
  → 回退到下一把 key 或 fallback 链
```

### 配置项

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `default_ttl_seconds` | int | 600 | 未设置 override 的 provider 默认冷却时长 |
| `ttl_overrides` | object | `{}` | 按 provider 覆盖 TTL，key 为 provider 名，value 为秒数 |