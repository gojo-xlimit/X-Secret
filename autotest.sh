#!/usr/bin/env bash
set -uo pipefail

echo "=== stress-lab :: auto-detect ==="
read -rp "Target (any format — example.com, example.com/path, https://host:port/path): " RAW_INPUT

if [[ -z "$RAW_INPUT" ]]; then
  echo "No input given. Exiting."
  exit 1
fi

INPUT="$RAW_INPUT"

if [[ "$INPUT" =~ ^([a-zA-Z][a-zA-Z0-9+.-]*)://(.*)$ ]]; then
  SCHEME="${BASH_REMATCH[1]}"
  REST="${BASH_REMATCH[2]}"
else
  SCHEME="https"
  REST="$INPUT"
fi

if [[ "$REST" =~ ^([^/]+)(/.*)?$ ]]; then
  HOSTPORT="${BASH_REMATCH[1]}"
  PATH_PART="${BASH_REMATCH[2]:-/}"
else
  HOSTPORT="$REST"
  PATH_PART="/"
fi

if [[ "$HOSTPORT" =~ ^([^:]+):([0-9]+)$ ]]; then
  HOST="${BASH_REMATCH[1]}"
  PORT="${BASH_REMATCH[2]}"
else
  HOST="$HOSTPORT"
  case "$SCHEME" in
    http) PORT=80 ;;
    https|wss) PORT=443 ;;
    ws) PORT=80 ;;
    *) PORT=443 ;;
  esac
fi

UUID=""
PASSWORD=""
if [[ "$RAW_INPUT" =~ uuid=([a-zA-Z0-9-]+) ]]; then UUID="${BASH_REMATCH[1]}"; fi
if [[ "$RAW_INPUT" =~ pass=([^ ]+) ]]; then PASSWORD="${BASH_REMATCH[1]}"; fi

echo ""
echo "--- Parsed target ---"
echo "host: $HOST   port: $PORT   path: $PATH_PART   scheme(hint): $SCHEME"
[[ -n "$UUID" ]] && echo "uuid: $UUID"
[[ -n "$PASSWORD" ]] && echo "password: (provided)"
echo ""

BASE_HTTPS="https://${HOST}:${PORT}${PATH_PART}"
BASE_WSS="wss://${HOST}:${PORT}${PATH_PART}"

echo "--- Probing target ---"

probe_status() {
  curl -s -o /dev/null -w "%{http_code}" --max-time 8 "$@" 2>/dev/null || echo "000"
}

PLAIN_STATUS=$(probe_status "$BASE_HTTPS")
echo "plain HTTPS GET      : status=$PLAIN_STATUS"

WS_STATUS=$(probe_status --http1.1 \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  "$BASE_HTTPS")
echo "WS upgrade (HTTP/1.1) : status=$WS_STATUS"

TCP_OPEN="no"
if timeout 3 bash -c "exec 3<>/dev/tcp/$HOST/$PORT" 2>/dev/null; then
  TCP_OPEN="yes"
  exec 3>&- 2>/dev/null || true
fi
echo "raw TCP connect       : $TCP_OPEN"

echo ""
echo "--- Detection result ---"

DETECTED="unknown"
if [[ "$WS_STATUS" == "101" ]]; then
  DETECTED="websocket"
  echo "-> WebSocket-based transport detected (VLESS/VMess/Trojan/SS likely candidates)."
elif [[ "$PLAIN_STATUS" =~ ^(200|301|302|403)$ ]]; then
  DETECTED="web"
  echo "-> Plain web/dashboard response detected (status=$PLAIN_STATUS)."
elif [[ "$TCP_OPEN" == "yes" ]]; then
  DETECTED="raw-tcp"
  echo "-> Port open but not HTTP-shaped — likely raw TCP protocol (SSH, custom TLS, etc)."
else
  echo "-> Could not get a response. Check host/port/path."
  exit 1
fi
echo ""

echo "--- Running load test (auto defaults) ---"

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 not found. Run setup.sh first."
  exit 1
fi

case "$DETECTED" in
  websocket)
    cat > /tmp/autotest_ws.js << 'EOJS'
import ws from 'k6/ws';
import { check } from 'k6';
const URL = __ENV.URL;
const res = ws.connect(URL, {}, function (socket) {
  socket.on('open', () => {
    socket.send('x'.repeat(1024));
    socket.setInterval(() => { socket.send('x'.repeat(1024)); }, 500);
  });
  socket.on('error', () => {});
  socket.setTimeout(() => { socket.close(); }, 20000);
});
check(res, { 'connected (101)': (r) => r && r.status === 101 });
EOJS
    k6 run -e URL="$BASE_WSS" \
      --vus 100 --duration 60s \
      /tmp/autotest_ws.js
    ;;
  web)
    cat > /tmp/autotest_http.js << 'EOJS'
import http from 'k6/http';
import { check } from 'k6';
const URL = __ENV.URL;
export default function () {
  const res = http.get(URL);
  check(res, { 'status is 2xx/3xx/4xx': (r) => r.status >= 200 && r.status < 500 });
}
EOJS
    k6 run -e URL="$BASE_HTTPS" \
      --vus 100 --duration 60s \
      /tmp/autotest_http.js
    ;;
  raw-tcp)
    echo "Raw TCP protocol detected. This needs the dedicated Go harness"
    echo "(ssh/xray subcommands) from the main stress-lab repo, not a quick"
    echo "auto-test — connection semantics vary too much to guess safely."
    ;;
esac
