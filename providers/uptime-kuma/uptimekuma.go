package uptimekuma

import (
	"encoding/json"
	"fmt"
	"net/http"
	"xray-checker/models"
	"xray-checker/utils"
)

func ProviderFactory(providerType string, data json.RawMessage) (models.Provider, error) {
	switch providerType {
	case "uptime-kuma":
		var uptimeKuma UptimeKuma
		err := json.Unmarshal(data, &uptimeKuma)
		if err != nil {
			return nil, err
		}
		return &uptimeKuma, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerType)
	}
}

func (u *UptimeKuma) ProcessResults(logData models.ConnectionData) error {
	if logData.VPNIP != logData.SourceIP {
		_, err := http.Get(logData.WebhookURL)
		if err != nil {
			logData.Error = fmt.Errorf("error sending status to Uptime-Kuma: %v", err)
			logData.Status = "Error"
		} else {
			logData.Status = "Success"
		}
	} else {
		logData.Status = "IP addresses match, status not sent"
	}

	utils.LogResult(logData)

	return logData.Error
}
