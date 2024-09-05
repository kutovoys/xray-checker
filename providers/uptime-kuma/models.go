package uptimekuma

import (
	"xray-checker/models"
)

type UptimeKuma struct {
	Name           string          `json:"name"`
	ProxyStartPort int             `json:"proxyStartPort"`
	Interval       int             `json:"interval"`
	Workers        int             `json:"Workers"`
	Configs        []models.Config `json:"configs"`
}

func (u *UptimeKuma) GetName() string {
	return u.Name
}

func (u *UptimeKuma) GetProxyStartPort() int {
	return u.ProxyStartPort
}

func (u *UptimeKuma) GetInterval() int {
	return u.Interval
}

func (u *UptimeKuma) GetWorkers() int {
	return u.Workers
}

func (u *UptimeKuma) GetConfigs() []models.Config {
	return u.Configs
}
