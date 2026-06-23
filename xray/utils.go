package xray

import (
	"fmt"
	"xray-checker/models"
)

func PrepareProxyConfigs(proxies []*models.ProxyConfig) {
	for i := range proxies {
		proxies[i].Index = i
		proxies[i].StableID = proxies[i].GenerateStableID()
	}

	deduplicateStableIDs(proxies)
}

// deduplicateStableIDs guarantees unique StableIDs even when proxies share
// identical connection parameters (e.g. the same node listed twice under
// different remarks in a subscription), since the frontend relies on
// StableID as a unique key.
func deduplicateStableIDs(proxies []*models.ProxyConfig) {
	seen := make(map[string]int, len(proxies))
	for _, p := range proxies {
		seen[p.StableID]++
		if n := seen[p.StableID]; n > 1 {
			p.StableID = fmt.Sprintf("%s-%d", p.StableID, n)
		}
	}
}

func IsConfigsEqual(old, new []*models.ProxyConfig) bool {
	if len(old) != len(new) {
		return false
	}

	oldMap := make(map[string]bool)
	newMap := make(map[string]bool)

	for _, cfg := range old {
		if cfg.StableID == "" {
			cfg.StableID = cfg.GenerateStableID()
		}
		oldMap[cfg.StableID] = true
	}

	for _, cfg := range new {
		if cfg.StableID == "" {
			cfg.StableID = cfg.GenerateStableID()
		}
		newMap[cfg.StableID] = true
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
