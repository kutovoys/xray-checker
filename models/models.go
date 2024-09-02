package models

type Provider interface {
	GetName() string
	GetProxySrartPort() int
	GetInterval() int
	GetConfigs() []Config
}

type Config struct {
	Link        string `json:"link"`
	MonitorLink string `json:"monitorLink"`
}

// Реализация для UptimeKuma
type UptimeKuma struct {
	Name           string   `json:"name"`
	ProxyStartPort int      `json:"proxyStartPort"`
	Interval       int      `json:"interval"`
	Configs        []Config `json:"configs"`
}

func (u *UptimeKuma) GetName() string {
	return u.Name
}

func (u *UptimeKuma) GetProxySrartPort() int {
	return u.ProxyStartPort
}

func (u *UptimeKuma) GetInterval() int {
	return u.Interval
}

func (u *UptimeKuma) GetConfigs() []Config {
	return u.Configs
}

type ParsedLink struct {
	Protocol    string
	UID         string
	Server      string
	Port        string
	Security    string
	Type        string
	HeaderType  string
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
	Log      map[string]interface{} `json:"log"`
	Inbounds []struct {
		Listen   string `json:"listen"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		Sniffing struct {
			Enabled      bool     `json:"enabled"`
			DestOverride []string `json:"destOverride"`
			RouteOnly    bool     `json:"routeOnly"`
		} `json:"sniffing"`
	} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Webhook   string                   `json:"webhook"`
}

type LogData struct {
	ConfigFile   string
	SourceIP     string
	VPNIP        string
	WebhookURL   string
	ProxyAddress string
	Status       string
	Error        error
}
