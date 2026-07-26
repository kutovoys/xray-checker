---
title: Environment Variables
description: Environment variables for Xray Checker
---

## Subscription

### SUBSCRIPTION_URL

- CLI: `--subscription-url`
- Required: Yes
- Default: None

URL, Base64 string or file path for proxy configuration. Supports multiple formats:

- HTTP/HTTPS URL with Base64 encoded content
- Direct Base64 encoded string
- Local file path with prefix `file://`
- Local folder path with prefix `folder://`

:::tip[Multiple Subscriptions]
You can specify multiple subscription sources:
- **CLI**: Use `--subscription-url` flag multiple times
- **Environment**: Separate URLs with commas: `SUBSCRIPTION_URL=url1,url2,url3`

All proxies from all sources will be combined and monitored together.
:::

### SUBSCRIPTION_UPDATE

- CLI: `--subscription-update`
- Required: No
- Default: `true`

Enables automatic updates of proxy configuration from subscription source. When enabled, Xray Checker will periodically check for changes and update configurations accordingly.

### SUBSCRIPTION_UPDATE_INTERVAL

- CLI: `--subscription-update-interval`
- Required: No
- Default: `300`

Time in seconds between subscription update checks. Only used when `SUBSCRIPTION_UPDATE` is enabled.

### SUBSCRIPTION_JSON_FORMAT

- CLI: `--subscription-json-format`
- Required: No
- Default: `false`

Requests the subscription as a full Xray JSON config instead of the default Base64 link list. Enable this for panels that serve JSON (e.g. Remnawave) and for subscriptions that use **balancers**: every outbound inside a balancer group is expanded into an individually checked proxy, so a single down node no longer hides behind the group. When enabled, the request is sent with an app-like `User-Agent` (overridable via `SUBSCRIPTION_USER_AGENT`). See [Subscription Format](/configuration/subscription#6-json-subscription-balancers).

### SUBSCRIPTION_USER_AGENT

- CLI: `--subscription-user-agent`
- Required: No
- Default: None

Overrides the `User-Agent` header sent when fetching a subscription. Useful for panels that return different content per client. When unset, Xray Checker uses `Xray-Checker`, or an app-like agent when `SUBSCRIPTION_JSON_FORMAT` is enabled.

### SUBSCRIPTION_HEADERS

- CLI: `--subscription-header`
- Required: No
- Default: None

Extra HTTP headers sent with every subscription request, each as a `Key: Value` pair.

- **CLI**: repeat `--subscription-header` for each header
- **Environment**: separate pairs with commas: `SUBSCRIPTION_HEADERS="X-Token: abc, X-Region: eu"`

## Proxy

### PROXY_CHECK_INTERVAL

- CLI: `--proxy-check-interval`
- Required: No
- Default: `300`

Time in seconds between proxy availability checks. Each check verifies all configured proxies.

### PROXY_CHECK_CONCURRENCY

- CLI: `--proxy-check-concurrency`
- Required: No
- Default: `0` (unlimited)

Maximum number of proxies checked in parallel within one cycle. `0` (the default) keeps the original behavior — every proxy is checked at once, so a cycle takes about as long as the slowest single proxy. A positive value bounds how many checks run simultaneously, limiting peak load and open connections on large subscriptions (the fixed per-proxy local ports are unchanged — checks are just dispatched in waves).

With a limit, a cycle's worst case is roughly `⌈N / concurrency⌉ × PROXY_TIMEOUT` (N = number of proxies), so keep `PROXY_CHECK_INTERVAL` at least that large. If a cycle overruns the interval it is logged as a warning and the next scheduled cycle is skipped — checks simply run less often rather than overlapping.

### PROXY_CHECK_METHOD

- CLI: `--proxy-check-method`
- Required: No
- Default: `ip`
- Values: `ip`, `status`, `download`

Method used to verify proxy functionality:

- `ip`: Compares IP addresses with and without proxy
- `status`: Checks HTTP status code from a test request
- `download`: Downloads a file and verifies minimum size received

### PROXY_IP_CHECK_URL

- CLI: `--proxy-ip-check-url`
- Required: No
- Default: `https://api.ipify.org?format=text`

URL used for IP verification when `PROXY_CHECK_METHOD=ip`. Should return current IP address in plain text format.

### PROXY_STATUS_CHECK_URL

- CLI: `--proxy-status-check-url`
- Required: No
- Default: `http://cp.cloudflare.com/generate_204`

URL used for status verification when `PROXY_CHECK_METHOD=status`. Should return HTTP 204/200 status code.

### PROXY_DOWNLOAD_URL

- CLI: `--proxy-download-url`
- Required: No
- Default: `https://proof.ovh.net/files/1Mb.dat`

URL used for download verification when `PROXY_CHECK_METHOD=download`. Should return a downloadable file.

### PROXY_DOWNLOAD_TIMEOUT

- CLI: `--proxy-download-timeout`
- Required: No
- Default: `60`

Maximum time in seconds to wait for download completion when using `PROXY_CHECK_METHOD=download`.

### PROXY_DOWNLOAD_MIN_SIZE

- CLI: `--proxy-download-min-size`
- Required: No
- Default: `51200` (50KB)

Minimum number of bytes that must be downloaded for the check to be considered successful when using `PROXY_CHECK_METHOD=download`.

### PROXY_TIMEOUT

- CLI: `--proxy-timeout`
- Required: No
- Default: `30`

Maximum time in seconds to wait for proxy response during checks.

### PROXY_RESOLVE_DOMAINS

CLI: `--proxy-resolve-domains`

Required: No

Default: `false`

When enabled, domain-based proxy configurations are expanded into multiple entries — one for each resolved IP address.
For example, a proxy with server: mydomain.com will be duplicated for every IP returned by DNS lookup.

This allows Xray Checker to monitor each resolved endpoint individually.

**Important notes:**

- This feature only works when the domain returns multiple IP addresses. If DNS returns only a single IP, no expansion will occur. Note that not all DNS providers return multiple IPs - for example, Amazon DNS typically returns only a single IP address.
- This feature works only with protocols that don't verify certificates against the domain name. It will work with Reality protocol, but will **not work** with standard vless and Trojan protocols, where the connection is established directly to the domain name and certificate validation is performed.

### SIMULATE_LATENCY

- CLI: `--simulate-latency`
- Required: No
- Default: `true`

Adds measured latency (TTFB - Time To First Byte) to endpoint responses, useful for monitoring systems that can interpret response delays.

## Web UI

### WEB_SHOW_DETAILS

- CLI: `--web-show-details`
- Required: No
- Default: `false`

Shows server IP addresses and ports in the web UI. When disabled, only proxy names are displayed for privacy.

### WEB_PUBLIC

- CLI: `--web-public`
- Required: No
- Default: `false`

Makes the dashboard publicly accessible without authentication. When enabled, the dashboard displays subscription name as title, hides admin controls and technical details (version, ports, config links).

:::caution[Requires Protected Metrics]
This option requires `METRICS_PROTECTED=true`. The `/metrics` endpoint and API will still require authentication, but the main dashboard (`/`) and individual proxy status pages (`/config/{id}`) will be public.
:::

### WEB_TRUSTED_EXTERNAL_AUTH

- CLI: `--web-trusted-external-auth`
- Required: No
- Default: `false`

Allows server details (`WEB_SHOW_DETAILS`) to be shown in public mode. By default, public mode hides server addresses and ports even when `WEB_SHOW_DETAILS=true`. Enable this only when the dashboard is protected by an **external auth proxy** (e.g. Authelia, Authentik, Cloudflare Access, or nginx basic auth) — you are asserting that the dashboard is already access-controlled, so it is safe to expose details.

### WEB_PUBLIC_SHOW_PROTOCOL

- CLI: `--web-public-show-protocol`
- Required: No
- Default: `false`

Shows the protocol badge (VLESS, VMess, Trojan, Shadowsocks, Hysteria2, WireGuard, SOCKS, HTTP) on dashboard cards in public mode. The badge is always shown in normal mode. This affects only the badge — server addresses, ports and config links remain hidden in public mode.

### WEB_CUSTOM_ASSETS_PATH

- CLI: `--web-custom-assets-path`
- Required: No
- Default: None

Path to a directory containing custom assets for the web interface. When set, files from this directory override the default assets.

Supported files:
- `index.html` — Full template replacement (Go template)
- `logo.svg` — Custom logo
- `favicon.ico` — Custom favicon
- `custom.css` — Additional styles (auto-injected)
- Any other files — Available at `/static/{filename}`

See [Web Customization](/configuration/web-customization) for details.

## Xray

### XRAY_START_PORT

- CLI: `--xray-start-port`
- Required: No
- Default: `10000`

Starting port number for SOCKS5 proxies. Each proxy will use sequential ports starting from this number.

### XRAY_LOG_LEVEL

- CLI: `--xray-log-level`
- Required: No
- Default: `none`
- Values: `debug`, `info`, `warning`, `error`, `none`

Controls Xray Core logging verbosity.

### XRAY_INTERFACE

- CLI: `--xray-interface`
- Required: No
- Default: None

Binds Xray proxy connections to a network interface, for example `wwan0` or
`eth2`. This is useful on multi-WAN and multi-modem routers. The value must be
the operating-system device name visible to the xray-checker process. When
running in a container, use host networking or otherwise expose the interface
inside the container. For WireGuard proxies, the setting binds the UDP peer
socket while the checker keeps the tunnel in userspace.

## Metrics

### METRICS_HOST

- CLI: `--metrics-host`
- Required: No
- Default: `0.0.0.0`

Host address for metrics and status endpoints.

### METRICS_PORT

- CLI: `--metrics-port`
- Required: No
- Default: `2112`

Port number for HTTP server exposing metrics and status endpoints.

### METRICS_PROTECTED

- CLI: `--metrics-protected`
- Required: No
- Default: `false`

Enables basic authentication for metrics and status endpoints.

### METRICS_USERNAME

- CLI: `--metrics-username`
- Required: No
- Default: `metricsUser`

Username for basic authentication when `METRICS_PROTECTED=true`.

### METRICS_PASSWORD

- CLI: `--metrics-password`
- Required: No
- Default: `MetricsVeryHardPassword`

Password for basic authentication when `METRICS_PROTECTED=true`.

### METRICS_INSTANCE

- CLI: `--metrics-instance`
- Required: No
- Default: None

Instance label added to all metrics. Useful for distinguishing multiple Xray Checker instances.

### METRICS_PUSH_URL

- CLI: `--metrics-push-url`
- Required: No
- Default: None

Prometheus Pushgateway URL for metric pushing. Format: `https://user:pass@host:port`

### METRICS_BASE_PATH

- CLI: `--metrics-base-path`
- Required: No
- Default: ""

URL path for host metrics and monitoring. Format: `/vpn/metrics`. Monitoring page could be available on `http://localhost:port/metrics-base-path`

## Other

### LOG_LEVEL

- CLI: `--log-level`
- Required: No
- Default: `info`
- Values: `debug`, `info`, `warn`, `error`, `none`

Controls Xray Checker application logging verbosity. Note: This is separate from `XRAY_LOG_LEVEL` which controls the Xray Core logging.

### RUN_ONCE

- CLI: `--run-once`
- Required: No
- Default: `false`

Performs single check cycle and exits. Useful for scheduled execution environments.
