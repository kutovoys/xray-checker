package web

import (
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
