package web

import (
	"strings"
	"unicode"
)

func ParseServerPaidUntil(raw string) map[string]string {
	result := make(map[string]string)

	for _, pair := range strings.Split(raw, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		name := normalizeServerName(parts[0])
		date := strings.TrimSpace(parts[1])
		if name == "" || date == "" {
			continue
		}

		result[name] = date
	}

	return result
}

func GetPaidUntilForProxyName(paidUntilByServer map[string]string, proxyName string) string {
	return paidUntilByServer[normalizeServerName(proxyName)]
}

func normalizeServerName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, name)

	return strings.Join(strings.Fields(cleaned), " ")
}
