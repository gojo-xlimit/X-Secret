// k6 native WebSocket load test. Uses k6's WS lifecycle (connect -> message
// loop -> close) with per-connection latency and disconnect tracking.
//
// Run directly:
//   k6 run -e TARGET_URL=wss://target.example/ws -e VUS=200 -e DURATION=120s \
//          -e PAYLOAD_SIZE=512 -e HOLD_SECONDS=5 \
//          transport/websocket/k6/ws_test.js

import { check } from 'k6';
import ws from 'k6/ws';
import { Trend, Rate, Counter } from 'k6/metrics';

const TARGET_URL = __ENV.TARGET_URL || 'wss://target.example/ws';
const VUS = parseInt(__ENV.VUS || '50');
const DURATION = __ENV.DURATION || '60s';
const RAMP = __ENV.RAMP || '10s';
const PAYLOAD_SIZE = parseInt(__ENV.PAYLOAD_SIZE || '512');
const HOLD_SECONDS = parseInt(__ENV.HOLD_SECONDS || '5');

const P95_MS = parseInt(__ENV.P95_MS || '600');
const P99_MS = parseInt(__ENV.P99_MS || '1200');
const ERROR_RATE_MAX = parseFloat(__ENV.ERROR_RATE_MAX || '0.02');

const connectFail = new Rate('ws_connect_fail_rate');
const messageLatency = new Trend('ws_message_latency_ms');
const disconnects = new Counter('ws_disconnects');

const payload = 'x'.repeat(PAYLOAD_SIZE);

export const options = {
  stages: [
    { duration: RAMP, target: VUS },
    { duration: DURATION, target: VUS },
  ],
  thresholds: {
    ws_message_latency_ms: [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    ws_connect_fail_rate: [`rate<${ERROR_RATE_MAX}`],
  },
};

export default function () {
  const start = Date.now();
  let gotEcho = false;

  const res = ws.connect(TARGET_URL, {}, function (socket) {
    socket.on('open', () => {
      socket.send(payload);
    });

    socket.on('message', (data) => {
      if (!gotEcho) {
        messageLatency.add(Date.now() - start);
        gotEcho = true;
      }
    });

    socket.on('close', () => {
      disconnects.add(1);
    });

    socket.on('error', () => {
      connectFail.add(1);
    });

    socket.setTimeout(() => {
      socket.close();
    }, HOLD_SECONDS * 1000);
  });

  const connected = check(res, { 'connected successfully': (r) => r && r.status === 101 });
  connectFail.add(!connected);
}
