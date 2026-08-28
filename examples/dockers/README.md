# celer-route Docker 部署示例（SQLite 单机版）

零外部依赖的单机部署：celer-route 使用内置 SQLite 文件作为 `config_store` 和 `logs_store`。
适合本地试用、单机小规模部署、CI 测试场景。

## 前置条件

- Docker Engine 20.10+
- Docker Compose v2（`docker compose` 子命令）

## 目录结构

```
.
├── docker-compose.yml   # celer-route 服务编排
├── data/
│   └── config.json      # celer-route 配置（SQLite 模式）
└── README.md
```

## 启动

```bash
# 拉取最新镜像并启动（后台运行）
docker compose up -d

# 查看服务状态
docker compose ps

# 实时查看日志
docker compose logs -f celer-route
```

启动成功后访问：

- 管理界面：<http://localhost:8080>
- 健康检查：<http://localhost:8080/health>

首次访问管理界面时按提示完成初始配置（设置 Provider Key、启用 Dashboard 密码保护等）。

## 数据持久化

`./data/` 目录挂载到容器内的 `/app/data/`，包含：

- `config.db` — config_store SQLite 文件
- `logs.db` — logs_store SQLite 文件
- `config.json` — celer-route 配置

**备份**：

```bash
# 停止服务后再备份，避免写入期间复制损坏
docker compose stop
tar czf celer-route-backup-$(date +%Y%m%d).tar.gz data/
docker compose start
```

或者使用 SQLite 在线备份（无需停服）：

```bash
docker compose exec celer-route \
  sqlite3 /app/data/config.db ".backup '/app/data/config.db.bak'"
```

## 升级 celer-route

```bash
docker compose pull
docker compose up -d
```

`./data/` 目录不受升级影响。

## 切换到 PostgreSQL 后端

本示例使用 SQLite，**不适用于生产级高并发或多节点部署**。

如需切换到 PostgreSQL，请使用 [`examples/dockers-postgres/`](../dockers-postgres/) 示例。
注意：celer-route 目前**未提供自动的 SQLite → PostgreSQL 数据迁移工具**。
切换后需要在新 PostgreSQL 库中重新初始化数据（通过管理界面或 API 重新配置）。

## 常见问题

### 端口冲突

如 8080 端口已被占用，编辑 `docker-compose.yml` 的 `ports` 段，例如 `- "9080:8080"`。

### 容器内用户权限

celer-route 容器默认使用非 root 用户运行。如挂载目录存在权限问题：

```bash
# 让当前目录归运行用户所有（UID 1000）
sudo chown -R 1000:1000 data/
```

### 重置数据

```bash
docker compose down
rm -rf data/*.db
docker compose up -d
```

### 健康检查失败

```bash
# 查看容器日志
docker compose logs celer-route

# 进入容器调试
docker compose exec celer-route sh
```

## 配置说明

`data/config.json` 的关键字段：

```json
{
  "config_store": {
    "enabled": true,
    "type": "sqlite",
    "config": { "path": "/app/data/config.db" }
  },
  "logs_store": {
    "enabled": true,
    "type": "sqlite",
    "config": { "path": "/app/data/logs.db" }
  }
}
```

如需调整 `log_retention_days`、连接池、Provider 列表等，参考
[`docs/index.html`](../../docs/index.html) 与 [`transports/config.schema.json`](../../transports/config.schema.json)。