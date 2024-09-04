package models

// Provider interface that all providers must implement
type Provider interface {
	GetName() string                     // Returns the provider's name
	GetProxyStartPort() int              // Returns the starting port for proxies
	GetInterval() int                    // Returns the interval between checks
	GetConfigs() []Config                // Returns the provider's connection configurations
	ProcessResults(ConnectionData) error // Processes the results of the connection tests
}

type Config struct {
	Link        string `json:"link"`
	MonitorLink string `json:"monitorLink"`
}

type ParsedLink struct {
	Protocol    string
	UID         string
	Server      string
	Port        string
	Security    string
	Type        string
	HeaderType  string
	Flow        string
	Path        string
	Host        string
	SNI         string
	FP          string
	PBK         string
	SID         string
	Name        string
	Method      string
	RandomPort  int
	MonitorLink string
}

type XrayConfig struct {
	Inbounds []struct {
		Listen   string `json:"listen"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Webhook   string                   `json:"webhook"`
}

type ConnectionData struct {
	ConfigFile   string
	SourceIP     string
	VPNIP        string
	WebhookURL   string
	ProxyAddress string
	Status       string
	Error        error
}
