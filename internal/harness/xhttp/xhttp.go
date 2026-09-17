// Package xhttp stresses the Xray-core "XHTTP" (a.k.a. SplitHTTP) transport
// in its most CDN-compatible mode: packet-up (sequenced POST for upload) +
// stream-down (long-lived GET for download), each request/stream carrying
// a session ID path segment, per XTLS/Xray-core's splithttp transport spec.
//
// Verified against upstream mechanics (splithttp/dialer.go, hub.go):
//   - Client generates a session ID (opaque string) per logical connection.
//   - Upload: one or more sequenced POST requests to <path>/<sessionID>/<seq>
//   - Download: single long-lived GET to <path>/<sessionID>
// This harness measures per-cycle: POST round-trip latency (packet-up) and
// GET-stream first-byte + full-drain latency (stream-down), independently,
// since CDN behavior often differs between the two paths.
package xhttp

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"stresslab/internal/config"
	"stresslab/internal/metrics"
)

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Run drives concurrent packet-up + stream-down cycles for cfg.Load.Duration,
// ramping up cfg.Load.VirtualUsers workers over cfg.Load.RampSeconds.
func Run(cfg *config.TargetConfig) (*metrics.Collector, error) {
	scheme := "https"
	if cfg.Target.Scheme == "http" {
		scheme = "http"
	}
	base := fmt.Sprintf("%s://%s:%d%s", scheme, cfg.Target.Host, cfg.Target.Port, normalizePath(cfg.Target.Path))

	payloadSize := cfg.Payload.SizeBytes
	if payloadSize <= 0 {
		payloadSize = 1024
	}
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte('x')
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         serverName(cfg),
				InsecureSkipVerify: cfg.Target.InsecureSkipVerify,
				MinVersion:         tls.VersionTLS12,
			},
			MaxIdleConnsPerHost: 200,
		},
	}

	collector := metrics.NewCollector()

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
				cycleOnce(client, base, payload, collector)
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

// cycleOnce: one packet-up POST followed by one stream-down GET against a
// freshly generated session ID, mirroring a single logical XHTTP connection.
func cycleOnce(client *http.Client, base string, payload []byte, c *metrics.Collector) {
	start := time.Now()
	sessionID := newSessionID()

	// --- packet-up: sequenced POST, sequence 0 ---
	upURL := fmt.Sprintf("%s/%s/0", base, sessionID)
	req, err := http.NewRequest(http.MethodPost, upURL, bytes.NewReader(payload))
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	c.AddBytesSent(int64(len(payload)))

	upOK := resp.StatusCode >= 200 && resp.StatusCode < 300

	// --- stream-down: long-lived GET on the same session ---
	downURL := fmt.Sprintf("%s/%s", base, sessionID)
	downResp, err := client.Get(downURL)
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	defer downResp.Body.Close()

	n, _ := io.Copy(io.Discard, downResp.Body)
	c.AddBytesRecv(n)

	downOK := downResp.StatusCode >= 200 && downResp.StatusCode < 300

	c.Record(metrics.Sample{LatencyMS: msSince(start), Success: upOK && downOK})
}

func normalizePath(p string) string {
	if p == "" {
		return "/xhttp"
	}
	if p[0] != '/' {
		return "/" + p
	}
	return p
}

func serverName(cfg *config.TargetConfig) string {
	if cfg.Target.SNI != "" {
		return cfg.Target.SNI
	}
	return cfg.Target.Host
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}
