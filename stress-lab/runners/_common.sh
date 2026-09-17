#!/usr/bin/env bash
# Shared helpers sourced by every runners/run_*.sh script.
# Requires: yq (mikefarah/yq, the Go one — NOT python-yq). On Termux:
#   pkg install yq
# or:
#   go install github.com/mikefarah/yq/v4@latest
set -euo pipefail

require_yq() {
  if ! command -v yq >/dev/null 2>&1; then
    echo "error: yq (mikefarah/yq) not found in PATH. Install it first." >&2
    exit 1
  fi
}

# yq_get <file> <path>  -- prints the value or empty string
yq_get() {
  local file="$1" path="$2"
  yq eval "$path" "$file"
}

# gate_authorized <target-yaml> -- refuses to continue unless
# meta.authorized is explicitly true. This mirrors the check already
# enforced inside the Go harness's config loader, but stops shell-only
# tool invocations (k6/ghz/artillery) before they even start.
gate_authorized() {
  local target_yaml="$1"
  local authorized
  authorized="$(yq_get "$target_yaml" '.meta.authorized')"
  if [[ "$authorized" != "true" ]]; then
    echo "error: meta.authorized is not 'true' in $target_yaml — refusing to run." >&2
    exit 1
  fi

  local host
  host="$(yq_get "$target_yaml" '.target.host')"
  if [[ -z "$host" || "$host" == "target.example" ]]; then
    echo "error: target.host is unset or still the placeholder in $target_yaml." >&2
    exit 1
  fi
}

# result_dir <target-yaml> -- prints (and creates) results/<name>/<timestamp>/
result_dir() {
  local target_yaml="$1"
  local name out_dir stamp
  name="$(yq_get "$target_yaml" '.meta.name')"
  out_dir="$(yq_get "$target_yaml" '.output.dir')"
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  local dir="${out_dir}/${name}/${stamp}"
  mkdir -p "$dir"
  echo "$dir"
}
