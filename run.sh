#!/usr/bin/env bash
set -e

echo "=== stress-lab (multi-scenario) ==="

if ! command -v k6 >/dev/null 2>&1; then
  echo "Installing k6..."
  sudo gpg -k
  sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
    --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
  echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
    | sudo tee /etc/apt/sources.list.d/k6.list
  sudo apt-get update -qq
  sudo apt-get install -y k6
fi

echo ""
read -rp "Target (URL only, or full URL with path): " RAW_TARGET
read -rp "Duration in minutes [default 3]: " IN_MINUTES
read -rp "Max concurrent WebSocket connections [default 500]: " IN_WS_VUS
read -rp "Max concurrent HTTP GET requests [default 500]: " IN_GET_VUS
read -rp "Max concurrent JSON POST requests [default 500]: " IN_POST_VUS

if [[ -z "$RAW_TARGET" ]]; then
  echo "No target given. Exiting."
  exit 0
fi

if [[ "$RAW_TARGET" =~ ^([a-zA-Z][a-zA-Z0-9+.-]*)://(.*)$ ]]; then
  REST="${BASH_REMATCH[2]}"
else
  REST="$RAW_TARGET"
fi

if [[ "$REST" =~ ^([^/]+)(/.*)?$ ]]; then
  HOSTPORT="${BASH_REMATCH[1]}"
  GIVEN_PATH="${BASH_REMATCH[2]:-}"
else
  HOSTPORT="$REST"
  GIVEN_PATH=""
fi

if [[ "$HOSTPORT" =~ ^([^:]+):([0-9]+)$ ]]; then
  HOST="${BASH_REMATCH[1]}"
  PORT="${BASH_REMATCH[2]}"
else
  HOST="$HOSTPORT"
  PORT=443
fi

if [[ -z "$GIVEN_PATH" || "$GIVEN_PATH" == "/" ]]; then
  echo ""
  echo "No path given — probing common paths on ${HOST}:${PORT} ..."
  CANDIDATE_PATHS=("/" "/ws" "/websocket" "/vless" "/vmess" "/trojan" "/tr-ws" "/tr-ws-xray" "/xray" "/proxy")
  FOUND_PATH=""

  for p in "${CANDIDATE_PATHS[@]}"; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 --http1.1 \
      -H "Connection: Upgrade" -H "Upgrade: websocket" \
      -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
      "https://${HOST}:${PORT}${p}" 2>/dev/null || true)
    echo "  trying ${p} ... status=${STATUS}"
    if [[ "$STATUS" == "101" ]]; then
      FOUND_PATH="$p"
      echo "  -> match found: ${p}"
      break
    fi
  done

  [[ -z "$FOUND_PATH" ]] && FOUND_PATH="/"
  GIVEN_PATH="$FOUND_PATH"
fi

WSS_URL="wss://${HOST}:${PORT}${GIVEN_PATH}"
HTTPS_URL="https://${HOST}:${PORT}${GIVEN_PATH}"

MINUTES="${IN_MINUTES:-3}"
WS_VUS="${IN_WS_VUS:-500}"
GET_VUS="${IN_GET_VUS:-500}"
POST_VUS="${IN_POST_VUS:-500}"

echo ""
echo "WS target   : $WSS_URL"
echo "HTTP target : $HTTPS_URL"
echo "duration    : ${MINUTES}m per scenario"
echo "ws_vus=$WS_VUS get_vus=$GET_VUS post_vus=$POST_VUS"
echo ""

cat > ./multitest.js << EOJS
import ws from 'k6/ws';
import http from 'k6/http';
import { check } from 'k6';

const WSS_URL = '${WSS_URL}';
const HTTPS_URL = '${HTTPS_URL}';
const DUR = '${MINUTES}m';

export const options = {
  scenarios: {
    websocket_connections: {
      executor: 'ramping-vus',
      exec: 'wsTest',
      startVUs: 0,
      stages: [ { duration: '20s', target: ${WS_VUS} }, { duration: DUR, target: ${WS_VUS} } ],
      gracefulRampDown: '10s',
    },
    frontend_assets_load: {
      executor: 'ramping-vus',
      exec: 'getTest',
      startVUs: 0,
      stages: [ { duration: '20s', target: ${GET_VUS} }, { duration: DUR, target: ${GET_VUS} } ],
      gracefulRampDown: '10s',
    },
    json_post_stress: {
      executor: 'ramping-vus',
      exec: 'postTest',
      startVUs: 0,
      stages: [ { duration: '20s', target: ${POST_VUS} }, { duration: DUR, target: ${POST_VUS} } ],
      gracefulRampDown: '10s',
    },
  },
};

export function wsTest() {
  const payload = 'x'.repeat(4096);
  const res = ws.connect(WSS_URL, {}, function (socket) {
    socket.on('open', () => {
      socket.send(payload);
      socket.setInterval(() => { socket.send(payload); }, 500);
    });
    socket.on('message', () => {});
    socket.on('error', () => {});
    socket.setTimeout(() => { socket.close(); }, 20000);
  });
  check(res, { 'ws connected (101)': (r) => r && r.status === 101 });
}

export function getTest() {
  const res = http.get(HTTPS_URL);
  check(res, { 'get status ok': (r) => r.status >= 200 && r.status < 500 });
}

export function postTest() {
  const body = JSON.stringify({ test: 'stresslab', ts: Date.now() });
  const res = http.post(HTTPS_URL, body, { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'post status ok': (r) => r.status >= 200 && r.status < 500 });
}
EOJS

k6 run ./multitest.js
