package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"xray-checker/models"

	"gopkg.in/yaml.v3"
)

// isMihomoSubscription prevents an accidental Mihomo input from being passed to
// the share-link parser, whose diagnostics may contain parts of the input.
func isMihomoSubscription(data []byte) bool {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err == nil {
		if root := mihomoRoot(&document); root != nil {
			if mihomoField(root, "proxies") != nil || mihomoField(root, "proxy-providers") != nil {
				return true
			}
		}
	}
	// Also identify a malformed block-style document without quoting its values.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "proxies:") || strings.HasPrefix(line, "proxy-providers:") {
			return true
		}
	}
	return false
}

func mihomoRoot(document *yaml.Node) *yaml.Node {
	if document.Kind == yaml.DocumentNode && len(document.Content) == 1 && document.Content[0].Kind == yaml.MappingNode {
		return document.Content[0]
	}
	return nil
}

func mihomoField(root *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

// parseMihomoSubscription accepts full Mihomo subscriptions and proxy-provider
// documents with an inline proxies list. Only individual proxy definitions are
// retained. Subscription rules, groups, listeners, controllers, DNS, and TUN
// settings must never become checker process settings.
func parseMihomoSubscription(data []byte) ([]*models.ProxyConfig, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid Mihomo YAML/JSON subscription")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("Mihomo subscription must contain one document")
	}
	root := mihomoRoot(&document)
	if root == nil {
		return nil, fmt.Errorf("Mihomo subscription requires a top-level proxies list")
	}
	// Reject duplicate top-level fields before choosing one of them. In
	// particular, silently choosing between two proxies lists could omit nodes.
	seen := make(map[string]bool)
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
			return nil, fmt.Errorf("Mihomo subscription has invalid or duplicate top-level fields")
		}
		seen[key.Value] = true
	}
	list := mihomoField(root, "proxies")
	if list == nil || list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
		if mihomoField(root, "proxy-providers") != nil {
			return nil, fmt.Errorf("Mihomo proxy-providers are not fetched; provide an inline proxies list")
		}
		return nil, fmt.Errorf("Mihomo subscription requires a nonempty top-level proxies list")
	}

	configs := make([]*models.ProxyConfig, 0, len(list.Content))
	for index, item := range list.Content {
		var definition map[string]any
		if (item.Kind != yaml.MappingNode && item.Kind != yaml.AliasNode) || item.Decode(&definition) != nil || len(definition) == 0 {
			return nil, fmt.Errorf("Mihomo proxy %d must be a mapping with unique string keys", index+1)
		}
		if _, err := json.Marshal(definition); err != nil {
			return nil, fmt.Errorf("Mihomo proxy %d contains unsupported YAML values", index+1)
		}
		pc, err := mihomoProxyMetadata(definition, index)
		if err != nil {
			return nil, fmt.Errorf("Mihomo proxy %d: %s", index+1, err)
		}
		configs = append(configs, pc)
	}
	return configs, nil
}

func mihomoString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func mihomoBool(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func mihomoInt(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case int64:
		return int(value), int64(int(value)) == value
	case uint64:
		return int(value), value <= uint64(^uint(0)>>1)
	case float64:
		if value < 0 || value > 65535 || math.Trunc(value) != value {
			return 0, false
		}
		return int(value), true
	case string:
		number, err := strconv.Atoi(value)
		return number, err == nil
	default:
		return 0, false
	}
}

func mihomoProxyMetadata(definition map[string]any, index int) (*models.ProxyConfig, error) {
	name := mihomoString(definition, "name")
	protocol := mihomoString(definition, "type")
	server := mihomoString(definition, "server")
	port, validPort := mihomoInt(definition["port"])
	if strings.TrimSpace(name) == "" || strings.TrimSpace(protocol) == "" || strings.TrimSpace(server) == "" {
		return nil, fmt.Errorf("name, type, and server must be nonempty strings")
	}
	if !validPort || port < 1 || port > 65535 {
		return nil, fmt.Errorf("port must be an integer from 1 through 65535")
	}
	pc := &models.ProxyConfig{
		Name: name, Protocol: protocol, Server: server, Port: port, Index: index,
		Mihomo: definition, Type: mihomoString(definition, "network"),
		UUID: mihomoString(definition, "uuid"), Flow: mihomoString(definition, "flow"),
		Username: mihomoString(definition, "username"), Password: mihomoString(definition, "password"),
		Method: mihomoString(definition, "cipher"), Fingerprint: mihomoString(definition, "client-fingerprint"),
		SNI: mihomoString(definition, "servername"), AllowInsecure: mihomoBool(definition, "skip-cert-verify"),
	}
	if pc.Type == "" {
		pc.Type = "tcp"
	}
	if pc.SNI == "" {
		pc.SNI = mihomoString(definition, "sni")
	}
	switch protocol {
	case "ss":
		pc.Protocol = "shadowsocks"
	case "socks5":
		pc.Protocol = "socks"
	case "hysteria2", "hy2":
		pc.Protocol = "hysteria"
		pc.HysteriaAuth = pc.Password
	}
	pc.AlterId, _ = mihomoInt(definition["alterId"])
	if mihomoBool(definition, "tls") {
		pc.Security = "tls"
	}
	if reality, ok := definition["reality-opts"].(map[string]any); ok {
		pc.Security = "reality"
		pc.PublicKey = mihomoString(reality, "public-key")
		pc.ShortID = mihomoString(reality, "short-id")
	}
	if alpn, ok := definition["alpn"].([]any); ok {
		for _, item := range alpn {
			if value, ok := item.(string); ok {
				pc.ALPN = append(pc.ALPN, value)
			}
		}
	}
	for _, key := range []string{"ws-opts", "http-opts", "h2-opts", "http-upgrade-opts", "xhttp-opts"} {
		if opts, ok := definition[key].(map[string]any); ok {
			pc.Path = mihomoString(opts, "path")
			pc.Host = mihomoString(opts, "host")
			pc.Mode = mihomoString(opts, "mode")
		}
	}
	if grpc, ok := definition["grpc-opts"].(map[string]any); ok {
		pc.ServiceName = mihomoString(grpc, "grpc-service-name")
	}
	return pc, nil
}
