#!/usr/bin/env bash
# send-llm-test.sh — 查询 local 环境 providers / models / routes，随机挑选并发送测试请求
#
# 用法:
#   ./send-llm-test.sh [count]
#
# 参数:
#   count  测试请求数量（默认 3）
#
# 环境变量:
#   PG_INSTANCE_HOST  API 主机（默认 localhost）
#   PG_INSTANCE_PORT  API 端口（默认 9080）

set -uo pipefail

COUNT="${1:-3}"
HOST="${PG_INSTANCE_HOST:-localhost}"
PORT="${PG_INSTANCE_PORT:-9080}"
BASE="http://${HOST}:${PORT}"

# 随机消息池
MESSAGES=(
  "Hello! What can you do?"
  "Explain quantum computing in one sentence."
  "Write a haiku about programming."
  "What is the capital of France?"
  "Count from 1 to 5."
  "Is the sky blue? Answer yes or no."
  "What is 2 + 2?"
  "Tell me a short joke."
)

echo "=== Bifrost LLM Test — sending ${COUNT} request(s) to ${BASE} ==="
echo ""

# ---- 1. 查询 providers ----
echo "--- Querying providers ---"
PROVIDERS=$(curl -s "${BASE}/api/providers" | python3 -c "
import json, sys
d = json.load(sys.stdin)
names = [p['name'] for p in d.get('providers', [])]
print(' '.join(names))
" 2>/dev/null || echo "")

if [[ -z "$PROVIDERS" ]]; then
  echo "WARN: 无法获取 providers 列表，继续使用空列表"
fi
echo "providers: ${PROVIDERS:-none}"
echo ""

# ---- 2. 查询 models ----
echo "--- Querying models ---"
MODELS=$(curl -s "${BASE}/api/models" | python3 -c "
import json, sys
d = json.load(sys.stdin)
names = [m['name'] for m in d.get('models', []) if m.get('name')]
print(' '.join(names))
" 2>/dev/null || echo "")

if [[ -z "$MODELS" ]]; then
  echo "WARN: 无法获取 models 列表，使用 fixture 模型名称"
  MODELS="deepseek-v4-flash-0731 deepseek-v4-pro glm-5.2 qwen3.6-flash qwen3.7-max"
fi
echo "models: ${MODELS}"
echo ""

# ---- 3. 查询 routing rules ----
echo "--- Querying routing rules ---"
ROUTES=$(curl -s "${BASE}/api/governance/routing-rules" | python3 -c "
import json, sys
d = json.load(sys.stdin)
names = [r['name'] for r in d.get('rules', []) if r.get('name')]
print(' '.join(names))
" 2>/dev/null || echo "")

if [[ -z "$ROUTES" ]]; then
  echo "WARN: 无法获取 routing rules 列表，使用 fixture 路由名称"
  ROUTES="pg-expert pg-master pg-associate hermes-default hermes-operator"
fi
echo "routes: ${ROUTES}"
echo ""

# ---- 将列表转数组 ----
IFS=' ' read -ra MODEL_ARRAY <<< "$MODELS"
IFS=' ' read -ra ROUTE_ARRAY <<< "$ROUTES"
IFS=' ' read -ra MSG_ARRAY <<< "${MESSAGES[*]}"

# ---- 4. 发送测试请求 ----
PASS=0
FAIL=0

for ((i = 1; i <= COUNT; i++)); do
  echo "--- Request #${i} ---"

  # 随机决定使用 model 还是 route
  USE_MODEL=$((RANDOM % 2))

  if [[ USE_MODEL -eq 0 && ${#MODEL_ARRAY[@]} -gt 0 ]]; then
    idx=$((RANDOM % ${#MODEL_ARRAY[@]}))
    MODEL="${MODEL_ARRAY[$idx]}"
    echo "mode: by-model | model: ${MODEL}"
  elif [[ ${#ROUTE_ARRAY[@]} -gt 0 ]]; then
    idx=$((RANDOM % ${#ROUTE_ARRAY[@]}))
    MODEL="${ROUTE_ARRAY[$idx]}"
    echo "mode: by-route | route: ${MODEL}"
  else
    idx=$((RANDOM % ${#MODEL_ARRAY[@]}))
    MODEL="${MODEL_ARRAY[$idx]}"
    echo "mode: by-model (route fallback) | model: ${MODEL}"
  fi

  msg_idx=$((RANDOM % ${#MESSAGES[@]}))
  MSG="${MESSAGES[$msg_idx]}"

  echo "request: model=${MODEL} message=\"${MSG:0:50}...\""

  START=$(date +%s%N)
  HTTP_CODE=$(curl -s -o /tmp/send-llm-test-resp-$$.json -w "%{http_code}" \
    -X POST "${BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "$(cat <<PAYLOAD
{
  "model": "${MODEL}",
  "messages": [
    {"role": "user", "content": "${MSG}"}
  ],
  "params": {
    "max_completion_tokens": 256,
    "stream": false
  }
}
PAYLOAD
)" 2>/dev/null)
  END=$(date +%s%N)
  ELAPSED_MS=$(( (END - START) / 1000000 ))

  if [[ "$HTTP_CODE" == "200" ]]; then
    CONTENT=$(python3 -c "
import json
with open('/tmp/send-llm-test-resp-$$.json') as f:
    d = json.load(f)
choices = d.get('choices', [])
if choices:
    msg = choices[0].get('message', {})
    print(msg.get('content', '(no content)')[:100])
else:
    print('(no choices)')
" 2>/dev/null || echo "(parse error)")
    echo "result: PASS (${HTTP_CODE}, ${ELAPSED_MS}ms) → ${CONTENT}"
    PASS=$((PASS + 1))
  else
    BODY=$(head -c 200 /tmp/send-llm-test-resp-$$.json 2>/dev/null || echo "(no response body)")
    echo "result: FAIL (${HTTP_CODE}, ${ELAPSED_MS}ms) → ${BODY}"
    FAIL=$((FAIL + 1))
  fi
  rm -f /tmp/send-llm-test-resp-$$.json
  echo ""
done

echo "=== Summary ==="
echo "Total: ${COUNT} | Pass: ${PASS} | Fail: ${FAIL}"

[[ "$FAIL" -eq 0 ]] && exit 0 || exit 1