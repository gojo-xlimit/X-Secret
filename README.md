# stress-lab

One-input, auto-detecting stress test for your own Cloud Run / VLESS / WebSocket infra.

## Usage

    curl -sL https://raw.githubusercontent.com/gojo-xlimit/X-Secret/main/run.sh -o run.sh && bash run.sh
    
Paste your target (any format: example.com, example.com/path,
https://host:port/path). It parses the URL, probes what protocol is
actually there (plain web, WebSocket upgrade, raw TCP), reports what it
found, then runs a load test automatically.
