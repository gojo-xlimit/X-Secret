// Package httpupgrade stresses the HTTP Upgrade transport mechanism used by
// Xray-core and Sing-box's "httpupgrade" transport. This is deliberately
// NOT the WebSocket upgrade path — it's a raw HTTP/1.1 Upgrade: as used by
// HU transport, which most load tools (k6/Artillery) don't model correctly
// because they special-case the WS handshake specifically. Hence a raw
// TCP+TLS harness that speaks the exact request line the transport expects.
package httpupgrade

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"stresslab/internal/config"
	"stresslab/internal/metrics"
)

// Run performs repeated HTTP Upgrade handshake cycles: TLS dial (with SNI) ->
// send "Upgrade: websocket"-style request line but non-101-checked generically
// as HU transport expects -> read response -> send payload -> read echo ->
// close. Concurrency and duration follow cfg.Load.
func Run(cfg *config.TargetConfig) (*metrics.Collector, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Target.Host, cfg.Target.Port)
	sni := cfg.Target.SNI
	if sni == "" {
		sni = cfg.Target.Host
	}
	path := cfg.Target.Path
	if path == "" {
		path = "/"
	}

	payloadSize := cfg.Payload.SizeBytes
	if payloadSize <= 0 {
		payloadSize = 1024
	}
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte('a' + (i % 26))
	}

	collector := metrics.NewCollector()

	tlsCfg := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: cfg.Target.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	duration := time.Duration(cfg.Load.DurationSeconds) * time.Second
	ramp := time.Duration(cfg.Load.RampSeconds) * time.Second
	vus := cfg.Load.VirtualUsers
	stopAt := time.Now().Add(duration)

	spawnDelay := time.Duration(0)
	if vus > 0 && ramp > 0 {
		spawnDelay = ramp / time.Duration(vus)
	}

	var wg sync.WaitGroup
	for i := 0; i < vus; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stopAt) {
				cycleOnce(addr, cfg.Target.Host, path, tlsCfg, payload, collector)
			}
		}()
		if spawnDelay > 0 {
			time.Sleep(spawnDelay)
		}
	}

	wg.Wait()
	collector.Finish()
	return collector, nil
}

func cycleOnce(addr, hostHeader, path string, tlsCfg *tls.Config, payload []byte, c *metrics.Collector) {
	start := time.Now()

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	rawConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	conn := tls.Client(rawConn, tlsCfg)
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		conn.Close()
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	if err := conn.Handshake(); err != nil {
		conn.Close()
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	defer conn.Close()

	// HTTPUpgrade transport request line (RFC 7230 Upgrade mechanics, as
	// implemented by Xray-core's "httpupgrade" transport):
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + hostHeader + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := conn.Write([]byte(req)); err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	c.AddBytesSent(int64(len(req)))

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	// Drain remaining headers until blank line.
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}

	success := len(statusLine) > 9 && (statusLine[9:12] == "101" || statusLine[9:12] == "200")
	if !success {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false,
			Err: fmt.Errorf("unexpected status: %s", statusLine)})
		return
	}

	// Post-upgrade, send payload as raw bytes over the now-upgraded stream
	// and expect an echo of equal length back (target must implement echo
	// for this to validate throughput — see docs/setting-up-echo-targets.md).
	if _, err := conn.Write(payload); err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	c.AddBytesSent(int64(len(payload)))

	buf := make([]byte, len(payload))
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			break
		}
		total += n
	}
	c.AddBytesRecv(int64(total))

	c.Record(metrics.Sample{LatencyMS: msSince(start), Success: total == len(payload)})
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}
