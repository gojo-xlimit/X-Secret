# stress-lab

Authorized infrastructure stress-testing framework for a self-owned
Cloud Run / Xray-core / SSH lab. Controlled, configurable, reproducible —
not a generic flooder. Every runner refuses to execute unless the target
config explicitly declares `meta.authorized: true` and a real (non-
placeholder) host.

## What this covers

| Transport | Tool | Why |
|---|---|---|
| HTTP/1.1, HTTP/2 | **k6** | Native support, auto H2 upgrade, built-in percentile thresholds |
| WebSocket | **k6** (default) / **Artillery** (ramp) | k6 for fixed-VU throughput; Artillery for arrival-rate ramp modeling |
| gRPC | **ghz** (primary) / **k6** (secondary) | ghz is purpose-built with load schedules + histograms; k6 for scripted multi-RPC flows |
| HTTPUpgrade | **Go harness** | No load tool models this transport's raw upgrade mechanics correctly |
| XHTTP (SplitHTTP) | **Go harness** | Session-based packet-up/stream-down split isn't expressible in generic HTTP tools |
| Xray (VLESS/VMess/Trojan) | **Go harness** (spawns real `xray-core`) | Measures the actual client-relay path, not a reimplementation of the protocol |
| SSH (OpenSSH/Dropbear) | **Go harness** | Connect-cycle + channel throughput isn't in any generic load tool |

**Dropped:** wrk2 — archived upstream (Nov 2022), broken aarch64 build
without unofficial patches, no maintained fork. Not viable as a base.

## Repo layout

```
stress-lab/
├── cmd/stresslab/              Unified Go CLI (ssh, xray, httpupgrade, xhttp subcommands)
├── internal/
│   ├── config/                 Shared YAML config loader + authorization gate
│   ├── metrics/                 Thread-safe sample collector, percentiles
│   ├── report/                  JSON/summary/CSV writer + threshold evaluation
│   └── harness/{ssh,xray,httpupgrade,xhttp}/   Protocol-specific stress logic
├── transport/
│   ├── http/k6/                 HTTP/1.1+2 k6 script
│   ├── websocket/{k6,artillery}/ WS scripts for both engines
│   └── grpc/{ghz,k6}/           gRPC proto + ghz config + k6 script
├── shared/
│   ├── schemas/target.schema.yaml   Documented schema, the source of truth
│   └── targets/*.target.yaml        Ready-to-edit example configs, one per scenario
├── runners/                     Shell wrappers: YAML -> tool-specific invocation
├── docs/                        Setup, env vars, echo-target reference
└── results/                     Timestamped output per test (gitignored)
```

## Quick start

```bash
# 1. build the Go harness (see docs/build-and-setup.md for per-environment notes)
go build -o bin/stresslab ./cmd/stresslab

# 2. copy and edit a target config
cp shared/targets/example-http2.target.yaml shared/targets/my-test.target.yaml
# edit target.host to point at infra you own

# 3. run it — single entrypoint dispatches by transport.kind
./runners/run.sh shared/targets/my-test.target.yaml
```

See `docs/build-and-setup.md` for tool installation per environment
(Termux / Cloud Shell / Codespaces), `docs/environment-variables.md` for
credential handling, and `docs/setting-up-echo-targets.md` for what the
HTTPUpgrade/XHTTP/Xray harnesses need on the server side to produce
meaningful throughput numbers (not just handshake success).

## Status / verification notes

- **k6, ghz, Artillery configs**: mechanics verified against current
  official docs (see inline comments citing source behavior — e.g. XHTTP
  packet-up/stream-down mode confirmed against Xray-core's splithttp
  dialer/hub source references).
- **Go harness code**: written and manually reviewed for import/signature
  consistency, but **not compiled or run** — the environment that generated
  this repo had no Go toolchain and no network egress to fetch
  `golang.org/x/crypto` / `gopkg.in/yaml.v3`. Run `go build ./...` in
  Codespaces or Cloud Shell before trusting it in a real test. Flagging
  this explicitly rather than claiming false confidence.
- **No live test was run** against real infrastructure during generation.
  All target configs point at the `target.example` placeholder and will
  be refused by the authorization gate until you edit them.

## Extending

- New HTTP/WS/gRPC scenario: copy a target YAML, adjust `load`/`payload`/
  `thresholds`, run through `runners/run.sh`.
- New protocol entirely: add a package under `internal/harness/<name>/`
  implementing `Run(cfg *config.TargetConfig) (*metrics.Collector, error)`,
  wire it into `cmd/stresslab/subcommands.go` and `runners/run_go_harness.sh`.
