// stresslab is the unified CLI for the transports that have no adequate
// off-the-shelf load tool: raw SSH connect-cycling, Xray-core proxy relay
// (VLESS/VMess/Trojan), HTTPUpgrade transport, and XHTTP (SplitHTTP)
// transport. HTTP/1.1, HTTP/2, WebSocket, and gRPC are handled by k6/ghz/
// Artillery — see runners/ for those.
package main

import (
	"fmt"
	"os"

	"stresslab/internal/config"
)

func usage() {
	fmt.Fprintln(os.Stderr, `stresslab - authorized infra stress-testing harness

Usage:
  stresslab ssh          -config <path/to/target.yaml>
  stresslab xray         -config <path/to/target.yaml> -upstream-test-url <url>
  stresslab httpupgrade  -config <path/to/target.yaml>
  stresslab xhttp        -config <path/to/target.yaml>

Every subcommand requires meta.authorized: true in the target config
and refuses to run otherwise. Credentials (SSH, Xray UUID/password) are
read from environment variables only — see docs/environment-variables.md.`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "ssh":
		runSSH(args)
	case "xray":
		runXray(args)
	case "httpupgrade":
		runHTTPUpgrade(args)
	case "xhttp":
		runXHTTP(args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", sub)
		usage()
		os.Exit(1)
	}
}

func loadOrExit(configPath string) *config.TargetConfig {
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: -config is required")
		os.Exit(1)
	}
	cfg, err := config.LoadTarget(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}


