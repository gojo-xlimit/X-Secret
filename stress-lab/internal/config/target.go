// Package config loads the shared target.schema.yaml files used across
// every runner in stress-lab. This is the single decode path — k6/ghz/
// artillery consume the same YAML via runners/*.sh (yq-based translation),
// but the Go harnesses (ssh/xray/httpupgrade/xhttp) decode it natively here.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Meta struct {
	Name       string `yaml:"name"`
	Owner      string `yaml:"owner"`
	Authorized bool   `yaml:"authorized"`
	Notes      string `yaml:"notes"`
}

type Target struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	Scheme             string `yaml:"scheme"`
	SNI                string `yaml:"sni"`
	Path               string `yaml:"path"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type Transport struct {
	Kind         string `yaml:"kind"`
	XrayProtocol string `yaml:"xray_protocol"`
	SSHServer    string `yaml:"ssh_server"`
}

type Load struct {
	VirtualUsers    int `yaml:"virtual_users"`
	RatePerSecond   int `yaml:"rate_per_second"`
	DurationSeconds int `yaml:"duration_seconds"`
	RampSeconds     int `yaml:"ramp_seconds"`
}

type Payload struct {
	SizeBytes int    `yaml:"size_bytes"`
	File      string `yaml:"file"`
}

type Thresholds struct {
	P95LatencyMS           int     `yaml:"p95_latency_ms"`
	P99LatencyMS           int     `yaml:"p99_latency_ms"`
	ErrorRateMax           float64 `yaml:"error_rate_max"`
	MinSuccessHandshakeRate float64 `yaml:"min_success_handshake_rate"`
}

type Output struct {
	Dir     string   `yaml:"dir"`
	Formats []string `yaml:"formats"`
}

type TargetConfig struct {
	Meta       Meta       `yaml:"meta"`
	Target     Target     `yaml:"target"`
	Transport  Transport  `yaml:"transport"`
	Load       Load       `yaml:"load"`
	Payload    Payload    `yaml:"payload"`
	Thresholds Thresholds `yaml:"thresholds"`
	Output     Output     `yaml:"output"`
}

// Load reads and validates a target YAML file. It refuses to return a usable
// config if meta.authorized is not explicitly true — every harness must
// call this before dialing anything.
func Load(path string) (*TargetConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg TargetConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config %s: %w", path, err)
	}

	return &cfg, nil
}

func validate(cfg *TargetConfig) error {
	if !cfg.Meta.Authorized {
		return fmt.Errorf("meta.authorized must be true — refusing to run against unauthorized target")
	}
	if cfg.Target.Host == "" || cfg.Target.Host == "target.example" {
		return fmt.Errorf("target.host is unset or still the placeholder — set it to infra you own/control")
	}
	if cfg.Load.VirtualUsers <= 0 {
		return fmt.Errorf("load.virtual_users must be > 0")
	}
	if cfg.Load.DurationSeconds <= 0 {
		return fmt.Errorf("load.duration_seconds must be > 0")
	}
	switch cfg.Transport.Kind {
	case "http1", "http2", "websocket", "httpupgrade", "xhttp", "grpc", "xray", "ssh":
	default:
		return fmt.Errorf("transport.kind %q is not a recognized value", cfg.Transport.Kind)
	}
	if cfg.Transport.Kind == "xray" {
		switch cfg.Transport.XrayProtocol {
		case "vless", "vmess", "trojan":
		default:
			return fmt.Errorf("transport.xray_protocol must be vless|vmess|trojan when kind=xray")
		}
	}
	if cfg.Transport.Kind == "ssh" {
		switch cfg.Transport.SSHServer {
		case "openssh", "dropbear":
		default:
			return fmt.Errorf("transport.ssh_server must be openssh|dropbear when kind=ssh")
		}
	}
	return nil
}
