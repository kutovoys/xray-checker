package models

type Provider interface {
	GetName() string
	GetProxyStartPort() int
	GetInterval() int
	GetWorkers() int
	GetCheckService() string
	GetConfigs() []Config
	ProcessResults(ConnectionData) error
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
