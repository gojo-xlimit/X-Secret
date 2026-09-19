#!/usr/bin/env bash
set -e

echo "=== stress-lab ==="

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

cat > ./wstest.js << 'EOJS'
import ws from 'k6/ws';
import { check } from 'k6';

const URL = __ENV.URL;
if (!URL) { throw new Error('Set -e URL=wss://your-target.example/path'); }

const RPS = parseInt(__ENV.RPS || '0');
const DURATION = __ENV.DURATION || '30s';
const CONCURRENCY = parseInt(__ENV.CONCURRENCY || '20');
const RAMP = __ENV.RAMP || '5s';
const HOLD_SECONDS = parseFloat(__ENV.HOLD_SECONDS || '10');
const PAYLOAD_BYTES = parseInt(__ENV.PAYLOAD_BYTES || '1024');

export const options = RPS > 0 ? {
  scenarios: { capped_rate: { executor: 'constant-arrival-rate', rate: RPS, timeUnit: '1s', duration: DURATION, preAllocatedVUs: CONCURRENCY, maxVUs: CONCURRENCY * 2 } }
} : {
  stages: [ { duration: RAMP, target: CONCURRENCY }, { duration: DURATION, target: CONCURRENCY } ]
};

export default function () {
  const payload = 'x'.repeat(PAYLOAD_BYTES);
  const res = ws.connect(URL, {}, function (socket) {
    socket.on('open', () => {
      socket.send(payload);
      socket.setInterval(() => { socket.send(payload); }, 500);
    });
    socket.on('message', () => {});
    socket.on('error', () => {});
    socket.setTimeout(() => { socket.close(); }, HOLD_SECONDS * 1000);
  });
  check(res, { 'connected (101)': (r) => r && r.status === 101 });
}
EOJS

echo ""
read -rp "Target (URL only, or full URL with path): " RAW_TARGET
read -rp "RPS [default 500]: " IN_RPS
read -rp "Duration in seconds [default 60]: " IN_DURATION
read -rp "Concurrency [default 1000]: " IN_CONCURRENCY
read -rp "Hold seconds per connection [default 30]: " IN_HOLD
read -rp "Payload bytes [default 4096]: " IN_PAYLOAD

if [[ -z "$RAW_TARGET" ]]; then
  echo "No target given. Exiting."
  exit 0
fi

if [[ "$RAW_TARGET" =~ ^([a-zA-Z][a-zA-Z0-9+.-]*)://(.*)$ ]]; then
  SCHEME="${BASH_REMATCH[1]}"
  REST="${BASH_REMATCH[2]}"
else
  SCHEME="https"
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
      "https://${HOST}:${PORT}${p}" 2>/dev/null)
    echo "  trying ${p} ... status=${STATUS}"
    if [[ "$STATUS" == "101" ]]; then
      FOUND_PATH="$p"
      echo "  -> match found: ${p}"
      break
    fi
  done

  if [[ -z "$FOUND_PATH" ]]; then
    echo ""
    echo "No common path returned a successful WebSocket upgrade (101)."
    echo "This target likely uses a custom/random path — for example, the"
    echo "path shown in your panel's connection link or QR code."
    echo "Re-run and provide the full URL with that path, e.g.:"
    echo "  wss://${HOST}:${PORT}/your-custom-path"
    exit 1
  fi
  GIVEN_PATH="$FOUND_PATH"
fi

TARGET_URL="wss://${HOST}:${PORT}${GIVEN_PATH}"
echo ""
echo "Using target: $TARGET_URL"

RPS="${IN_RPS:-500}"
DURATION="${IN_DURATION:-60}s"
CONCURRENCY="${IN_CONCURRENCY:-1000}"
HOLD_SECONDS="${IN_HOLD:-30}"
PAYLOAD_BYTES="${IN_PAYLOAD:-4096}"

echo ""
echo "Running: RPS=$RPS DURATION=$DURATION CONCURRENCY=$CONCURRENCY HOLD_SECONDS=$HOLD_SECONDS PAYLOAD_BYTES=$PAYLOAD_BYTES"
echo ""

k6 run -e URL="$TARGET_URL" -e RPS="$RPS" -e DURATION="$DURATION" \
       -e CONCURRENCY="$CONCURRENCY" -e HOLD_SECONDS="$HOLD_SECONDS" \
       -e PAYLOAD_BYTES="$PAYLOAD_BYTES" ./wstest.js
