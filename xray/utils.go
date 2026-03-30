package xray

import (
	"fmt"
	"xray-checker/models"
)

func PrepareProxyConfigs(proxies []*models.ProxyConfig) {
	seenIDs := make(map[string]int, len(proxies))

	for i := range proxies {
		proxies[i].Index = i

		baseID := proxies[i].GenerateStableID()
		seenIDs[baseID]++

		if seenIDs[baseID] == 1 {
			proxies[i].StableID = baseID
			continue
		}

		proxies[i].StableID = fmt.Sprintf("%s-%d", baseID, seenIDs[baseID])
	}
}

func IsConfigsEqual(old, new []*models.ProxyConfig) bool {
	if len(old) != len(new) {
		return false
	}

	oldCounts := make(map[string]int, len(old))
	newCounts := make(map[string]int, len(new))

	for _, cfg := range old {
		oldCounts[cfg.GenerateStableID()]++
	}

	for _, cfg := range new {
		newCounts[cfg.GenerateStableID()]++
	}

	for id, count := range oldCounts {
		if newCounts[id] != count {
			return false
		}
	}

	for id, count := range newCounts {
		if oldCounts[id] != count {
			return false
		}
	}

	return true
}
