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
PORT="${UI_DEV_PORT:-3008}"
BIFROST_API_PORT="${BIFROST_API_PORT:-9080}"

# 清理占用端口
if check_port "$PORT"; then
    echo "端口 $PORT 已被占用，清理中..."
    kill_port "$PORT" "ui-dev"
    sleep 1
fi

if ! pid=$(pg_run_bash "$LOG_DIR/ui-dev.log" "$PID_DIR/ui-dev.pid" \
        "PORT=$PORT" "BIFROST_PORT=$BIFROST_API_PORT" "BIFROST_DISABLE_PROFILER=1" "PATH=$PATH" -- \
        "cd '$PROJECT_ROOT/ui' && npm run dev -- --port $PORT"); then
    pg_fail --category=service_start_failure --code=PG-E-0800 \
        --message="启动 UI dev server 失败" \
        --hint="Check npm install / node version" \
        --agent-recoverable=true
fi

if ! wait_for_port_with_monitor "$PORT" "ui-dev" 60 \
        "$PID_DIR/ui-dev.pid" "$LOG_DIR/ui-dev.log"; then
    pg_fail --category=service_start_timeout --code=PG-E-0801 \
        --message="UI dev server 启动超时 (60s)" \
        --agent-recoverable=true
fi

pg_exit --status=pass --duration=$(( $(date +%s) - $(date +%s) )) \
        --metadata="role=\"${PG_ROLE:-}\" instance=\"${PG_INSTANCE_NAME:-}\" port=\"$PORT\""