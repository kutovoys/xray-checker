package models

import (
	"strings"
	"testing"
)

func mihomoTestProxy() *ProxyConfig {
	return &ProxyConfig{
		Protocol: "trojan", Server: "node.example.com", Port: 443, Name: "JP", Password: "first-secret",
		Mihomo: map[string]any{
			"name": "JP", "type": "trojan", "server": "node.example.com", "port": 443,
			"password":     "first-secret",
			"smux":         map[string]any{"enabled": true, "protocol": "h2mux", "max-connections": 2},
			"reality-opts": map[string]any{"support-x25519mlkem768": true},
			"ws-opts":      map[string]any{"headers": map[string]any{"Authorization": "private-header"}},
		},
	}
}

func TestMihomoPublicIdentityAndPrivateReloadSignature(t *testing.T) {
	a := mihomoTestProxy()
	b := mihomoTestProxy()
	b.Password = "second-secret"
	b.Mihomo["password"] = "second-secret"
	b.Mihomo["ws-opts"].(map[string]any)["headers"].(map[string]any)["Authorization"] = "rotated-header"
	if a.GenerateStableID() != b.GenerateStableID() {
		t.Fatal("weak credentials changed a public stable ID")
	}
	if a.ConnectionSignature() == b.ConnectionSignature() {
		t.Fatal("credential changes must trigger a private configuration reload")
	}
	b = mihomoTestProxy()
	b.Mihomo["smux"].(map[string]any)["max-connections"] = 1
	if a.GenerateStableID() == b.GenerateStableID() || a.ConnectionSignature() == b.ConnectionSignature() {
		t.Fatal("mux behavior change was not reflected in identity and reload signature")
	}
	b = mihomoTestProxy()
	b.Mihomo["reality-opts"].(map[string]any)["support-x25519mlkem768"] = false
	if a.GenerateStableID() == b.GenerateStableID() {
		t.Fatal("REALITY hybrid behavior change was not reflected in public identity")
	}
}

func TestMihomoRenameKeepsIdentityAndRefreshesMetadata(t *testing.T) {
	a := mihomoTestProxy()
	b := mihomoTestProxy()
	b.Name = "renamed"
	b.Mihomo["name"] = "renamed"
	if a.GenerateStableID() != b.GenerateStableID() {
		t.Fatal("rename changed public connection identity")
	}
	if a.ConnectionSignature() == b.ConnectionSignature() {
		t.Fatal("rename must refresh names exposed by API and metrics")
	}
}

func TestMihomoDebugStringDoesNotPrintCredentials(t *testing.T) {
	proxy := mihomoTestProxy()
	proxy.UUID = "private-test-uuid"
	output := proxy.DebugString()
	for _, secret := range []string{"first-secret", "private-header", "private-test-uuid"} {
		if strings.Contains(output, secret) {
			t.Fatal("debug output disclosed a credential")
		}
	}
}
