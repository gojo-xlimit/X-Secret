#!/usr/bin/env bash
set -e

echo "=== stress-lab setup ==="

if ! command -v k6 >/dev/null 2>&1; then
  echo "[1/2] Installing k6..."
  sudo gpg -k
  sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
    --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
  echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
    | sudo tee /etc/apt/sources.list.d/k6.list
  sudo apt-get update -qq
  sudo apt-get install -y k6 tmux
else
  echo "[1/2] k6 already installed, skipping."
fi

echo "[2/2] Writing wstest.js..."
cat > wstest.js << 'EOJS'
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
echo "=== READY ==="
echo ""

read -rp "Target URL (e.g. wss://your-service.run.app/path): " TARGET_URL
read -rp "RPS [default 500]: " IN_RPS
read -rp "Duration in seconds [default 60]: " IN_DURATION
read -rp "Concurrency [default 1000]: " IN_CONCURRENCY
read -rp "Hold seconds per connection [default 30]: " IN_HOLD
read -rp "Payload bytes [default 4096]: " IN_PAYLOAD

RPS="${IN_RPS:-500}"
DURATION="${IN_DURATION:-60}s"
CONCURRENCY="${IN_CONCURRENCY:-1000}"
HOLD_SECONDS="${IN_HOLD:-30}"
PAYLOAD_BYTES="${IN_PAYLOAD:-4096}"

if [[ -z "$TARGET_URL" ]]; then
  echo "No URL given. Run manually later with:"
  echo "k6 run -e URL=wss://your-target/path -e RPS=500 -e DURATION=60s -e CONCURRENCY=1000 -e HOLD_SECONDS=30 -e PAYLOAD_BYTES=4096 wstest.js"
  exit 0
fi

echo ""
echo "Running: RPS=$RPS DURATION=$DURATION CONCURRENCY=$CONCURRENCY HOLD_SECONDS=$HOLD_SECONDS PAYLOAD_BYTES=$PAYLOAD_BYTES"
echo ""

k6 run -e URL="$TARGET_URL" -e RPS="$RPS" -e DURATION="$DURATION" \
       -e CONCURRENCY="$CONCURRENCY" -e HOLD_SECONDS="$HOLD_SECONDS" \
       -e PAYLOAD_BYTES="$PAYLOAD_BYTES" wstest.js
