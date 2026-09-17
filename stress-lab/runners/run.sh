#!/usr/bin/env bash
# Single entrypoint for any test. Dispatches to the correct tool-specific
# runner based on transport.kind in the target YAML.
#
# Usage: runners/run.sh shared/targets/<name>.target.yaml [--ws-engine artillery]
#
# transport.kind -> runner mapping:
#   http1, http2   -> run_http.sh          (k6)
#   websocket      -> run_ws.sh            (k6, default) or run_ws_artillery.sh with --ws-engine artillery
#   grpc           -> run_grpc_ghz.sh      (ghz)
#   ssh            -> run_go_harness.sh    (Go, subcommand ssh)
#   xray           -> run_go_harness.sh    (Go, subcommand xray)
#   httpupgrade    -> run_go_harness.sh    (Go, subcommand httpupgrade)
#   xhttp          -> run_go_harness.sh    (Go, subcommand xhttp)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

require_yq
TARGET_YAML="${1:?usage: run.sh <target.yaml> [--ws-engine artillery]}"
shift || true

WS_ENGINE="k6"
if [[ "${1:-}" == "--ws-engine" ]]; then
  WS_ENGINE="${2:?}"
  shift 2
fi

gate_authorized "$TARGET_YAML"
KIND="$(yq_get "$TARGET_YAML" '.transport.kind')"

case "$KIND" in
  http1|http2)
    "$SCRIPT_DIR/run_http.sh" "$TARGET_YAML"
    ;;
  websocket)
    if [[ "$WS_ENGINE" == "artillery" ]]; then
      "$SCRIPT_DIR/run_ws_artillery.sh" "$TARGET_YAML"
    else
      "$SCRIPT_DIR/run_ws.sh" "$TARGET_YAML"
    fi
    ;;
  grpc)
    "$SCRIPT_DIR/run_grpc_ghz.sh" "$TARGET_YAML"
    ;;
  ssh|xray|httpupgrade|xhttp)
    "$SCRIPT_DIR/run_go_harness.sh" "$TARGET_YAML" "$@"
    ;;
  *)
    echo "error: unrecognized transport.kind '$KIND' in $TARGET_YAML" >&2
    exit 1
    ;;
esac
