package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"xray-checker/models"
)

func node(name, server string, port int) *models.ProxyConfig {
	return &models.ProxyConfig{Name: name, Server: server, Port: port, Protocol: "vless"}
}

func TestNameGroupedProxies_SingleNodeTakesRemarks(t *testing.T) {
	g := []*models.ProxyConfig{node("original-tag", "s1", 443)}
	nameGroupedProxies("🇳🇱 Netherlands", g)
	if g[0].Name != "🇳🇱 Netherlands" {
		t.Errorf("single node should take the group remarks, got %q", g[0].Name)
	}
}

func TestNameGroupedProxies_SingleNodeKeepsTagWhenNoRemarks(t *testing.T) {
	g := []*models.ProxyConfig{node("original-tag", "s1", 443)}
	nameGroupedProxies("", g)
	if g[0].Name != "original-tag" {
		t.Errorf("single node with no remarks should keep its tag, got %q", g[0].Name)
	}
}

func TestNameGroupedProxies_MultiNodeCombinesRemarksAndTag(t *testing.T) {
	g := []*models.ProxyConfig{
		node("NL", "nl.example.com", 443),
		node("DE", "de.example.com", 443),
	}
	nameGroupedProxies("Auto", g)
	want := map[string]bool{"Auto | NL": true, "Auto | DE": true}
	for _, pc := range g {
		if !want[pc.Name] {
			t.Errorf("unexpected node name %q", pc.Name)
		}
	}
}

func TestNameGroupedProxies_DisambiguatesDuplicateTags(t *testing.T) {
	g := []*models.ProxyConfig{
		node("NL", "nl-1.example.com", 443),
		node("NL", "nl-2.example.com", 443),
	}
	nameGroupedProxies("Auto", g)
	if g[0].Name == g[1].Name {
		t.Fatalf("nodes with duplicate tags must get distinct names, both %q", g[0].Name)
	}
	for _, pc := range g {
		// duplicated tag -> server:port appended for disambiguation
		if pc.Name != "Auto | NL (nl-1.example.com:443)" && pc.Name != "Auto | NL (nl-2.example.com:443)" {
			t.Errorf("expected disambiguated name, got %q", pc.Name)
		}
	}
}

func TestNameGroupedProxies_MultiNodeFallsBackToServerWhenNoTag(t *testing.T) {
	g := []*models.ProxyConfig{
		node("", "a.example.com", 443),
		node("", "b.example.com", 443),
	}
	nameGroupedProxies("Auto", g)
	want := map[string]bool{"Auto | a.example.com:443": true, "Auto | b.example.com:443": true}
	for _, pc := range g {
		if !want[pc.Name] {
			t.Errorf("expected server-based node name, got %q", pc.Name)
		}
	}
}

func TestParseProxyURI(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantNil  bool
		protocol string
		server   string
		port     int
		user     string
		pass     string
		security string
		sni      string
		pinned   string
		dispName string
	}{
		{name: "socks5 plain creds", line: "socks5://user:pass@1.2.3.4:1080#my-socks",
			protocol: "socks", server: "1.2.3.4", port: 1080, user: "user", pass: "pass", dispName: "my-socks"},
		{name: "socks base64 creds, no fragment", line: "socks://dXNlcjpwYXNz@1.2.3.4:1080",
			protocol: "socks", server: "1.2.3.4", port: 1080, user: "user", pass: "pass", dispName: "1.2.3.4:1080"},
		{name: "socks5h no creds", line: "socks5h://1.2.3.4:1080",
			protocol: "socks", server: "1.2.3.4", port: 1080, dispName: "1.2.3.4:1080"},
		{name: "http with creds", line: "http://user:pass@proxy.example.com:8080#h",
			protocol: "http", server: "proxy.example.com", port: 8080, user: "user", pass: "pass", dispName: "h"},
		{name: "https tls + pinned cert + sni", line: "https://1.2.3.4:8443?pinnedPeerCertSha256=aabbcc&sni=cdn.example.com#tls",
			protocol: "http", server: "1.2.3.4", port: 8443, security: "tls", sni: "cdn.example.com", pinned: "aabbcc", dispName: "tls"},
		{name: "https tls default sni = host", line: "https://1.2.3.4:8443",
			protocol: "http", server: "1.2.3.4", port: 8443, security: "tls", sni: "1.2.3.4", dispName: "1.2.3.4:8443"},
		{name: "vless not consumed", line: "vless://uuid@h:443?type=tcp#x", wantNil: true},
		{name: "web url with path not consumed", line: "https://example.com:443/sub/abc", wantNil: true},
		{name: "missing port not consumed", line: "socks5://1.2.3.4", wantNil: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pc := parseProxyURI(c.line)
			if c.wantNil {
				if pc != nil {
					t.Fatalf("expected nil, got %+v", pc)
				}
				return
			}
			if pc == nil {
				t.Fatalf("expected a config, got nil")
			}
			if pc.Protocol != c.protocol || pc.Server != c.server || pc.Port != c.port {
				t.Errorf("got protocol=%q server=%q port=%d", pc.Protocol, pc.Server, pc.Port)
			}
			if pc.Username != c.user || pc.Password != c.pass {
				t.Errorf("creds got user=%q pass=%q want user=%q pass=%q", pc.Username, pc.Password, c.user, c.pass)
			}
			if pc.Security != c.security || pc.SNI != c.sni || pc.PinnedPeerCertSha256 != c.pinned {
				t.Errorf("tls got security=%q sni=%q pinned=%q", pc.Security, pc.SNI, pc.PinnedPeerCertSha256)
			}
			if pc.Name != c.dispName {
				t.Errorf("name got %q want %q", pc.Name, c.dispName)
			}
			if pc.Type != "tcp" {
				t.Errorf("type got %q want tcp", pc.Type)
			}
		})
	}
}

func TestExtractDirectProxyLines(t *testing.T) {
	p := NewParser()
	blob := "vless://uuid@vlesshost:443?type=tcp#vless\nsocks5://user:pass@1.2.3.4:1080#s\nhttps://1.2.3.4:8443#tls\n"
	configs, remaining := p.extractDirectProxyLines([]byte(blob))
	if len(configs) != 2 {
		t.Fatalf("expected 2 direct configs, got %d", len(configs))
	}
	rem := string(remaining)
	if !contains(rem, "vless://") {
		t.Errorf("vless line should remain for the libXray path, remaining=%q", rem)
	}
	if contains(rem, "socks5://") || contains(rem, "https://1.2.3.4:8443") {
		t.Errorf("direct proxy lines should be removed from remaining, remaining=%q", rem)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestParseWireGuardURI(t *testing.T) {
	conf := "[Interface]\n" +
		"PrivateKey = WBkVvO3vdhF9VOaSokEQPSLpGQajKi2fpwKLODlySmk=\n" +
		"Address = 10.9.59.216/32\n" +
		"DNS = 10.12.0.1\n\n" +
		"[Peer]\n" +
		"PublicKey = xBsu74OtcatjpRMfW58muk/95FkaiSSYbZeM+6bRZ1Y=\n" +
		"AllowedIPs = 0.0.0.0/0, ::/0\n" +
		"Endpoint = wg.example.com:51820\n" +
		"PersistentKeepalive = 25\n"
	pc := parseWireGuardURI("wg://" + base64.StdEncoding.EncodeToString([]byte(conf)) + "#WG Node")
	if pc == nil {
		t.Fatal("wg parse returned nil")
	}
	if pc.Protocol != "wireguard" {
		t.Errorf("protocol = %q, want wireguard", pc.Protocol)
	}
	if pc.WGPrivateKey != "WBkVvO3vdhF9VOaSokEQPSLpGQajKi2fpwKLODlySmk=" || pc.WGPeerPublicKey != "xBsu74OtcatjpRMfW58muk/95FkaiSSYbZeM+6bRZ1Y=" {
		t.Errorf("keys: priv=%q peer=%q", pc.WGPrivateKey, pc.WGPeerPublicKey)
	}
	if pc.Server != "wg.example.com" || pc.Port != 51820 {
		t.Errorf("endpoint = %s:%d", pc.Server, pc.Port)
	}
	if len(pc.WGAddresses) != 1 || pc.WGAddresses[0] != "10.9.59.216/32" {
		t.Errorf("addresses = %v", pc.WGAddresses)
	}
	if len(pc.WGAllowedIPs) != 2 || pc.WGKeepalive != 25 {
		t.Errorf("allowedIPs=%v keepalive=%d", pc.WGAllowedIPs, pc.WGKeepalive)
	}
	if pc.Name != "WG Node" {
		t.Errorf("name = %q", pc.Name)
	}

	// url-safe no-padding base64 + name fallback to wireguard-<server>
	pc2 := parseWireGuardURI("wg://" + base64.RawURLEncoding.EncodeToString([]byte(conf)))
	if pc2 == nil || pc2.Name != "wireguard-wg.example.com" {
		t.Errorf("url-safe/no-pad or name fallback failed: %+v", pc2)
	}

	// non-wireguard lines are not consumed
	for _, l := range []string{"vless://uuid@h:443?type=tcp#x", "socks5://u:p@1.2.3.4:1080", "awg://abc#x"} {
		if parseWireGuardURI(l) != nil {
			t.Errorf("%q should not be consumed by parseWireGuardURI", l)
		}
	}
}

func TestConvertOutboundJSON_WireGuardAndSocksHttp(t *testing.T) {
	p := NewParser()

	// WireGuard outbound (xray JSON form)
	wgRaw := []byte(`{
		"protocol":"wireguard","tag":"WG-DE",
		"settings":{
			"secretKey":"WBkVvO3vdhF9VOaSokEQPSLpGQajKi2fpwKLODlySmk=",
			"address":["10.2.0.2/32"],"mtu":1420,
			"peers":[{"publicKey":"xBsu74OtcatjpRMfW58muk/95FkaiSSYbZeM+6bRZ1Y=",
				"endpoint":"de.example.com:51820","allowedIPs":["0.0.0.0/0","::/0"],"keepAlive":25}]
		}}`)
	pc, err := p.convertOutbound(wgRaw, 0, nil)
	if err != nil || pc == nil {
		t.Fatalf("wireguard convert err=%v pc=%v", err, pc)
	}
	if pc.Protocol != "wireguard" || pc.Server != "de.example.com" || pc.Port != 51820 {
		t.Errorf("wg got protocol=%q %s:%d", pc.Protocol, pc.Server, pc.Port)
	}
	if pc.WGPrivateKey == "" || pc.WGPeerPublicKey == "" || pc.WGMTU != 1420 || pc.WGKeepalive != 25 {
		t.Errorf("wg fields: priv=%q pub=%q mtu=%d ka=%d", pc.WGPrivateKey, pc.WGPeerPublicKey, pc.WGMTU, pc.WGKeepalive)
	}
	if len(pc.WGAddresses) != 1 || len(pc.WGAllowedIPs) != 2 {
		t.Errorf("wg addr=%v allowed=%v", pc.WGAddresses, pc.WGAllowedIPs)
	}
	if pc.Name != "WG-DE" {
		t.Errorf("wg name=%q", pc.Name)
	}

	// socks outbound with users (standard form) — regression for JSON socks/http
	sk, err := p.convertOutbound([]byte(`{"protocol":"socks","tag":"S","settings":{"servers":[{"address":"1.2.3.4","port":1080,"users":[{"user":"u","pass":"p"}]}]}}`), 1, nil)
	if err != nil || sk == nil || sk.Protocol != "socks" || sk.Server != "1.2.3.4" || sk.Port != 1080 || sk.Username != "u" || sk.Password != "p" {
		t.Fatalf("socks JSON convert failed: %+v err=%v", sk, err)
	}

	// http outbound, flat form
	ht, err := p.convertOutbound([]byte(`{"protocol":"http","tag":"H","settings":{"address":"5.6.7.8","port":8080,"user":"x","pass":"y"}}`), 2, nil)
	if err != nil || ht == nil || ht.Protocol != "http" || ht.Server != "5.6.7.8" || ht.Port != 8080 || ht.Username != "x" || ht.Password != "y" {
		t.Fatalf("http JSON convert failed: %+v err=%v", ht, err)
	}
}

func TestConvertOutboundJSON_MetricsLabels(t *testing.T) {
	p := NewParser()

	raw := []byte(`{
		"protocol":"trojan","tag":"proxy",
		"settings":{"servers":[{"address":"1.1.1.1","port":443,"password":"pw"}]},
		"metricsLabels":{"location":"Netherlands, Amsterdam","hoster":"FreeVDS"," ":"x","empty":""}
	}`)
	pc, err := p.convertOutbound(raw, 0, nil)
	if err != nil || pc == nil {
		t.Fatalf("convert err=%v pc=%v", err, pc)
	}
	if pc.MetricsLabels["location"] != "Netherlands, Amsterdam" || pc.MetricsLabels["hoster"] != "FreeVDS" {
		t.Errorf("metricsLabels not parsed: %v", pc.MetricsLabels)
	}
	// blank key and empty value are dropped by sanitizeMetricsLabels
	if _, ok := pc.MetricsLabels[" "]; ok {
		t.Errorf("blank key should be dropped: %v", pc.MetricsLabels)
	}
	if _, ok := pc.MetricsLabels["empty"]; ok {
		t.Errorf("empty value should be dropped: %v", pc.MetricsLabels)
	}
	if len(pc.MetricsLabels) != 2 {
		t.Errorf("expected 2 labels, got %d: %v", len(pc.MetricsLabels), pc.MetricsLabels)
	}

	// MetricsLabels must NOT affect the connection-identity hash (stableID).
	pc2, _ := p.convertOutbound([]byte(`{
		"protocol":"trojan","tag":"proxy",
		"settings":{"servers":[{"address":"1.1.1.1","port":443,"password":"pw"}]},
		"metricsLabels":{"location":"Germany"}
	}`), 0, nil)
	if pc.GenerateStableID() != pc2.GenerateStableID() {
		t.Errorf("metricsLabels must not change stableID: %s vs %s", pc.GenerateStableID(), pc2.GenerateStableID())
	}

	// No metricsLabels -> nil map.
	pc3, _ := p.convertOutbound([]byte(`{"protocol":"trojan","tag":"p","settings":{"servers":[{"address":"2.2.2.2","port":443,"password":"pw"}]}}`), 0, nil)
	if pc3.MetricsLabels != nil {
		t.Errorf("expected nil MetricsLabels, got %v", pc3.MetricsLabels)
	}
}

func TestConvertOutboundJSON_FinalMaskTcp(t *testing.T) {
	p := NewParser()

	const base = `{
		"protocol":"vless","tag":"proxy",
		"settings":{"vnext":[{"address":"1.1.1.1","port":443,"users":[{"id":"00000000-0000-0000-0000-000000000000","encryption":"none"}]}]},
		"streamSettings":{"network":"tcp","security":"tls","tlsSettings":{"serverName":"example.com"}%s}
	}`
	withMask := []byte(strings.Replace(base, "%s",
		`,"finalmask":{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","length":"100-200","delay":"10-20"}}]}`, 1))

	pc, err := p.convertOutbound(withMask, 0, nil)
	if err != nil || pc == nil {
		t.Fatalf("convert err=%v pc=%v", err, pc)
	}
	var masks []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(pc.RawFinalMaskTcp), &masks); err != nil {
		t.Fatalf("RawFinalMaskTcp is not a JSON array: %q (%v)", pc.RawFinalMaskTcp, err)
	}
	if len(masks) != 1 || masks[0].Type != "fragment" {
		t.Errorf("finalmask.tcp not preserved: %q", pc.RawFinalMaskTcp)
	}

	// No mask / empty list -> nothing to pass through.
	for name, extra := range map[string]string{
		"absent":     "",
		"empty list": `,"finalmask":{"tcp":[]}`,
		"null":       `,"finalmask":{"tcp":null}`,
	} {
		plain, err := p.convertOutbound([]byte(strings.Replace(base, "%s", extra, 1)), 0, nil)
		if err != nil || plain == nil {
			t.Fatalf("%s: convert err=%v", name, err)
		}
		if plain.RawFinalMaskTcp != "" {
			t.Errorf("%s: expected empty RawFinalMaskTcp, got %q", name, plain.RawFinalMaskTcp)
		}
		// Masks must NOT affect the connection-identity hash (stableID).
		if plain.GenerateStableID() != pc.GenerateStableID() {
			t.Errorf("%s: finalmask.tcp must not change stableID", name)
		}
	}
}
