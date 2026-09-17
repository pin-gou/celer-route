#!/usr/bin/env bash
# celer-route-api role tail action: 持续跟随日志 stream（tail -f），Ctrl-C 结束.
# 对应 project.yaml: environments.local.roles.celer-route-api.actions.tail
# 由 pg-run-hook.py 调起 (PG_HOOK_TYPE=tail). wait_for_completion 对 tail 强制
# True 且 project.yaml 不设 timeout_seconds → runner 无限等待 (proc.wait(None)),
# 调用方 Ctrl-C (SIGINT) 时 runner 按键盘中断处理为成功 (ok=true, exit 0).
# 脚本自身 trap INT/TERM 干净退出, 避免 runner 拿到非零退出码.
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

trap 'pg_exit --status=pass --duration=$(( $(date +%s) - START_TIME )) \
        --metadata="role=\"${PG_ROLE:-}\" instance=\"${PG_INSTANCE_NAME:-}\" log=\"$logfile\" interrupted=true"' INT TERM

if [ ! -f "$logfile" ]; then
    pg_fail --category=prereq_missing --code=PG-E-0940 \
        --message="日志文件 $logfile 不存在" \
        --hint="服务可能未启动，先执行 start action 启动 celer-route-api" \
        --agent-recoverable=true
fi

echo "=== celer-route-api (${PG_INSTANCE_NAME:-celer-route-api-1}) 持续跟随日志 (Ctrl-C 结束): $logfile ==="

tail -n "$lines" -f "$logfile" || true
exit 0
