package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"xray-checker/models"
)

func ParseProxyURL(proxyURL string) (*models.ProxyConfig, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return nil, fmt.Errorf("empty proxy URL")
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing proxy URL: %v", err)
	}

	if u.Scheme == "" {
		return nil, fmt.Errorf("protocol is missing in URL: %s", proxyURL)
	}

	switch u.Scheme {
	case "vless":
		return ParseVLESSConfig(u)
	case "vmess":
		return ParseVMessConfig(u)
	case "trojan":
		return ParseTrojanConfig(u)
	case "ss":
		return ParseShadowsocksConfig(u)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", u.Scheme)
	}
}

func ParseVLESSConfig(u *url.URL) (*models.ProxyConfig, error) {
	config := &models.ProxyConfig{
		Protocol: "vless",
		Name:     strings.TrimPrefix(u.Fragment, ""),
		Settings: make(map[string]string),
	}

	config.UUID = u.User.Username()

	hostParts := strings.Split(u.Host, ":")
	if len(hostParts) != 2 {
		return nil, fmt.Errorf("invalid server address format: %s", u.Host)
	}

	config.Server = hostParts[0]
	if _, err := fmt.Sscanf(hostParts[1], "%d", &config.Port); err != nil {
		return nil, fmt.Errorf("invalid port number: %v", err)
	}

	if config.Port == 0 || config.Port == 1 {
		return nil, fmt.Errorf("skipping port: %d", config.Port)
	}

	query := u.Query()

	config.Security = query.Get("security")
	config.Type = query.Get("type")
	config.Flow = query.Get("flow")

	config.HeaderType = query.Get("headerType")
	config.Path = query.Get("path")
	config.Host = query.Get("host")

	config.SNI = query.Get("sni")
	config.Fingerprint = query.Get("fp")
	config.PublicKey = query.Get("pbk")
	config.ShortID = query.Get("sid")

	if config.Type == "xhttp" {
		config.Mode = query.Get("mode")
		config.ExtraXhttp = query.Get("extra")
	}

	if config.Type == "grpc" {
		config.ServiceName = query.Get("serviceName")
		config.MultiMode = query.Get("multiMode") == "true"
		if idleTimeout := query.Get("idleTimeout"); idleTimeout != "" {
			if timeout, err := strconv.Atoi(idleTimeout); err == nil {
				config.IdleTimeout = timeout
			}
		}
		if windowSize := query.Get("windowSize"); windowSize != "" {
			if size, err := strconv.Atoi(windowSize); err == nil {
				config.WindowsSize = size
			}
		}
	}

	config.AllowInsecure = query.Get("allowInsecure") == "true"
	if alpn := query.Get("alpn"); alpn != "" {
		config.ALPN = strings.Split(alpn, ",")
	}

	if level := query.Get("level"); level != "" {
		if l, err := strconv.Atoi(level); err == nil {
			config.Level = l
		}
	}

	for k, v := range query {
		if len(v) > 0 {
			config.Settings[k] = v[0]
		}
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func ParseVMessConfig(u *url.URL) (*models.ProxyConfig, error) {
	vmessStr := strings.TrimPrefix(u.String(), "vmess://")
	decoded, err := base64.RawStdEncoding.DecodeString(vmessStr)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(vmessStr)
		if err != nil {
			return nil, fmt.Errorf("error decoding VMess link: %v", err)
		}
	}

	var vmessConfig map[string]interface{}
	if err := json.Unmarshal(decoded, &vmessConfig); err != nil {
		return nil, fmt.Errorf("error parsing VMess config: %v", err)
	}

	config := &models.ProxyConfig{
		Protocol: "vmess",
		Settings: make(map[string]string),
	}

	if ps, ok := vmessConfig["ps"].(string); ok {
		config.Name = ps
	}
	if add, ok := vmessConfig["add"].(string); ok {
		config.Server = add
	}
	if port, ok := vmessConfig["port"].(float64); ok {
		config.Port = int(port)
	}
	if id, ok := vmessConfig["id"].(string); ok {
		config.UUID = id
	}
	if aid, ok := vmessConfig["aid"].(float64); ok {
		config.VMessAid = int(aid)
	}
	if net, ok := vmessConfig["net"].(string); ok {
		config.Type = net
	}
	if host, ok := vmessConfig["host"].(string); ok {
		config.Host = host
	}
	if path, ok := vmessConfig["path"].(string); ok {
		config.Path = path
	}

	if tls, ok := vmessConfig["tls"].(string); ok && tls == "tls" {
		config.Security = "tls"
		if sni, ok := vmessConfig["sni"].(string); ok {
			config.SNI = sni
		}
		if fp, ok := vmessConfig["fp"].(string); ok {
			config.Fingerprint = fp
		} else {
			config.Fingerprint = "chrome"
		}
		if alpn, ok := vmessConfig["alpn"].(string); ok {
			config.ALPN = strings.Split(alpn, ",")
		}
	}

	if config.Port == 0 || config.Port == 1 {
		return nil, fmt.Errorf("skipping port: %d", config.Port)
	}

	if config.Type == "grpc" {
		if svcName, ok := vmessConfig["serviceName"].(string); ok {
			config.ServiceName = svcName
		}
		if multiMode, ok := vmessConfig["multiMode"].(bool); ok {
			config.MultiMode = multiMode
		}
		if timeout, ok := vmessConfig["idle_timeout"].(float64); ok {
			config.IdleTimeout = int(timeout)
		}
		if size, ok := vmessConfig["initial_windows_size"].(float64); ok {
			config.WindowsSize = int(size)
		}
	}

	if level, ok := vmessConfig["level"].(float64); ok {
		config.Level = int(level)
	}

	for k, v := range vmessConfig {
		if str, ok := v.(string); ok {
			config.Settings[k] = str
		}
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func ParseTrojanConfig(u *url.URL) (*models.ProxyConfig, error) {
	config := &models.ProxyConfig{
		Protocol: "trojan",
		Name:     strings.TrimPrefix(u.Fragment, ""),
		Settings: make(map[string]string),
	}

	config.Password = u.User.Username()

	hostParts := strings.Split(u.Host, ":")
	if len(hostParts) != 2 {
		return nil, fmt.Errorf("invalid server address format: %s", u.Host)
	}

	config.Server = hostParts[0]
	if _, err := fmt.Sscanf(hostParts[1], "%d", &config.Port); err != nil {
		return nil, fmt.Errorf("invalid port number: %v", err)
	}

	if config.Port == 0 || config.Port == 1 {
		return nil, fmt.Errorf("skipping port: %d", config.Port)
	}

	query := u.Query()

	config.Security = query.Get("security")
	config.Type = query.Get("type")
	config.Flow = query.Get("flow")

	config.Path = query.Get("path")
	config.Host = query.Get("host")

	config.SNI = query.Get("sni")
	config.Fingerprint = query.Get("fp")
	config.AllowInsecure = query.Get("allowInsecure") == "true"
	if alpn := query.Get("alpn"); alpn != "" {
		config.ALPN = strings.Split(alpn, ",")
	}

	if config.Type == "grpc" {
		config.ServiceName = query.Get("serviceName")
		config.MultiMode = query.Get("multiMode") == "true"
		if idleTimeout := query.Get("idleTimeout"); idleTimeout != "" {
			if timeout, err := strconv.Atoi(idleTimeout); err == nil {
				config.IdleTimeout = timeout
			}
		}
		if windowSize := query.Get("windowSize"); windowSize != "" {
			if size, err := strconv.Atoi(windowSize); err == nil {
				config.WindowsSize = size
			}
		}
	}

	for k, v := range query {
		if len(v) > 0 {
			config.Settings[k] = v[0]
		}
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func ParseShadowsocksConfig(u *url.URL) (*models.ProxyConfig, error) {
	config := &models.ProxyConfig{
		Protocol: "shadowsocks",
		Name:     strings.TrimPrefix(u.Fragment, ""),
		Settings: make(map[string]string),
	}

	methodPass, err := base64.RawURLEncoding.DecodeString(u.User.String())
	if err != nil {
		methodPass, err = base64.RawStdEncoding.DecodeString(u.User.String())
		if err != nil {
			return nil, fmt.Errorf("error decoding method and password: %v", err)
		}
	}

	parts := strings.SplitN(string(methodPass), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid method:password format")
	}

	config.Method = parts[0]
	config.Password = parts[1]

	hostParts := strings.Split(u.Host, ":")
	if len(hostParts) != 2 {
		return nil, fmt.Errorf("invalid server address format: %s", u.Host)
	}

	config.Server = hostParts[0]
	if _, err := fmt.Sscanf(hostParts[1], "%d", &config.Port); err != nil {
		return nil, fmt.Errorf("invalid port number: %v", err)
	}

	if config.Port == 0 || config.Port == 1 {
		return nil, fmt.Errorf("skipping port: %d", config.Port)
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}
