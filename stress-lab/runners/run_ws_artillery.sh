#!/usr/bin/env bash
# Usage: runners/run_ws_artillery.sh shared/targets/example-websocket.target.yaml
# Requires: npm install -g artillery
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

require_yq
TARGET_YAML="${1:?usage: run_ws_artillery.sh <target.yaml>}"
gate_authorized "$TARGET_YAML"

HOST="$(yq_get "$TARGET_YAML" '.target.host')"
PORT="$(yq_get "$TARGET_YAML" '.target.port')"
SCHEME="$(yq_get "$TARGET_YAML" '.target.scheme')"
PATH_="$(yq_get "$TARGET_YAML" '.target.path')"
VUS="$(yq_get "$TARGET_YAML" '.load.virtual_users')"

OUT_DIR="$(result_dir "$TARGET_YAML")"
TARGET_URL="${SCHEME}://${HOST}:${PORT}${PATH_}"

cd "$(dirname "$SCRIPT_DIR")"

# Generate a run-specific copy of the base scenario with the real target
# and peak arrival rate substituted in, rather than mutating the template.
GENERATED="$OUT_DIR/ws_ramp.generated.yml"
sed -e "s#wss://target.example/ws#${TARGET_URL}#g" \
    -e "s/arrivalRate: 50/arrivalRate: ${VUS}/g" \
    -e "s/rampTo: 50/rampTo: ${VUS}/g" \
    transport/websocket/artillery/ws_ramp.yml > "$GENERATED"

echo "[run_ws_artillery] target=$TARGET_URL peak_rate=$VUS -> $OUT_DIR"

artillery run "$GENERATED" --output "$OUT_DIR/report.json" | tee "$OUT_DIR/console.log"
artillery report "$OUT_DIR/report.json" --output "$OUT_DIR/report.html" || true

echo "[run_ws_artillery] done -> $OUT_DIR"
