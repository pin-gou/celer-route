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

HOST="${PG_INSTANCE_HOST:-localhost}"
PORT="${PG_INSTANCE_PORT:-9080}"
pg_http_health_check "celer-route-api" "${PG_INSTANCE_NAME:-}" "$HOST" "$PORT" "/health" \
    || pg_fail --category=service_health_check --code=PG-E-0902 \
               --message="celer-route-api health check failed at ${HOST}:${PORT}/health" \
               --hint="Check celer-route-api logs at $LOG_DIR/celer-route-api.log" \
               --agent-recoverable=true

pg_exit --status=pass --duration=$(( $(date +%s) - $(date +%s) )) \
        --metadata="role=\"celer-route-api\" instance=\"${PG_INSTANCE_NAME:-}\" host=\"$HOST\""