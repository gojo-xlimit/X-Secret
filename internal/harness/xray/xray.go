// Package xray drives load against VLESS/VMess/Trojan endpoints by
// spawning the real xray-core binary as a subprocess with a generated
// client config (SOCKS inbound on localhost), then hammering that local
// proxy with HTTP requests to an authorized upstream test endpoint.
//
// Rationale (per user's own stack conventions): xray-core is NOT vendored
// as a Go library here — that would mean tracking its entire module graph
// and internal APIs, which are not designed for external embedding. The
// subprocess+local-proxy approach is what every real Xray client does
// operationally, so it measures the same connect/relay path a real user
// session would.
//
// Requires: `xray` binary in PATH (from XTLS/Xray-core releases).
package xray

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"stresslab/internal/config"
	"stresslab/internal/metrics"
)

const envUUID = "STRESSLAB_XRAY_UUID"       // VLESS/VMess UUID
const envPassword = "STRESSLAB_XRAY_PASSWORD" // Trojan password

type xrayProcess struct {
	cmd       *exec.Cmd
	socksPort int
	workDir   string
}

// buildClientConfig writes a minimal xray-core client JSON config: one
// SOCKS inbound on 127.0.0.1:<socksPort>, one outbound matching the
// configured protocol/transport, streamSettings honoring TLS SNI + the
// requested transport network (tcp here; ws/httpupgrade/xhttp handled by
// their own dedicated harnesses upstream of this package when combined).
func buildClientConfig(cfg *config.TargetConfig, socksPort int) (map[string]interface{}, error) {
	var user map[string]interface{}
	protocol := cfg.Transport.XrayProtocol

	switch protocol {
	case "vless":
		uuid := os.Getenv(envUUID)
		if uuid == "" {
			return nil, fmt.Errorf("%s not set", envUUID)
		}
		user = map[string]interface{}{
			"id":         uuid,
			"encryption": "none",
		}
	case "vmess":
		uuid := os.Getenv(envUUID)
		if uuid == "" {
			return nil, fmt.Errorf("%s not set", envUUID)
		}
		user = map[string]interface{}{
			"id":      uuid,
			"alterId": 0,
		}
	case "trojan":
		pass := os.Getenv(envPassword)
		if pass == "" {
			return nil, fmt.Errorf("%s not set", envPassword)
		}
		user = map[string]interface{}{
			"password": pass,
		}
	default:
		return nil, fmt.Errorf("unsupported xray protocol %q", protocol)
	}

	var settings map[string]interface{}
	if protocol == "trojan" {
		settings = map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{
					"address":  cfg.Target.Host,
					"port":     cfg.Target.Port,
					"password": user["password"],
				},
			},
		}
	} else {
		vnext := map[string]interface{}{
			"address": cfg.Target.Host,
			"port":    cfg.Target.Port,
			"users":   []interface{}{user},
		}
		settings = map[string]interface{}{
			"vnext": []interface{}{vnext},
		}
	}

	sni := cfg.Target.SNI
	if sni == "" {
		sni = cfg.Target.Host
	}

	streamSettings := map[string]interface{}{
		"network":  "ws",
		"security": "tls",
		"tlsSettings": map[string]interface{}{
			"serverName":    sni,
			"allowInsecure": cfg.Target.InsecureSkipVerify,
		},
		"wsSettings": map[string]interface{}{
			"path": normalizePath(cfg.Target.Path),
		},
	}

	return map[string]interface{}{
		"log": map[string]interface{}{"loglevel": "warning"},
		"inbounds": []interface{}{
			map[string]interface{}{
				"listen":   "127.0.0.1",
				"port":     socksPort,
				"protocol": "socks",
				"settings": map[string]interface{}{
					"udp": false,
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{
				"protocol":      protocol,
				"settings":      settings,
				"streamSettings": streamSettings,
			},
		},
	}, nil
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	return p
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func startXray(cfg *config.TargetConfig) (*xrayProcess, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("allocate local socks port: %w", err)
	}

	clientCfg, err := buildClientConfig(cfg, port)
	if err != nil {
		return nil, err
	}

	rnd := make([]byte, 4)
	rand.Read(rnd)
	workDir := filepath.Join(os.TempDir(), "stresslab-xray-"+hex.EncodeToString(rnd))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}

	cfgPath := filepath.Join(workDir, "config.json")
	b, err := json.MarshalIndent(clientCfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("xray", "run", "-config", cfgPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start xray subprocess (is `xray` in PATH?): %w", err)
	}

	// Wait for local SOCKS port to come up.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	return &xrayProcess{cmd: cmd, socksPort: port, workDir: workDir}, nil
}

func (p *xrayProcess) Stop() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
	os.RemoveAll(p.workDir)
}

// Run spawns xray-core, then drives HTTP requests through its local SOCKS
// proxy toward an authorized upstream test endpoint reachable via the
// tunnel (in a lab this is typically the same Cloud Run service, verifying
// the full proxy relay path end to end).
func Run(cfg *config.TargetConfig, upstreamTestURL string) (*metrics.Collector, error) {
	proc, err := startXray(cfg)
	if err != nil {
		return nil, err
	}
	defer proc.Stop()

	socksAddr := fmt.Sprintf("127.0.0.1:%d", proc.socksPort)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialSocks5(ctx, socksAddr, addr)
		},
		MaxIdleConnsPerHost: 200,
	}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}

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
				requestOnce(client, upstreamTestURL, collector)
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

func requestOnce(client *http.Client, url string, c *metrics.Collector) {
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	defer resp.Body.Close()
	n, _ := io.Copy(io.Discard, resp.Body)
	c.AddBytesRecv(n)
	c.Record(metrics.Sample{LatencyMS: msSince(start), Success: resp.StatusCode >= 200 && resp.StatusCode < 300})
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}
