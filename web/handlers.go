package web

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/metrics"
	"xray-checker/models"
	"xray-checker/subscription"
)

var (
	registeredEndpoints []EndpointInfo
	endpointsMu         sync.RWMutex
)

type EndpointInfo struct {
	Name         string          `json:"name"`
	ServerInfo   string          `json:"serverInfo,omitempty"`
	URL          string          `json:"url,omitempty"`
	ProxyPort    int             `json:"proxyPort,omitempty"`
	Index        int             `json:"index"`
	Status       bool            `json:"status"`
	Latency      time.Duration   `json:"-"`
	LatencyLabel string          `json:"latency"`
	LatencyMs    int64           `json:"latencyMs"`
	StableID     string          `json:"stableId"`
	SubName      string          `json:"subName,omitempty"`
	Group        *ProxyGroupInfo `json:"group,omitempty"`
	Details      *ProxyDetails   `json:"details,omitempty"`
}

func IndexHandler(version string, proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		RegisterConfigEndpoints(proxyChecker.GetProxies(), proxyChecker, config.CLIConfig.Xray.StartPort)

		endpointsMu.RLock()
		allEndpoints := make([]EndpointInfo, len(registeredEndpoints))
		copy(allEndpoints, registeredEndpoints)
		endpointsMu.RUnlock()

		isPublic := config.CLIConfig.Web.Public
		showServerDetails := shouldShowServerDetails(
			config.CLIConfig.Web.ShowServerDetails,
			isPublic,
			config.CLIConfig.Web.TrustedExternalAuth,
		)

		endpoints := make([]EndpointInfo, len(allEndpoints))
		for i, ep := range allEndpoints {
			endpoints[i] = EndpointInfo{
				Name:         ep.Name,
				Index:        ep.Index,
				Status:       ep.Status,
				Latency:      ep.Latency,
				LatencyLabel: ep.LatencyLabel,
				LatencyMs:    ep.LatencyMs,
				StableID:     ep.StableID,
				SubName:      ep.SubName,
				Group:        ep.Group,
			}
			if !isPublic {
				endpoints[i].URL = ep.URL
			}
			if showServerDetails {
				endpoints[i].ServerInfo = ep.ServerInfo
				endpoints[i].ProxyPort = ep.ProxyPort
				endpoints[i].Details = ep.Details
			}
		}

		data := PageData{
			Version:                    version,
			Host:                       config.CLIConfig.Metrics.Host,
			Port:                       config.CLIConfig.Metrics.Port,
			CheckInterval:              config.CLIConfig.Proxy.CheckInterval,
			IPCheckUrl:                 config.CLIConfig.Proxy.IpCheckUrl,
			CheckMethod:                config.CLIConfig.Proxy.CheckMethod,
			StatusCheckUrl:             config.CLIConfig.Proxy.StatusCheckUrl,
			DownloadUrl:                config.CLIConfig.Proxy.DownloadUrl,
			SimulateLatency:            config.CLIConfig.Proxy.SimulateLatency,
			Timeout:                    config.CLIConfig.Proxy.Timeout,
			SubscriptionUpdate:         config.CLIConfig.Subscription.Update,
			SubscriptionUpdateInterval: config.CLIConfig.Subscription.UpdateInterval,
			StartPort:                  config.CLIConfig.Xray.StartPort,
			Instance:                   config.CLIConfig.Metrics.Instance,
			PushUrl:                    metrics.GetPushURL(config.CLIConfig.Metrics.PushURL),
			Endpoints:                  endpoints,
			ShowServerDetails:          showServerDetails,
			IsPublic:                   isPublic,
			SubscriptionName:           subscription.GetSubscriptionName(),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		if err := RenderIndex(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func shouldShowServerDetails(showServerDetails bool, isPublic bool, trustedExternalAuth bool) bool {
	if isPublic && !trustedExternalAuth {
		return false
	}
	return showServerDetails
}

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

func BasicAuthMiddleware(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || user != username || pass != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
				http.Error(w, "Unauthorized.", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ConfigStatusHandler(proxyChecker *checker.ProxyChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/config/"):]
		if path == "" {
			http.Error(w, "Config path is required", http.StatusBadRequest)
			return
		}

		found, exists := proxyChecker.GetProxyByStableID(path)
		if !exists {
			http.Error(w, "Config not found", http.StatusNotFound)
			return
		}

		status, latency, err := proxyChecker.GetProxyStatus(found)
		if err != nil {
			http.Error(w, "Status not available", http.StatusNotFound)
			return
		}

		if config.CLIConfig.Proxy.SimulateLatency {
			time.Sleep(time.Duration(latency))
		}

		if status {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Failed"))
		}
	}
}

func RegisterConfigEndpoints(proxies []*models.ProxyConfig, proxyChecker *checker.ProxyChecker, startPort int) {
	if needsStableIDs(proxies) {
		models.AssignStableIDs(proxies)
	}
	endpoints := make([]EndpointInfo, 0, len(proxies))

	for _, proxy := range proxies {
		endpoint := fmt.Sprintf("./config/%s", proxy.StableID)

		status, latency, _ := proxyChecker.GetProxyStatus(proxy)

		endpoints = append(endpoints, EndpointInfo{
			Name:         proxy.Name,
			ServerInfo:   fmt.Sprintf("%s:%d", proxy.Server, proxy.Port),
			URL:          endpoint,
			ProxyPort:    startPort + proxy.Index,
			Index:        proxy.Index,
			Status:       status,
			Latency:      latency,
			LatencyLabel: formatLatency(latency),
			LatencyMs:    latency.Milliseconds(),
			StableID:     proxy.StableID,
			SubName:      proxy.SubName,
			Group:        toProxyGroupInfo(proxy),
			Details:      toProxyDetails(proxy, startPort),
		})
	}

	endpointsMu.Lock()
	registeredEndpoints = endpoints
	endpointsMu.Unlock()
}

func needsStableIDs(proxies []*models.ProxyConfig) bool {
	seen := make(map[string]bool, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil || proxy.StableID == "" || seen[proxy.StableID] {
			return true
		}
		seen[proxy.StableID] = true
	}
	return false
}

func formatLatency(d time.Duration) string {
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

type PrefixServeMux struct {
	prefix string
	mux    *http.ServeMux
}

func NewPrefixServeMux(prefix string) (*PrefixServeMux, error) {
	if strings.HasSuffix(prefix, "/") {
		return nil, fmt.Errorf("served url path prefix '%s' should not ends with a '/'", prefix)
	}
	return &PrefixServeMux{
		prefix: prefix,
		mux:    http.NewServeMux(),
	}, nil
}

func (pm *PrefixServeMux) Handle(pattern string, handler http.Handler) {
	pm.mux.Handle(pm.prefix+pattern, http.StripPrefix(pm.prefix, handler))
}

func (pm *PrefixServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == pm.prefix || strings.HasPrefix(r.URL.Path, pm.prefix+"/") {
		pm.mux.ServeHTTP(w, r)
	} else {
		http.NotFound(w, r)
	}
}
