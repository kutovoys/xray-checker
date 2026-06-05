package models

import "testing"

func TestGenerateStableIDIncludesDisplayIdentity(t *testing.T) {
	base := ProxyConfig{
		Protocol:  "vless",
		Server:    "example.com",
		Port:      443,
		UUID:      "00000000-0000-0000-0000-000000000000",
		Type:      "tcp",
		Security:  "reality",
		SNI:       "cover.example.com",
		PublicKey: "public-key",
	}

	first := base
	first.Name = "Proxy Basic"

	second := base
	second.Name = "Proxy Smart"

	if first.GenerateStableID() == second.GenerateStableID() {
		t.Fatal("stable IDs must differ when proxy names differ")
	}
}

func TestGenerateStableIDIncludesXHTTPShape(t *testing.T) {
	base := ProxyConfig{
		Protocol:  "vless",
		Server:    "example.com",
		Port:      443,
		Name:      "Proxy Smart",
		UUID:      "00000000-0000-0000-0000-000000000000",
		Type:      "xhttp",
		Security:  "reality",
		SNI:       "cover.example.com",
		PublicKey: "public-key",
		Mode:      "auto",
	}

	first := base
	first.Host = "one.example.com"
	first.Path = "/one"
	first.RawXhttpSettings = `{"host":"one.example.com","path":"/one","mode":"auto"}`

	second := base
	second.Host = "two.example.com"
	second.Path = "/two"
	second.RawXhttpSettings = `{"host":"two.example.com","path":"/two","mode":"auto"}`

	if first.GenerateStableID() == second.GenerateStableID() {
		t.Fatal("stable IDs must differ when xHTTP settings differ")
	}
}

func TestGenerateStableIDIncludesIndex(t *testing.T) {
	base := ProxyConfig{
		Protocol:  "vless",
		Server:    "example.com",
		Port:      443,
		Name:      "Proxy Smart",
		UUID:      "00000000-0000-0000-0000-000000000000",
		Type:      "tcp",
		Security:  "reality",
		SNI:       "cover.example.com",
		PublicKey: "public-key",
	}

	first := base
	first.Index = 1

	second := base
	second.Index = 2

	if first.GenerateStableID() == second.GenerateStableID() {
		t.Fatal("stable IDs must differ when proxy indexes differ")
	}
}
