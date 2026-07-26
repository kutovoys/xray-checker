package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"xray-checker/logger"
	"xray-checker/models"

	"github.com/xtls/xray-core/infra/conf/serial"
)

type ConfigGenerator struct {
	outboundInterface string
}

func NewConfigGenerator(outboundInterfaces ...string) *ConfigGenerator {
	var outboundInterface string
	if len(outboundInterfaces) > 0 {
		outboundInterface = strings.TrimSpace(outboundInterfaces[0])
	}
	return &ConfigGenerator{
		outboundInterface: outboundInterface,
	}
}

func (g *ConfigGenerator) GenerateConfig(proxies []*models.ProxyConfig, startPort int, xrayLogLevel string) ([]byte, error) {
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": xrayLogLevel,
		},
		"inbounds":  g.generateInbounds(proxies, startPort),
		"outbounds": g.generateOutbounds(proxies),
		"routing":   g.generateRouting(proxies),
	}

	return json.MarshalIndent(config, "", "  ")
}

// validateConfigBuild runs the generated JSON through xray-core's decode+build path
// (the same path the runner uses at startup), so a config that xray-core would reject
// is caught here — letting us report and prune the offending node instead of having
// the whole instance abort on Start.
func validateConfigBuild(configBytes []byte) error {
	xrayConfig, err := serial.DecodeJSONConfig(bytes.NewReader(configBytes))
	if err != nil {
		return err
	}
	_, err = xrayConfig.Build()
	return err
}

func outboundTag(proxy *models.ProxyConfig) string {
	return fmt.Sprintf("%s_%d", proxy.Name, proxy.Index)
}

// extractFailingOutboundTag pulls the outbound tag from an xray build error shaped
// like: "... failed to build outbound config with tag <TAG> > <reason>".
func extractFailingOutboundTag(errMsg string) string {
	const marker = "with tag "
	i := strings.Index(errMsg, marker)
	if i < 0 {
		return ""
	}
	rest := errMsg[i+len(marker):]
	if j := strings.Index(rest, " > "); j >= 0 {
		return strings.TrimSpace(rest[:j])
	}
	return strings.TrimSpace(rest)
}

// shortBuildReason returns the leaf segment of xray's ">"-chained error message.
func shortBuildReason(err error) string {
	parts := strings.Split(err.Error(), " > ")
	return strings.TrimSpace(parts[len(parts)-1])
}

// GenerateValidatedConfig generates the xray config and, if xray-core rejects a
// specific outbound at build time, drops that proxy, logs why it was excluded, and
// regenerates. This prevents a single unsupported/legacy node (e.g. a transport
// field removed in a newer xray-core) from bringing down the whole instance. It
// writes the final, buildable config to filename and returns the surviving proxies.
func (g *ConfigGenerator) GenerateValidatedConfig(proxies []*models.ProxyConfig, startPort int, filename, xrayLogLevel string) ([]*models.ProxyConfig, error) {
	current := proxies
	for {
		configBytes, err := g.GenerateConfig(current, startPort, xrayLogLevel)
		if err != nil {
			return current, fmt.Errorf("error generating config: %v", err)
		}

		buildErr := validateConfigBuild(configBytes)
		if buildErr == nil {
			if err := os.WriteFile(filename, configBytes, 0644); err != nil {
				return current, fmt.Errorf("error saving config: %v", err)
			}
			if len(current) != len(proxies) {
				logger.Warn("Config validated after excluding %d unbuildable prox(ies); %d remain", len(proxies)-len(current), len(current))
			}
			return current, nil
		}

		tag := extractFailingOutboundTag(buildErr.Error())
		idx := -1
		if tag != "" {
			for i, p := range current {
				if outboundTag(p) == tag {
					idx = i
					break
				}
			}
		}

		if idx < 0 {
			// Failure can't be attributed to a single proxy — keep the config so the
			// error surfaces at startup instead of silently dropping everything.
			logger.Error("Xray config build failed and no offending proxy could be identified; keeping config as-is: %v", buildErr)
			if werr := os.WriteFile(filename, configBytes, 0644); werr != nil {
				return current, werr
			}
			return current, nil
		}

		bad := current[idx]
		logger.Warn("Excluding proxy %q (%s %s:%d): xray rejected its config: %s",
			bad.Name, bad.Protocol, bad.Server, bad.Port, shortBuildReason(buildErr))
		current = append(current[:idx:idx], current[idx+1:]...)
	}
}

func (g *ConfigGenerator) generateInbounds(proxies []*models.ProxyConfig, startPort int) []map[string]interface{} {
	var inbounds []map[string]interface{}

	for _, proxy := range proxies {
		inbound := map[string]interface{}{
			"listen":   "127.0.0.1",
			"port":     startPort + proxy.Index,
			"protocol": "socks",
			"tag":      fmt.Sprintf("%s_%s_%d_Inbound", proxy.Name, proxy.Protocol, proxy.Index),
			"sniffing": map[string]interface{}{
				"enabled":      true,
				"destOverride": []string{"http", "tls", "quic"},
				"routeOnly":    true,
			},
			"settings": map[string]interface{}{
				"auth":      "noauth",
				"udp":       true,
				"userLevel": 0,
			},
		}
		inbounds = append(inbounds, inbound)
	}

	return inbounds
}

func (g *ConfigGenerator) generateOutbounds(proxies []*models.ProxyConfig) []map[string]interface{} {
	var outbounds []map[string]interface{}

	outbounds = append(outbounds, map[string]interface{}{
		"tag":      "direct",
		"protocol": "freedom",
		"settings": map[string]interface{}{"domainStrategy": "UseIP"},
	})

	outbounds = append(outbounds, map[string]interface{}{
		"tag":      "block",
		"protocol": "blackhole",
		"settings": map[string]interface{}{},
	})

	for _, proxy := range proxies {
		outbound := g.generateProxyOutbound(proxy)
		outbounds = append(outbounds, outbound)
	}

	return outbounds
}

// GenerateProxyOutbound returns the xray outbound configuration map for a single
// proxy. It is exported so the web layer can surface a sanitized view of the
// generated config for debugging without rebuilding the whole config.
func (g *ConfigGenerator) GenerateProxyOutbound(proxy *models.ProxyConfig) map[string]interface{} {
	return g.generateProxyOutbound(proxy)
}

func (g *ConfigGenerator) generateProxyOutbound(proxy *models.ProxyConfig) map[string]interface{} {
	outbound := map[string]interface{}{
		"tag":      fmt.Sprintf("%s_%d", proxy.Name, proxy.Index),
		"protocol": proxy.Protocol,
	}

	switch proxy.Protocol {
	case "vless":
		user := map[string]interface{}{
			"id":    proxy.UUID,
			"level": proxy.GetUserLevel(),
		}
		if proxy.Encryption != "" {
			user["encryption"] = proxy.Encryption
		} else {
			user["encryption"] = "none"
		}
		if proxy.Flow != "" {
			user["flow"] = proxy.Flow
		}
		outbound["settings"] = map[string]interface{}{
			"vnext": []map[string]interface{}{
				{"address": proxy.Server, "port": proxy.Port, "users": []map[string]interface{}{user}},
			},
		}

	case "vmess":
		outbound["settings"] = map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": proxy.Server,
					"port":    proxy.Port,
					"users": []map[string]interface{}{
						{
							"id":       proxy.UUID,
							"alterId":  proxy.GetAlterId(),
							"security": proxy.GetVMessSecurity(),
							"level":    proxy.GetUserLevel(),
						},
					},
				},
			},
		}

	case "trojan":
		server := map[string]interface{}{
			"address":  proxy.Server,
			"port":     proxy.Port,
			"password": proxy.Password,
		}
		if proxy.Flow != "" {
			server["flow"] = proxy.Flow
		}
		outbound["settings"] = map[string]interface{}{
			"servers": []map[string]interface{}{server},
		}

	case "shadowsocks":
		outbound["settings"] = map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"address":  proxy.Server,
					"port":     proxy.Port,
					"method":   proxy.Method,
					"password": proxy.Password,
				},
			},
		}

	case "hysteria":
		// xray-core's outbound HysteriaClientConfig has only version/address/port.
		// The auth lives in streamSettings.hysteriaSettings.auth (set below).
		outbound["settings"] = map[string]interface{}{
			"address": proxy.Server,
			"port":    proxy.Port,
			"version": 2,
		}

	case "socks", "http":
		// Plain forward proxies. For an https:// proxy the protocol is "http"
		// with TLS applied via streamSettings (proxy.Security == "tls").
		server := map[string]interface{}{
			"address": proxy.Server,
			"port":    proxy.Port,
		}
		if proxy.Username != "" || proxy.Password != "" {
			server["users"] = []map[string]interface{}{
				{"user": proxy.Username, "pass": proxy.Password},
			}
		}
		outbound["settings"] = map[string]interface{}{
			"servers": []map[string]interface{}{server},
		}

	case "wireguard":
		peer := map[string]interface{}{
			"publicKey": proxy.WGPeerPublicKey,
			"endpoint":  fmt.Sprintf("%s:%d", proxy.Server, proxy.Port),
		}
		if len(proxy.WGAllowedIPs) > 0 {
			peer["allowedIPs"] = proxy.WGAllowedIPs
		} else {
			peer["allowedIPs"] = []string{"0.0.0.0/0", "::/0"}
		}
		if proxy.WGPreSharedKey != "" {
			peer["preSharedKey"] = proxy.WGPreSharedKey
		}
		if proxy.WGKeepalive > 0 {
			peer["keepAlive"] = proxy.WGKeepalive
		}
		settings := map[string]interface{}{
			"secretKey": proxy.WGPrivateKey,
			"address":   proxy.WGAddresses,
			"peers":     []map[string]interface{}{peer},
			// A health checker does not need a host kernel TUN. Keeping WireGuard
			// userspace-only avoids changing host interfaces, routes, or sysctls
			// when xray-checker runs with CAP_NET_ADMIN (for example as root on
			// OpenWrt).
			"noKernelTun": true,
		}
		if proxy.WGMTU > 0 {
			settings["mtu"] = proxy.WGMTU
		}
		outbound["settings"] = settings
	}

	if proxy.Protocol == "wireguard" {
		// WireGuard does not use a stream transport/security method, but Xray's
		// WireGuard client consumes streamSettings.sockopt for the UDP socket it
		// opens to the peer endpoint.
		if g.outboundInterface != "" {
			outbound["streamSettings"] = g.generateSocketSettings()
		}
	} else {
		outbound["streamSettings"] = g.generateStreamSettings(proxy)
	}

	return outbound
}

func (g *ConfigGenerator) generateStreamSettings(proxy *models.ProxyConfig) map[string]interface{} {
	network := proxy.Type
	if network == "" {
		if proxy.Protocol == "hysteria" {
			network = "hysteria"
		} else {
			network = "tcp"
		}
	}

	security := proxy.Security
	if security == "" {
		if proxy.Protocol == "hysteria" {
			security = "tls"
		} else {
			security = "none"
		}
	}

	ss := map[string]interface{}{
		"network":  network,
		"security": security,
	}
	if g.outboundInterface != "" {
		ss["sockopt"] = g.generateSocketOptions()
	}

	if security == "tls" {
		tlsSettings := map[string]interface{}{
			"serverName": proxy.SNI,
		}
		// xray-core removed "allowInsecure"; accepting a specific (e.g.
		// self-signed) cert is now done via pinnedPeerCertSha256, and a
		// mismatched SNI via verifyPeerCertByName.
		if proxy.PinnedPeerCertSha256 != "" {
			tlsSettings["pinnedPeerCertSha256"] = proxy.PinnedPeerCertSha256
		}
		if proxy.VerifyPeerCertByName != "" {
			tlsSettings["verifyPeerCertByName"] = proxy.VerifyPeerCertByName
		}
		if proxy.AllowInsecure && proxy.PinnedPeerCertSha256 == "" && proxy.VerifyPeerCertByName == "" {
			logger.Debug("proxy %q requested allowInsecure, which xray-core no longer supports; verifying TLS normally (use pinnedPeerCertSha256 to pin a cert)", proxy.Name)
		}
		if proxy.Fingerprint != "" {
			tlsSettings["fingerprint"] = proxy.Fingerprint
		}
		if len(proxy.ALPN) > 0 {
			tlsSettings["alpn"] = proxy.ALPN
		} else if proxy.Protocol == "http" {
			// An https:// forward proxy speaks HTTP/1.1 CONNECT; pin ALPN so the
			// TLS handshake does not negotiate h2, which the http proxy protocol
			// cannot parse ("http2: frame too large").
			tlsSettings["alpn"] = []string{"http/1.1"}
		}
		ss["tlsSettings"] = tlsSettings
	}

	if security == "reality" {
		realitySettings := map[string]interface{}{
			"serverName":  proxy.SNI,
			"fingerprint": proxy.Fingerprint,
			"publicKey":   proxy.PublicKey,
		}
		if proxy.ShortID != "" {
			realitySettings["shortId"] = proxy.ShortID
		}
		ss["realitySettings"] = realitySettings
	}

	switch network {
	case "tcp":
		if proxy.HeaderType != "" && proxy.HeaderType != "none" {
			header := map[string]interface{}{"type": proxy.HeaderType}
			if proxy.HeaderType == "http" {
				header["request"] = map[string]interface{}{
					"path":    []string{proxy.Path},
					"headers": map[string]interface{}{"Host": []string{proxy.Host}},
				}
			}
			ss["tcpSettings"] = map[string]interface{}{"header": header}
		}

	case "ws":
		wsSettings := map[string]interface{}{"path": proxy.Path}
		if proxy.Host != "" {
			wsSettings["headers"] = map[string]interface{}{"Host": proxy.Host}
		}
		ss["wsSettings"] = wsSettings

	case "grpc":
		ss["grpcSettings"] = map[string]interface{}{
			"serviceName": proxy.GetServiceName(),
			"multiMode":   proxy.MultiMode,
		}

	case "kcp", "mkcp":
		if proxy.RawKcpSettings != "" {
			var rawSettings map[string]interface{}
			if err := json.Unmarshal([]byte(proxy.RawKcpSettings), &rawSettings); err == nil {
				// xray-core removed kcp "header" and "seed" (migrated to finalmask).
				// Emitting them aborts the WHOLE config build, taking every proxy
				// down, so drop them — the node degrades to plain mKCP instead.
				delete(rawSettings, "header")
				delete(rawSettings, "seed")
				ss["kcpSettings"] = rawSettings
			}
		}

	case "http", "h2":
		httpSettings := map[string]interface{}{"path": proxy.Path}
		if proxy.Host != "" {
			httpSettings["host"] = strings.Split(proxy.Host, ",")
		}
		ss["httpSettings"] = httpSettings

	case "httpupgrade":
		httpUpgradeSettings := map[string]interface{}{"path": proxy.Path}
		if proxy.Host != "" {
			httpUpgradeSettings["host"] = proxy.Host
		}
		ss["httpupgradeSettings"] = httpUpgradeSettings

	case "splithttp":
		if proxy.RawXhttpSettings != "" {
			var rawSettings map[string]interface{}
			if err := json.Unmarshal([]byte(proxy.RawXhttpSettings), &rawSettings); err == nil {
				ss["splithttpSettings"] = rawSettings
			}
		} else {
			splitSettings := map[string]interface{}{"path": proxy.Path}
			if proxy.Host != "" {
				splitSettings["host"] = proxy.Host
			}
			if proxy.Mode != "" {
				splitSettings["mode"] = proxy.Mode
			}
			ss["splithttpSettings"] = splitSettings
		}

	case "xhttp":
		if proxy.RawXhttpSettings != "" {
			var rawSettings map[string]interface{}
			if err := json.Unmarshal([]byte(proxy.RawXhttpSettings), &rawSettings); err == nil {
				ss["xhttpSettings"] = rawSettings
			}
		} else {
			xhttpSettings := map[string]interface{}{"path": proxy.Path}
			if proxy.Host != "" {
				xhttpSettings["host"] = proxy.Host
			}
			if proxy.Mode != "" {
				xhttpSettings["mode"] = proxy.Mode
			}
			ss["xhttpSettings"] = xhttpSettings
		}

	case "hysteria":
		ss["hysteriaSettings"] = map[string]interface{}{
			"version": 2,
			"auth":    proxy.HysteriaAuth,
		}

		// Build FinalMask with port-hopping (udpHop) and Salamander obfuscation.
		//
		// Congestion/bandwidth control (brutal up/down) is intentionally NOT emitted:
		// it is irrelevant for a connectivity check, and an out-of-range value (e.g. a
		// bare "100" parsed as 100 bytes/s, below xray-core's 65536 minimum) would make
		// xray-core reject the *entire* config and bring every proxy down. Only obfs and
		// port-hopping affect whether the proxy is reachable, so only those are kept.
		var finalMask map[string]interface{}

		// Port-hopping (matches libXray output: finalmask.quicParams.udpHop.ports)
		if proxy.HysteriaPorts != "" {
			udpHop := map[string]interface{}{
				"ports": proxy.HysteriaPorts,
			}
			if proxy.HysteriaHopInterval > 0 {
				// xray-core's UdpHop.Interval is an Int32Range, decoded from a plain
				// integer (or a "min-max" string) — NOT an object.
				udpHop["interval"] = proxy.HysteriaHopInterval
			}
			finalMask = map[string]interface{}{
				"quicParams": map[string]interface{}{
					"udpHop": udpHop,
				},
			}
		}

		// Salamander obfuscation (matches libXray output: finalmask.udp[].salamander)
		if proxy.HysteriaObfs == "salamander" && proxy.HysteriaObfsPassword != "" {
			if finalMask == nil {
				finalMask = map[string]interface{}{}
			}
			finalMask["udp"] = []map[string]interface{}{
				{
					"type": "salamander",
					"settings": map[string]interface{}{
						"password": proxy.HysteriaObfsPassword,
					},
				},
			}
		}

		// xray-core reads finalmask at the streamSettings top level (json:"finalmask"),
		// NOT under sockopt (SocketConfig has no such field, so it would be dropped).
		if finalMask != nil {
			ss["finalmask"] = finalMask
		}
	}

	return ss
}

func (g *ConfigGenerator) generateSocketSettings() map[string]interface{} {
	return map[string]interface{}{
		"sockopt": g.generateSocketOptions(),
	}
}

func (g *ConfigGenerator) generateSocketOptions() map[string]interface{} {
	return map[string]interface{}{
		"interface": g.outboundInterface,
	}
}

func (g *ConfigGenerator) generateRouting(proxies []*models.ProxyConfig) map[string]interface{} {
	var rules []map[string]interface{}

	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"protocol":    []string{"dns"},
		"outboundTag": "dns-out",
	})

	for _, proxy := range proxies {
		inboundTag := fmt.Sprintf("%s_%s_%d_Inbound", proxy.Name, proxy.Protocol, proxy.Index)
		outboundTag := fmt.Sprintf("%s_%d", proxy.Name, proxy.Index)

		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"inboundTag":  []string{inboundTag},
			"outboundTag": outboundTag,
		})
	}

	return map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules":          rules,
	}
}
