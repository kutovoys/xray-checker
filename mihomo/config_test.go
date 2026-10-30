package mihomo

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"xray-checker/models"

	"gopkg.in/yaml.v3"
)

func nativeProxy() *models.ProxyConfig {
	return &models.ProxyConfig{
		Name: "Almaty test", Protocol: "vless", Server: "example.invalid", Port: 443,
		Mihomo: map[string]any{
			"name": "Almaty test", "type": "vless", "server": "example.invalid", "port": 443,
			"uuid": "00000000-0000-4000-8000-000000000001", "tls": true, "network": "tcp",
			"reality-opts": map[string]any{"public-key": "fixture-public-key", "short-id": "aabb", "support-x25519mlkem768": true},
			"smux":         map[string]any{"enabled": true, "protocol": "h2mux", "padding": true, "min-streams": 8, "max-connections": 2},
		},
	}
}

func TestGeneratePreservesNativeOptionsAndIsolatesListeners(t *testing.T) {
	p := nativeProxy()
	// Domain expansion may produce two structs sharing the same original map.
	clone := *p
	clone.Index, clone.Server = 1, "192.0.2.2"
	data, err := NewConfigGenerator("").GenerateConfig([]*models.ProxyConfig{p, &clone}, 18000, "none")
	if err != nil {
		t.Fatal(err)
	}
	var config generatedConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Proxies) != 2 || len(config.Listeners) != 2 {
		t.Fatalf("unexpected generated proxy/listener count")
	}
	for i, listener := range config.Listeners {
		if listener.Listen != "127.0.0.1" || listener.Type != "socks" || listener.Port != 18000+i || listener.UDP || listener.Proxy != config.Proxies[i]["name"] {
			t.Fatalf("listener does not select its individual proxy: %+v", listener)
		}
		mux := config.Proxies[i]["smux"].(map[string]any)
		if mux["protocol"] != "h2mux" || mux["enabled"] != true || mux["padding"] != true || mux["min-streams"] != 8 || mux["max-connections"] != 2 {
			t.Fatalf("native multiplex configuration was lost: %+v", mux)
		}
		if config.Proxies[i]["reality-opts"].(map[string]any)["support-x25519mlkem768"] != true {
			t.Fatal("native REALITY option was lost")
		}
	}
	if config.Proxies[0]["server"] != "example.invalid" || config.Proxies[1]["server"] != "192.0.2.2" {
		t.Fatal("resolved server overrides are not independent")
	}
	if p.Mihomo["server"] != "example.invalid" || p.Mihomo["name"] != "Almaty test" {
		t.Fatal("source proxy was mutated")
	}
	if config.AllowLAN || config.ExternalController != "" || config.TUN["enable"] != false || config.DNS["enable"] != false || !reflect.DeepEqual(config.Rules, []string{"MATCH,REJECT"}) {
		t.Fatal("generated configuration must isolate checker networking")
	}
}

func TestGenerateRejectsUnsupportedDependenciesWithoutLeaking(t *testing.T) {
	p := nativeProxy()
	p.Mihomo["dialer-proxy"] = "private-dependency-name"
	_, err := NewConfigGenerator("").GenerateConfig([]*models.ProxyConfig{p}, 18000, "none")
	if err == nil || !strings.Contains(err.Error(), "dialer-proxy") || strings.Contains(err.Error(), "private-dependency-name") {
		t.Fatalf("unexpected dependency diagnostic: %v", err)
	}
}

func TestGenerateRejectsMissingDefinitionsAndPortCollisions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		proxies []*models.ProxyConfig
		start   int
	}{
		{"empty", nil, 18000},
		{"nil", []*models.ProxyConfig{nil}, 18000},
		{"xray", []*models.ProxyConfig{{Server: "example.invalid", Port: 443}}, 18000},
		{"collision", []*models.ProxyConfig{nativeProxy(), nativeProxy()}, 18000},
		{"overflow", []*models.ProxyConfig{nativeProxy()}, 65536},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewConfigGenerator("").GenerateConfig(tc.proxies, tc.start, "none"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidationKeepsPreviousConfigOnFailureAndSuppressesChildSecrets(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "reject")
	filename := filepath.Join(t.TempDir(), "mihomo.yaml")
	if err := os.WriteFile(filename, []byte("previous configuration"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewConfigGenerator(binary).GenerateValidatedConfig([]*models.ProxyConfig{nativeProxy()}, 18000, filename, "none")
	if err == nil || !strings.Contains(err.Error(), "exit 7") || strings.Contains(err.Error(), "fixture-secret-password") {
		t.Fatalf("unexpected validation error: %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "previous configuration" {
		t.Fatalf("failed validation replaced the previous config: %q, %v", data, err)
	}
}

func TestValidatedConfigurationIsPrivate(t *testing.T) {
	t.Setenv("XRAY_CHECKER_MIHOMO_HELPER", "serve")
	filename := filepath.Join(t.TempDir(), "mihomo.yaml")
	// Replacing an existing broader-permission file must tighten permissions.
	if err := os.WriteFile(filename, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	binary, _ := os.Executable()
	proxies := []*models.ProxyConfig{nativeProxy()}
	valid, err := NewConfigGenerator(binary).GenerateValidatedConfig(proxies, 18000, filename, "none")
	if err != nil || len(valid) != 1 {
		t.Fatalf("validation failed: %v", err)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("configuration permissions: %v", info.Mode().Perm())
	}
}

func TestChildEnvironmentDropsCoreOverridesButKeepsProxyTrust(t *testing.T) {
	t.Setenv("CLASH_POST_UP", "unwanted hook")
	t.Setenv("CLASH_CONFIG_STRING", "unwanted configuration")
	t.Setenv("MIHOMO_TEST_OVERRIDE", "unwanted override")
	t.Setenv("SAFE_PATHS", "/unwanted")
	t.Setenv("SKIP_SAFE_PATH_CHECK", "true")
	t.Setenv("HTTPS_PROXY", "http://fixture.invalid:8080")
	t.Setenv("SSL_CERT_FILE", "/fixture/ca.pem")
	got := strings.Join(childEnvironment(), "\n")
	for _, prefix := range []string{"CLASH_POST_UP=", "CLASH_CONFIG_STRING=", "MIHOMO_TEST_OVERRIDE=", "SAFE_PATHS=", "SKIP_SAFE_PATH_CHECK="} {
		if strings.Contains(got, prefix) {
			t.Fatalf("child inherited %s", prefix)
		}
	}
	if !strings.Contains(got, "HTTPS_PROXY=http://fixture.invalid:8080") || !strings.Contains(got, "SSL_CERT_FILE=/fixture/ca.pem") {
		t.Fatal("proxy and CA trust environment must be retained")
	}
}
