#!/usr/bin/env bash
# Usage: runners/run_ws.sh shared/targets/example-websocket.target.yaml
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

require_yq
TARGET_YAML="${1:?usage: run_ws.sh <target.yaml>}"
gate_authorized "$TARGET_YAML"

HOST="$(yq_get "$TARGET_YAML" '.target.host')"
PORT="$(yq_get "$TARGET_YAML" '.target.port')"
SCHEME="$(yq_get "$TARGET_YAML" '.target.scheme')"
PATH_="$(yq_get "$TARGET_YAML" '.target.path')"

VUS="$(yq_get "$TARGET_YAML" '.load.virtual_users')"
DURATION="$(yq_get "$TARGET_YAML" '.load.duration_seconds')"
RAMP="$(yq_get "$TARGET_YAML" '.load.ramp_seconds')"

PAYLOAD_SIZE="$(yq_get "$TARGET_YAML" '.payload.size_bytes')"

P95="$(yq_get "$TARGET_YAML" '.thresholds.p95_latency_ms')"
P99="$(yq_get "$TARGET_YAML" '.thresholds.p99_latency_ms')"
ERR_MAX="$(yq_get "$TARGET_YAML" '.thresholds.error_rate_max')"

OUT_DIR="$(result_dir "$TARGET_YAML")"
TARGET_URL="${SCHEME}://${HOST}:${PORT}${PATH_}"

echo "[run_ws] target=$TARGET_URL vus=$VUS duration=${DURATION}s -> $OUT_DIR"

cd "$(dirname "$SCRIPT_DIR")"

k6 run \
  -e TARGET_URL="$TARGET_URL" \
  -e VUS="$VUS" \
  -e DURATION="${DURATION}s" \
  -e RAMP="${RAMP}s" \
  -e PAYLOAD_SIZE="$PAYLOAD_SIZE" \
  -e HOLD_SECONDS=5 \
  -e P95_MS="$P95" \
  -e P99_MS="$P99" \
  -e ERROR_RATE_MAX="$ERR_MAX" \
  transport/websocket/k6/ws_test.js \
  | tee "$OUT_DIR/console.log"

echo "[run_ws] done -> $OUT_DIR"
echo "[run_ws] tip: for ramping-arrival-rate modeling instead of fixed VUs, use run_ws_artillery.sh"
