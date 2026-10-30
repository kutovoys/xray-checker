// Package probecore selects the runtime used to probe subscription entries.
package probecore

import (
	"xray-checker/config"
	"xray-checker/mihomo"
	"xray-checker/models"
	"xray-checker/xray"
)

type Runner interface {
	Start() error
	Stop() error
}

type Generator interface {
	GenerateValidatedConfig([]*models.ProxyConfig, int, string, string) ([]*models.ProxyConfig, error)
}

func IsMihomo() bool {
	return config.CLIConfig.Proxy.Engine == "mihomo"
}

func ConfigFile() string {
	if IsMihomo() {
		return "mihomo_config.yaml"
	}
	return "xray_config.json"
}

func NewGenerator() Generator {
	if IsMihomo() {
		return mihomo.NewConfigGenerator(config.CLIConfig.Mihomo.Binary)
	}
	return xray.NewConfigGenerator()
}

func NewRunner(configFile string) Runner {
	if IsMihomo() {
		return mihomo.NewRunner(config.CLIConfig.Mihomo.Binary, configFile)
	}
	return xray.NewRunner(configFile)
}
