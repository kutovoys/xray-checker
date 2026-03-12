package web

<<<<<<< codex/add-expiration-date-to-server-info-jekem3
import (
	"strings"
	"unicode"
)
=======
import "strings"
>>>>>>> main

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

<<<<<<< codex/add-expiration-date-to-server-info-jekem3
		name := normalizeServerName(parts[0])
=======
		name := strings.TrimSpace(parts[0])
>>>>>>> main
		date := strings.TrimSpace(parts[1])
		if name == "" || date == "" {
			continue
		}

		result[name] = date
	}

	return result
}
<<<<<<< codex/add-expiration-date-to-server-info-jekem3

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
=======
>>>>>>> main
