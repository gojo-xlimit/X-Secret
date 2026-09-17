// k6 gRPC test — secondary to ghz. Use this when you need scripted,
// multi-step, or mixed-RPC scenarios (e.g. call A then call B with data
// from A's response) that ghz's single fixed-call model can't express.
// For plain single-RPC throughput/latency benchmarking, prefer ghz
// (transport/grpc/ghz/) — it has purpose-built load schedules and
// percentile reporting with far less scripting overhead.
//
// Run directly:
//   k6 run -e TARGET=target.example:443 -e VUS=50 -e DURATION=60s \
//          transport/grpc/k6/grpc_test.js

import grpc from 'k6/net/grpc';
import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const TARGET = __ENV.TARGET || 'target.example:443';
const VUS = parseInt(__ENV.VUS || '50');
const DURATION = __ENV.DURATION || '60s';
const RAMP = __ENV.RAMP || '10s';
const INSECURE = (__ENV.INSECURE || 'false') === 'true';

const P95_MS = parseInt(__ENV.P95_MS || '300');
const P99_MS = parseInt(__ENV.P99_MS || '700');
const ERROR_RATE_MAX = parseFloat(__ENV.ERROR_RATE_MAX || '0.01');

const rpcLatency = new Trend('grpc_rpc_latency_ms');
const rpcFail = new Rate('grpc_rpc_fail_rate');

const client = new grpc.Client();
client.load(['transport/grpc/ghz'], 'echo.proto');

export const options = {
  stages: [
    { duration: RAMP, target: VUS },
    { duration: DURATION, target: VUS },
  ],
  thresholds: {
    grpc_rpc_latency_ms: [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    grpc_rpc_fail_rate: [`rate<${ERROR_RATE_MAX}`],
  },
};

export default function () {
  client.connect(TARGET, { plaintext: INSECURE });

  const start = Date.now();
  const response = client.invoke('stresslab.EchoService/Echo', {
    payload: 'stresslab-payload',
  });
  const elapsed = Date.now() - start;

  const ok = check(response, {
    'status is OK': (r) => r && r.status === grpc.StatusOK,
  });

  rpcLatency.add(elapsed);
  rpcFail.add(!ok);

  client.close();
}
