package xray

import (
	"xray-checker/models"
)

func PrepareProxyConfigs(proxies []*models.ProxyConfig) {
	for i := range proxies {
		proxies[i].Index = i
		proxies[i].StableID = proxies[i].GenerateStableID()
	}
}

func IsConfigsEqual(old, new []*models.ProxyConfig) bool {
	if len(old) != len(new) {
		return false
	}

	oldMap := make(map[string]bool)
	newMap := make(map[string]bool)

	for _, cfg := range old {
		oldMap[cfg.GenerateStableID()] = true
	}

	for _, cfg := range new {
		newMap[cfg.GenerateStableID()] = true
	}

	for id := range oldMap {
		if !newMap[id] {
			return false
		}
	}

	for id := range newMap {
		if !oldMap[id] {
			return false
		}
	}

	return true
}
