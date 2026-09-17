# Build & Setup

This repo runs across three environments per your stack: Termux (on-device,
lightweight ops), Cloud Shell (GCP-native scripting), and GitHub Codespaces
(full dev environment, best for the actual Go build).

## 1. Building the Go binary

**Not yet compiled/verified in this delivery** — the sandbox that generated
this repo had no Go toolchain and no network egress to fetch modules. Build
it yourself in Codespaces or Cloud Shell first:

```bash
cd stress-lab
go mod tidy         # fetches golang.org/x/crypto, gopkg.in/yaml.v3
go build -o bin/stresslab ./cmd/stresslab
./bin/stresslab -h  # should print usage
```

If `go mod tidy` reports version conflicts, pin explicitly:

```bash
go get golang.org/x/crypto@v0.31.0
go get gopkg.in/yaml.v3@v3.0.1
```

### Cross-compiling for Termux (Android arm64)

Build on Codespaces/Cloud Shell, then copy the resulting binary to your
device — don't build the Go toolchain on Termux itself:

```bash
GOOS=linux GOARCH=arm64 go build -o bin/stresslab-arm64 ./cmd/stresslab
```

Copy `bin/stresslab-arm64` to your device (via `scp`, Google Drive, or
`termux-storage`), rename to `stresslab`, `chmod +x`, and place it at
`stress-lab/bin/stresslab` so `runners/run_go_harness.sh` finds it without
rebuilding.

## 2. Installing the other tools

| Tool | Termux | Cloud Shell / Codespaces |
|---|---|---|
| `yq` (mikefarah, Go version) | `pkg install yq` | `go install github.com/mikefarah/yq/v4@latest` or apt |
| `k6` | `pkg install k6` if available, else build from source | `sudo apt install k6` (add Grafana apt repo) or use `xk6` |
| `ghz` | cross-compiled binary copied over, same as stresslab | `go install github.com/bojand/ghz/cmd/ghz@latest` |
| `artillery` | `pkg install nodejs && npm install -g artillery` | `npm install -g artillery` |
| `xray` (xray-core) | download prebuilt release binary for your arch | download prebuilt release binary, or build from source |

Verify each with `--version` / `-h` before running any test.

## 3. Running a test

```bash
# single entrypoint, dispatches by transport.kind automatically
./runners/run.sh shared/targets/example-http2.target.yaml

# websocket via Artillery's ramping-arrival-rate model instead of k6's fixed VUs
./runners/run.sh shared/targets/example-websocket.target.yaml --ws-engine artillery

# ssh — export credentials first (see docs/environment-variables.md)
export STRESSLAB_SSH_USER=labuser STRESSLAB_SSH_PASS=labpass
./runners/run.sh shared/targets/example-ssh-openssh.target.yaml

# xray vless — export UUID first
export STRESSLAB_XRAY_UUID=00000000-0000-0000-0000-000000000000
./runners/run_go_harness.sh shared/targets/example-vless-ws.target.yaml \
  -- -upstream-test-url https://your-authorized-upstream.example/health
```

## 4. Creating a new test target

Copy an existing file under `shared/targets/`, rename it, and edit:

```bash
cp shared/targets/example-websocket.target.yaml shared/targets/my-test.target.yaml
```

Required edits before running:

- `target.host` — must not be `target.example` (the loader/gate refuses placeholders)
- `meta.authorized` — must be `true`
- `load.*` — concurrency, rate, duration, ramp to fit your scenario
- `thresholds.*` — pass/fail gates for CI-style automated runs

## 5. Reading results

Every runner writes to `results/<meta.name>/<timestamp>/`:

- `result.json` — machine-readable, tool-native or normalized (Go harnesses use `internal/report`'s schema)
- `summary.txt` — human-readable (Go harnesses only)
- `console.log` — raw stdout from the underlying tool
- `report.html` (Artillery only) — visual report

Go harness subcommands exit code `2` if any threshold was violated —
useful for scripting a pass/fail gate around repeated runs.
