# Mihomo end-to-end check

`mihomo_e2e.py` runs real loopback traffic through the checker, an external
Mihomo process, and a REALITY server with h2mux support. It needs Python 3,
OpenSSL, a built checker, and the two core executables. It generates disposable
UUIDs, REALITY keys, and a TLS cover certificate; production subscriptions are
not used. All probe traffic stays on loopback.

Run from the repository root:

```sh
go build -o ./xray-checker .
python3 integration/mihomo_e2e.py \
  --checker ./xray-checker \
  --mihomo /path/to/mihomo \
  --xray /path/to/jolymmiles-xray
```

The test checks that:

- A valid VLESS/REALITY user is online and an invalid UUID is offline.
- The REALITY server receives an authenticated sing-mux carrier while the
  source configuration selects h2mux with padding, rather than merely making
  an ordinary VLESS connection.
- Subscription-level rules cannot redirect probes away from their named proxy.
- Changing only mux options triggers a refresh.
- An invalid replacement subscription preserves the working configuration.
- SIGTERM stops the owned Mihomo child and its listeners. Child ownership is
  also checked through `/proc` on Linux.

Use `--keep-workdir` to retain private logs for a failed run. The reported
temporary directory contains only generated test credentials; delete it when
finished investigating.

## Reference binaries

The validation environment uses the following official published assets.
Verify their archive digests before extracting or executing them:

| Core | Asset | Archive SHA256 |
| --- | --- | --- |
| Mihomo v1.19.33 | [Linux amd64 v1 gzip](https://github.com/MetaCubeX/mihomo/releases/download/v1.19.33/mihomo-linux-amd64-v1-v1.19.33.gz) | `8ec92e7b2fdc6b853ba00156bab7e4e965c8b0d843b8c4ac8ab98795f02ad524` |
| Jolymmiles Xray v26.10.28-1202 | [Linux amd64 zip](https://github.com/Jolymmiles/Xray-core/releases/download/v26.10.28-1202/Xray-linux-64.zip) | `eed1647c164b0066029f3332eb60e1b591387d1bcb1e290f01b3e4637b5a6659` |

The Jolymmiles executable is used only as a test server supporting h2mux.
It is not bundled into the checker or required by ordinary deployments.
