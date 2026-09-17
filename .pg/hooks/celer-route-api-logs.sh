#!/usr/bin/env bash
# celer-route-api role logs action: 输出实例最近 N 行业务日志 (默认 100).
# 对应 project.yaml: environments.local.roles.celer-route-api.actions.logs
# 由 pg-run-hook.py 调起 (PG_HOOK_TYPE=logs), 日志文件与 start hook 保持一致:
#   $LOG_DIR/celer-route-api.log (pg_resolve_paths 按 caller × session × env 路由).
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

START_TIME=$(date +%s)
lines="${1:-100}"
logfile="$LOG_DIR/${PG_ROLE:-celer-route-api}.log"

if [ ! -f "$logfile" ]; then
    pg_fail --category=prereq_missing --code=PG-E-0940 \
        --message="日志文件 $logfile 不存在" \
        --hint="服务可能未启动，先执行 start action 启动 celer-route-api" \
        --agent-recoverable=true
fi

echo "=== celer-route-api (${PG_INSTANCE_NAME:-celer-route-api-1}) 最近 ${lines} 行日志: $logfile ==="
tail -n "$lines" "$logfile"

DURATION=$(( $(date +%s) - START_TIME ))
pg_exit --status=pass --duration=$DURATION \
        --metadata="role=\"${PG_ROLE:-}\" instance=\"${PG_INSTANCE_NAME:-}\" lines=\"$lines\" log=\"$logfile\""
