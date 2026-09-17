// Package ssh implements a controlled SSH connect-cycle and channel
// throughput stress harness. It targets both OpenSSH and Dropbear —
// server identity only affects which host-key algorithms/ciphers are
// commonly negotiable, so this harness advertises a broad-compatible
// set and lets the server pick.
//
// Credentials are read only from environment variables — never hardcoded,
// never accepted as CLI flags (to avoid landing in shell history / ps).
package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"stresslab/internal/config"
	"stresslab/internal/metrics"
)

const (
	envUser = "STRESSLAB_SSH_USER"
	envPass = "STRESSLAB_SSH_PASS"
	envKey  = "STRESSLAB_SSH_KEY" // path to private key file, optional alt to password
)

func loadAuthMethod() ([]ssh.AuthMethod, string, error) {
	user := os.Getenv(envUser)
	if user == "" {
		return nil, "", fmt.Errorf("%s not set", envUser)
	}

	if keyPath := os.Getenv(envKey); keyPath != "" {
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, "", fmt.Errorf("read key %s: %w", keyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, "", fmt.Errorf("parse key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, user, nil
	}

	pass := os.Getenv(envPass)
	if pass == "" {
		return nil, "", fmt.Errorf("neither %s nor %s set", envKey, envPass)
	}
	return []ssh.AuthMethod{ssh.Password(pass)}, user, nil
}

// Run executes the connect-cycle + channel throughput test according to
// cfg.Load (virtual_users = concurrent connect cycles, duration_seconds,
// ramp_seconds) and cfg.Payload (size_bytes echoed per session via a
// simple exec channel: "cat" is used as a raw byte echo where available).
func Run(cfg *config.TargetConfig) (*metrics.Collector, error) {
	auths, user, err := loadAuthMethod()
	if err != nil {
		return nil, fmt.Errorf("ssh auth setup: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Target.Host, cfg.Target.Port)
	payloadSize := cfg.Payload.SizeBytes
	if payloadSize <= 0 {
		payloadSize = 512
	}
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte('A' + (i % 26))
	}

	collector := metrics.NewCollector()

	clientCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // lab context: authorized infra, host key pinning out of scope
		Timeout:         10 * time.Second,
		// Broad algorithm set for OpenSSH/Dropbear compatibility.
		Config: ssh.Config{
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
				"aes128-gcm@openssh.com", "chacha20-poly1305@openssh.com",
			},
		},
	}

	duration := time.Duration(cfg.Load.DurationSeconds) * time.Second
	ramp := time.Duration(cfg.Load.RampSeconds) * time.Second
	vus := cfg.Load.VirtualUsers

	stopAt := time.Now().Add(duration)
	var wg sync.WaitGroup

	spawnDelay := time.Duration(0)
	if vus > 0 && ramp > 0 {
		spawnDelay = ramp / time.Duration(vus)
	}

	for i := 0; i < vus; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for time.Now().Before(stopAt) {
				cycleOnce(addr, clientCfg, payload, collector)
			}
		}(i)
		if spawnDelay > 0 {
			time.Sleep(spawnDelay)
		}
	}

	wg.Wait()
	collector.Finish()
	return collector, nil
}

// cycleOnce performs: TCP dial -> SSH handshake+auth -> open session ->
// exec a byte-echo command -> write payload -> read back -> close.
// Every stage is timed cumulatively as one "request" latency sample,
// matching how a real bughost SSH-tunnel client cycles connections.
func cycleOnce(addr string, cfg *ssh.ClientConfig, payload []byte, c *metrics.Collector) {
	start := time.Now()

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer func() {
		if err := client.Close(); err != nil {
			c.RecordDisconnect()
		}
	}()

	session, err := client.NewSession()
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}

	if err := session.Start("cat"); err != nil {
		c.Record(metrics.Sample{LatencyMS: msSince(start), Success: false, Err: err})
		return
	}

	go func() {
		stdin.Write(payload)
		stdin.Close()
	}()

	buf := make([]byte, len(payload))
	n, _ := io.ReadFull(stdout, buf)
	c.AddBytesSent(int64(len(payload)))
	c.AddBytesRecv(int64(n))

	_ = session.Wait()

	success := n == len(payload)
	c.Record(metrics.Sample{LatencyMS: msSince(start), Success: success})
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}
