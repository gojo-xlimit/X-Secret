#!/usr/bin/env bash
# Usage: runners/run_http.sh shared/targets/example-http2.target.yaml
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

require_yq
TARGET_YAML="${1:?usage: run_http.sh <target.yaml>}"
gate_authorized "$TARGET_YAML"

HOST="$(yq_get "$TARGET_YAML" '.target.host')"
PORT="$(yq_get "$TARGET_YAML" '.target.port')"
SCHEME="$(yq_get "$TARGET_YAML" '.target.scheme')"
PATH_="$(yq_get "$TARGET_YAML" '.target.path')"
INSECURE="$(yq_get "$TARGET_YAML" '.target.insecure_skip_verify')"

VUS="$(yq_get "$TARGET_YAML" '.load.virtual_users')"
RATE="$(yq_get "$TARGET_YAML" '.load.rate_per_second')"
DURATION="$(yq_get "$TARGET_YAML" '.load.duration_seconds')"
RAMP="$(yq_get "$TARGET_YAML" '.load.ramp_seconds')"

PAYLOAD_SIZE="$(yq_get "$TARGET_YAML" '.payload.size_bytes')"

P95="$(yq_get "$TARGET_YAML" '.thresholds.p95_latency_ms')"
P99="$(yq_get "$TARGET_YAML" '.thresholds.p99_latency_ms')"
ERR_MAX="$(yq_get "$TARGET_YAML" '.thresholds.error_rate_max')"

OUT_DIR="$(result_dir "$TARGET_YAML")"
TARGET_URL="${SCHEME}://${HOST}:${PORT}${PATH_}"

echo "[run_http] target=$TARGET_URL vus=$VUS duration=${DURATION}s rate=${RATE} -> $OUT_DIR"

cd "$(dirname "$SCRIPT_DIR")"

k6 run \
  -e TARGET_URL="$TARGET_URL" \
  -e VUS="$VUS" \
  -e DURATION="${DURATION}s" \
  -e RAMP="${RAMP}s" \
  -e PAYLOAD_SIZE="$PAYLOAD_SIZE" \
  -e RATE_PER_SECOND="$RATE" \
  -e INSECURE_SKIP_VERIFY="$INSECURE" \
  -e P95_MS="$P95" \
  -e P99_MS="$P99" \
  -e ERROR_RATE_MAX="$ERR_MAX" \
  transport/http/k6/http_test.js \
  | tee "$OUT_DIR/console.log"

# k6's handleSummary already wrote result.json to CWD; move it into the
# timestamped result dir for consistency with the Go harness output layout.
[[ -f result.json ]] && mv result.json "$OUT_DIR/result.json"

echo "[run_http] done -> $OUT_DIR"
