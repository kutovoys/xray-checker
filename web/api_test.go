package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"xray-checker/models"
)

func TestToProxyInfoOmitsDetailsWhenDisabled(t *testing.T) {
	proxy := &models.ProxyConfig{
		Index:     2,
		Name:      "test",
		Protocol:  "vless",
		Server:    "example.com",
		Port:      443,
		StableID:  "stable-id",
		PublicKey: "public-key",
	}

	info := toProxyInfo(proxy, true, 100*time.Millisecond, 10000, false)

	if info.Details != nil {
		t.Fatal("Details should be omitted when includeDetails is false")
	}
}

func TestToProxyGroupInfoKeyAvoidsDelimiterCollisions(t *testing.T) {
	first := &models.ProxyConfig{
		SubName:    "a|b",
		GroupName:  "c",
		GroupIndex: 0,
		GroupSize:  2,
	}
	second := &models.ProxyConfig{
		SubName:    "a",
		GroupName:  "b|c",
		GroupIndex: 0,
		GroupSize:  2,
	}

	firstGroup := toProxyGroupInfo(first)
	secondGroup := toProxyGroupInfo(second)

	if firstGroup.Key == secondGroup.Key {
		t.Fatalf("group keys collided: %q", firstGroup.Key)
	}
}

func TestShouldShowServerDetailsRequiresTrustedAuthInPublicMode(t *testing.T) {
	if shouldShowServerDetails(true, true, false) {
		t.Fatal("public mode should hide server details without trusted external auth")
	}
	if !shouldShowServerDetails(true, true, true) {
		t.Fatal("trusted external auth should allow details in public mode")
	}
	if !shouldShowServerDetails(true, false, false) {
		t.Fatal("private mode should honor web-show-details")
	}
	if shouldShowServerDetails(false, false, true) {
		t.Fatal("web-show-details=false should hide details")
	}
}

func TestToProxyDetailsIncludesSanitizedGeneratedConfig(t *testing.T) {
	uuid := "12345678-1234-1234-1234-123456789abc"
	proxy := &models.ProxyConfig{
		Index:       2,
		Name:        "test-proxy",
		Protocol:    "vless",
		Server:      "example.com",
		Port:        443,
		Type:        "tcp",
		Security:    "reality",
		UUID:        uuid,
		SNI:         "example.com",
		Fingerprint: "chrome",
		PublicKey:   "public-key",
		ShortID:     "short-id",
	}

	details := toProxyDetails(proxy, 10000)

	if details.GeneratedConfig == nil {
		t.Fatal("GeneratedConfig should be included")
	}
	if details.Inbound.Port != 10002 {
		t.Fatalf("unexpected inbound port: got %d", details.Inbound.Port)
	}

	payload, err := json.Marshal(details.GeneratedConfig)
	if err != nil {
		t.Fatalf("marshal generated config: %v", err)
	}
	text := string(payload)

	if strings.Contains(text, uuid) {
		t.Fatal("generated config should not expose full UUID")
	}
	if !strings.Contains(text, "1234...9abc") {
		t.Fatalf("generated config should contain masked UUID, got %s", text)
	}
}
