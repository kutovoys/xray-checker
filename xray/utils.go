package xray

import (
	"xray-checker/models"
)

func PrepareProxyConfigs(proxies []*models.ProxyConfig) {
	for i := range proxies {
		proxies[i].Index = i
	}
	models.AssignStableIDs(proxies)
}

func IsConfigsEqual(old, new []*models.ProxyConfig) bool {
	if len(old) != len(new) {
		return false
	}

	oldIDs := stableIDCounts(old)
	newIDs := stableIDCounts(new)

	for id, oldCount := range oldIDs {
		if newIDs[id] != oldCount {
			return false
		}
	}

	for id, newCount := range newIDs {
		if oldIDs[id] != newCount {
			return false
		}
	}

	return true
}

func stableIDCounts(configs []*models.ProxyConfig) map[string]int {
	clones := make([]*models.ProxyConfig, 0, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		clone := *cfg
		clones = append(clones, &clone)
	}
	models.AssignStableIDs(clones)

	counts := make(map[string]int, len(clones))
	for _, cfg := range clones {
		counts[cfg.StableID]++
	}
	return counts
}
