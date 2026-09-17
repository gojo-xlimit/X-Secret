package main

import (
	"flag"
	"fmt"
	"os"

	"stresslab/internal/config"
	"stresslab/internal/harness/httpupgrade"
	"stresslab/internal/harness/ssh"
	"stresslab/internal/harness/xhttp"
	"stresslab/internal/harness/xray"
	"stresslab/internal/metrics"
	"stresslab/internal/report"
)

func runSSH(args []string) {
	fs := flag.NewFlagSet("ssh", flag.ExitOnError)
	configPath := fs.String("config", "", "path to target.yaml")
	fs.Parse(args)

	cfg := loadOrExit(*configPath)
	if cfg.Transport.Kind != "ssh" {
		fmt.Fprintf(os.Stderr, "error: config transport.kind is %q, expected \"ssh\"\n", cfg.Transport.Kind)
		os.Exit(1)
	}

	fmt.Printf("[stresslab] ssh (%s) -> %s:%d | vus=%d duration=%ds\n",
		cfg.Transport.SSHServer, cfg.Target.Host, cfg.Target.Port, cfg.Load.VirtualUsers, cfg.Load.DurationSeconds)

	collector, err := ssh.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}

	finish(cfg, collector.Summarize())
}

func runXray(args []string) {
	fs := flag.NewFlagSet("xray", flag.ExitOnError)
	configPath := fs.String("config", "", "path to target.yaml")
	upstreamURL := fs.String("upstream-test-url", "", "URL to fetch through the xray relay to measure end-to-end performance")
	fs.Parse(args)

	cfg := loadOrExit(*configPath)
	if cfg.Transport.Kind != "xray" {
		fmt.Fprintf(os.Stderr, "error: config transport.kind is %q, expected \"xray\"\n", cfg.Transport.Kind)
		os.Exit(1)
	}
	if *upstreamURL == "" {
		fmt.Fprintln(os.Stderr, "error: -upstream-test-url is required (an HTTP endpoint reachable through the tunnel)")
		os.Exit(1)
	}

	fmt.Printf("[stresslab] xray (%s) -> %s:%d | vus=%d duration=%ds\n",
		cfg.Transport.XrayProtocol, cfg.Target.Host, cfg.Target.Port, cfg.Load.VirtualUsers, cfg.Load.DurationSeconds)

	collector, err := xray.Run(cfg, *upstreamURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}

	finish(cfg, collector.Summarize())
}

func runHTTPUpgrade(args []string) {
	fs := flag.NewFlagSet("httpupgrade", flag.ExitOnError)
	configPath := fs.String("config", "", "path to target.yaml")
	fs.Parse(args)

	cfg := loadOrExit(*configPath)
	if cfg.Transport.Kind != "httpupgrade" {
		fmt.Fprintf(os.Stderr, "error: config transport.kind is %q, expected \"httpupgrade\"\n", cfg.Transport.Kind)
		os.Exit(1)
	}

	fmt.Printf("[stresslab] httpupgrade -> %s:%d%s | vus=%d duration=%ds\n",
		cfg.Target.Host, cfg.Target.Port, cfg.Target.Path, cfg.Load.VirtualUsers, cfg.Load.DurationSeconds)

	collector, err := httpupgrade.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}

	finish(cfg, collector.Summarize())
}

func runXHTTP(args []string) {
	fs := flag.NewFlagSet("xhttp", flag.ExitOnError)
	configPath := fs.String("config", "", "path to target.yaml")
	fs.Parse(args)

	cfg := loadOrExit(*configPath)
	if cfg.Transport.Kind != "xhttp" {
		fmt.Fprintf(os.Stderr, "error: config transport.kind is %q, expected \"xhttp\"\n", cfg.Transport.Kind)
		os.Exit(1)
	}

	fmt.Printf("[stresslab] xhttp -> %s:%d%s | vus=%d duration=%ds\n",
		cfg.Target.Host, cfg.Target.Port, cfg.Target.Path, cfg.Load.VirtualUsers, cfg.Load.DurationSeconds)

	collector, err := xhttp.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}

	finish(cfg, collector.Summarize())
}

// finish writes result artifacts, prints a compact console summary, and
// exits non-zero if thresholds were violated (useful for CI-style gating).
func finish(cfg *config.TargetConfig, summary metrics.Summary) {
	dir, err := report.Write(cfg, summary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "report write error: %v\n", err)
		os.Exit(1)
	}

	pass, violations := report.Evaluate(cfg, summary)

	fmt.Printf(`
--- result: %s ---
requests    : %d (success=%d fail=%d disconnects=%d)
success_rate: %.4f  error_rate: %.4f
latency p50 : %.1fms  p95: %.1fms  p99: %.1fms
throughput  : %.2f req/s (%.3f Mbps)
written to  : %s
`,
		cfg.Meta.Name,
		summary.TotalRequests, summary.Successes, summary.Failures, summary.Disconnects,
		summary.SuccessRate, summary.ErrorRate,
		summary.P50LatencyMS, summary.P95LatencyMS, summary.P99LatencyMS,
		summary.Throughput, summary.ThroughputMbps,
		dir,
	)

	if !pass {
		fmt.Println("threshold violations:")
		for _, v := range violations {
			fmt.Println("  - " + v)
		}
		os.Exit(2)
	}
	fmt.Println("status: PASS")
}
