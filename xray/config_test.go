package xray

import (
	"encoding/json"
	"testing"

	"xray-checker/models"
)

func TestGenerateConfigKeepsKcpSettings(t *testing.T) {
	proxy := &models.ProxyConfig{
		Protocol:       "vless",
		Server:         "kcp.example.com",
		Port:           8443,
		Name:           "KCP",
		Type:           "kcp",
		UUID:           "00000000-0000-0000-0000-000000000000",
		Encryption:     "none",
		RawKcpSettings: `{"seed":"frdm-seed","header":{"type":"dtls"},"mtu":1350,"tti":20}`,
	}

	configBytes, err := NewConfigGenerator().GenerateConfig([]*models.ProxyConfig{proxy}, 10000, "none")
	if err != nil {
		t.Fatalf("GenerateConfig returned error: %v", err)
	}

	var generated struct {
		Outbounds []struct {
			Tag            string `json:"tag"`
			StreamSettings struct {
				Network     string                 `json:"network"`
				KcpSettings map[string]interface{} `json:"kcpSettings"`
			} `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(configBytes, &generated); err != nil {
		t.Fatalf("generated config is invalid JSON: %v", err)
	}

	var kcpSettings map[string]interface{}
	for _, outbound := range generated.Outbounds {
		if outbound.Tag == "KCP_0" {
			if outbound.StreamSettings.Network != "kcp" {
				t.Fatalf("network = %q, want kcp", outbound.StreamSettings.Network)
			}
			kcpSettings = outbound.StreamSettings.KcpSettings
		}
	}

	if kcpSettings == nil {
		t.Fatal("kcpSettings were not generated")
	}
	if kcpSettings["seed"] != "frdm-seed" {
		t.Fatalf("seed = %v, want frdm-seed", kcpSettings["seed"])
	}
}
