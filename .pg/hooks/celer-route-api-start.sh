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
BIFROST_BIN="${BIFROST_BIN:-$PROJECT_ROOT/tmp/celer-route-http}"
DATA_DIR="$HOOK_DIR/local/data"
PORT="${BIFROST_START_PORT:-${PG_INSTANCE_PORT:-9080}}"
HOST="${PG_INSTANCE_HOST:-localhost}"

# 确保 UI 嵌入资源存在 (go:embed all:ui 要求目录含可嵌入文件)
ensure_ui_embed() {
    local ui_embed_dir="$PROJECT_ROOT/transports/celer-route-http/ui"
    local ui_embed_parent="$(dirname "$ui_embed_dir")"
    local ui_out_dir="$PROJECT_ROOT/ui/out"

    if [[ -f "$ui_embed_dir/index.html" ]]; then
        # 嵌入目录存在且可用；若 ui/out 产物更新则同步（避免嵌旧版）
        if [[ -f "$ui_out_dir/index.html" && "$ui_out_dir/index.html" -nt "$ui_embed_dir/index.html" ]]; then
            echo "检测到更新的 UI 构建产物 (ui/out)，同步到嵌入目录..."
            rm -rf "$ui_embed_dir"
            mkdir -p "$ui_embed_parent"
            cp -r "$ui_out_dir" "$ui_embed_dir"
        fi
        return 0
    fi

    if [[ -f "$ui_out_dir/index.html" ]]; then
        echo "嵌入目录缺少 UI 资源，使用已有构建产物 ui/out..."
        rm -rf "$ui_embed_dir"
        mkdir -p "$ui_embed_parent"
        cp -r "$ui_out_dir" "$ui_embed_dir"
        return 0
    fi

    echo "未找到 UI 构建产物，执行 make build-ui..."
    (cd "$PROJECT_ROOT" && make build-ui >/dev/null) || return 1
}

ensure_ui_embed || {
    pg_fail --category=build_failure --code=PG-E-0800 \
        --message="UI 嵌入资源准备失败" \
        --hint="Run 'make build-ui' in project root; check node/npm installation" \
        --agent-recoverable=true
}

# 重新构建
echo "重新构建 celer-route-http..."
if [[ ! -f "$PROJECT_ROOT/go.work" ]]; then
    (cd "$PROJECT_ROOT" && make setup-workspace >/dev/null 2>&1) || true
fi
(cd "$PROJECT_ROOT/transports/celer-route-http" && go build -tags dev -ldflags="-w -s" -o "$BIFROST_BIN" .) || {
    pg_fail --category=build_failure --code=PG-E-0800 \
        --message="celer-route-http 构建失败" \
        --hint="Run 'make setup-workspace && make build LOCAL=1' in project root" \
        --agent-recoverable=true
}

if [[ ! -d "$DATA_DIR" ]]; then
    pg_fail --category=prereq_missing --code=PG-E-0900 \
        --message="celer-route 数据目录不存在 ($DATA_DIR)" \
        --hint="先运行 prepare_env 初始化数据" \
        --agent-recoverable=true
fi

# 清理占用端口
if check_port "$PORT"; then
    echo "端口 $PORT 已被占用，清理中..."
    kill_port "$PORT" "celer-route-api"
    sleep 1
fi

# Inject bootstrap setup token (G-2 fix: enable first-admin creation so /workspace/home
# is reachable by Playwright instead of being redirected to /workspace/onboarding).
# When BIFROST_SETUP_TOKEN is unset we fall back to a dev-only token so prepare_env
# data + admin account can still be created; production deployments must override
# this via the environment.
export BIFROST_SETUP_TOKEN="${BIFROST_SETUP_TOKEN:-dev-setup-token-pg-build}"

if ! pid=$(pg_start_bg "$LOG_DIR/celer-route-api.log" "$PID_DIR/celer-route-api.pid" \
        "BIFROST_PORT=$PORT" "BIFROST_UI_DEV=true" "BIFROST_SETUP_TOKEN=$BIFROST_SETUP_TOKEN" "PATH=$PATH" -- \
        "$BIFROST_BIN" -app-dir "$DATA_DIR" -port "$PORT" -host "$HOST" -log-level info -log-style pretty); then
    pg_fail --category=service_start_failure --code=PG-E-0800 \
        --message="启动 celer-route-api 失败" \
        --hint="Check $LOG_DIR/celer-route-api.log" \
        --agent-recoverable=true
fi

if ! wait_for_port_with_monitor "$PORT" "celer-route-api" 120 \
        "$PID_DIR/celer-route-api.pid" "$LOG_DIR/celer-route-api.log"; then
    pg_fail --category=service_start_timeout --code=PG-E-0801 \
        --message="celer-route-api 启动超时 (120s)" \
        --agent-recoverable=true
fi

pg_exit --status=pass --duration=$(( $(date +%s) - $(date +%s) )) \
        --metadata="role=\"${PG_ROLE:-}\" instance=\"${PG_INSTANCE_NAME:-}\" port=\"$PORT\""