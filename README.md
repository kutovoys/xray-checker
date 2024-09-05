# Xray Checker (Beta Version)

[English](https://github.com/kutovoys/xray-checker) | [Russian](https://github.com/kutovoys/xray-checker/blob/main/README_RU.md)

## Overview

Xray Checker is a beta version of a tool designed for testing connections through various protocols such as VLESS, Trojan, and Shadowsocks. The application parses provided configurations, creates Xray configuration files, and then tests the connectivity through the Xray proxy. If the external IP after using the proxy differs from the original, the tool sends the results to the Uptime Kuma monitoring system. While currently only Uptime Kuma is supported, additional providers will be added in the future.

This tool is still in development, and some features may not be fully implemented or stable. Feedback and contributions are welcome as we continue to improve the application.

## Features

- **Protocol Support**: VLESS, Trojan, and Shadowsocks.
- **Provider Integration**: Currently supports Uptime Kuma, with plans to add more providers.
- **Automated Testing**: Automatically tests connections and sends results to the monitoring system.
- **Docker Support**: Easily deployable using Docker and Docker Compose.

## Logic of Connection Testing

1. **Parse Configuration**: The application parses the provided configuration file to extract details about the connections to be tested.
2. **Generate Xray Configurations**: Based on the parsed data, the application generates configuration files for Xray.
3. **Check Current External IP**: Before running any tests, the application checks the current external IP address.
4. **Run Xray with Each Configuration**: Xray is started with each generated configuration file.
5. **Check IP Through Xray Proxy**: The external IP is checked again through the Xray proxy to see if it differs from the original.
6. **Send Data to Monitoring System**: If the IP addresses are different, the data is sent to the Uptime Kuma system.

## How to Use

### Docker

You can run the Xray Checker using Docker. Make sure you have a valid `config.json` file that contains your connection settings.

```bash
docker run --rm -v ./config.json:/app/config.json kutovoys/xray-checker
```

### Docker Compose

For a more complex setup or if you need to manage multiple services, you can use Docker Compose. Below is an example docker-compose.yml:

```yaml
services:
  xray-checker:
    image: kutovoys/xray-checker
    container_name: xray-checker
    volumes:
      - ./config.json:/app/config.json
    restart: unless-stopped
```

To start the service, run:

```
docker-compose up -d
```

## Configuration

The application requires a config.json file mounted to the Docker container. This file should contain the necessary information about the connections to be tested, including the protocol type, server details, and monitoring setup.

An example configuration file is provided as config.json.example in the repository. You can copy this file and modify it according to your needs:

```json
{
  "provider": {
    "name": "uptime-kuma",
    "proxyStartPort": 10000,
    "interval": 40,
    "configs": [
      {
        "link": "vless://uid@server:port?security=security&type=type&headerType=headerType&path=path&host=host&sni=sni&fp=fp&pbk=pbk&sid=#sid",
        "monitorLink": "https://uptime-kuma-url/api/push/MonitorID?status=up&msg=OK&ping="
      },
      {
        "link": "trojan://password@server:port?security=security&type=type&headerType=headerType&path=path&host=host&sni=sni&fp=fp#name",
        "monitorLink": "https://uptime-kuma-url/api/push/MonitorID?status=up&msg=OK&ping="
      },
      {
        "link": "ss://methodpassword@server:port#name",
        "monitorLink": "https://uptime-kuma-url/api/push/MonitorID?status=up&msg=OK&ping="
      }
    ]
  }
}
```

Simply rename config.json.example to config.json and adjust the values to fit your requirements.

## Plans

- Prometheus metric endpoint

## Contributing

Since this is a beta version, there is plenty of room for improvement. If you encounter issues or have suggestions, feel free to open an issue or submit a pull request.

## Disclaimer

This software is still under development. Use it at your own risk. The developers are not responsible for any damages or issues caused by using this tool.
