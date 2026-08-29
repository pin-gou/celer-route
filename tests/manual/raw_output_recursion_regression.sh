#!/usr/bin/env bash
# Regression check for the raw-output recursion bug.
#
# Drives processRtkTextWithCommand in-process via the RTK test endpoint
# (no LLM involved), then feeds the resulting recovered body BACK through
# the same endpoint. Pre-fix, the second call would generate a NEW
# raw_output_id — i.e. one fetch begets another. Post-fix, the recovered
# body is wrapped with a server-side sentinel that the compression pipeline
# strips and bypasses, so no new id is produced.
#
# Usage:
#   bash tests/manual/raw_output_recursion_regression.sh
#   GATEWAY=http://192.168.3.18:20128 bash tests/manual/raw_output_recursion_regression.sh
#
# Exit codes:
#   0  PASS — no recursion detected
#   1  FAIL — recursion detected (new id produced on the second pass)
#   2  SETUP — environment not ready (gateway unreachable, fixture broken)

set -euo pipefail

GATEWAY="${GATEWAY:-http://127.0.0.1:20128}"
TEST_PATH="/api/context/rtk/test"
RAW_PATH="/api/context/rtk/raw-output"

# 50 KB markdown fixture — sized to trigger smartTruncate / charlimit on the
# first pass, which is what makes the original pointer appear in the wild.
FIXTURE="$(mktemp -t rtk_regression_fixture.XXXXXX.md)"
RECOVERED_FILE=""
trap 'rm -f "$FIXTURE" "$RECOVERED_FILE"' EXIT
{
  echo "# regression fixture"
  echo
  for i in $(seq 1 1000); do
    echo "| column-$i | value-$i |"
  done
} > "$FIXTURE"

if [ "$(wc -c <"$FIXTURE")" -lt 10240 ]; then
  echo "SETUP: fixture too small ($(wc -c <"$FIXTURE") bytes) — needs >= 10KB" >&2
  exit 2
fi

# Probe gateway.
if ! curl -sf "${GATEWAY}${TEST_PATH}" -X POST \
    -H "Content-Type: application/json" \
    -d '{"output": "ping", "command": "echo"}' >/dev/null 2>&1; then
  echo "SETUP: gateway unreachable at ${GATEWAY}${TEST_PATH}" >&2
  exit 2
fi

# Pass 1 — compress the fixture. Capture the first raw_output_id (if any).
PASS1_RESP="$(curl -sf "${GATEWAY}${TEST_PATH}" -X POST \
  -H "Content-Type: application/json" \
  --data-binary "$(jq -n --rawfile body "$FIXTURE" '{output: $body, command: "kubectl get pods"}')")"

ID1="$(printf '%s' "$PASS1_RESP" | jq -r '.stats.rawOutputPointers[0].id // empty')"
if [ -z "$ID1" ]; then
  echo "SETUP: pass 1 produced no raw_output_id — fixture may be too small or plugin disabled" >&2
  echo "pass 1 response: $PASS1_RESP" >&2
  exit 2
fi
echo "pass 1: produced raw_output_id=$ID1"

# Pass 2 — fetch the recovered body (default response carries the sentinel)
# and feed it BACK through the test endpoint. Pre-fix this triggers another
# round of compression and yields a new id; post-fix the sentinel bypass
# short-circuits the pipeline and no new id is produced.
RECOVERED_FILE="$(mktemp -t rtk_regression_recovered.XXXXXX.bin)"
curl -sf "${GATEWAY}${RAW_PATH}/${ID1}" -o "$RECOVERED_FILE"
if [ ! -s "$RECOVERED_FILE" ]; then
  echo "FAIL: recovered body is empty for id=$ID1" >&2
  exit 1
fi

# Verify the recovered body actually carries the sentinel. If it does not,
# the handler has regressed (or somebody reverted the wrap).
if ! head -c 30 "$RECOVERED_FILE" | grep -q $'^\x00RTK_RAW_OUTPUT_BEGIN\x00'; then
  echo "FAIL: recovered body does not start with sentinel — handler regression?" >&2
  exit 2
fi

# Pass 2 — feed the recovered body back through the test endpoint. We
# cannot use jq -Rs here because it escapes NUL as \u0000 which the
# stripper will not recognise; use python to build the JSON payload
# byte-for-byte.
PASS2_RESP="$(python3 - "$RECOVERED_FILE" <<'PY' | curl -sf "${GATEWAY}${TEST_PATH}" -X POST \
    -H "Content-Type: application/json" \
    --data-binary @-
import json, sys
with open(sys.argv[1], "rb") as f:
    body = f.read().decode("utf-8")
print(json.dumps({"output": body, "command": "kubectl get pods"}))
PY
)"

ID2="$(printf '%s' "$PASS2_RESP" | jq -r '.stats.rawOutputPointers[0].id // empty')"
TECHNIQUES="$(printf '%s' "$PASS2_RESP" | jq -r '.stats.techniques // [] | join(",")')"

if [ -z "$ID2" ]; then
  echo "pass 2: no new raw_output_id (techniques=[$TECHNIQUES])"
  echo "PASS — anti-recursion bypass active"
  exit 0
fi

echo "FAIL — recursion detected: pass 2 produced id=$ID2 (should be empty)"
echo "  pass 1 id: $ID1"
echo "  pass 2 id: $ID2 (recursion)"
echo "  pass 2 techniques: $TECHNIQUES"
exit 1
