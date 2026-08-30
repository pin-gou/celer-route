#!/bin/bash
RUN_OPENCODE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROMPT_FILE="$RUN_OPENCODE_SCRIPT_DIR/prompt.txt"
cd "$RUN_OPENCODE_SCRIPT_DIR/../.." || exit 1

if [ ! -f "$PROMPT_FILE" ] || [ -z "$(cat "$PROMPT_FILE")" ]; then
    echo ">>> prompt.txt not found or empty, skip"
    exit 0
fi

# 暂存本地修改，避免 git 操作丢失变更
STASH_RESULT=$(git stash push -m "cronjob-auto-stash-$(date +%Y%m%d%H%M%S)" 2>&1) || true

# 无论脚本后续是否出错，退出时恢复暂存
trap 'git stash pop 2>/dev/null || true' EXIT

git checkout master

git pull --rebase


echo ">>> opencode run --file $PROMPT_FILE"
opencode run --agent pg-manager "执行 .pg/cronjobs/prompt.txt 中的指令" --file "$PROMPT_FILE"
