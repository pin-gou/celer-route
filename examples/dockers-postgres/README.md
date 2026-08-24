# pg-gateway + PostgreSQL 一体化部署示例

面向生产级 / 多节点场景：pg-gateway 与 PostgreSQL 16+ 通过 docker-compose 一同编排。
pg-gateway 使用 PostgreSQL 作为 `config_store` 和 `logs_store`，获得跨节点一致性、
连接池调优与 pg-gateway 专属的物化视图 / GIN 索引优化。

## 前置条件

- Docker Engine 20.10+
- Docker Compose v2（`docker compose` 子命令）
- 至少 2 GB 可用内存（PostgreSQL + pg-gateway）
- PostgreSQL **16 或更高版本**（pg-gateway logstore 硬性要求 ≥ 16，低于此版本将拒绝启动）

## 目录结构

```
.
├── docker-compose.yml   # pg-gateway + postgres:16-alpine 编排
├── data/
│   └── config.json      # pg-gateway 配置（postgres 模式）
├── example.env          # 环境变量模板（数据库密码等）
└── README.md
```

## 启动

```bash
# 1. 复制环境变量模板并按需修改
cp example.env .env

# 2. 启动两个服务（后台运行）
docker compose up -d

# 3. 查看启动状态（注意等 Postgres healthy 后 pg-gateway 才会启动）
docker compose ps
```

预期输出（`Status` 列）：

```
NAME            SERVICE       STATUS
pg-gateway-pg   postgres      Up (healthy)
pg-gateway      pg-gateway    Up (healthy)
```

健康检查 URL：<http://localhost:8080/health>

## 网络拓扑

两个容器位于默认 bridge 网络 `pg-gateway-postgres_default` 中。
pg-gateway 通过 **服务名** `postgres`（而非 `localhost`）访问数据库 —— Docker 在同一网络内
提供 DNS 解析，把 `postgres` 解析到 postgres 容器的内部 IP。

> **为什么不需要固定 IP**：`tests/docker-compose.yml` 中的 `172.28.0.16` 固定 IP 是 GitHub Actions
> harden-runner + 容器内嵌 DNS（127.0.0.11:53）兼容性的特殊处理，与用户场景无关。
> 在用户自己的 docker compose 中，靠服务名 + 健康检查即可，无需固定 IP。

## 数据持久化

| 容器 | 容器内路径 | 卷名 | 用途 |
|---|---|---|---|
| postgres | `/var/lib/postgresql/data` | `pg-data` | 数据库文件 |
| pg-gateway | `/app/data` | `./data:/app/data` | 仅持久化 `config.json` |

**手动备份**：

```bash
docker compose exec -T postgres \
  pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  | gzip > pg-gateway-$(date +%Y%m%d).sql.gz
```

**恢复**：

```bash
gunzip -c pg-gateway-YYYYMMDD.sql.gz | \
  docker compose exec -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"
```

## 升级

```bash
docker compose pull
docker compose up -d
```

数据库 schema 变更由 pg-gateway 进程启动时自动迁移，无需手工执行。
跨节点升级时，多副本之间通过 PostgreSQL advisory lock 串行化迁移，保证一致性。

## 从 SQLite 版迁移过来

pg-gateway **目前未提供自动的 SQLite → PostgreSQL 数据迁移工具**。

如需把现有 SQLite 数据搬到 PostgreSQL，请按以下顺序操作：

1. **导出旧数据**：通过 pg-gateway 管理界面 / API 把所有 Virtual Key、Provider Key、
   Model Config、Routing Rule 等配置记录下来（建议截图或导出 CSV）。
2. **启动新环境**：`docker compose up -d` 后 pg-gateway 会在空 PostgreSQL 库中自动建表。
3. **重新配置**：在新环境中通过管理界面或 API 重新填入配置。
4. **日志数据可选**：历史 `logs.db` 中的请求日志不会自动迁移。如需保留，
   可直接连接旧 SQLite 文件用 SQL 工具导出分析（无业务依赖）。

## 配置说明

### `data/config.json` 关键字段

```json
{
  "config_store": {
    "type": "postgres",
    "config": {
      "host": "postgres",          // docker 服务名，不是 localhost
      "port": "5432",
      "user": "${POSTGRES_USER}",  // ⚠ 见下方"敏感信息"小节
      "password": "${POSTGRES_PASSWORD}",
      "db_name": "pg_gateway",
      "ssl_mode": "disable",       // 仅容器内同网络；跨主机需 require/verify-full
      "max_idle_conns": 10,
      "max_open_conns": 100,
      "conn_max_lifetime": "1h",
      "conn_max_idle_time": "10m"
    }
  }
}
```

### 连接池调优建议

| 场景 | `max_open_conns` | `max_idle_conns` | `conn_max_lifetime` |
|---|---|---|---|
| 单节点 / 低并发 | 50 | 5 | 1h |
| 多节点 / 高并发 | 100–200 | 20 | 30m |
| Serverless / 短连接频繁 | 20 | 2 | 10m |

PostgreSQL 服务端需相应调高 `max_connections`（默认 100）。多副本部署时
`max_connections ≥ 副本数 × max_open_conns`。

### 敏感信息

**⚠️**：`config.json` 中的 `${POSTGRES_PASSWORD}` 占位符 **不会被 pg-gateway 解析**。
pg-gateway 启动时按字面量读取此字符串。本仓库示例中使用明文占位仅为了演示 schema
完整字段，**生产部署禁止将真实密码写入 `config.json`**。

**生产推荐做法**：

- **方式 A（推荐）**：在挂载前用 `envsubst` 渲染模板：
  ```bash
  envsubst < data/config.json.tpl > data/config.json
  docker compose up -d
  ```
- **方式 B**：使用 `password_command` 字段，让 pg-gateway 启动时执行命令获取动态凭证
  （如 AWS RDS IAM token、Vault token）。详见 `transports/config.schema.json` 中
  `password_command` 字段说明。
- **方式 C**：将 `config.json` 整体托管到 Vault / k8s Secret，通过 init container 注入。

## 常见问题

### pg-gateway 启动后立刻退出

查看日志：

```bash
docker compose logs pg-gateway
```

最常见原因：
- PostgreSQL 还没就绪 → 等 `postgres` 服务显示 `healthy` 后再访问
- `server_version_num` 低于 160000 → 升级到 PostgreSQL 16+
- `config.json` 中 `host` 写成了 `localhost` → 应改成 `postgres`（容器外访问容器内服务）

### Postgres 连接数耗尽

```bash
docker compose exec postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "SELECT count(*) FROM pg_stat_activity;"
```

如接近 `max_connections` 上限，调高 PostgreSQL `max_connections` 或降低
`max_open_conns`。

### 数据目录迁移

如需把数据搬到另一台机器：

```bash
# 在新机器上启动空 stack
docker compose up -d postgres

# 在旧机器导出
docker compose exec -T postgres \
  pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  | gzip > dump.sql.gz

# 在新机器导入
gunzip -c dump.sql.gz | \
  docker compose exec -T postgres \
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"

# 启动 pg-gateway
docker compose up -d pg-gateway
```

### 横向扩容（多 pg-gateway 实例）

PostgreSQL 后端天然支持多副本。复制 `pg-gateway` 服务定义、改 `container_name`
即可共享同一 Postgres：

```yaml
services:
  pg-gateway-1:
    image: ghcr.io/pin-gou/pg-gateway:latest
    ports: ["8080:8080"]
    ...

  pg-gateway-2:
    image: ghcr.io/pin-gou/pg-gateway:latest
    ports: ["8081:8080"]
    ...
```

两个实例都连同一个 `postgres` 服务。PostgreSQL advisory lock 负责跨实例的
schema 迁移与物化视图刷新序列化。