package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"xray-checker/config"
)

const mihomoFixture = `
mixed-port: 9876
external-controller: 0.0.0.0:9090
tun: {enable: true}
rules: ["MATCH,DIRECT"]
proxy-groups: [{name: Auto, type: select, proxies: ["Almaty"]}]
proxies:
  - name: Almaty
    type: vless
    server: node.example.com
    port: 443
    uuid: 6d3d5258-b767-4bb2-96fd-5b8b1125579b
    network: tcp
    tls: true
    udp: true
    servername: cover.example.com
    client-fingerprint: chrome
    reality-opts:
      public-key: test-public-key
      short-id: abcdef0123456789
      support-x25519mlkem768: true
    smux:
      enabled: true
      protocol: h2mux
      padding: true
      min-streams: 8
      max-connections: 2
`

func useMihomoEngine(t *testing.T) {
	t.Helper()
	previous := config.CLIConfig
	t.Cleanup(func() { config.CLIConfig = previous })
	config.CLIConfig.Proxy.Engine = "mihomo"
}

func TestParseMihomoPreservesProxyOptionsOnly(t *testing.T) {
	useMihomoEngine(t)
	result, err := NewParser().Parse(mihomoFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Configs) != 1 {
		t.Fatalf("got %d proxies", len(result.Configs))
	}
	proxy := result.Configs[0]
	if proxy.Name != "Almaty" || proxy.Server != "node.example.com" || proxy.Port != 443 || proxy.Protocol != "vless" || proxy.Security != "reality" {
		t.Fatalf("wrong public metadata: name=%q server=%q port=%d protocol=%q security=%q", proxy.Name, proxy.Server, proxy.Port, proxy.Protocol, proxy.Security)
	}
	if proxy.SNI != "cover.example.com" || proxy.Fingerprint != "chrome" || proxy.PublicKey != "test-public-key" || proxy.ShortID != "abcdef0123456789" {
		t.Fatal("REALITY metadata was not retained")
	}
	wantMux := map[string]any{"enabled": true, "protocol": "h2mux", "padding": true, "min-streams": 8, "max-connections": 2}
	if !reflect.DeepEqual(proxy.Mihomo["smux"], wantMux) {
		t.Fatal("smux settings changed while parsing")
	}
	reality := proxy.Mihomo["reality-opts"].(map[string]any)
	if reality["support-x25519mlkem768"] != true {
		t.Fatal("hybrid REALITY option was lost")
	}
	for _, key := range []string{"mixed-port", "external-controller", "tun", "rules", "proxy-groups"} {
		if _, exists := proxy.Mihomo[key]; exists {
			t.Fatalf("top-level %s leaked into retained proxy configuration", key)
		}
	}
}

func TestParseMihomoProviderAndJSON(t *testing.T) {
	useMihomoEngine(t)
	for name, input := range map[string]string{
		"provider yaml": "proxies:\n  - {name: local, type: socks5, server: 127.0.0.1, port: '1080', password: private-password}\n",
		"json":          `{"proxies":[{"name":"local","type":"socks5","server":"127.0.0.1","port":1080,"password":"private-password"}]}`,
		"merge anchor":  "defaults: &base {type: socks5, server: 127.0.0.1, port: 1080}\nproxies:\n  - <<: *base\n    name: local\n    password: private-password\n",
	} {
		t.Run(name, func(t *testing.T) {
			result, err := NewParser().Parse(input)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Configs) != 1 || result.Configs[0].Protocol != "socks" || result.Configs[0].Port != 1080 {
				t.Fatal("wrong provider metadata")
			}
			if result.Configs[0].Mihomo["password"] != "private-password" {
				t.Fatal("credential was not retained for the core")
			}
		})
	}
}

func TestParseMihomoRejectsInvalidInputWithoutEchoingValues(t *testing.T) {
	useMihomoEngine(t)
	for name, input := range map[string]string{
		"xray json":          `{"outbounds":[{"protocol":"vless","secret":"do-not-print-me"}]}`,
		"share links":        "vless://do-not-print-me@example.com:443",
		"provider only":      "proxy-providers: {remote: {url: https://example.com/do-not-print-me}}",
		"empty":              "proxies: []",
		"missing metadata":   "proxies: [{name: do-not-print-me, type: vless}]",
		"bad port":           "proxies: [{name: a, type: socks5, server: localhost, port: do-not-print-me}]",
		"fractional port":    "proxies: [{name: a, type: socks5, server: localhost, port: 1080.5}]",
		"duplicate proxy":    "proxies: [{name: a, type: socks5, server: localhost, port: 1080, password: one, password: do-not-print-me}]",
		"duplicate document": "proxies: []\nproxies: [{password: do-not-print-me}]",
		"malformed yaml":     "proxies: [{password: do-not-print-me\n",
		"multiple documents": "proxies: []\n---\nsecret: do-not-print-me",
		"unsupported number": "proxies: [{name: a, type: socks5, server: localhost, port: 1080, option: .nan}]",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewParser().Parse(input)
			if err == nil {
				t.Fatal("invalid input was accepted")
			}
			if strings.Contains(err.Error(), "do-not-print-me") {
				t.Fatal("error disclosed a subscription value")
			}
		})
	}
}

func TestMihomoInputRequiresExplicitEngine(t *testing.T) {
	previous := config.CLIConfig
	t.Cleanup(func() { config.CLIConfig = previous })
	config.CLIConfig.Proxy.Engine = "xray"
	for _, input := range []string{mihomoFixture, `{"proxies":[]}`, "proxies: [{password: do-not-print-me\n"} {
		_, err := NewParser().Parse(input)
		if err == nil || !strings.Contains(err.Error(), "--proxy-engine=mihomo") {
			t.Fatalf("expected actionable engine error, got %v", err)
		}
	}
}

func TestMihomoSubscriptionRequestHeaders(t *testing.T) {
	useMihomoEngine(t)
	for _, custom := range []string{"", "custom-client/1.0"} {
		t.Run(custom, func(t *testing.T) {
			config.CLIConfig.Subscription.UserAgent = custom
			config.CLIConfig.Subscription.Headers = nil
			type requestHeaders struct{ userAgent, hwid string }
			received := make(chan requestHeaders, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- requestHeaders{r.UserAgent(), r.Header.Get("X-Hwid")}
				_, _ = w.Write([]byte(mihomoFixture))
			}))
			defer server.Close()
			if _, err := NewParser().Parse(server.URL); err != nil {
				t.Fatal(err)
			}
			headers := <-received
			want := custom
			if want == "" {
				want = "mihomo"
				if headers.hwid == "" {
					t.Fatal("Mihomo preset omitted the checker device identity")
				}
			}
			if headers.userAgent != want {
				t.Fatalf("got UA %q, want %q", headers.userAgent, want)
			}
		})
	}
}

func TestMihomoFolderReadsYamlAndJSON(t *testing.T) {
	useMihomoEngine(t)
	directory := t.TempDir()
	for _, name := range []string{"a.yaml", "b.yml", "c.json"} {
		data := []byte(mihomoFixture)
		if name == "c.json" {
			data = []byte(`{"proxies":[{"name":"local","type":"socks5","server":"localhost","port":1080}]}`)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewParser().Parse("folder://" + directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Configs) != 3 {
		t.Fatalf("loaded %d files, want 3", len(result.Configs))
	}
	for index, proxy := range result.Configs {
		if proxy.Index != index {
			t.Fatal("folder proxies did not receive consecutive indices")
		}
	}
}

func TestMihomoRawDefinitionIsNotJSONSerialized(t *testing.T) {
	useMihomoEngine(t)
	result, err := NewParser().Parse(`{"proxies":[{"name":"local","type":"socks5","server":"localhost","port":1080,"plugin-opts":{"secret":"raw-secret-only"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Configs[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "raw-secret-only") || strings.Contains(string(encoded), "Mihomo") {
		t.Fatal("raw definition was serialized")
	}
}

func TestSubscriptionFetchHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := NewParserWithContext(ctx).Parse(server.URL)
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscription request did not stop after cancellation")
	}
}
