#!/usr/bin/env bash
# Usage: runners/run_grpc_ghz.sh shared/targets/example-grpc.target.yaml
# Requires: ghz in PATH (go install github.com/bojand/ghz/cmd/ghz@latest)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

require_yq
TARGET_YAML="${1:?usage: run_grpc_ghz.sh <target.yaml>}"
gate_authorized "$TARGET_YAML"

HOST="$(yq_get "$TARGET_YAML" '.target.host')"
PORT="$(yq_get "$TARGET_YAML" '.target.port')"
INSECURE="$(yq_get "$TARGET_YAML" '.target.insecure_skip_verify')"

VUS="$(yq_get "$TARGET_YAML" '.load.virtual_users')"
RATE="$(yq_get "$TARGET_YAML" '.load.rate_per_second')"
DURATION="$(yq_get "$TARGET_YAML" '.load.duration_seconds')"

OUT_DIR="$(result_dir "$TARGET_YAML")"

cd "$(dirname "$SCRIPT_DIR")"

INSECURE_FLAG=""
if [[ "$INSECURE" == "true" ]]; then
  INSECURE_FLAG="--insecure"
fi

RPS_FLAG=""
if [[ "$RATE" != "0" && -n "$RATE" ]]; then
  RPS_FLAG="--rps $RATE"
fi

echo "[run_grpc_ghz] target=${HOST}:${PORT} concurrency=$VUS duration=${DURATION}s -> $OUT_DIR"

ghz $INSECURE_FLAG \
  --proto transport/grpc/ghz/echo.proto \
  --call stresslab.EchoService.Echo \
  -d '{"payload":"c3RyZXNzbGFiLXBheWxvYWQ="}' \
  -c "$VUS" \
  -z "${DURATION}s" \
  $RPS_FLAG \
  --format json \
  -o "$OUT_DIR/result.json" \
  "${HOST}:${PORT}" \
  | tee "$OUT_DIR/console.log"

# Also emit the human-readable pretty summary alongside the JSON.
ghz $INSECURE_FLAG \
  --proto transport/grpc/ghz/echo.proto \
  --call stresslab.EchoService.Echo \
  -d '{"payload":"c3RyZXNzbGFiLXBheWxvYWQ="}' \
  -c "$VUS" \
  -z "5s" \
  --format pretty \
  "${HOST}:${PORT}" \
  > "$OUT_DIR/summary_sample.txt" 2>&1 || true

echo "[run_grpc_ghz] done -> $OUT_DIR"
