# Setting Up Echo Targets

The `httpupgrade` and `xhttp` harnesses measure round-trip throughput by
sending a payload and expecting an equal-length echo back. Without an echo
implementation on the server side, the harness still measures handshake
success/latency correctly, but throughput/payload-integrity numbers are
meaningless (you'd just be timing however the real backend responds, which
may not echo at all).

This doc describes what to deploy behind your Cloud Run / Xray-core lab
instance to get meaningful numbers for each transport.

## HTTPUpgrade echo

After the `Upgrade: websocket` handshake completes, the harness writes the
raw payload bytes to the now-upgraded TCP+TLS stream and reads back the
same number of bytes. A minimal echo backend just needs to:

1. Accept the HTTP/1.1 request, respond `101 Switching Protocols` (or your
   transport's expected response, matching what Xray-core's httpupgrade
   inbound produces).
2. After the response, copy bytes read <-> bytes written on the same
   connection (`io.Copy(conn, conn)` in Go, or equivalent).

If you're testing against a real Xray-core `httpupgrade` inbound with a
VLESS/VMess/Trojan outbound behind it (the realistic bughost topology),
the "echo" is effectively whatever your upstream target application does —
point the outbound at a TCP echo service (e.g. `xinetd` echo on port 7,
or a 5-line Go/Python echo server) instead of expecting Xray itself to echo.

## XHTTP echo

The harness performs a `packet-up` POST followed by a `stream-down` GET on
the same generated session ID. For a standalone protocol-level test (no
real Xray-core in the loop), your echo backend needs to:

1. Accept `POST <path>/<sessionID>/<seq>` — read the body, store/discard it,
   respond `200`.
2. Accept `GET <path>/<sessionID>` — respond with a body (ideally echoing
   what was POSTed for that session ID, if you want to validate payload
   integrity end-to-end rather than just connection viability).

If testing against a real Xray-core XHTTP inbound, same note as above:
point the outbound at a TCP echo service.

## Xray harness (VLESS/VMess/Trojan) upstream test URL

`stresslab xray -upstream-test-url <url>` fetches that URL through the
local SOCKS proxy that the spawned xray-core client creates, which is
relayed through your real VLESS/VMess/Trojan server. This exercises the
full path: local xray client -> your Cloud Run/VPS Xray inbound -> its
configured outbound -> upstream.

Point `-upstream-test-url` at anything reachable from your server's egress
that you're authorized to hit repeatedly — a Cloud Run health-check
endpoint you control is the safest choice, since hammering a third-party
URL through your own proxy still generates real traffic against that
third party.

## Minimal Go TCP echo server (reference)

```go
package main

import (
	"io"
	"log"
	"net"
)

func main() {
	ln, err := net.Listen("tcp", ":7000")
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			io.Copy(c, c)
		}(conn)
	}
}
```

Deploy this as the upstream behind your Xray-core inbound/outbound chain,
or adapt it into an HTTP handler for the HTTPUpgrade/XHTTP cases above.
