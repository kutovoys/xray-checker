# Xray Checker

<div align="center">

[![GitHub Release](https://img.shields.io/github/v/release/kutovoys/xray-checker?color=blue)](https://github.com/kutovoys/xray-checker/releases/latest)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/kutovoys/xray-checker/build-publish.yml)](https://github.com/kutovoys/xray-checker/actions/workflows/build-publish.yml)
[![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/kutovoys/xray-checker/total?logo=github&color=blue)](https://github.com/kutovoys/xray-checker/releases/latest)
[![Docker Pulls](https://img.shields.io/docker/pulls/kutovoys/xray-checker?logo=docker&label=pulls)](https://hub.docker.com/r/kutovoys/xray-checker/)
[![GitHub License](https://img.shields.io/github/license/kutovoys/xray-checker?color=greeen)](https://github.com/kutovoys/xray-checker/blob/main/LICENSE)
[![ru](https://img.shields.io/badge/lang-ru-blue)](https://github.com/kutovoys/xray-checker/blob/main/README_RU.md)
[![en](https://img.shields.io/badge/lang-en-red)](https://github.com/kutovoys/xray-checker/blob/main/README.md)

</div>
<div align="center">

[![Documentation](https://img.shields.io/badge/Docs-xray--checker.kutovoy.dev-blue)](https://xray-checker.kutovoy.dev/)
[![DockerHub](https://img.shields.io/badge/DockerHub-kutovoys%2Fxray--checker-blue)](https://hub.docker.com/r/kutovoys/xray-checker/)
[![Live Demo](https://img.shields.io/badge/Demo-live-green)](https://demo-xray-checker.kutovoy.dev/)
[![Telegram Chat](https://img.shields.io/badge/Telegram-Chat-blue?logo=telegram&)](https://t.me/+uZCGx_FRY0tiOGIy)

</div>

Xray Checker is a tool for monitoring proxy server availability with support for VLESS, VMess, Trojan, and Shadowsocks protocols. It automatically tests connections through Xray Core and provides metrics for Prometheus, as well as API endpoints for integration with monitoring systems.

<div align="center">
  <img src=".github/screen/xray-checker.webp" alt="Dashboard Screenshot">
</div>

> [!TIP]
> **Try the Live Demo:** See Xray Checker in action at [demo-xray-checker.kutovoy.dev](https://demo-xray-checker.kutovoy.dev/)

## 🚀 Key Features

- 🔍 Monitoring of Xray proxy servers (VLESS, VMess, Trojan, Shadowsocks)
- 🧪 Optional Mihomo backend for native Mihomo subscriptions and h2mux probes
- 🔄 Automatic configuration updates from subscription (multiple subscriptions supported)
- 📊 Prometheus metrics export with Pushgateway support
- 🌐 REST API with OpenAPI/Swagger documentation
- 🌓 Web interface with dark/light theme
- 🎨 Full web customization (custom logo, styles, or entire template)
- 📄 Public status page for VPN services (no authentication required)
- 📥 Endpoints for monitoring system integration (Uptime Kuma, etc.)
- 🔒 Basic Auth protection for metrics and web interface
- 🐳 Docker and Docker Compose support
- 🌍 Automatic geo files management (geoip.dat, geosite.dat)
- 📝 Flexible configuration loading:
  - URL subscriptions (base64, JSON)
  - Share links (vless://, vmess://, trojan://, ss://)
  - JSON configuration files
  - Folders with configurations
  - Mihomo YAML with inline `proxies` when `PROXY_ENGINE=mihomo`

Full list of features available in the [documentation](https://xray-checker.kutovoy.dev/intro/features).

## 🚀 Quick Start

### Docker

```bash
docker run -d \
  -e SUBSCRIPTION_URL=https://your-subscription-url/sub \
  -p 2112:2112 \
  kutovoys/xray-checker
```

### Docker Compose

```yaml
services:
  xray-checker:
    image: kutovoys/xray-checker
    environment:
      - SUBSCRIPTION_URL=https://your-subscription-url/sub
    ports:
      - "2112:2112"
```

Detailed installation and configuration documentation is available at [xray-checker.kutovoy.dev](https://xray-checker.kutovoy.dev/intro/quick-start)

### Mihomo subscriptions and h2mux

The default backend remains embedded Xray. To check a Mihomo subscription using
its native `smux` and REALITY options, install a maintained
[Mihomo release](https://github.com/MetaCubeX/mihomo/releases), pin the version in
your deployment, and select its executable:

```sh
export PROXY_ENGINE=mihomo
export MIHOMO_BINARY=/usr/local/bin/mihomo
export SUBSCRIPTION_URL='https://subscription.example/PRIVATE_TOKEN/mihomo'
export SUBSCRIPTION_JSON_FORMAT=false
./xray-checker
```

Equivalent CLI options are `--proxy-engine=mihomo` and
`--mihomo-binary=/usr/local/bin/mihomo`. The default binary name is `mihomo` on
`PATH`. Mihomo is installed separately: the checker and Docker image neither
bundle nor download it. Docker deployments must supply a matching executable.

This mode reads inline `proxies` from a full Mihomo YAML subscription or a
proxy-provider document and runs each proxy through its own loopback SOCKS
listener. It preserves h2mux settings instead of converting the subscription to
ordinary Xray outbounds. Subscription routing, groups, TUN, DNS and controller
settings are not imported; external providers and `dialer-proxy` chains are not
supported. `SUBSCRIPTION_JSON_FORMAT=true` is an Xray option and conflicts with
Mihomo mode.

The status-page API and Prometheus metrics remain compatible. Changing backends
creates new stable IDs and metric series; existing status history is not
automatically migrated. See the
[Mihomo setup guide](docs/src/content/docs/configuration/subscription.md#10-mihomo-subscriptions-and-h2mux)
for Windows, OpenWrt ARM64, Docker and subscription-header details.

## 📈 Project Statistics

<a href="https://star-history.com/#kutovoys/xray-checker&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=kutovoys/xray-checker&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=kutovoys/xray-checker&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=kutovoys/xray-checker&type=Date" />
 </picture>
</a>

## 🤝 Contributing

We welcome any contributions to Xray Checker! If you want to help:

1. Fork the repository
2. Create a branch for your changes
3. Make and test your changes
4. Create a Pull Request

For more details on how to contribute, read the [contributor's guide](https://xray-checker.kutovoy.dev/contributing/development-guide).

<p align="center">
Thanks to the all contributors who have helped improve Xray Checker:
</p>
<p align="center">
<a href="https://github.com/kutovoys/xray-checker/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=kutovoys/xray-checker" />
</a>
</p>
<p align="center">
  Made with <a rel="noopener noreferrer" target="_blank" href="https://contrib.rocks">contrib.rocks</a>
</p>

## VPN Recommendation

For secure and reliable internet access, we recommend [BlancVPN](https://getblancvpn.com/pricing?promo=klugscl&ref=xc-readme). Use promo code `KLUGSCL` for 15% off your subscription.
