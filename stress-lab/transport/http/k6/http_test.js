// k6 HTTP/1.1 + HTTP/2 load test.
// k6 auto-upgrades to HTTP/2 when the server advertises it (ALPN h2) — no
// special scripting needed; see k6 docs "Using HTTP/2".
//
// All parameters are read from environment variables, populated by
// runners/run_http.sh from the shared target YAML — nothing is hardcoded.
//
// Run directly (bypassing the wrapper) with:
//   k6 run -e TARGET_URL=https://target.example/ -e VUS=100 -e DURATION=60s \
//          -e PAYLOAD_SIZE=1024 -e P95_MS=500 -e P99_MS=1000 -e ERROR_RATE_MAX=0.02 \
//          transport/http/k6/http_test.js

import http from 'k6/http';
import { check } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';

const TARGET_URL = __ENV.TARGET_URL || 'https://target.example/';
const VUS = parseInt(__ENV.VUS || '50');
const DURATION = __ENV.DURATION || '60s';
const RAMP = __ENV.RAMP || '10s';
const PAYLOAD_SIZE = parseInt(__ENV.PAYLOAD_SIZE || '1024');
const RATE_PER_SECOND = parseInt(__ENV.RATE_PER_SECOND || '0');
const INSECURE_SKIP_VERIFY = (__ENV.INSECURE_SKIP_VERIFY || 'false') === 'true';

const P95_MS = parseInt(__ENV.P95_MS || '500');
const P99_MS = parseInt(__ENV.P99_MS || '1000');
const ERROR_RATE_MAX = parseFloat(__ENV.ERROR_RATE_MAX || '0.02');

const handshakeFail = new Rate('handshake_fail_rate');
const bytesTransferred = new Counter('app_bytes_transferred');
const httpVersionUsed = new Trend('http_version_used');

const payload = 'x'.repeat(PAYLOAD_SIZE);

export const options = RATE_PER_SECOND > 0
  ? {
      // Fixed-arrival-rate mode: sustained req/s independent of response time,
      // matching load.rate_per_second in the shared schema.
      scenarios: {
        constant_rate: {
          executor: 'constant-arrival-rate',
          rate: RATE_PER_SECOND,
          timeUnit: '1s',
          duration: DURATION,
          preAllocatedVUs: VUS,
          maxVUs: VUS * 2,
        },
      },
      thresholds: {
        http_req_duration: [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
        http_req_failed: [`rate<${ERROR_RATE_MAX}`],
      },
    }
  : {
      // VU-driven mode: fixed concurrency, each VU loops as fast as it can.
      stages: [
        { duration: RAMP, target: VUS },
        { duration: DURATION, target: VUS },
      ],
      thresholds: {
        http_req_duration: [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
        http_req_failed: [`rate<${ERROR_RATE_MAX}`],
      },
      insecureSkipTLSVerify: INSECURE_SKIP_VERIFY,
    };

export default function () {
  const res = http.post(TARGET_URL, payload, {
    headers: { 'Content-Type': 'application/octet-stream' },
    tags: { name: 'stresslab_http' },
  });

  const ok = check(res, {
    'status is 2xx/3xx': (r) => r.status >= 200 && r.status < 400,
  });

  handshakeFail.add(!ok);
  bytesTransferred.add(res.body ? res.body.length : 0);
  httpVersionUsed.add(res.proto === 'HTTP/2.0' ? 2 : 1);
}

export function handleSummary(data) {
  return {
    stdout: JSON.stringify(data, null, 2),
    'result.json': JSON.stringify(data, null, 2),
  };
}
