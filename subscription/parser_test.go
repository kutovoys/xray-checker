package subscription

import (
	"fmt"
	"net/http"
	"testing"

	"xray-checker/config"
)

func TestParseJSONConfigsNamesBalancerNodes(t *testing.T) {
	data := fmt.Sprintf(`[
		{"remarks":"Germany","outbounds":[%s,%s]},
		{"remarks":"France","outbounds":[%s]}
	]`,
		testVLESSOutbound("de-01.example.com"),
		testVLESSOutbound("de-02.example.com"),
		testVLESSOutbound("fr-01.example.com"),
	)

	configs, err := NewParser().parseJSONConfigs([]byte(data))
	if err != nil {
		t.Fatalf("parseJSONConfigs returned error: %v", err)
	}

	wantNames := []string{
		"Germany - de-01.example.com",
		"Germany - de-02.example.com",
		"France",
	}

	if len(configs) != len(wantNames) {
		t.Fatalf("got %d configs, want %d", len(configs), len(wantNames))
	}

	for i, cfg := range configs {
		if cfg.Name != wantNames[i] {
			t.Fatalf("config %d name = %q, want %q", i, cfg.Name, wantNames[i])
		}
		if cfg.Index != i {
			t.Fatalf("config %d index = %d, want %d", i, cfg.Index, i)
		}
		if cfg.StableID != cfg.GenerateStableID() {
			t.Fatalf("config %d StableID was not generated from final identity", i)
		}
		if i < 2 {
			if cfg.GroupName != "Germany" || cfg.GroupIndex != i || cfg.GroupSize != 2 {
				t.Fatalf("config %d group metadata = (%q, %d, %d), want (Germany, %d, 2)", i, cfg.GroupName, cfg.GroupIndex, cfg.GroupSize, i)
			}
		}
	}
}

func TestParseSingleConfigFileKeepsStartIndexForBalancerNodes(t *testing.T) {
	data := fmt.Sprintf(`{"remarks":"Germany","outbounds":[%s,%s]}`,
		testVLESSOutbound("de-01.example.com"),
		testVLESSOutbound("de-02.example.com"),
	)

	configs, err := NewParser().parseSingleConfigFile([]byte(data), 5)
	if err != nil {
		t.Fatalf("parseSingleConfigFile returned error: %v", err)
	}

	wantNames := []string{
		"Germany - de-01.example.com",
		"Germany - de-02.example.com",
	}

	if len(configs) != len(wantNames) {
		t.Fatalf("got %d configs, want %d", len(configs), len(wantNames))
	}

	for i, cfg := range configs {
		wantIndex := i + 5
		if cfg.Name != wantNames[i] {
			t.Fatalf("config %d name = %q, want %q", i, cfg.Name, wantNames[i])
		}
		if cfg.Index != wantIndex {
			t.Fatalf("config %d index = %d, want %d", i, cfg.Index, wantIndex)
		}
		if cfg.StableID != cfg.GenerateStableID() {
			t.Fatalf("config %d StableID was not generated from final identity", i)
		}
		if cfg.GroupName != "Germany" || cfg.GroupIndex != i || cfg.GroupSize != 2 {
			t.Fatalf("config %d group metadata = (%q, %d, %d), want (Germany, %d, 2)", i, cfg.GroupName, cfg.GroupIndex, cfg.GroupSize, i)
		}
	}
}

func TestParseSingleConfigFileJSONArrayKeepsStartIndex(t *testing.T) {
	data := fmt.Sprintf(`[
		{"remarks":"Germany","outbounds":[%s,%s]}
	]`,
		testVLESSOutbound("de-01.example.com"),
		testVLESSOutbound("de-02.example.com"),
	)

	configs, err := NewParser().parseSingleConfigFile([]byte(data), 7)
	if err != nil {
		t.Fatalf("parseSingleConfigFile returned error: %v", err)
	}

	if len(configs) != 2 {
		t.Fatalf("got %d configs, want 2", len(configs))
	}

	for i, cfg := range configs {
		wantIndex := i + 7
		if cfg.Index != wantIndex {
			t.Fatalf("config %d index = %d, want %d", i, cfg.Index, wantIndex)
		}
		if cfg.StableID != cfg.GenerateStableID() {
			t.Fatalf("config %d StableID was not generated from final identity", i)
		}
	}
}

func TestParseJSONConfigsKeepsPortOne(t *testing.T) {
	data := fmt.Sprintf(`[{"remarks":"Port One","outbounds":[%s]}]`, testVLESSOutboundWithPort("port-one.example.com", 1))

	configs, err := NewParser().parseJSONConfigs([]byte(data))
	if err != nil {
		t.Fatalf("parseJSONConfigs returned error: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(configs))
	}
	if configs[0].Port != 1 {
		t.Fatalf("port = %d, want 1", configs[0].Port)
	}
}

func TestApplySubscriptionHeadersSupportsJSONFormatAndOverrides(t *testing.T) {
	oldSubscription := config.CLIConfig.Subscription
	oldVersion := config.Version
	defer func() {
		config.CLIConfig.Subscription = oldSubscription
		config.Version = oldVersion
	}()

	config.Version = "test-version"
	config.CLIConfig.Subscription.JSONFormat = true
	config.CLIConfig.Subscription.UserAgent = "Custom-Agent"
	config.CLIConfig.Subscription.Headers = []string{
		"X-Hwid: fixed-device-id",
		"X-Test: yes",
		"broken",
	}

	req, err := http.NewRequest("GET", "https://example.com/sub", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	NewParser().applySubscriptionHeaders(req)

	if got := req.Header.Get("User-Agent"); got != "Custom-Agent" {
		t.Fatalf("User-Agent = %q, want %q", got, "Custom-Agent")
	}
	if got := req.Header.Get("X-Hwid"); got != "fixed-device-id" {
		t.Fatalf("X-Hwid = %q, want %q", got, "fixed-device-id")
	}
	if got := req.Header.Get("X-Test"); got != "yes" {
		t.Fatalf("X-Test = %q, want %q", got, "yes")
	}
	if got := req.Header.Get("Accept"); got != "*/*" {
		t.Fatalf("Accept = %q, want %q", got, "*/*")
	}
}

func testVLESSOutbound(server string) string {
	return testVLESSOutboundWithPort(server, 443)
}

func testVLESSOutboundWithPort(server string, port int) string {
	return fmt.Sprintf(`{
		"protocol":"vless",
		"tag":"%s",
		"settings":{
			"vnext":[{
				"address":"%s",
				"port":%d,
				"users":[{
					"id":"00000000-0000-0000-0000-000000000000",
					"encryption":"none"
				}]
			}]
		},
		"streamSettings":{
			"network":"tcp",
			"security":"none"
		}
	}`, server, server, port)
}
