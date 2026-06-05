package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/models"
	"xray-checker/xray"
)

//go:embed openapi.yaml
var openAPISpec []byte

type ProxyInfo struct {
	Index     int             `json:"index"`
	StableID  string          `json:"stableId"`
	Name      string          `json:"name"`
	SubName   string          `json:"subName"`
	Group     *ProxyGroupInfo `json:"group,omitempty"`
	Server    string          `json:"server"`
	Port      int             `json:"port"`
	Protocol  string          `json:"protocol"`
	ProxyPort int             `json:"proxyPort"`
	Online    bool            `json:"online"`
	LatencyMs int64           `json:"latencyMs"`
	Details   *ProxyDetails   `json:"details,omitempty"`
}

type PublicProxyInfo struct {
	StableID  string `json:"stableId"`
	Name      string `json:"name"`
	Online    bool   `json:"online"`
	LatencyMs int64  `json:"latencyMs"`
}

type ProxyGroupInfo struct {
	Key       string `json:"key,omitempty"`
	Name      string `json:"name,omitempty"`
	Index     int    `json:"index"`
	Size      int    `json:"size"`
	Collapsed bool   `json:"collapsed"`
}

type ProxyCredentialInfo struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

type ProxyInboundInfo struct {
	Listen   string `json:"listen"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Tag      string `json:"tag"`
}

type ProxyOutboundInfo struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
}

type ProxyDetails struct {
	Protocol         string                 `json:"protocol"`
	Address          string                 `json:"address"`
	Port             int                    `json:"port"`
	Transport        string                 `json:"transport"`
	Security         string                 `json:"security"`
	SNI              string                 `json:"sni,omitempty"`
	Host             string                 `json:"host,omitempty"`
	Path             string                 `json:"path,omitempty"`
	Mode             string                 `json:"mode,omitempty"`
	Flow             string                 `json:"flow,omitempty"`
	Encryption       string                 `json:"encryption,omitempty"`
	HeaderType       string                 `json:"headerType,omitempty"`
	Fingerprint      string                 `json:"fingerprint,omitempty"`
	PublicKey        string                 `json:"publicKey,omitempty"`
	ShortID          string                 `json:"shortId,omitempty"`
	ServiceName      string                 `json:"serviceName,omitempty"`
	MultiMode        bool                   `json:"multiMode,omitempty"`
	AllowInsecure    bool                   `json:"allowInsecure,omitempty"`
	ALPN             []string               `json:"alpn,omitempty"`
	Credential       ProxyCredentialInfo    `json:"credential,omitempty"`
	Inbound          ProxyInboundInfo       `json:"inbound"`
	Outbound         ProxyOutboundInfo      `json:"outbound"`
	RawXhttpSettings map[string]interface{} `json:"rawXhttpSettings,omitempty"`
	GeneratedConfig  map[string]interface{} `json:"generatedConfig,omitempty"`
}

type StatusResponse struct {
	Total        int   `json:"total"`
	Online       int   `json:"online"`
	Offline      int   `json:"offline"`
	AvgLatencyMs int64 `json:"avgLatencyMs"`
}

type ConfigResponse struct {
	CheckInterval              int      `json:"checkInterval"`
	CheckMethod                string   `json:"checkMethod"`
	Timeout                    int      `json:"timeout"`
	StartPort                  int      `json:"startPort"`
	SubscriptionUpdate         bool     `json:"subscriptionUpdate"`
	SubscriptionUpdateInterval int      `json:"subscriptionUpdateInterval"`
	SimulateLatency            bool     `json:"simulateLatency"`
	SubscriptionNames          []string `json:"subscriptionNames"`
}

type SystemInfoResponse struct {
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	UptimeSec int64  `json:"uptimeSec"`
	Instance  string `json:"instance"`
}

type SystemIPResponse struct {
	IP string `json:"ip"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}

func toProxyInfo(proxy *models.ProxyConfig, online bool, latency time.Duration, startPort int, includeDetails bool) ProxyInfo {
	info := ProxyInfo{
		Index:     proxy.Index,
		StableID:  proxy.StableID,
		Name:      proxy.Name,
		SubName:   proxy.SubName,
		Group:     toProxyGroupInfo(proxy),
		Server:    proxy.Server,
		Port:      proxy.Port,
		Protocol:  proxy.Protocol,
		ProxyPort: startPort + proxy.Index,
		Online:    online,
		LatencyMs: latency.Milliseconds(),
	}
	if includeDetails {
		info.Details = toProxyDetails(proxy, startPort)
	}
	return info
}

func toProxyGroupInfo(proxy *models.ProxyConfig) *ProxyGroupInfo {
	if proxy.GroupName == "" || proxy.GroupSize <= 1 {
		return nil
	}

	keyBytes, _ := json.Marshal([]string{proxy.SubName, proxy.GroupName})
	return &ProxyGroupInfo{
		Key:       string(keyBytes),
		Name:      proxy.GroupName,
		Index:     proxy.GroupIndex,
		Size:      proxy.GroupSize,
		Collapsed: true,
	}
}

func toProxyDetails(proxy *models.ProxyConfig, startPort int) *ProxyDetails {
	details := &ProxyDetails{
		Protocol:      proxy.Protocol,
		Address:       proxy.Server,
		Port:          proxy.Port,
		Transport:     proxy.GetTransportType(),
		Security:      proxy.GetSecurityType(),
		SNI:           proxy.SNI,
		Host:          proxy.Host,
		Path:          proxy.Path,
		Mode:          proxy.Mode,
		Flow:          proxy.Flow,
		Encryption:    proxy.Encryption,
		HeaderType:    proxy.HeaderType,
		Fingerprint:   proxy.Fingerprint,
		PublicKey:     proxy.PublicKey,
		ShortID:       proxy.ShortID,
		ServiceName:   proxy.ServiceName,
		MultiMode:     proxy.MultiMode,
		AllowInsecure: proxy.AllowInsecure,
		ALPN:          proxy.ALPN,
		Inbound: ProxyInboundInfo{
			Listen:   "127.0.0.1",
			Port:     startPort + proxy.Index,
			Protocol: "socks",
			Tag:      fmt.Sprintf("%s_%s_%d_Inbound", proxy.Name, proxy.Protocol, proxy.Index),
		},
		Outbound: ProxyOutboundInfo{
			Tag:      fmt.Sprintf("%s_%d", proxy.Name, proxy.Index),
			Protocol: proxy.Protocol,
			Address:  proxy.Server,
			Port:     proxy.Port,
		},
	}

	switch proxy.Protocol {
	case "vless", "vmess":
		details.Credential = ProxyCredentialInfo{Type: "uuid", Value: maskMiddle(proxy.UUID)}
	case "trojan", "shadowsocks":
		details.Credential = ProxyCredentialInfo{Type: "password", Value: maskMiddle(proxy.Password)}
	}

	if proxy.Protocol == "shadowsocks" && proxy.Method != "" {
		details.Credential.Type = "method/password"
		details.Credential.Value = proxy.Method + " / " + maskMiddle(proxy.Password)
	}

	if proxy.RawXhttpSettings != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(proxy.RawXhttpSettings), &raw); err == nil {
			details.RawXhttpSettings = raw
		}
	}

	details.GeneratedConfig = sanitizeGeneratedConfig(map[string]interface{}{
		"inbound":     xray.GenerateProxyInbound(proxy, startPort),
		"outbound":    xray.GenerateProxyOutbound(proxy),
		"routingRule": xray.GenerateProxyRoutingRule(proxy),
	})

	return details
}

func sanitizeGeneratedConfig(value interface{}) map[string]interface{} {
	sanitized, ok := sanitizeGeneratedValue(value).(map[string]interface{})
	if !ok {
		return nil
	}
	return sanitized
}

func sanitizeGeneratedValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			switch strings.ToLower(key) {
			case "id", "password":
				if text, ok := nested.(string); ok {
					result[key] = maskMiddle(text)
					continue
				}
			}
			result[key] = sanitizeGeneratedValue(nested)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for i, nested := range typed {
			result[i] = sanitizeGeneratedValue(nested)
		}
		return result
	case []map[string]interface{}:
		result := make([]interface{}, len(typed))
		for i, nested := range typed {
			result[i] = sanitizeGeneratedValue(nested)
		}
		return result
	case []string:
		result := make([]interface{}, len(typed))
		for i, nested := range typed {
			result[i] = nested
		}
		return result
	default:
		return value
	}
}

func maskMiddle(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

// APIPublicProxiesHandler returns public info for all proxies (no auth required)
// @Summary List all proxies (public)
// @Description Returns a list of all proxies with status (no sensitive data, no auth)
// @Tags public
// @Produce json
// @Success 200 {array} PublicProxyInfo
// @Router /api/v1/public/proxies [get]
func APIPublicProxiesHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxies := proxyChecker.GetProxies()
		result := make([]PublicProxyInfo, 0, len(proxies))

		for _, proxy := range proxies {
			status, latency, _ := proxyChecker.GetProxyStatus(proxy)
			result = append(result, PublicProxyInfo{
				StableID:  proxy.StableID,
				Name:      proxy.Name,
				Online:    status,
				LatencyMs: latency.Milliseconds(),
			})
		}

		writeJSON(w, result)
	}
}

// APIProxiesHandler returns info for all proxies
// @Summary List all proxies
// @Description Returns a list of all proxies with status information
// @Tags proxies
// @Produce json
// @Success 200 {array} ProxyInfo
// @Router /api/v1/proxies [get]
func APIProxiesHandler(proxyChecker *checker.ProxyChecker, startPort int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxies := proxyChecker.GetProxies()
		result := make([]ProxyInfo, 0, len(proxies))
		includeDetails := config.CLIConfig.Web.ShowServerDetails

		for _, proxy := range proxies {
			status, latency, _ := proxyChecker.GetProxyStatus(proxy)
			result = append(result, toProxyInfo(proxy, status, latency, startPort, includeDetails))
		}

		writeJSON(w, result)
	}
}

// APIProxyHandler returns info for a single proxy
// @Summary Get proxy by ID
// @Description Returns information for a specific proxy
// @Tags proxies
// @Produce json
// @Param stableID path string true "Proxy Stable ID"
// @Success 200 {object} ProxyInfo
// @Failure 404 {object} map[string]string
// @Router /api/v1/proxies/{stableID} [get]
func APIProxyHandler(proxyChecker *checker.ProxyChecker, startPort int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		prefix := "/api/v1/proxies/"
		if !strings.HasPrefix(path, prefix) {
			writeError(w, "Invalid path", http.StatusBadRequest)
			return
		}

		stableID := strings.TrimPrefix(path, prefix)
		if stableID == "" {
			writeError(w, "Proxy ID is required", http.StatusBadRequest)
			return
		}

		proxy, exists := proxyChecker.GetProxyByStableID(stableID)
		if !exists {
			writeError(w, "Proxy not found", http.StatusNotFound)
			return
		}

		status, latency, _ := proxyChecker.GetProxyStatus(proxy)
		writeJSON(w, toProxyInfo(proxy, status, latency, startPort, config.CLIConfig.Web.ShowServerDetails))
	}
}

// APIStatusHandler returns system status summary
// @Summary Get system status
// @Description Returns summary statistics about all proxies
// @Tags status
// @Produce json
// @Success 200 {object} StatusResponse
// @Router /api/v1/status [get]
func APIStatusHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxies := proxyChecker.GetProxies()

		var online, offline int
		var totalLatency int64
		var latencyCount int

		for _, proxy := range proxies {
			status, latency, _ := proxyChecker.GetProxyStatus(proxy)
			if status {
				online++
				if latency > 0 {
					totalLatency += latency.Milliseconds()
					latencyCount++
				}
			} else {
				offline++
			}
		}

		var avgLatency int64
		if latencyCount > 0 {
			avgLatency = totalLatency / int64(latencyCount)
		}

		writeJSON(w, StatusResponse{
			Total:        len(proxies),
			Online:       online,
			Offline:      offline,
			AvgLatencyMs: avgLatency,
		})
	}
}

// APIConfigHandler returns current configuration
// @Summary Get current configuration
// @Description Returns the current checker configuration
// @Tags config
// @Produce json
// @Success 200 {object} ConfigResponse
// @Router /api/v1/config [get]
func APIConfigHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subNames := CollectSubscriptionNames(proxyChecker.GetProxies())
		writeJSON(w, ConfigResponse{
			CheckInterval:              config.CLIConfig.Proxy.CheckInterval,
			CheckMethod:                config.CLIConfig.Proxy.CheckMethod,
			Timeout:                    config.CLIConfig.Proxy.Timeout,
			StartPort:                  config.CLIConfig.Xray.StartPort,
			SubscriptionUpdate:         config.CLIConfig.Subscription.Update,
			SubscriptionUpdateInterval: config.CLIConfig.Subscription.UpdateInterval,
			SimulateLatency:            config.CLIConfig.Proxy.SimulateLatency,
			SubscriptionNames:          subNames,
		})
	}
}

func CollectSubscriptionNames(proxies []*models.ProxyConfig) []string {
	seen := make(map[string]bool)
	var names []string
	for _, proxy := range proxies {
		if proxy.SubName != "" && !seen[proxy.SubName] {
			seen[proxy.SubName] = true
			names = append(names, proxy.SubName)
		}
	}
	return names
}

// APISystemInfoHandler returns system info
// @Summary Get system info
// @Description Returns version, uptime, and instance information
// @Tags system
// @Produce json
// @Success 200 {object} SystemInfoResponse
// @Router /api/v1/system/info [get]
func APISystemInfoHandler(version string, startTime time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime)
		writeJSON(w, SystemInfoResponse{
			Version:   version,
			Uptime:    formatDuration(uptime),
			UptimeSec: int64(uptime.Seconds()),
			Instance:  config.CLIConfig.Metrics.Instance,
		})
	}
}

// APISystemIPHandler returns current IP
// @Summary Get current IP
// @Description Returns the current detected IP address
// @Tags system
// @Produce json
// @Success 200 {object} SystemIPResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/system/ip [get]
func APISystemIPHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, err := proxyChecker.GetCurrentIP()
		if err != nil {
			writeError(w, "Failed to get IP", http.StatusInternalServerError)
			return
		}
		writeJSON(w, SystemIPResponse{IP: ip})
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func APIOpenAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(openAPISpec)
	}
}

func APIDocsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(swaggerUIHTML))
	}
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Xray Checker API</title>
  <style>
    body { margin: 0; padding: 0; }
    .swagger-ui .topbar { display: none; }
  </style>
  <script>
    // Detect base path from current URL (e.g., /xray/api/v1/docs -> /xray)
    (function() {
      const path = window.location.pathname;
      const apiIdx = path.indexOf('/api/v1/docs');
      const basePath = apiIdx > 0 ? path.substring(0, apiIdx) : '';
      document.write('<link rel="stylesheet" href="' + basePath + '/static/swagger-ui.css">');
    })();
  </script>
</head>
<body>
  <div id="swagger-ui"></div>
  <script>
    (function() {
      const path = window.location.pathname;
      const apiIdx = path.indexOf('/api/v1/docs');
      const basePath = apiIdx > 0 ? path.substring(0, apiIdx) : '';

      const script = document.createElement('script');
      script.src = basePath + '/static/swagger-ui-bundle.js';
      script.onload = function() {
        SwaggerUIBundle({
          url: './openapi.yaml',
          dom_id: '#swagger-ui',
          presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
          layout: 'BaseLayout'
        });
      };
      document.body.appendChild(script);
    })();
  </script>
</body>
</html>`
