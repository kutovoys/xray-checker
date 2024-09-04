package uptimekuma

import (
	"xray-checker/models"
)

// Модель UptimeKuma, перенесённая из models.go
type UptimeKuma struct {
	Name           string          `json:"name"`
	ProxyStartPort int             `json:"proxyStartPort"`
	Interval       int             `json:"interval"`
	Configs        []models.Config `json:"configs"`
}

// Реализация методов интерфейса Provider для UptimeKuma
func (u *UptimeKuma) GetName() string {
	return u.Name
}

func (u *UptimeKuma) GetProxyStartPort() int {
	return u.ProxyStartPort
}

func (u *UptimeKuma) GetInterval() int {
	return u.Interval
}

func (u *UptimeKuma) GetConfigs() []models.Config {
	return u.Configs
}
