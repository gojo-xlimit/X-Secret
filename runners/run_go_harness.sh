#!/usr/bin/env bash
# Usage: runners/run_go_harness.sh shared/targets/example-ssh-openssh.target.yaml [-- extra flags]
# Dispatches to the correct `stresslab` subcommand based on transport.kind
# in the YAML. Builds the binary on first use if not already built.
#
# Credentials must already be exported before calling this script:
#   export STRESSLAB_SSH_USER=... STRESSLAB_SSH_PASS=...      (ssh)
#   export STRESSLAB_XRAY_UUID=...                             (vless/vmess)
#   export STRESSLAB_XRAY_PASSWORD=...                         (trojan)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

require_yq
TARGET_YAML="${1:?usage: run_go_harness.sh <target.yaml> [-- extra flags]}"
shift || true
gate_authorized "$TARGET_YAML"

KIND="$(yq_get "$TARGET_YAML" '.transport.kind')"

BIN="$REPO_ROOT/bin/stresslab"
if [[ ! -x "$BIN" ]]; then
  echo "[run_go_harness] binary not found, building..." >&2
  mkdir -p "$REPO_ROOT/bin"
  (cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/stresslab)
fi

case "$KIND" in
  ssh)
    exec "$BIN" ssh -config "$TARGET_YAML" "$@"
    ;;
  xray)
    exec "$BIN" xray -config "$TARGET_YAML" "$@"
    ;;
  httpupgrade)
    exec "$BIN" httpupgrade -config "$TARGET_YAML" "$@"
    ;;
  xhttp)
    exec "$BIN" xhttp -config "$TARGET_YAML" "$@"
    ;;
  *)
    echo "error: transport.kind '$KIND' is not handled by the Go harness (use k6/ghz/artillery runners instead)." >&2
    exit 1
    ;;
esac
