#!/usr/bin/env bash
# local-pg environment Postgres bootstrap helper.
#
# 本文件由 local-pg 的 prepare_env / clean_env / describe_env hook source 使用，
# 提供：
#   pg_resolve_settings  解析/导出 PG_HOST PG_PORT PG_USER PG_PASSWORD PG_DB（含 LOCAL_PG_* 覆盖）
#   pg_ensure            确保 Postgres 可用（默认自动拉起专用 docker 容器；外部实例则探测连通）
#   pg_stop              仅停止容器（保留数据）
#   pg_drop              停止并删除容器（连数据一并清理）
#
# 默认配置：专用容器 celer-pg-local-pg，host 端口 55432（避开 CI tests/docker-compose
# 的 5432 与可能的系统 Postgres），库 celer_route，用户/密码 celer。
#
# 覆盖方式（全部可选，设置任一即跳过对应默认）：
#   LOCAL_PG_HOST / LOCAL_PG_PORT / LOCAL_PG_USER / LOCAL_PG_PASSWORD / LOCAL_PG_DB
#   LOCAL_PG_CONTAINER（设为空字符串则完全不管理容器，仅探测外部实例）
#   LOCAL_PG_IMAGE
set -uo pipefail

# === 配置（可覆盖） ===
# LOCAL_PG_CONTAINER 使用 `-`（非 `:-`）：显式置空表示"不管理容器，仅探测外部实例"
LOCAL_PG_HOST="${LOCAL_PG_HOST:-localhost}"
LOCAL_PG_PORT="${LOCAL_PG_PORT:-55432}"
LOCAL_PG_USER="${LOCAL_PG_USER:-celer}"
LOCAL_PG_PASSWORD="${LOCAL_PG_PASSWORD:-celer}"
LOCAL_PG_DB="${LOCAL_PG_DB:-celer_route}"
LOCAL_PG_CONTAINER="${LOCAL_PG_CONTAINER-celer-pg-local-pg}"
LOCAL_PG_IMAGE="${LOCAL_PG_IMAGE:-postgres:16-alpine}"

# 导出解析后的连接参数，供 prepare.sh 生成 config.json 使用
pg_resolve_settings() {
    PG_HOST="$LOCAL_PG_HOST"
    PG_PORT="$LOCAL_PG_PORT"
    PG_USER="$LOCAL_PG_USER"
    PG_PASSWORD="$LOCAL_PG_PASSWORD"
    PG_DB="$LOCAL_PG_DB"
    export PG_HOST PG_PORT PG_USER PG_PASSWORD PG_DB
}

# 探测 PostgreSQL 是否可达（TCP 端口探测 + 可选 pg_isready）
pg_probe() {
    local host="$1" port="$2"
    if command -v pg_isready >/dev/null 2>&1; then
        pg_isready -h "$host" -p "$port" -U "${LOCAL_PG_USER:-postgres}" >/dev/null 2>&1 && return 0
    fi
    # 兜底 TCP 探测
    if command -v nc >/dev/null 2>&1; then
        nc -z -w 2 "$host" "$port" >/dev/null 2>&1 && return 0
    fi
    timeout 3 bash -c "echo > /dev/tcp/$host/$port" 2>/dev/null && return 0
    return 1
}

pg_docker_ensure() {
    local i=0
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx "$LOCAL_PG_CONTAINER"; then
        if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$LOCAL_PG_CONTAINER"; then
            echo "Postgres 容器 $LOCAL_PG_CONTAINER 已存在但未运行，启动中..."
            docker start "$LOCAL_PG_CONTAINER" >/dev/null || return 1
        fi
    else
        echo "创建 Postgres 容器 $LOCAL_PG_CONTAINER (${LOCAL_PG_IMAGE}, host 端口 ${LOCAL_PG_PORT})..."
        docker run -d --name "$LOCAL_PG_CONTAINER" \
            -p "${LOCAL_PG_PORT}:5432" \
            -e "POSTGRES_USER=${LOCAL_PG_USER}" \
            -e "POSTGRES_PASSWORD=${LOCAL_PG_PASSWORD}" \
            -e "POSTGRES_DB=${LOCAL_PG_DB}" \
            "$LOCAL_PG_IMAGE" >/dev/null || return 1
    fi
    # 等待就绪
    while ! docker exec "$LOCAL_PG_CONTAINER" pg_isready -U "$LOCAL_PG_USER" -d "$LOCAL_PG_DB" >/dev/null 2>&1; do
        i=$((i + 1))
        if [ "$i" -ge 60 ]; then
            echo "ERROR: Postgres 容器 $LOCAL_PG_CONTAINER 未在 60s 内就绪（docker logs 查看原因）" >&2
            return 1
        fi
        sleep 1
    done
    echo "Postgres 容器 $LOCAL_PG_CONTAINER 就绪 (${LOCAL_PG_HOST}:${LOCAL_PG_PORT}/${LOCAL_PG_DB})"
    return 0
}

# pg_ensure: 确保 Postgres 可用。
#  - LOCAL_PG_CONTAINER 为空 → 不管理容器，仅探测外部实例
#  - docker 不可用 → 报错并提示覆盖配置；否则自动拉起容器
pg_ensure() {
    pg_resolve_settings
    if [ -z "${LOCAL_PG_CONTAINER}" ]; then
        # 外部实例模式：仅探测
        if pg_probe "$LOCAL_PG_HOST" "$LOCAL_PG_PORT"; then
            echo "Postgres 外部实例可达 (${LOCAL_PG_HOST}:${LOCAL_PG_PORT}/${LOCAL_PG_DB})"
            return 0
        fi
        echo "ERROR: 外部 Postgres 不可达 ${LOCAL_PG_HOST}:${LOCAL_PG_PORT}（LOCAL_PG_CONTAINER 置空时需自行保证实例运行）" >&2
        return 1
    fi
    if ! command -v docker >/dev/null 2>&1; then
        echo "ERROR: docker 不可用，无法自动拉起 Postgres；请安装 docker，或设置 LOCAL_PG_HOST/PORT/USER/PASSWORD/DB 与 LOCAL_PG_CONTAINER='' 指向已有实例" >&2
        return 1
    fi
    pg_docker_ensure
}

# pg_stop: 仅停止容器（保留数据与容器），外部实例模式为空操作
pg_stop() {
    if [ -z "${LOCAL_PG_CONTAINER}" ]; then
        return 0
    fi
    if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$LOCAL_PG_CONTAINER"; then
        echo "停止 Postgres 容器 $LOCAL_PG_CONTAINER..."
        docker stop "$LOCAL_PG_CONTAINER" >/dev/null || return 1
    fi
    return 0
}

# pg_drop: 停止并删除容器（连数据一并清理）；外部实例模式为空操作
pg_drop() {
    if [ -z "${LOCAL_PG_CONTAINER}" ]; then
        return 0
    fi
    if command -v docker >/dev/null 2>&1 && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx "$LOCAL_PG_CONTAINER"; then
        echo "删除 Postgres 容器 $LOCAL_PG_CONTAINER..."
        docker rm -f -v "$LOCAL_PG_CONTAINER" >/dev/null || return 1
    fi
    return 0
}
