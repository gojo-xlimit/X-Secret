// Package metrics provides a thread-safe sample collector shared by every
// Go harness (ssh, xray, httpupgrade, xhttp). It intentionally avoids a
// full HdrHistogram dependency — at the sample counts these lab tests run
// (tens of thousands, not billions), a sorted-slice percentile is accurate
// enough and keeps the dependency graph small for Termux builds.
package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Sample struct {
	LatencyMS float64
	Success   bool
	Err       error
}

type Collector struct {
	mu          sync.Mutex
	latencies   []float64
	successes   int64
	failures    int64
	disconnects int64
	bytesSent   int64
	bytesRecv   int64
	startedAt   time.Time
	endedAt     time.Time
}

func NewCollector() *Collector {
	return &Collector{
		latencies: make([]float64, 0, 4096),
		startedAt: time.Now(),
	}
}

func (c *Collector) Record(s Sample) {
	c.mu.Lock()
	c.latencies = append(c.latencies, s.LatencyMS)
	c.mu.Unlock()

	if s.Success {
		atomic.AddInt64(&c.successes, 1)
	} else {
		atomic.AddInt64(&c.failures, 1)
	}
}

func (c *Collector) RecordDisconnect() {
	atomic.AddInt64(&c.disconnects, 1)
}

func (c *Collector) AddBytesSent(n int64) {
	atomic.AddInt64(&c.bytesSent, n)
}

func (c *Collector) AddBytesRecv(n int64) {
	atomic.AddInt64(&c.bytesRecv, n)
}

func (c *Collector) Finish() {
	c.endedAt = time.Now()
}

type Summary struct {
	TotalRequests    int64   `json:"total_requests"`
	Successes        int64   `json:"successes"`
	Failures         int64   `json:"failures"`
	Disconnects      int64   `json:"disconnects"`
	ErrorRate        float64 `json:"error_rate"`
	SuccessRate      float64 `json:"success_rate"`
	P50LatencyMS     float64 `json:"p50_latency_ms"`
	P95LatencyMS     float64 `json:"p95_latency_ms"`
	P99LatencyMS     float64 `json:"p99_latency_ms"`
	MinLatencyMS     float64 `json:"min_latency_ms"`
	MaxLatencyMS     float64 `json:"max_latency_ms"`
	AvgLatencyMS     float64 `json:"avg_latency_ms"`
	DurationSeconds  float64 `json:"duration_seconds"`
	Throughput       float64 `json:"throughput_req_per_sec"`
	BytesSent        int64   `json:"bytes_sent"`
	BytesRecv        int64   `json:"bytes_recv"`
	ThroughputMbps   float64 `json:"throughput_mbps"`
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (c *Collector) Summarize() Summary {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(c.latencies)
	sorted := make([]float64, n)
	copy(sorted, c.latencies)
	sort.Float64s(sorted)

	var sum, min, max float64
	if n > 0 {
		min = sorted[0]
		max = sorted[n-1]
		for _, v := range sorted {
			sum += v
		}
	}

	total := c.successes + c.failures
	dur := c.endedAt.Sub(c.startedAt).Seconds()
	if dur <= 0 {
		dur = time.Since(c.startedAt).Seconds()
	}

	var errRate, successRate, throughput, mbps float64
	if total > 0 {
		errRate = float64(c.failures) / float64(total)
		successRate = float64(c.successes) / float64(total)
	}
	if dur > 0 {
		throughput = float64(total) / dur
		mbps = (float64(c.bytesSent+c.bytesRecv) * 8 / 1_000_000) / dur
	}
	var avg float64
	if n > 0 {
		avg = sum / float64(n)
	}

	return Summary{
		TotalRequests:   total,
		Successes:       c.successes,
		Failures:        c.failures,
		Disconnects:     c.disconnects,
		ErrorRate:       errRate,
		SuccessRate:     successRate,
		P50LatencyMS:    percentile(sorted, 0.50),
		P95LatencyMS:    percentile(sorted, 0.95),
		P99LatencyMS:    percentile(sorted, 0.99),
		MinLatencyMS:    min,
		MaxLatencyMS:    max,
		AvgLatencyMS:    avg,
		DurationSeconds: dur,
		Throughput:      throughput,
		BytesSent:       c.bytesSent,
		BytesRecv:       c.bytesRecv,
		ThroughputMbps:  mbps,
	}
}
