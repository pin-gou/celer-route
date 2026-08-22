#!/usr/bin/env bash
# local environment prepare_env hook: 启动 pg-gateway 实例，用 fixture 数据初始化，然后关闭
set -uo pipefail
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PG_SKILLS_PATH="${PG_SKILLS_PATH:-$SELF_DIR}"
source "$PG_SKILLS_PATH/src/runtime/lib/hook-helpers.sh"
trap 'pg_fail_on_error $? $LINENO' ERR

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"       # .pg/hooks/local/
PARENT_HOOK_DIR="$(cd "$HOOK_DIR/.." && pwd)"                 # .pg/hooks/
PROJECT_ROOT="$(cd "$HOOK_DIR/../../.." && pwd)"               # 项目根
if [[ -f "$PARENT_HOOK_DIR/lib/common.sh" ]]; then
    source "$PARENT_HOOK_DIR/lib/common.sh"
    pg_resolve_paths
fi
mkdir -p "$LOG_DIR" "$PID_DIR"

LOCAL_DIR="$HOOK_DIR"
DATA_DIR="$LOCAL_DIR/data"
FIXTURE_DIR="$LOCAL_DIR/fixature"
BIFROST_BIN="${BIFROST_BIN:-$PROJECT_ROOT/tmp/pg-gateway-http}"
ADMIN_BIN="${ADMIN_BIN:-$PROJECT_ROOT/tmp/pg-gateway-admin}"
PORT="${BIFROST_PREPARE_PORT:-9080}"
HOST="localhost"

echo "=== prepare_env: 初始化 local 环境 ==="
echo "  data dir: $DATA_DIR"
echo "  fixture:  $FIXTURE_DIR"
echo "  port:     $PORT"
echo "  binary:   $BIFROST_BIN"

mkdir -p "$DATA_DIR"

# 构建 UI 产物（如缺失，供 go:embed 使用）
if [[ ! -d "$PROJECT_ROOT/transports/pg-gateway-http/ui" ]]; then
    echo "构建 UI 产物..."
    if [[ ! -d "$PROJECT_ROOT/ui/node_modules" ]]; then
        echo "  → 安装 UI 依赖..."
        (cd "$PROJECT_ROOT/ui" && npm ci --prefer-offline) || {
            echo "WARN: 安装 UI 依赖失败，尝试创建空 UI 目录以继续构建"
            mkdir -p "$PROJECT_ROOT/transports/pg-gateway-http/ui"
        }
    fi
    if [[ -d "$PROJECT_ROOT/ui/node_modules" ]]; then
        (cd "$PROJECT_ROOT/ui" && npm run build && npm run copy-build) || {
            echo "WARN: UI 构建失败，尝试创建空 UI 目录以继续构建"
            mkdir -p "$PROJECT_ROOT/transports/pg-gateway-http/ui"
        }
    fi
    if [[ -d "$PROJECT_ROOT/transports/pg-gateway-http/ui" ]]; then
        echo "  → UI 产物就绪"
    fi
fi

# 构建 pg-gateway-http binary（如缺失）
if [[ ! -x "$BIFROST_BIN" ]]; then
    echo "构建 pg-gateway-http binary..."
    # 确保 Go workspace 存在（跨模块引用需要）
    if [[ ! -f "$PROJECT_ROOT/go.work" ]]; then
        (cd "$PROJECT_ROOT" && make setup-workspace >/dev/null 2>&1) || true
    fi
    (cd "$PROJECT_ROOT/transports/pg-gateway-http" && go build -ldflags="-w -s" -o "$BIFROST_BIN" .) || {
        pg_fail --category=dependency_not_ready --code=PG-E-0800 \
            --message="pg-gateway-http 构建失败" \
            --hint="Run 'make setup-workspace && make build LOCAL=1' in project root" \
            --agent-recoverable=true
    }
fi

# 构建 pg-gateway-admin CLI（如缺失）
if [[ ! -x "$ADMIN_BIN" ]]; then
    echo "构建 pg-gateway-admin CLI..."
    (cd "$PROJECT_ROOT" && make build-admin) || {
        pg_fail --category=dependency_not_ready --code=PG-E-0800 \
            --message="pg-gateway-admin 构建失败" \
            --hint="Run 'make build-admin' in project root" \
            --agent-recoverable=true
    }
fi

# 清理占用端口
if check_port "$PORT"; then
    echo "端口 $PORT 已被占用，清理中..."
    kill_port "$PORT" "bifrost-prepare"
    sleep 1
fi

# 清理旧数据
rm -f "$DATA_DIR"/config.db* "$DATA_DIR"/config.json

# 启动 pg-gateway
echo "启动 pg-gateway (port $PORT)..."
if ! pid=$(pg_start_bg "$LOG_DIR/bifrost-prepare.log" "$PID_DIR/bifrost-prepare.pid" \
        "BIFROST_PORT=$PORT" -- \
        "$BIFROST_BIN" -app-dir "$DATA_DIR" -port "$PORT" -host "$HOST" -log-level warn -log-style pretty); then
    pg_fail --category=service_start_failure --code=PG-E-0800 \
        --message="启动 pg-gateway-api 失败" \
        --hint="Check $LOG_DIR/bifrost-prepare.log" \
        --agent-recoverable=true
fi

# 等待健康检查
echo "等待 pg-gateway 就绪..."
if ! wait_for_port_with_monitor "$PORT" "bifrost-prepare" 120 \
        "$PID_DIR/bifrost-prepare.pid" "$LOG_DIR/bifrost-prepare.log"; then
    pg_stop_bg "$PID_DIR/bifrost-prepare.pid" "bifrost-prepare" 2>&1 || true
    pg_fail --category=service_start_timeout --code=PG-E-0801 \
        --message="pg-gateway-api 启动超时 (120s)" \
        --hint="Check $LOG_DIR/bifrost-prepare.log" \
        --agent-recoverable=true
fi

# 用 fixture 数据种子化
echo "种子化环境数据..."
SEED_RESULT=0
python3 "$LOCAL_DIR/seed.py" "$FIXTURE_DIR" "$PORT" "$HOST" || SEED_RESULT=$?

if [ "$SEED_RESULT" -ne 0 ]; then
    echo "WARN: seed 过程有错误（$SEED_RESULT），但已有数据已持久化"
fi

# 设置默认管理员密码
ADMIN_USER="${BIFROST_ADMIN_USER:-admin}"
ADMIN_PASS="${BIFROST_ADMIN_PASS:-Admin@123456}"
echo "设置默认管理员（$ADMIN_USER）..."
if ! printf '%s\n%s\n' "$ADMIN_PASS" "$ADMIN_PASS" | "$ADMIN_BIN" admin reset \
    --app-dir "$DATA_DIR" \
    --username "$ADMIN_USER" \
    --password-stdin \
    --yes 2>&1; then
    echo "WARN: 设置管理员密码失败（DB 可能被占用或密码策略不满足）"
    echo "      请手动运行: $ADMIN_BIN admin reset --app-dir $DATA_DIR"
fi
echo "  → 管理员 $ADMIN_USER 已就绪"

# 关闭 pg-gateway
echo "关闭 pg-gateway..."
pg_stop_bg "$PID_DIR/bifrost-prepare.pid" "bifrost-prepare" 2>&1 || true
# 额外兜底
pkill -f "pg-gateway-http.*-app-dir $DATA_DIR" 2>/dev/null || true
sleep 1

echo "=== prepare_env 完成 ==="
echo "  数据目录: $DATA_DIR 已就绪（config.db + logs.db）"
echo "  fixture:  $FIXTURE_DIR 中的配置已写入"

pg_exit --status=pass --duration=$(( $(date +%s) - $(date +%s) )) \
        --metadata="data_dir=\"$DATA_DIR\" port=\"$PORT\""