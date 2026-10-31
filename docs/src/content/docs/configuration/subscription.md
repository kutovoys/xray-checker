---
title: Subscription Format
description: Subscription format options and examples
---

Xray Checker supports the configuration sources below. Use the [environment variable](/configuration/envs#subscription_url) `SUBSCRIPTION_URL` for setup. Embedded Xray is the default backend; [Mihomo subscriptions](#10-mihomo-subscriptions-and-h2mux) require `PROXY_ENGINE=mihomo` and a separately installed Mihomo executable.

For information about how proxies are verified, see [check methods](/configuration/check-methods).

### 1. Subscription URL (Default)

Standard subscription URL returning Base64 encoded list of proxy links.

Example:

```bash
SUBSCRIPTION_URL=https://example.com/subscription
```

Requirements:

- HTTPS URL
- Returns Base64 encoded content
- Content is newline-separated proxy URLs
- Supports standard User-Agent headers

Headers sent:

```
Accept: */*
User-Agent: Xray-Checker
```

### 2. Base64 String

Direct Base64 encoded string containing proxy configuration links.

Example:

```bash
SUBSCRIPTION_URL=dmxlc3M6Ly91dWlkQGV4YW1wbGUuY29tOjQ0MyVlbmNyeXB0aW9uPW5vbmUmc2VjdXJpdHk9dGxzI3Byb3h5MQ==
```

Content format (before encoding):

```
vless://uuid@example.com:443?encryption=none&security=tls#proxy1
trojan://password@example.com:443?security=tls#proxy2
vmess://base64encodedconfig
ss://base64encodedconfig
hysteria2://password@example.com:443?sni=example.com#proxy5
```

:::note[Supported protocols]
VLESS, VMess, Trojan, Shadowsocks and **Hysteria2** are supported, including transports such as Reality, xhttp, gRPC, WebSocket and mKCP. Plain **SOCKS / HTTP / HTTPS forward proxies** can also be checked — see [SOCKS, HTTP and HTTPS proxies](#7-socks-http-and-https-proxies) below.
:::

### 3. V2Ray JSON File

Single JSON configuration file in V2Ray/Xray format.

Example:

```bash
SUBSCRIPTION_URL=file:///path/to/config.json
```

File format:

```json
{
  "outbounds": [
    {
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "example.com",
            "port": 443,
            "users": [
              {
                "id": "uuid",
                "encryption": "none"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "tls"
      }
    }
  ]
}
```

### 4. Xray JSON Array (Multi-config)

JSON array containing multiple Xray configurations with remarks. This format is useful when exporting configurations from GUI clients or managing multiple named configurations in a single file.

Example:

```bash
SUBSCRIPTION_URL=file:///path/to/configs.json
```

File format:

```json
[
  {
    "remarks": "US Server 1",
    "outbounds": [
      {
        "protocol": "vless",
        "settings": {
          "vnext": [
            {
              "address": "us1.example.com",
              "port": 443,
              "users": [{ "id": "uuid-1", "encryption": "none" }]
            }
          ]
        },
        "streamSettings": { "network": "tcp", "security": "tls" }
      }
    ]
  },
  {
    "remarks": "EU Server 1",
    "outbounds": [
      {
        "protocol": "trojan",
        "settings": {
          "servers": [
            {
              "address": "eu1.example.com",
              "port": 443,
              "password": "password123"
            }
          ]
        },
        "streamSettings": { "network": "tcp", "security": "tls" }
      }
    ]
  }
]
```

The `remarks` field from each configuration will be used as the proxy name in the dashboard.

### 5. Configuration Folder

Directory containing multiple V2Ray/Xray JSON configuration files.

Example:

```bash
SUBSCRIPTION_URL=folder:///path/to/configs
```

Requirements:

- Directory must contain .json files
- Each file follows V2Ray JSON format
- Files are processed in alphabetical order
- Invalid files are skipped with warning

### 6. JSON Subscription (Balancers)

Some panels (e.g. Remnawave) serve a full Xray JSON config instead of a Base64 link list, and group several servers under a single **balancer** (`balancer` / `leastPing`). With the default Base64 format such a group is collapsed into one entry, so you can't tell which node is down.

Enable [`SUBSCRIPTION_JSON_FORMAT`](/configuration/envs#subscription_json_format) to request the JSON form and expand every outbound in a balancer into an **individually checked proxy**:

```bash
SUBSCRIPTION_URL=https://panel.example.com/sub
SUBSCRIPTION_JSON_FORMAT=true
```

Nodes within a group are named `<group> | <node>` and share a `group_name` (used by the [grouped dashboard](/configuration/status-page) and the [`group_name` metric label](/integrations/metrics)). When the JSON format is enabled the request is sent with an app-like `User-Agent`; override it with `SUBSCRIPTION_USER_AGENT` if your panel expects a specific client.

### 7. SOCKS, HTTP and HTTPS Proxies

Besides Xray protocols, plain forward proxies can be health-checked. Add them as subscription lines (any source — URL, `base64://`, `file://`, or inline):

```
socks://base64(user:pass)@host:port#name
socks5://user:pass@host:port#name
http://user:pass@host:port#name
https://user:pass@host:port#name
```

- `socks://`, `socks5://` and `socks5h://` map to a SOCKS outbound. Credentials may be plain `user:pass` or, for `socks://`, the standard Base64-encoded `user:pass` token.
- `http://` is a plain HTTP CONNECT proxy.
- `https://` is an HTTP proxy reached over TLS.
- The `#name` fragment sets the display name (defaults to `host:port`).

For an `https://` proxy with a self-signed or private certificate, pin it instead of disabling verification (xray-core no longer supports `allowInsecure`):

```
https://user:pass@host:port?pinnedPeerCertSha256=<sha256-hex>#name
https://user:pass@host:port?sni=real.example.com&verifyPeerCertByName=real.example.com#name
```

| Query param | Alias | Description |
|-------------|-------|-------------|
| `pinnedPeerCertSha256` | `pcs` | Accept the peer cert whose SHA-256 (hex, colons allowed) matches — for self-signed/internal certs |
| `verifyPeerCertByName` | `vcn` | Verify the cert against this name instead of the host |
| `sni` | — | TLS Server Name (defaults to the host) |

### 8. WireGuard

WireGuard servers can be health-checked too. Add them as subscription lines (any source — URL, `base64://`, `file://`, or inline) using the `wg://` scheme, where the payload is the **Base64 of a standard WireGuard `.conf`**:

```
wg://<base64 of the .conf>#name
```

The decoded `.conf` is the regular WireGuard config you get from your provider:

```ini
[Interface]
PrivateKey = <client private key>
Address = 10.9.0.2/32
DNS = 1.1.1.1            # optional, ignored by the checker
MTU = 1420               # optional (default 1420)

[Peer]
PublicKey = <server public key>
PresharedKey = <psk>     # optional
Endpoint = wg.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25 # optional
```

- The peer `Endpoint` (`host:port`) is what gets checked. The first `[Peer]` is used.
- The `#name` fragment sets the display name (defaults to `wireguard-<host>`).
- Standard, unobfuscated WireGuard only. (AmneziaWG / `awg://` is not supported.)

WireGuard also works inside a [JSON subscription](#6-json-subscription-balancers) — a `wireguard` outbound in the Xray JSON config is parsed automatically:

```json
{
  "protocol": "wireguard",
  "settings": {
    "secretKey": "<client private key>",
    "address": ["10.9.0.2/32"],
    "mtu": 1420,
    "peers": [
      {
        "publicKey": "<server public key>",
        "endpoint": "wg.example.com:51820",
        "allowedIPs": ["0.0.0.0/0", "::/0"],
        "keepAlive": 25
      }
    ]
  }
}
```

:::note[TUN mode and performance]
WireGuard runs in userspace (no kernel module needed). The network layer is either a real **kernel TUN** interface (fast, scales to many tunnels) or a **gVisor** userspace netstack (works with no privileges, e.g. macOS, but heavier). Kernel TUN needs `/dev/net/tun` and `CAP_NET_ADMIN`; in Docker pass `--cap-add NET_ADMIN --device /dev/net/tun`. For subscriptions with many WireGuard configs, kernel TUN is recommended (and give `PROXY_TIMEOUT` some headroom).
:::

### 9. Custom Metric Labels

Any outbound in a JSON subscription (sections [3](#3-v2ray-json-file), [4](#4-xray-json-array-multi-config) and [6](#6-json-subscription-balancers)) may carry a `metricsLabels` object with operator-defined static labels. Each entry becomes an extra label on that proxy's `xray_proxy_status` and `xray_proxy_latency_ms` metrics, and is returned by the API under `metricsLabels`. This lets you filter and aggregate by attributes like location or hoster directly in PromQL and Grafana.

```json
{
  "protocol": "trojan",
  "tag": "proxy",
  "settings": { "servers": [{ "address": "1.1.1.1", "port": 443, "password": "..." }] },
  "metricsLabels": {
    "location": "Netherlands, Amsterdam",
    "hoster": "FreeVDS"
  }
}
```

The labels are then attached to the metric:

```text
xray_proxy_status{protocol="trojan",address="1.1.1.1:443",name="proxy",...,location="Netherlands, Amsterdam",hoster="FreeVDS"} 1
```

Notes:

- Keys are sanitized to valid Prometheus label names (e.g. `data center` → `data_center`); keys that collide with built-in labels (`protocol`, `address`, `name`, `sub_name`, `stable_id`, `group_name`, `instance`) are ignored.
- Labels are a JSON-subscription feature only — share links (`vless://`, …) have nowhere to carry them.
- Changing a label and updating the subscription applies on the next refresh **without resetting other proxies' series to 0**. See [`metricsLabels` on metrics](/integrations/metrics#custom-labels).

### 10. Mihomo Subscriptions and h2mux

Select `PROXY_ENGINE=mihomo` to check proxies with the same native Mihomo options
used by compatible clients. In particular, an enabled `smux.protocol: h2mux`
is passed to Mihomo and used for the probe; it is not discarded during a
conversion to Xray JSON.

#### Core and input format

Install a maintained [Mihomo release](https://github.com/MetaCubeX/mihomo/releases)
for the checker machine's OS and architecture, verify the release checksum, and
pin that version in your deployment. The core must support named SOCKS listeners
with a `proxy` target and the options present in your subscription. Xray Checker
does not bundle, download or update the Mihomo executable.

The development test core is Mihomo **v1.19.33** on Linux.

| Environment variable | CLI option | Default |
| --- | --- | --- |
| `PROXY_ENGINE` | `--proxy-engine` | `xray`; select `mihomo` for this mode |
| `MIHOMO_BINARY` | `--mihomo-binary` | `mihomo`, resolved on `PATH` |
| `XRAY_START_PORT` | `--xray-start-port` | `10000`, used by either backend |

`SUBSCRIPTION_URL` can point to a full Mihomo YAML subscription or a provider
document containing a nonempty inline `proxies` list. Local files use the usual
`file:///absolute/path/subscription.yaml` source. JSON with the same top-level
`proxies` shape is accepted too; `folder:///absolute/path/configs` reads
`.yaml`, `.yml` and `.json` files in this mode. All sources in one Mihomo-mode checker must use
this format; Xray JSON outbounds and share-link lists belong to the Xray backend.

Each entry requires nonempty `name`, `type`, and `server` strings and a scalar
`port` from 1 through 65535. A port-hopping entry with only `ports` and no `port`
is not supported.

For URL subscriptions, the default request `User-Agent` is `mihomo`.
`SUBSCRIPTION_USER_AGENT` and custom `SUBSCRIPTION_HEADERS` can override it;
update any existing Xray/Happ-specific override. With panels such as Remnawave,
use the explicit Mihomo subscription URL, commonly the existing subscription
URL followed by `/mihomo`, when that path is enabled by the panel. The monitoring
account must have access to the desired inbounds and hosts in the Mihomo format.
`SUBSCRIPTION_JSON_FORMAT=true` requests Xray JSON and is rejected in Mihomo mode.

A VLESS/REALITY proxy may contain:

```yaml
proxies:
  - name: H2MUX example
    type: vless
    server: proxy.example.com
    port: 443
    uuid: YOUR_USER_UUID
    network: tcp
    tls: true
    servername: cover.example.com
    client-fingerprint: chrome
    reality-opts:
      public-key: YOUR_REALITY_PUBLIC_KEY
      short-id: YOUR_SHORT_ID
      support-x25519mlkem768: true
    smux:
      enabled: true
      protocol: h2mux
      padding: true
      min-streams: 8
      max-connections: 2
```

Use the values supplied by your panel. Proxy transport options, `smux`, and
`reality-opts` are preserved. The checker generates its own unique internal
proxy names and retains the original server hostname for Mihomo's DNS resolution.
`PROXY_RESOLVE_DOMAINS=true` is rejected in this mode because replacing the
hostname with an IP can change implicit TLS SNI. Use the proxy's native
`ip-version` option when you need to choose an address family.

Only inline proxy entries are checked. Remote `proxy-providers` are not fetched;
a provider-only document is rejected. Top-level routing rules, proxy groups,
listeners, DNS, TUN, controller settings and scripts are not imported. Each
checker-owned SOCKS listener targets one proxy directly. `dialer-proxy` chains
are rejected rather than resolving through a group or falling back to another
node. The candidate configuration is validated by the selected Mihomo executable.

The core runs with the checker account's OS permissions. Native proxy options
can reference local files, so use subscriptions from sources you trust.

Checks use the existing HTTP-based `ip`, `status` or `download` method through
each proxy's TCP path. They exercise h2mux when configured; they do not separately
measure UDP health, multiplexing capacity or the negotiated TLS key-exchange group.

#### Native OpenWrt ARM64 or Linux

An OpenWrt machine reporting `aarch64` needs compatible Linux ARM64 checker and
Mihomo executables. Place the selected versions in a separate test directory;
these examples do not replace an existing router proxy service. The API port
`2212` and SOCKS range beginning at `12000` allow a trial beside a checker using
the defaults, provided those ports are free.

```sh
cd /opt/xray-checker-test
export PROXY_ENGINE=mihomo
export MIHOMO_BINARY=/opt/xray-checker-test/mihomo
export SUBSCRIPTION_URL='https://subscription.example/PRIVATE_TOKEN/mihomo'
export SUBSCRIPTION_JSON_FORMAT=false
export XRAY_START_PORT=12000
export METRICS_HOST=127.0.0.1
export METRICS_PORT=2212
./xray-checker
```

The checker needs an executable core and a writable working directory for its
generated configuration. For a service deployment, store the private
subscription URL in that service's protected environment configuration. Keep
the running checker attached to the service manager so it can shut down its
Mihomo child normally.

#### Native Windows

Install both matching Windows executables into a test directory and create its
`tmp` subdirectory. Before starting, restrict the NTFS permissions on both
directories and their contents to the checker account, SYSTEM and Administrators.
The generated `mihomo_config.yaml` in the working directory and the validation
and runtime configurations under `TEMP` contain proxy credentials. Go's Unix
`0600` file mode does not enforce Windows ACLs.

In PowerShell, point `TEMP` and `TMP` at that protected temporary directory:

```powershell
Set-Location 'C:\Tools\xray-checker-test'
$env:TEMP = 'C:\Tools\xray-checker-test\tmp'
$env:TMP = $env:TEMP
$env:PROXY_ENGINE = 'mihomo'
$env:MIHOMO_BINARY = 'C:\Tools\xray-checker-test\mihomo.exe'
$env:SUBSCRIPTION_URL = 'https://subscription.example/PRIVATE_TOKEN/mihomo'
$env:SUBSCRIPTION_JSON_FORMAT = 'false'
$env:XRAY_START_PORT = '12000'
$env:METRICS_HOST = '127.0.0.1'
$env:METRICS_PORT = '2212'
.\xray-checker.exe
```

After testing, apply the same variables to the checker service using its actual
service manager and protected working directory. Redirect service stdout and
stderr into a directory with the same permissions: startup and subscription-fetch
errors can include the private subscription URL. An unrelated system Xray or
Mihomo service does not need to be replaced.

#### Docker and existing status pages

The standard checker image contains embedded Xray but **does not contain
Mihomo**. To use this mode, supply a compatible Linux executable for the
container's architecture and runtime, for example as a read-only bind mount:

```yaml
services:
  xray-checker:
    image: kutovoys/xray-checker
    environment:
      PROXY_ENGINE: mihomo
      MIHOMO_BINARY: /opt/mihomo/mihomo
      SUBSCRIPTION_URL: ${SUBSCRIPTION_URL}
      SUBSCRIPTION_JSON_FORMAT: "false"
    volumes:
      - ./mihomo:/opt/mihomo/mihomo:ro
    ports:
      - "127.0.0.1:2212:2112"
```

The mounted file must be executable by the container user. Supply
`SUBSCRIPTION_URL` through your deployment environment.

`/api/v1/public/proxies`, proxy status endpoints and Prometheus metrics keep
their existing structure, so status pages can consume this backend through the
same API. The optional `generatedConfig` detail is omitted for Mihomo proxies
because raw definitions can contain credentials in arbitrary nested options.

Switching a proxy from the Xray backend to Mihomo produces a different stable ID
and therefore new monitoring series, even if its address is unchanged. Existing
status-page history is not automatically migrated between those identities.

## Custom Request Headers

Panels that gate the subscription behind a token or a specific client can be satisfied with a custom `User-Agent` and arbitrary headers:

```bash
SUBSCRIPTION_USER_AGENT="Happ/1.0"
SUBSCRIPTION_HEADERS="X-Token: abc, X-Region: eu"
```

See [`SUBSCRIPTION_USER_AGENT`](/configuration/envs#subscription_user_agent) and [`SUBSCRIPTION_HEADERS`](/configuration/envs#subscription_headers).
