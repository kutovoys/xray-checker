package web

import "strings"

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

		name := strings.TrimSpace(parts[0])
		date := strings.TrimSpace(parts[1])
		if name == "" || date == "" {
			continue
		}

		result[name] = date
	}

	return result
}
