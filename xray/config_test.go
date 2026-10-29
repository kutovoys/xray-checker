package xray

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"xray-checker/models"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	"github.com/xtls/xray-core/transport/internet/finalmask/salamander"
	"github.com/xtls/xray-core/transport/internet/finalmask/udphop"
)

func TestExtractFailingOutboundTag(t *testing.T) {
	cases := map[string]string{
		"infra/conf: failed to build outbound config with tag BAD-xhttp_1 > infra/conf: unsupported mode: x": "BAD-xhttp_1",
		"failed to build outbound config with tag My Server | node_7 > some reason":                          "My Server | node_7",
		"some unrelated error":   "",
		"with tag trailing-only": "trailing-only",
	}
	for in, want := range cases {
		if got := extractFailingOutboundTag(in); got != want {
			t.Errorf("extractFailingOutboundTag(%q) = %q, want %q", in, got, want)
		}
	}
}

// A single unbuildable proxy must be excluded (with the rest kept) rather than
// aborting the whole config.
func TestGenerateValidatedConfigPrunesUnbuildable(t *testing.T) {
	good := &models.ProxyConfig{
		Protocol: "vless", Server: "good.example.com", Port: 443, Name: "good",
		UUID: "00000000-0000-0000-0000-000000000000", Type: "tcp", Security: "tls", SNI: "good.example.com", Index: 0,
	}
	bad := &models.ProxyConfig{
		Protocol: "vless", Server: "bad.example.com", Port: 443, Name: "bad",
		UUID: "00000000-0000-0000-0000-000000000000", Type: "xhttp", Security: "tls", SNI: "bad.example.com",
		RawXhttpSettings: `{"mode":"bogus-mode","path":"/"}`, Index: 1,
	}

	f := filepath.Join(t.TempDir(), "cfg.json")
	g := NewConfigGenerator()
	survivors, err := g.GenerateValidatedConfig([]*models.ProxyConfig{good, bad}, 20000, f, "none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(survivors) != 1 || survivors[0].Name != "good" {
		t.Fatalf("expected only 'good' to survive, got %d proxies", len(survivors))
	}
	data, _ := os.ReadFile(f)
	if err := validateConfigBuild(data); err != nil {
		t.Fatalf("written config must be buildable, got: %v", err)
	}
}

// xray-core (v26.9+) refuses to build VLESS (encryption "none") or Trojan without TLS
// to a public address; such a proxy is excluded, while one on a private address stays.
func TestGenerateValidatedConfigPrunesPlaintextToPublicAddress(t *testing.T) {
	public := &models.ProxyConfig{
		Protocol: "vless", Server: "public.example.com", Port: 80, Name: "public",
		UUID: "00000000-0000-0000-0000-000000000000", Type: "ws", Security: "none", Index: 0,
	}
	private := &models.ProxyConfig{
		Protocol: "vless", Server: "10.0.0.1", Port: 80, Name: "private",
		UUID: "00000000-0000-0000-0000-000000000000", Type: "ws", Security: "none", Index: 1,
	}

	f := filepath.Join(t.TempDir(), "cfg.json")
	survivors, err := NewConfigGenerator().GenerateValidatedConfig([]*models.ProxyConfig{public, private}, 20000, f, "none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(survivors) != 1 || survivors[0].Name != "private" {
		t.Fatalf("expected only 'private' to survive, got %d proxies", len(survivors))
	}
}

// A rejected proxy whose name contains " > " (xray's error separator) must still be
// attributed and pruned; otherwise the unbuildable config is kept and the whole
// instance fails to start.
func TestGenerateValidatedConfigPrunesNameWithSeparator(t *testing.T) {
	good := &models.ProxyConfig{
		Protocol: "vless", Server: "good.example.com", Port: 443, Name: "good",
		UUID: "00000000-0000-0000-0000-000000000000", Type: "tcp", Security: "tls", SNI: "good.example.com", Index: 0,
	}
	bad := &models.ProxyConfig{
		Protocol: "vless", Server: "relay.example.com", Port: 80, Name: "Relay > NL",
		UUID: "00000000-0000-0000-0000-000000000000", Type: "ws", Security: "none", Index: 1,
	}

	f := filepath.Join(t.TempDir(), "cfg.json")
	survivors, err := NewConfigGenerator().GenerateValidatedConfig([]*models.ProxyConfig{good, bad}, 20000, f, "none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(survivors) != 1 || survivors[0].Name != "good" {
		t.Fatalf("expected only 'good' to survive, got %d proxies", len(survivors))
	}
	data, _ := os.ReadFile(f)
	if err := validateConfigBuild(data); err != nil {
		t.Fatalf("written config must be buildable, got: %v", err)
	}
}

// buildsWithXrayCore feeds a generated config through the exact decode+build path
// that xray/runner.go uses at startup. This validates the JSON against xray-core's
// real schema, catching key/placement mistakes that json.Unmarshal would otherwise
// silently ignore at runtime.
func buildsWithXrayCore(t *testing.T, proxies []*models.ProxyConfig) []byte {
	t.Helper()
	g := NewConfigGenerator()
	configBytes, err := g.GenerateConfig(proxies, 10000, "none")
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}
	xrayConfig, err := serial.DecodeJSONConfig(bytes.NewReader(configBytes))
	if err != nil {
		t.Fatalf("xray-core rejected generated config (decode): %v\nconfig:\n%s", err, configBytes)
	}
	if _, err := xrayConfig.Build(); err != nil {
		t.Fatalf("xray-core rejected generated config (build): %v\nconfig:\n%s", err, configBytes)
	}
	return configBytes
}

// streamSettingsOf extracts the streamSettings map of the first proxy outbound
// (skipping the freedom/blackhole/dns outbounds appended by the generator).
func streamSettingsOf(t *testing.T, configBytes []byte) map[string]json.RawMessage {
	t.Helper()
	var parsed struct {
		Outbounds []struct {
			Protocol       string                     `json:"protocol"`
			StreamSettings map[string]json.RawMessage `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(configBytes, &parsed); err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}
	for _, ob := range parsed.Outbounds {
		if ob.StreamSettings != nil {
			return ob.StreamSettings
		}
	}
	t.Fatalf("no outbound with streamSettings found")
	return nil
}

func TestGenerateHysteriaConfigWithObfsAndPortHopping(t *testing.T) {
	proxy := &models.ProxyConfig{
		Protocol:             "hysteria",
		Server:               "example.com",
		Port:                 443,
		Name:                 "hy2-advanced",
		SNI:                  "example.com",
		Security:             "tls",
		HysteriaAuth:         "secret-auth",
		HysteriaObfs:         "salamander",
		HysteriaObfsPassword: "obfs-pass",
		HysteriaPorts:        "20000-50000",
		HysteriaHopInterval:  30,
		Index:                0,
	}

	configBytes := buildsWithXrayCore(t, []*models.ProxyConfig{proxy})
	ss := streamSettingsOf(t, configBytes)

	// finalmask must be at the streamSettings top level, NOT under sockopt.
	if _, ok := ss["finalmask"]; !ok {
		t.Errorf("expected top-level streamSettings.finalmask, got keys: %v", keysOf(ss))
	}
	if _, ok := ss["sockopt"]; ok {
		t.Errorf("finalmask must not be placed under sockopt")
	}

	// Salamander first, then port hopping as a "udphop" mask (quicParams.udpHop is
	// silently ignored by xray-core v26.9+).
	var fm struct {
		QuicParams json.RawMessage `json:"quicParams"`
		Udp        []struct {
			Type     string `json:"type"`
			Settings struct {
				Mode        string `json:"mode"`
				RemotePorts string `json:"remotePorts"`
				Interval    int32  `json:"interval"`
			} `json:"settings"`
		} `json:"udp"`
	}
	if err := json.Unmarshal(ss["finalmask"], &fm); err != nil {
		t.Fatalf("failed to parse finalmask: %v", err)
	}
	if fm.QuicParams != nil {
		t.Errorf("finalmask.quicParams must not be emitted, got %s", fm.QuicParams)
	}
	if len(fm.Udp) != 2 || fm.Udp[0].Type != "salamander" || fm.Udp[1].Type != "udphop" {
		t.Fatalf("expected finalmask.udp == [salamander, udphop], got %+v", fm.Udp)
	}
	if s := fm.Udp[1].Settings; s.Mode != "intervalLocal,intervalRemote" || s.RemotePorts != "20000-50000" || s.Interval != 30 {
		t.Errorf("udphop settings = %+v", s)
	}

	// The built core config must carry the hop mask, not just the JSON (mask settings
	// are decoded leniently, so a wrong key would be dropped silently).
	var masks []interface{}
	for _, ob := range builtOutbounds(t, configBytes) {
		sender, err := ob.SenderSettings.GetInstance()
		if err != nil {
			t.Fatalf("sender settings: %v", err)
		}
		if sc, ok := sender.(*proxyman.SenderConfig); ok && sc.StreamSettings != nil {
			for _, m := range sc.StreamSettings.Udpmasks {
				instance, err := m.GetInstance()
				if err != nil {
					t.Fatalf("udp mask %s: %v", m.Type, err)
				}
				masks = append(masks, instance)
			}
		}
	}
	if len(masks) != 2 {
		t.Fatalf("built UDP masks = %d, want salamander then udphop", len(masks))
	}
	if _, ok := masks[0].(*salamander.Config); !ok {
		t.Errorf("first built mask = %T, want *salamander.Config", masks[0])
	}
	hop, ok := masks[1].(*udphop.Config)
	if !ok {
		t.Fatalf("second built mask = %T, want *udphop.Config", masks[1])
	}
	if !hop.Local || !hop.Remote || hop.RemoteOnce || hop.IntervalMin != 30 || hop.IntervalMax != 30 {
		t.Errorf("built udphop = local %v remote %v once %v interval %d-%d", hop.Local, hop.Remote, hop.RemoteOnce, hop.IntervalMin, hop.IntervalMax)
	}
	if n := len(hop.RemotePorts); n != 30001 || hop.RemotePorts[0] != 20000 || hop.RemotePorts[n-1] != 50000 {
		t.Errorf("built udphop has %d remote ports", n)
	}
}

// builtOutbounds builds a generated config with xray-core and returns its outbound
// handler configs.
func builtOutbounds(t *testing.T, configBytes []byte) []*core.OutboundHandlerConfig {
	t.Helper()
	xrayConfig, err := serial.DecodeJSONConfig(bytes.NewReader(configBytes))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	built, err := xrayConfig.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return built.Outbound
}

func TestGenerateBasicHysteriaConfig(t *testing.T) {
	proxy := &models.ProxyConfig{
		Protocol:     "hysteria",
		Server:       "example.com",
		Port:         443,
		Name:         "hy2-basic",
		SNI:          "example.com",
		Security:     "tls",
		HysteriaAuth: "secret-auth",
		Index:        0,
	}
	configBytes := buildsWithXrayCore(t, []*models.ProxyConfig{proxy})
	ss := streamSettingsOf(t, configBytes)
	// No obfs/port-hopping -> no finalmask emitted.
	if _, ok := ss["finalmask"]; ok {
		t.Errorf("basic hysteria should not emit finalmask")
	}
}

func TestGenerateVlessConfigStillBuilds(t *testing.T) {
	// Regression guard for the non-hysteria path after the dependency bump.
	proxy := &models.ProxyConfig{
		Protocol:  "vless",
		Server:    "example.com",
		Port:      443,
		Name:      "vless-reality",
		UUID:      "00000000-0000-0000-0000-000000000000",
		Flow:      "xtls-rprx-vision",
		Type:      "tcp",
		Security:  "reality",
		SNI:       "example.com",
		PublicKey: "jnsDTya4elAlV-czGFJbvOHJFdXWn7MGGwmKzZ_hoTQ",
		ShortID:   "64d5300f209d1abb",
		Index:     0,
	}
	buildsWithXrayCore(t, []*models.ProxyConfig{proxy})
}

func TestGenerateSocksHttpConfigsBuild(t *testing.T) {
	proxies := []*models.ProxyConfig{
		{Protocol: "socks", Server: "1.2.3.4", Port: 1080, Name: "socks-auth", Type: "tcp", Username: "user", Password: "pass", Index: 0},
		{Protocol: "socks", Server: "1.2.3.4", Port: 1081, Name: "socks-noauth", Type: "tcp", Index: 1},
		{Protocol: "http", Server: "1.2.3.4", Port: 8080, Name: "http-auth", Type: "tcp", Username: "user", Password: "pass", Index: 2},
		{Protocol: "http", Server: "1.2.3.4", Port: 8443, Name: "https-tls", Type: "tcp", Security: "tls", SNI: "1.2.3.4", Index: 3},
	}
	configBytes := buildsWithXrayCore(t, proxies)

	var parsed struct {
		Outbounds []struct {
			Protocol       string                 `json:"protocol"`
			Settings       map[string]interface{} `json:"settings"`
			StreamSettings map[string]interface{} `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(configBytes, &parsed); err != nil {
		t.Fatalf("parse generated config: %v", err)
	}

	var socksAuth, httpsTLS bool
	for _, ob := range parsed.Outbounds {
		if ob.Protocol != "socks" && ob.Protocol != "http" {
			continue
		}
		servers, _ := ob.Settings["servers"].([]interface{})
		if len(servers) == 0 {
			t.Fatalf("%s outbound has no servers: %v", ob.Protocol, ob.Settings)
		}
		srv := servers[0].(map[string]interface{})
		if users, ok := srv["users"].([]interface{}); ok && len(users) > 0 {
			u := users[0].(map[string]interface{})
			if u["user"] == "user" && u["pass"] == "pass" {
				socksAuth = true
			}
		}
		if ob.StreamSettings != nil && ob.StreamSettings["security"] == "tls" {
			httpsTLS = true
			// An https forward proxy must pin ALPN to http/1.1, otherwise the
			// TLS handshake negotiates h2 and the http proxy can't parse it.
			tls, _ := ob.StreamSettings["tlsSettings"].(map[string]interface{})
			alpn, _ := tls["alpn"].([]interface{})
			if len(alpn) != 1 || alpn[0] != "http/1.1" {
				t.Errorf("https proxy alpn = %v, want [http/1.1]", alpn)
			}
		}
	}
	if !socksAuth {
		t.Error("expected a socks/http outbound carrying user/pass")
	}
	if !httpsTLS {
		t.Error("expected the https proxy to produce streamSettings security=tls")
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestGenerateWireGuardConfigBuild(t *testing.T) {
	wg := &models.ProxyConfig{
		Protocol: "wireguard", Name: "wg", Server: "1.2.3.4", Port: 51820, Index: 0,
		WGPrivateKey:    "WBkVvO3vdhF9VOaSokEQPSLpGQajKi2fpwKLODlySmk=",
		WGPeerPublicKey: "xBsu74OtcatjpRMfW58muk/95FkaiSSYbZeM+6bRZ1Y=",
		WGAddresses:     []string{"10.0.0.2/32"}, WGAllowedIPs: []string{"0.0.0.0/0", "::/0"},
		WGKeepalive: 25, WGMTU: 1420,
	}
	configBytes := buildsWithXrayCore(t, []*models.ProxyConfig{wg})

	var parsed struct {
		Outbounds []struct {
			Protocol       string                 `json:"protocol"`
			Settings       map[string]interface{} `json:"settings"`
			StreamSettings map[string]interface{} `json:"streamSettings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(configBytes, &parsed); err != nil {
		t.Fatalf("parse generated config: %v", err)
	}
	var found bool
	for _, ob := range parsed.Outbounds {
		if ob.Protocol != "wireguard" {
			continue
		}
		found = true
		if ob.StreamSettings != nil {
			t.Errorf("wireguard outbound must not carry streamSettings")
		}
		if _, hasAwg := ob.Settings["awg"]; hasAwg {
			t.Errorf("plain wireguard must not emit an awg block (stock xray-core)")
		}
		if ob.Settings["secretKey"] == nil || ob.Settings["peers"] == nil {
			t.Errorf("wireguard settings missing secretKey/peers: %v", ob.Settings)
		}
	}
	if !found {
		t.Error("expected a wireguard outbound")
	}
}
