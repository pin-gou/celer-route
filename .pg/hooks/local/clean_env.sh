#!/usr/bin/env bash
# .pg/hooks/local/clean_env.sh — local 环境 clean_env hook（权威实现）
#
# 与 prepare.sh 配对：prepare 负责初始化（构建/种子化），本 hook 负责收回资源。
# 由 pg-run-hook.py 在 stage 结束时调起（PG_HOOK_TYPE=clean），也可被根目录
# clean.sh（交互式入口）委托调用。输出必须简洁，非交互。
#
# 清理内容：
#   1. 停止本地服务（celer-route-http / vite / air），释放 9080/3008/9082 端口
#   2. 清除 .pg/ 下运行时会话目录（logs / pids）
#   3. 清除 .pg/changes/ 构建产物（2-build、日志），保留提案文档与 archive
#   4. 清除 local 环境数据（.pg/hooks/local/data/）
#   5. 清除构建与 UI 产物（tmp/、ui/out/、嵌入 UI、测试报告）
#
# 参数：
#   --keep-data  保留 .pg/hooks/local/data/（种子数据库）
#   --deep       深度清理：额外移除 ui/node_modules、go.work/go.work.sum、/tmp 测试残留
#
# 幂等：重复执行不报错；进程/文件不存在时走 || true。

set -uo pipefail
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../skills" && pwd)"
export PG_SKILLS_PATH="${PG_SKILLS_PATH:-$SELF_DIR}"
source "$PG_SKILLS_PATH/src/runtime/lib/hook-helpers.sh"
trap 'pg_fail_on_error $? $LINENO' ERR

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$HOOK_DIR/../lib/common.sh" ]]; then
    source "$HOOK_DIR/../lib/common.sh"
    pg_resolve_paths
fi

PROJECT_ROOT="$(cd "$HOOK_DIR/../../.." && pwd)"
START_TIME=$(date +%s)

DATA_DIR="$HOOK_DIR/data"

KEEP_DATA=0
for arg in "$@"; do
    case "$arg" in
        --keep-data) KEEP_DATA=1 ;;
        --deep) DEEP=1 ;;
        *) echo "WARN: 忽略未知参数: $arg" >&2 ;;
    esac
done

# === 工具函数 ===

# 释放端口（lsof / fuser / ss 依次探测）
kill_port() {
    local port="$1" name="$2" pids=""
    if command -v lsof >/dev/null 2>&1; then
        pids="$(lsof -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
    elif command -v fuser >/dev/null 2>&1; then
        pids="$(fuser "$port/tcp" 2>/dev/null | tr -s ' ' '\n' | grep -E '^[0-9]+$' || true)"
    elif command -v ss >/dev/null 2>&1; then
        pids="$(ss -tulnp 2>/dev/null | grep ":$port " | grep -oP 'pid=\K[0-9]+' | sort -u || true)"
    fi
    for pid in $pids; do
        echo "释放 $name 端口 $port（PID $pid）"
        kill -9 "$pid" 2>/dev/null || true
    done
    sleep 1
}

# === 1. 停止本地服务 ===
stop_services() {
    pkill -f 'celer-route-http' 2>/dev/null || true
    pkill -f 'air -c' 2>/dev/null || true
    pkill -f 'vite --port' 2>/dev/null || true

    kill_port 9080 "celer-route-api"
    kill_port 3008 "ui-dev"
    kill_port 9082 "agent"

    find "$PROJECT_ROOT/.pg" -name '*.pid' -delete 2>/dev/null || true
}

# === 2. 清除 .pg 运行时会话目录 ===
clean_pg_runtime_dirs() {
    rm -rf "$PROJECT_ROOT/.pg/agent" "$PROJECT_ROOT/.pg/ad-hoc" \
           "$PROJECT_ROOT/.pg/regression" "$PROJECT_ROOT/.pg/fix-issue" \
           "$PROJECT_ROOT/.pg/quick-build"
}

# === 3. 清除 .pg/changes 构建产物（保留提案文档与 archive） ===
clean_pg_changes() {
    find "$PROJECT_ROOT/.pg/changes" -maxdepth 1 -mindepth 1 -type d -not -name archive 2>/dev/null -exec sh -c '
        for d; do find "$d" -type d -name "2-build" -prune -exec rm -rf {} +; done
    ' sh {} + || true
    find "$PROJECT_ROOT/.pg/changes" -path "$PROJECT_ROOT/.pg/changes/archive" -prune -o \
        -type f \( -name '*.log' -o -name '*.pid' \) -print 2>/dev/null | while IFS= read -r f; do
        rm -f "$f"
    done
}

# === 4. 清除 local 环境数据（保留 logs.db） ===
clean_local_data() {
    if [[ "$KEEP_DATA" -eq 1 ]]; then
        # 仍清掉 SQLite 的 WAL/SHM（避免文件锁残留），保留 DB 本体
        rm -f "$DATA_DIR"/*.db-wal "$DATA_DIR"/*.db-shm 2>/dev/null || true
        return
    fi
    rm -f "$DATA_DIR"/config.db* "$DATA_DIR"/config.json "$DATA_DIR"/fixature_plugins.json
}

# === 5. 清除构建与 UI 产物 ===
clean_build_artifacts() {
    rm -rf "$PROJECT_ROOT/tmp"
    rm -rf "$PROJECT_ROOT/ui/out"
    rm -rf "$PROJECT_ROOT/transports/celer-route-http/ui"
    rm -rf "$PROJECT_ROOT/transports/celer-route-http/lib/ui"
    rm -rf "$PROJECT_ROOT/transports/celer-route-http/tmp"
    rm -rf "$PROJECT_ROOT/transports/celer-route-http/logs"
    rm -f "$PROJECT_ROOT/transports/celer-route-http/build-errors.log"
    rm -rf "$PROJECT_ROOT/test-reports"
    rm -rf "$PROJECT_ROOT/tests/e2e/api/newman-reports"
}

# === 深度清理（--deep） ===
deep_clean() {
    rm -rf "$PROJECT_ROOT/ui/node_modules"
    rm -f "$PROJECT_ROOT/go.work" "$PROJECT_ROOT/go.work.sum"
    if [[ -d /tmp ]]; then
        rm -f /tmp/bifrost-*.so 2>/dev/null || true
        rm -rf /tmp/bifrost-test-* 2>/dev/null || true
    fi
}

# === 执行 ===
stop_services
clean_pg_runtime_dirs
clean_pg_changes
clean_local_data
clean_build_artifacts
if [[ "${DEEP:-0}" -eq 1 ]]; then deep_clean; fi

DURATION=$(( $(date +%s) - START_TIME ))
pg_exit --status=pass --duration=$DURATION \
        --metadata="env=\"${PG_ENV:-local}\" keep_data=\"$KEEP_DATA\" deep=\"${DEEP:-0}\""