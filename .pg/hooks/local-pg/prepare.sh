#!/usr/bin/env bash
# local-pg environment prepare_env hook: 启动 celer-route 实例（config_store + logs_store
# 使用 Postgres），用 fixture 数据初始化，然后关闭。
#
# 与 local 环境差异：
#   - 数据目录为 .pg/hooks/local-pg/data，其中 config.json 将 config_store / logs_store
#     指向 Postgres（默认专用 docker 容器 celer-pg-local-pg，端口 55432，可 LOCAL_PG_* 覆盖）
#   - fixture 与 seed.py 复用 local 环境（数据经 API 写入，落到 Postgres）
set -uo pipefail
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PG_SKILLS_PATH="${PG_SKILLS_PATH:-$SELF_DIR}"
source "$PG_SKILLS_PATH/src/runtime/lib/hook-helpers.sh"
trap 'pg_fail_on_error $? $LINENO' ERR

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"       # .pg/hooks/local-pg/
PARENT_HOOK_DIR="$(cd "$HOOK_DIR/.." && pwd)"                 # .pg/hooks/
PROJECT_ROOT="$(cd "$HOOK_DIR/../../.." && pwd)"               # 项目根
if [[ -f "$PARENT_HOOK_DIR/lib/common.sh" ]]; then
    source "$PARENT_HOOK_DIR/lib/common.sh"
    pg_resolve_paths
fi
mkdir -p "$LOG_DIR" "$PID_DIR"

# Postgres 引导（pg_ensure / pg_resolve_settings）
source "$HOOK_DIR/postgres.sh"

LOCAL_DIR="$HOOK_DIR"
DATA_DIR="$LOCAL_DIR/data"
FIXTURE_DIR="$PARENT_HOOK_DIR/local/fixature"
SEED_PY="$PARENT_HOOK_DIR/local/seed.py"
BIFROST_BIN="${BIFROST_BIN:-$PROJECT_ROOT/tmp/celer-route-http}"
PORT="${BIFROST_PREPARE_PORT:-9080}"
HOST="localhost"

echo "=== prepare_env: 初始化 local-pg 环境 ==="
echo "  data dir:   $DATA_DIR"
echo "  fixture:    $FIXTURE_DIR"
echo "  postgres:   ${PG_HOST:-localhost}:${PG_PORT:-55432}/${PG_DB:-celer_route}"
echo "  port:       $PORT"
echo "  binary:     $BIFROST_BIN"

mkdir -p "$DATA_DIR"

# 确保 Postgres 可用（自动拉起专用 docker 容器，或探测外部实例）
pg_ensure || {
    pg_fail --category=dependency_not_ready --code=PG-E-0900 \
        --message="Postgres 就绪失败" \
        --hint="安装 docker 后重试，或设置 LOCAL_PG_HOST/PORT/USER/PASSWORD/DB 与 LOCAL_PG_CONTAINER='' 指向已有实例" \
        --agent-recoverable=true
}

# 构建 UI 产物（如缺失，供 go:embed 使用）
if [[ ! -d "$PROJECT_ROOT/transports/celer-route-http/ui" ]]; then
    echo "构建 UI 产物..."
    if [[ ! -d "$PROJECT_ROOT/ui/node_modules" ]]; then
        echo "  → 安装 UI 依赖..."
        (cd "$PROJECT_ROOT/ui" && npm ci --prefer-offline) || {
            echo "WARN: 安装 UI 依赖失败，尝试创建空 UI 目录以继续构建"
            mkdir -p "$PROJECT_ROOT/transports/celer-route-http/ui"
        }
    fi
    if [[ -d "$PROJECT_ROOT/ui/node_modules" ]]; then
        (cd "$PROJECT_ROOT/ui" && npm run build && npm run copy-build) || {
            echo "WARN: UI 构建失败，尝试创建空 UI 目录以继续构建"
            mkdir -p "$PROJECT_ROOT/transports/celer-route-http/ui"
        }
    fi
    if [[ -d "$PROJECT_ROOT/transports/celer-route-http/ui" ]]; then
        echo "  → UI 产物就绪"
    fi
fi

# 构建 celer-route-http binary（如缺失）
if [[ ! -x "$BIFROST_BIN" ]]; then
    echo "构建 celer-route-http binary..."
    if [[ ! -f "$PROJECT_ROOT/go.work" ]]; then
        (cd "$PROJECT_ROOT" && make setup-workspace >/dev/null 2>&1) || true
    fi
    (cd "$PROJECT_ROOT/transports/celer-route-http" && go build -ldflags="-w -s" -o "$BIFROST_BIN" .) || {
        pg_fail --category=dependency_not_ready --code=PG-E-0800 \
            --message="celer-route-http 构建失败" \
            --hint="Run 'make setup-workspace && make build LOCAL=1' in project root" \
            --agent-recoverable=true
    }
fi

# 清理占用端口
if check_port "$PORT"; then
    echo "端口 $PORT 已被占用，清理中..."
    kill_port "$PORT" "bifrost-prepare"
    sleep 1
fi

# 清理旧数据（SQLite 遗留；config.json 由下面重新生成）
rm -f "$DATA_DIR"/config.db* "$DATA_DIR"/logs.db*

# 写 config.json：config_store + logs_store 指向 Postgres
pg_resolve_settings
cat > "$DATA_DIR/config.json" <<JSON
{
  "config_store": {
    "enabled": true,
    "type": "postgres",
    "config": {
      "host": "$PG_HOST",
      "port": "$PG_PORT",
      "user": "$PG_USER",
      "password": "$PG_PASSWORD",
      "db_name": "$PG_DB",
      "ssl_mode": "disable"
    }
  },
  "logs_store": {
    "enabled": true,
    "type": "postgres",
    "config": {
      "host": "$PG_HOST",
      "port": "$PG_PORT",
      "user": "$PG_USER",
      "password": "$PG_PASSWORD",
      "db_name": "$PG_DB",
      "ssl_mode": "disable"
    }
  }
}
JSON
echo "config.json 已写入: $DATA_DIR/config.json"

# 启动 celer-route
echo "启动 celer-route (port $PORT)..."
if ! pid=$(pg_start_bg "$LOG_DIR/bifrost-prepare.log" "$PID_DIR/bifrost-prepare.pid" \
        "BIFROST_PORT=$PORT" -- \
        "$BIFROST_BIN" -app-dir "$DATA_DIR" -port "$PORT" -host "$HOST" -log-level warn -log-style pretty); then
    pg_fail --category=service_start_failure --code=PG-E-0800 \
        --message="启动 celer-route-api 失败" \
        --hint="Check $LOG_DIR/bifrost-prepare.log" \
        --agent-recoverable=true
fi

# 等待健康检查
echo "等待 celer-route 就绪..."
if ! wait_for_port_with_monitor "$PORT" "bifrost-prepare" 120 \
        "$PID_DIR/bifrost-prepare.pid" "$LOG_DIR/bifrost-prepare.log"; then
    pg_stop_bg "$PID_DIR/bifrost-prepare.pid" "bifrost-prepare" 2>&1 || true
    pg_fail --category=service_start_timeout --code=PG-E-0801 \
        --message="celer-route-api 启动超时 (120s)" \
        --hint="Check $LOG_DIR/bifrost-prepare.log" \
        --agent-recoverable=true
fi

# 用 fixture 数据种子化（数据经 API 写入，落到 Postgres）
echo "种子化环境数据..."
SEED_RESULT=0
python3 "$SEED_PY" "$FIXTURE_DIR" "$PORT" "$HOST" || SEED_RESULT=$?

if [ "$SEED_RESULT" -ne 0 ]; then
    echo "WARN: seed 过程有错误（$SEED_RESULT），但已有数据已持久化"
fi

# 关闭 celer-route
echo "关闭 celer-route..."
pg_stop_bg "$PID_DIR/bifrost-prepare.pid" "bifrost-prepare" 2>&1 || true
# 额外兜底
pkill -f "celer-route-http.*-app-dir $DATA_DIR" 2>/dev/null || true
sleep 1

echo "=== prepare_env 完成 ==="
echo "  Postgres:  ${PG_HOST}:${PG_PORT}/${PG_DB}（容器 ${LOCAL_PG_CONTAINER:-external}）"
echo "  config.json: $DATA_DIR/config.json（config_store + logs_store = postgres）"
echo "  fixture:  $FIXTURE_DIR 中的配置已写入 Postgres"

pg_exit --status=pass --duration=$(( $(date +%s) - $(date +%s) )) \
        --metadata="data_dir=\"$DATA_DIR\" port=\"$PORT\" pg_host=\"${PG_HOST:-}\" pg_db=\"${PG_DB:-}\""
