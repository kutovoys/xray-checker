package xray

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"xray-checker/logger"
	"xray-checker/models"
)

type ConfigGenerator struct{}

func NewConfigGenerator() *ConfigGenerator {
	return &ConfigGenerator{}
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

func (g *ConfigGenerator) GenerateAndSaveConfig(proxies []*models.ProxyConfig, startPort int, filename string, xrayLogLevel string) error {
	configBytes, err := g.GenerateConfig(proxies, startPort, xrayLogLevel)
	if err != nil {
		return fmt.Errorf("error generating config: %v", err)
	}

	if err := g.ValidateConfig(configBytes); err != nil {
		logger.Warn("Config validation failed: %v", err)
	}

	if err := os.WriteFile(filename, configBytes, 0644); err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}

	return nil
}

func (g *ConfigGenerator) ValidateConfig(configBytes []byte) error {
	var config map[string]interface{}
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	required := []string{"inbounds", "outbounds", "routing"}
	for _, field := range required {
		if _, ok := config[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}

func (g *ConfigGenerator) generateInbounds(proxies []*models.ProxyConfig, startPort int) []map[string]interface{} {
	var inbounds []map[string]interface{}

	for _, proxy := range proxies {
		inbounds = append(inbounds, GenerateProxyInbound(proxy, startPort))
	}

	return inbounds
}

func GenerateProxyInbound(proxy *models.ProxyConfig, startPort int) map[string]interface{} {
	return map[string]interface{}{
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
	}

	outbound["streamSettings"] = g.generateStreamSettings(proxy)

	return outbound
}

func GenerateProxyOutbound(proxy *models.ProxyConfig) map[string]interface{} {
	return NewConfigGenerator().generateProxyOutbound(proxy)
}

func (g *ConfigGenerator) generateStreamSettings(proxy *models.ProxyConfig) map[string]interface{} {
	network := proxy.Type
	if network == "" {
		network = "tcp"
	}

	security := proxy.Security
	if security == "" {
		security = "none"
	}

	ss := map[string]interface{}{
		"network":  network,
		"security": security,
		"sockopt":  map[string]interface{}{},
	}

	if security == "tls" {
		tlsSettings := map[string]interface{}{
			"serverName":    proxy.SNI,
			"allowInsecure": proxy.AllowInsecure,
		}
		if proxy.Fingerprint != "" {
			tlsSettings["fingerprint"] = proxy.Fingerprint
		}
		if len(proxy.ALPN) > 0 {
			tlsSettings["alpn"] = proxy.ALPN
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

	case "kcp":
		if proxy.RawKcpSettings != "" {
			var rawSettings map[string]interface{}
			if err := json.Unmarshal([]byte(proxy.RawKcpSettings), &rawSettings); err == nil {
				ss["kcpSettings"] = rawSettings
			}
		}
		if proxy.RawFinalMask != "" {
			var rawFinalMask map[string]interface{}
			if err := json.Unmarshal([]byte(proxy.RawFinalMask), &rawFinalMask); err == nil {
				ss["finalmask"] = rawFinalMask
			}
		}

	case "grpc":
		ss["grpcSettings"] = map[string]interface{}{
			"serviceName": proxy.GetServiceName(),
			"multiMode":   proxy.MultiMode,
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
	}

	return ss
}

func (g *ConfigGenerator) generateRouting(proxies []*models.ProxyConfig) map[string]interface{} {
	var rules []map[string]interface{}

	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"protocol":    []string{"dns"},
		"outboundTag": "dns-out",
	})

	for _, proxy := range proxies {
		rules = append(rules, GenerateProxyRoutingRule(proxy))
	}

	return map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules":          rules,
	}
}

func GenerateProxyRoutingRule(proxy *models.ProxyConfig) map[string]interface{} {
	inboundTag := fmt.Sprintf("%s_%s_%d_Inbound", proxy.Name, proxy.Protocol, proxy.Index)
	outboundTag := fmt.Sprintf("%s_%d", proxy.Name, proxy.Index)

	return map[string]interface{}{
		"type":        "field",
		"inboundTag":  []string{inboundTag},
		"outboundTag": outboundTag,
	}
}
