---
title: CLI
description: CLI usage of Xray Checker
---

### Basic Command Line Usage

The CLI interface provides complete control over Xray Checker's functionality through command-line arguments.

### Installation

Download the latest binary from releases:

```bash
# For Linux amd64
curl -sL -o - $(curl -s https://api.github.com/repos/kutovoys/xray-checker/releases/latest | grep "browser_download_url.*linux-amd64.tar.gz" | cut -d'"' -f4) | tar -xz
chmod +x xray-checker

# For Linux arm64
curl -sL -o - $(curl -s https://api.github.com/repos/kutovoys/xray-checker/releases/latest | grep "browser_download_url.*linux-arm64.tar.gz" | cut -d'"' -f4) | tar -xz
chmod +x xray-checker

# For macOS (Intel)
curl -sL -o - $(curl -s https://api.github.com/repos/kutovoys/xray-checker/releases/latest | grep "browser_download_url.*darwin-amd64.tar.gz" | cut -d'"' -f4) | tar -xz
chmod +x xray-checker

# For macOS (Apple Silicon)
curl -sL -o - $(curl -s https://api.github.com/repos/kutovoys/xray-checker/releases/latest | grep "browser_download_url.*darwin-arm64.tar.gz" | cut -d'"' -f4) | tar -xz
chmod +x xray-checker
```

### Basic Usage

Minimum required configuration:

```bash
./xray-checker --subscription-url=https://your-subscription-url/sub
```

### Multiple Subscriptions

You can specify multiple subscription URLs by using the `--subscription-url` flag multiple times:

```bash
./xray-checker \
  --subscription-url=https://provider1.com/sub \
  --subscription-url=https://provider2.com/sub \
  --subscription-url=file:///path/to/local/config.json
```

All proxies from all subscriptions will be combined and monitored together.

### Full Configuration Example

```bash
./xray-checker \
  --subscription-url=https://your-subscription-url/sub \
  --subscription-update=true \
  --subscription-update-interval=300 \
  --subscription-json-format=false \
  --subscription-user-agent="" \
  --subscription-header="X-Token: abc" \
  --proxy-check-interval=300 \
  --proxy-check-concurrency=0 \
  --proxy-timeout=30 \
  --proxy-check-method=ip \
  --proxy-ip-check-url="https://api.ipify.org?format=text" \
  --proxy-status-check-url="http://cp.cloudflare.com/generate_204" \
  --proxy-download-url="https://proof.ovh.net/files/1Mb.dat" \
  --proxy-download-timeout=60 \
  --proxy-download-min-size=51200 \
  --proxy-resolve-domains=false \
  --simulate-latency=true \
  --xray-start-port=10000 \
  --xray-log-level=none \
  --xray-interface="" \
  --metrics-host=0.0.0.0 \
  --metrics-port=2112 \
  --metrics-protected=true \
  --metrics-username=custom_user \
  --metrics-password=custom_pass \
  --metrics-instance=node-1 \
  --metrics-push-url="https://push.example.com" \
  --metrics-base-path="/xray/monitor" \
  --web-show-details=false \
  --web-public=false \
  --web-trusted-external-auth=false \
  --web-public-show-protocol=false \
  --web-custom-assets-path="" \
  --log-level=info \
  --run-once=false
```

### Common CLI Operations

Check version:

```bash
./xray-checker --version
```

Run single check cycle:

```bash
./xray-checker --subscription-url=https://your-sub-url --run-once
```

Enable metrics authentication:

```bash
./xray-checker \
  --subscription-url=https://your-sub-url \
  --metrics-protected=true \
  --metrics-username=user \
  --metrics-password=pass
```

Change ports:

```bash
./xray-checker \
  --subscription-url=https://your-sub-url \
  --metrics-host=127.0.0.1 \
  --metrics-port=3000 \
  --xray-start-port=20000
```
