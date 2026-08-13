#!/usr/bin/env bash
set -uo pipefail
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PG_SKILLS_PATH="${PG_SKILLS_PATH:-$SELF_DIR}"
source "$PG_SKILLS_PATH/src/runtime/lib/hook-helpers.sh"
trap 'pg_fail_on_error $? $LINENO' ERR

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$HOOK_DIR/lib/common.sh" ]]; then
    source "$HOOK_DIR/lib/common.sh"
    pg_resolve_paths
fi
mkdir -p "$LOG_DIR" "$PID_DIR"

PROJECT_ROOT="$(cd "$HOOK_DIR/../.." && pwd)"
BIFROST_BIN="${BIFROST_BIN:-$PROJECT_ROOT/tmp/bifrost-http}"
DATA_DIR="$HOOK_DIR/local/data"
PORT="${BIFROST_START_PORT:-${PG_INSTANCE_PORT:-9080}}"
HOST="${PG_INSTANCE_HOST:-localhost}"

# 重新构建
echo "重新构建 bifrost-http..."
if [[ ! -f "$PROJECT_ROOT/go.work" ]]; then
    (cd "$PROJECT_ROOT" && make setup-workspace >/dev/null 2>&1) || true
fi
(cd "$PROJECT_ROOT/transports/bifrost-http" && go build -ldflags="-w -s" -o "$BIFROST_BIN" .) || {
    pg_fail --category=build_failure --code=PG-E-0800 \
        --message="bifrost-http 构建失败" \
        --hint="Run 'make setup-workspace && make build LOCAL=1' in project root" \
        --agent-recoverable=true
}

if [[ ! -d "$DATA_DIR" ]]; then
    pg_fail --category=prereq_missing --code=PG-E-0900 \
        --message="bifrost 数据目录不存在 ($DATA_DIR)" \
        --hint="先运行 prepare_env 初始化数据" \
        --agent-recoverable=true
fi

# 清理占用端口
if check_port "$PORT"; then
    echo "端口 $PORT 已被占用，清理中..."
    kill_port "$PORT" "bifrost-api"
    sleep 1
fi

if ! pid=$(pg_start_bg "$LOG_DIR/bifrost-api.log" "$PID_DIR/bifrost-api.pid" \
        "BIFROST_PORT=$PORT" "PATH=$PATH" -- \
        "$BIFROST_BIN" -app-dir "$DATA_DIR" -port "$PORT" -host "$HOST" -log-level info -log-style pretty); then
    pg_fail --category=service_start_failure --code=PG-E-0800 \
        --message="启动 bifrost-api 失败" \
        --hint="Check $LOG_DIR/bifrost-api.log" \
        --agent-recoverable=true
fi

if ! wait_for_port_with_monitor "$PORT" "bifrost-api" 120 \
        "$PID_DIR/bifrost-api.pid" "$LOG_DIR/bifrost-api.log"; then
    pg_fail --category=service_start_timeout --code=PG-E-0801 \
        --message="bifrost-api 启动超时 (120s)" \
        --agent-recoverable=true
fi

pg_exit --status=pass --duration=$(( $(date +%s) - $(date +%s) )) \
        --metadata="role=\"${PG_ROLE:-}\" instance=\"${PG_INSTANCE_NAME:-}\" port=\"$PORT\""