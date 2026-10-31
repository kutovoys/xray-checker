package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/models"
)

func mihomoAPIFixture() (*checker.ProxyChecker, []string) {
	secrets := []string{
		"12345678-1234-4321-8765-123456789abc",
		"mihomo-fixture-password",
		"mihomo-fixture-private-key",
		"Bearer mihomo-fixture-header-token",
	}
	proxy := &models.ProxyConfig{
		Name: "Mihomo h2mux", StableID: "mihomo-fixture", GroupName: "Example group",
		Protocol: "vless", Server: "proxy.example", Port: 443, UUID: secrets[0],
		Password: secrets[1], Security: "reality", Type: "tcp", Index: 0,
		Mihomo: map[string]any{
			"name": "Mihomo h2mux", "type": "vless", "server": "proxy.example", "port": 443,
			"uuid": secrets[0], "password": secrets[1], "private-key": secrets[2],
			"ws-opts": map[string]any{"headers": map[string]any{"Authorization": secrets[3]}},
			"smux":    map[string]any{"enabled": true, "protocol": "h2mux"},
		},
	}
	legacy := &models.ProxyConfig{
		Name: "Xray", StableID: "xray-fixture", Protocol: "vless", Server: "legacy.example",
		Port: 443, UUID: "87654321-4321-4321-8765-123456789abc", Type: "tcp", Index: 1,
	}
	return checker.NewProxyChecker([]*models.ProxyConfig{proxy, legacy}, 10000,
		"", 1, "", "", 1, 1, "status", 0), secrets
}

func TestMihomoAPIDoesNotExposeRawConfig(t *testing.T) {
	previous := config.CLIConfig.Web
	t.Cleanup(func() { config.CLIConfig.Web = previous })
	pc, secrets := mihomoAPIFixture()

	for _, showDetails := range []bool{false, true} {
		name := "details disabled"
		if showDetails {
			name = "details enabled"
		}
		t.Run(name, func(t *testing.T) {
			config.CLIConfig.Web.ShowServerDetails = showDetails
			config.CLIConfig.Web.Public = false
			config.CLIConfig.Web.TrustedExternalAuth = false
			for _, single := range []bool{false, true} {
				path := "/api/v1/proxies"
				handler := APIProxiesHandler(pc, 10000)
				if single {
					path += "/mihomo-fixture"
					handler = APIProxyHandler(pc, 10000)
				}
				response := httptest.NewRecorder()
				handler(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("%s status = %d", path, response.Code)
				}
				for _, secret := range secrets {
					if strings.Contains(response.Body.String(), secret) {
						t.Fatalf("%s exposed a Mihomo credential", path)
					}
				}
				var envelope struct {
					Success bool            `json:"success"`
					Data    json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if !envelope.Success {
					t.Fatalf("%s returned an unsuccessful response", path)
				}
				var entries []map[string]json.RawMessage
				if single {
					var entry map[string]json.RawMessage
					if err := json.Unmarshal(envelope.Data, &entry); err != nil {
						t.Fatal(err)
					}
					entries = append(entries, entry)
				} else if err := json.Unmarshal(envelope.Data, &entries); err != nil {
					t.Fatal(err)
				}
				wantCount := 2
				if single {
					wantCount = 1
				}
				if len(entries) != wantCount {
					t.Fatalf("%s returned %d proxies, want %d", path, len(entries), wantCount)
				}
				if _, exists := entries[0]["generatedConfig"]; exists {
					t.Fatalf("%s returned a generated config for a Mihomo proxy", path)
				}
				if _, exists := entries[0]["Mihomo"]; exists {
					t.Fatalf("%s returned the raw Mihomo definition", path)
				}
				if !single {
					_, hasXrayConfig := entries[1]["generatedConfig"]
					if hasXrayConfig != showDetails {
						t.Errorf("existing Xray details behavior changed: present=%v, want %v", hasXrayConfig, showDetails)
					}
				}
			}
		})
	}
}

func TestMihomoPublicAPIKeepsStatusPageContract(t *testing.T) {
	previous := config.CLIConfig.Web
	t.Cleanup(func() { config.CLIConfig.Web = previous })
	config.CLIConfig.Web.ShowServerDetails = true
	config.CLIConfig.Web.Public = true
	config.CLIConfig.Web.TrustedExternalAuth = true
	pc, secrets := mihomoAPIFixture()
	response := httptest.NewRecorder()
	APIPublicProxiesHandler(pc)(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/proxies", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	for _, secret := range append(secrets, "proxy.example", "legacy.example") {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("public API exposed a credential or server address")
		}
	}
	var envelope struct {
		Success bool                         `json:"success"`
		Data    []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || len(envelope.Data) != 2 {
		t.Fatalf("expected both unchecked proxies to remain visible, got %s", response.Body.String())
	}
	for _, entry := range envelope.Data {
		if len(entry) != 6 {
			t.Fatalf("public proxy fields changed: %v", entry)
		}
		for _, field := range []string{"stableId", "name", "groupName", "online", "latencyMs", "lastCheck"} {
			if _, exists := entry[field]; !exists {
				t.Errorf("public proxy is missing %s", field)
			}
		}
	}
	var proxy PublicProxyInfo
	encoded, err := json.Marshal(envelope.Data[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &proxy); err != nil {
		t.Fatal(err)
	}
	if proxy.StableID != "mihomo-fixture" || proxy.Name != "Mihomo h2mux" || proxy.GroupName != "Example group" || proxy.Online || proxy.LastCheck != 0 {
		t.Fatalf("unexpected public proxy: %+v", proxy)
	}
}
