# Environment Variables Reference

All credentials are read from environment variables only. Never pass them
as CLI flags (they'd land in shell history and `ps` output) and never
hardcode them in any YAML/config file in this repo.

## SSH harness (`stresslab ssh`)

| Variable | Required | Notes |
|---|---|---|
| `STRESSLAB_SSH_USER` | yes | SSH username |
| `STRESSLAB_SSH_PASS` | one of PASS/KEY | Password auth |
| `STRESSLAB_SSH_KEY` | one of PASS/KEY | Path to private key file (takes precedence over PASS if both set) |

```bash
export STRESSLAB_SSH_USER="labuser"
export STRESSLAB_SSH_PASS="labpassword"
# or
export STRESSLAB_SSH_KEY="$HOME/.ssh/id_ed25519_lab"
```

## Xray harness (`stresslab xray`)

| Variable | Required when | Notes |
|---|---|---|
| `STRESSLAB_XRAY_UUID` | `transport.xray_protocol` is `vless` or `vmess` | Client UUID configured on the server |
| `STRESSLAB_XRAY_PASSWORD` | `transport.xray_protocol` is `trojan` | Trojan password |

```bash
export STRESSLAB_XRAY_UUID="00000000-0000-0000-0000-000000000000"
```

## Setting variables per-session on Termux

Termux processes don't persist env vars across app restarts. Either:

- Export them in the current shell session before running `runners/run.sh`, or
- Put them in a local, gitignored file (e.g. `secrets.env`, already excluded
  via `.gitignore`) and `source secrets.env` before running tests:

```bash
cat > secrets.env << 'EOF'
export STRESSLAB_SSH_USER="labuser"
export STRESSLAB_SSH_PASS="labpassword"
EOF
chmod 600 secrets.env
source secrets.env
```

Never commit `secrets.env` or any file matching the secret-like patterns
already excluded in `.gitignore` (`*.pem`, `*.key`, `id_rsa*`, `*.env`).
