// Package report writes standardized result artifacts (JSON + human summary)
// and evaluates the run against thresholds declared in the target config.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"stresslab/internal/config"
	"stresslab/internal/metrics"
)

type Result struct {
	TestName    string           `json:"test_name"`
	Transport   string           `json:"transport"`
	Target      string           `json:"target"`
	Timestamp   string           `json:"timestamp"`
	Summary     metrics.Summary  `json:"summary"`
	ThresholdOK bool             `json:"threshold_pass"`
	Violations  []string         `json:"threshold_violations,omitempty"`
}

// Evaluate checks a metrics.Summary against the thresholds block of a
// TargetConfig and returns pass/fail plus a list of human-readable
// violation strings.
func Evaluate(cfg *config.TargetConfig, s metrics.Summary) (bool, []string) {
	var violations []string

	if cfg.Thresholds.P95LatencyMS > 0 && s.P95LatencyMS > float64(cfg.Thresholds.P95LatencyMS) {
		violations = append(violations, fmt.Sprintf(
			"p95 latency %.1fms exceeds threshold %dms", s.P95LatencyMS, cfg.Thresholds.P95LatencyMS))
	}
	if cfg.Thresholds.P99LatencyMS > 0 && s.P99LatencyMS > float64(cfg.Thresholds.P99LatencyMS) {
		violations = append(violations, fmt.Sprintf(
			"p99 latency %.1fms exceeds threshold %dms", s.P99LatencyMS, cfg.Thresholds.P99LatencyMS))
	}
	if cfg.Thresholds.ErrorRateMax > 0 && s.ErrorRate > cfg.Thresholds.ErrorRateMax {
		violations = append(violations, fmt.Sprintf(
			"error rate %.4f exceeds threshold %.4f", s.ErrorRate, cfg.Thresholds.ErrorRateMax))
	}
	if cfg.Thresholds.MinSuccessHandshakeRate > 0 && s.SuccessRate < cfg.Thresholds.MinSuccessHandshakeRate {
		violations = append(violations, fmt.Sprintf(
			"success rate %.4f below required %.4f", s.SuccessRate, cfg.Thresholds.MinSuccessHandshakeRate))
	}

	return len(violations) == 0, violations
}

// Write persists the result as JSON (always) and a plaintext summary
// (if "summary" is in output.formats) under output.dir/<name>/<timestamp>/.
func Write(cfg *config.TargetConfig, s metrics.Summary) (string, error) {
	pass, violations := Evaluate(cfg, s)

	res := Result{
		TestName:    cfg.Meta.Name,
		Transport:   cfg.Transport.Kind,
		Target:      fmt.Sprintf("%s:%d", cfg.Target.Host, cfg.Target.Port),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Summary:     s,
		ThresholdOK: pass,
		Violations:  violations,
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(cfg.Output.Dir, cfg.Meta.Name, stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create result dir: %w", err)
	}

	for _, fmtName := range cfg.Output.Formats {
		switch fmtName {
		case "json":
			if err := writeJSON(filepath.Join(dir, "result.json"), res); err != nil {
				return dir, err
			}
		case "summary":
			if err := writeSummary(filepath.Join(dir, "summary.txt"), res); err != nil {
				return dir, err
			}
		case "csv":
			if err := writeCSVRow(filepath.Join(cfg.Output.Dir, "history.csv"), res); err != nil {
				return dir, err
			}
		}
	}

	return dir, nil
}

func writeJSON(path string, res Result) error {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeSummary(path string, res Result) error {
	status := "PASS"
	if !res.ThresholdOK {
		status = "FAIL"
	}
	out := fmt.Sprintf(`stress-lab result :: %s
transport   : %s
target      : %s
timestamp   : %s
status      : %s

requests    : %d (success=%d fail=%d disconnects=%d)
success_rate: %.4f
error_rate  : %.4f

latency p50 : %.1f ms
latency p95 : %.1f ms
latency p99 : %.1f ms
latency min : %.1f ms
latency max : %.1f ms
latency avg : %.1f ms

duration    : %.1f s
throughput  : %.2f req/s
throughput  : %.3f Mbps
bytes sent  : %d
bytes recv  : %d
`,
		res.TestName, res.Transport, res.Target, res.Timestamp, status,
		res.Summary.TotalRequests, res.Summary.Successes, res.Summary.Failures, res.Summary.Disconnects,
		res.Summary.SuccessRate, res.Summary.ErrorRate,
		res.Summary.P50LatencyMS, res.Summary.P95LatencyMS, res.Summary.P99LatencyMS,
		res.Summary.MinLatencyMS, res.Summary.MaxLatencyMS, res.Summary.AvgLatencyMS,
		res.Summary.DurationSeconds, res.Summary.Throughput, res.Summary.ThroughputMbps,
		res.Summary.BytesSent, res.Summary.BytesRecv,
	)
	if !res.ThresholdOK {
		out += "\nthreshold violations:\n"
		for _, v := range res.Violations {
			out += "  - " + v + "\n"
		}
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func writeCSVRow(path string, res Result) error {
	header := "timestamp,test_name,transport,target,status,total_requests,success_rate,error_rate,p50_ms,p95_ms,p99_ms,throughput_rps,throughput_mbps\n"
	needsHeader := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		needsHeader = true
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if needsHeader {
		if _, err := f.WriteString(header); err != nil {
			return err
		}
	}

	status := "PASS"
	if !res.ThresholdOK {
		status = "FAIL"
	}
	row := fmt.Sprintf("%s,%s,%s,%s,%s,%d,%.4f,%.4f,%.1f,%.1f,%.1f,%.2f,%.3f\n",
		res.Timestamp, res.TestName, res.Transport, res.Target, status,
		res.Summary.TotalRequests, res.Summary.SuccessRate, res.Summary.ErrorRate,
		res.Summary.P50LatencyMS, res.Summary.P95LatencyMS, res.Summary.P99LatencyMS,
		res.Summary.Throughput, res.Summary.ThroughputMbps,
	)
	_, err = f.WriteString(row)
	return err
}
