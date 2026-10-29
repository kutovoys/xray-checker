package subscription

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"xray-checker/models"

	"github.com/xtls/xray-core/common/uuid"
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

const testRealityKey = "zs8T79DuKjScDtY3ZCObgOWBNg5n2T9xtPFv5sGC4O0"

func parseOne(t *testing.T, data string) *models.ProxyConfig {
	t.Helper()
	result, err := NewParser().Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(result.Configs))
	}
	return result.Configs[0]
}

func vmessQR(fields string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(fields))
}

// libXray emits the REALITY key only as realitySettings.password; it must still end up
// in PublicKey, or xray-core rejects the outbound with `empty "password"`.
func TestParseShareLink_RealityKey(t *testing.T) {
	pc := parseOne(t, "vless://1e358465-bb5f-4a3b-bdc1-c02e1c30be94@203.0.113.10:443?type=tcp&security=reality&sni=example.com&fp=chrome&pbk="+testRealityKey+"&sid=d44019b1bc0b1d9a&flow=xtls-rprx-vision#Reality%20NL")
	if pc.PublicKey != testRealityKey || pc.ShortID != "d44019b1bc0b1d9a" || pc.SNI != "example.com" {
		t.Errorf("reality fields: pbk=%q sid=%q sni=%q", pc.PublicKey, pc.ShortID, pc.SNI)
	}
	if pc.Protocol != "vless" || pc.Security != "reality" || pc.Type != "tcp" || pc.Flow != "xtls-rprx-vision" || pc.Name != "Reality NL" {
		t.Errorf("got protocol=%q security=%q type=%q flow=%q name=%q", pc.Protocol, pc.Security, pc.Type, pc.Flow, pc.Name)
	}
}

func TestConvertOutboundJSON_RealityPasswordKey(t *testing.T) {
	p := NewParser()
	for name, reality := range map[string]string{
		"password":           `{"serverName":"example.com","fingerprint":"chrome","password":"` + testRealityKey + `"}`,
		"publicKey":          `{"serverName":"example.com","fingerprint":"chrome","publicKey":"` + testRealityKey + `"}`,
		"password overrides": `{"serverName":"example.com","fingerprint":"chrome","publicKey":"stale","password":"` + testRealityKey + `"}`,
	} {
		raw := []byte(`{"protocol":"vless","tag":"R","settings":{"vnext":[{"address":"203.0.113.10","port":443,"users":[{"id":"1e358465-bb5f-4a3b-bdc1-c02e1c30be94","encryption":"none"}]}]},"streamSettings":{"network":"raw","security":"reality","realitySettings":` + reality + `}}`)
		pc, err := p.convertOutbound(raw, 0, nil)
		if err != nil || pc == nil {
			t.Fatalf("%s: convert err=%v pc=%v", name, err, pc)
		}
		if pc.PublicKey != testRealityKey {
			t.Errorf("%s: PublicKey = %q", name, pc.PublicKey)
		}
	}
}

// libXray dropped v2rayN vmess://base64(JSON) links; they are converted to VMessAEAD
// URIs first, so they keep parsing as before.
func TestParseShareLink_VMessQR(t *testing.T) {
	pc := parseOne(t, vmessQR(`{"v":"2","ps":"VMess WS","add":"vm.example.com","port":"443","id":"b4f0644f-9c25-4c67-83f0-621997cb87b2","aid":"0","scy":"auto","net":"ws","type":"none","host":"cdn.example.com","path":"/vm","tls":"tls"}`))
	if pc.Protocol != "vmess" || pc.Server != "vm.example.com" || pc.Port != 443 || pc.UUID != "b4f0644f-9c25-4c67-83f0-621997cb87b2" {
		t.Errorf("got protocol=%q %s:%d uuid=%q", pc.Protocol, pc.Server, pc.Port, pc.UUID)
	}
	if pc.Type != "ws" || pc.Path != "/vm" || pc.Host != "cdn.example.com" || pc.Security != "tls" || pc.Name != "VMess WS" {
		t.Errorf("got type=%q path=%q host=%q security=%q name=%q", pc.Type, pc.Path, pc.Host, pc.Security, pc.Name)
	}
	// Without "sni", the WebSocket Host is the SNI, as with libXray's former QR importer.
	if pc.SNI != "cdn.example.com" {
		t.Errorf("SNI = %q, want the ws host", pc.SNI)
	}

	grpc := parseOne(t, vmessQR(`{"ps":"VMess gRPC","add":"198.51.100.10","port":443,"id":"b4f0644f-9c25-4c67-83f0-621997cb87b2","net":"grpc","type":"multi","path":"Pp4TYP7CSS","tls":"tls","sni":"g.example.com"}`))
	if grpc.Type != "grpc" || grpc.ServiceName != "Pp4TYP7CSS" || !grpc.MultiMode || grpc.SNI != "g.example.com" {
		t.Errorf("grpc QR: type=%q service=%q multi=%v sni=%q", grpc.Type, grpc.ServiceName, grpc.MultiMode, grpc.SNI)
	}
}

func TestVmessQRToURI(t *testing.T) {
	p := NewParser()
	uri, ok := p.vmessQRToURI(vmessQR(`{"ps":"v6","add":"2001:db8::1","port":"10086","id":"b4f0644f-9c25-4c67-83f0-621997cb87b2","net":"tcp","type":"http","host":"h.example.com","path":"/p","scy":"aes-128-gcm"}`))
	if !ok {
		t.Fatal("expected a converted link")
	}
	want := "vmess://b4f0644f-9c25-4c67-83f0-621997cb87b2@[2001:db8::1]:10086?encryption=aes-128-gcm&headerType=http&host=h.example.com&path=%2Fp&type=tcp#v6"
	if uri != want {
		t.Errorf("uri = %q\nwant  %q", uri, want)
	}
	for _, link := range []string{
		"vmess://b4f0644f-9c25-4c67-83f0-621997cb87b2@vm.example.com:443?type=ws#aead",
		vmessQR(`{"add":"vm.example.com","port":"0","id":"x"}`),
		vmessQR(`not json`),
	} {
		if _, ok := p.vmessQRToURI(link); ok {
			t.Errorf("%q should not be converted", link)
		}
	}

	// Forms libXray's former QR importer accepted: a float port, padded "add", and an
	// id with "+" (passed as the UUID xray-core derives, since "+" would turn into a
	// space on the way through libXray).
	uri, ok = p.vmessQRToURI(vmessQR(`{"add":" vm.example.com ","port":443.0,"id":"908ddd9f+7081fd86","net":"tcp"}`))
	derived, err := uuid.ParseString("908ddd9f+7081fd86")
	if err != nil {
		t.Fatal(err)
	}
	if want := "vmess://" + derived.String() + "@vm.example.com:443?type=tcp"; !ok || uri != want {
		t.Errorf("uri = %q (ok=%v)\nwant  %q", uri, ok, want)
	}
}

// libXray rejects Hysteria2 links with insecure=1; the checker drops the flag (it
// verifies TLS anyway) and reads the port hopping libXray emits as a udphop mask.
func TestParseShareLink_Hysteria2(t *testing.T) {
	pc := parseOne(t, "hysteria2://BZknC9wzd1MG@hy.example.com:443?sni=hy.example.com&insecure=1&obfs=salamander&obfs-password=gTzGgKZhQR&ports=20000-30000&hop-interval=15#HY2")
	if pc.Protocol != "hysteria" || pc.Server != "hy.example.com" || pc.HysteriaAuth != "BZknC9wzd1MG" || pc.Name != "HY2" {
		t.Errorf("got protocol=%q server=%q auth=%q name=%q", pc.Protocol, pc.Server, pc.HysteriaAuth, pc.Name)
	}
	if pc.HysteriaObfs != "salamander" || pc.HysteriaObfsPassword != "gTzGgKZhQR" {
		t.Errorf("obfs=%q password=%q", pc.HysteriaObfs, pc.HysteriaObfsPassword)
	}
	if pc.HysteriaPorts != "20000-30000" || pc.HysteriaHopInterval != 15 || pc.Port != 20000 {
		t.Errorf("hopping ports=%q interval=%d port=%d", pc.HysteriaPorts, pc.HysteriaHopInterval, pc.Port)
	}
}

func TestNormalizeHysteria2Link(t *testing.T) {
	cases := map[string]string{
		"hy2://a@h:443?insecure=1&sni=h#n":                                           "hy2://a@h:443?sni=h#n",
		"hysteria2://a@h:443?allowInsecure=true":                                     "hysteria2://a@h:443",
		"hy2://a@h:443?sni=h&insecure=0&obfs=salamander":                             "hy2://a@h:443?sni=h&obfs=salamander",
		"hy2://a@h:443#no-query":                                                     "hy2://a@h:443#no-query",
		"hy2://a@h:443?sni=h&pinSHA256=e27e3b209329211967c23f1d912a284161e1bf70ea72": "hy2://a@h:443?sni=h",
		"hy2://a@h:443?sni=h&sni=other":                                              "hy2://a@h:443?sni=h",
		"hy2://a@h:443?security=none&obfs-password=xyzw":                             "hy2://a@h:443",
		"hy2://a@h:443?hop-interval=30":                                              "hy2://a@h:443",
		"hy2://a@h:443?ports=20000-30000&hop-interval=30":                            "hy2://a@h:443?ports=20000-30000&hop-interval=30",
		"hy2://a@h:20000-30000?hop-interval=30":                                      "hy2://a@h:20000-30000?hop-interval=30",
		"hy2://a@[2001:db8::1]:443,8443/?hop-interval=30":                            "hy2://a@[2001:db8::1]:443,8443/?hop-interval=30",
	}
	for in, want := range cases {
		if got := normalizeHysteria2Link(in); got != want {
			t.Errorf("normalizeHysteria2Link(%q) = %q, want %q", in, got, want)
		}
	}
}

// libXray (v26.9.30+) takes the server address as SNI for vless/vmess links without
// "sni"; a TLS WebSocket link keeps its WebSocket Host as SNI, as before.
func TestParseShareLink_WSHostAsSNI(t *testing.T) {
	pc := parseOne(t, "vless://9c24b220-1ac5-42fb-8191-e26a664e42b0@198.51.100.50:443?encryption=none&security=tls&type=ws&host=cdn.example.com&path=%2Fws#WS")
	if pc.SNI != "cdn.example.com" || pc.Host != "cdn.example.com" || pc.Path != "/ws" {
		t.Errorf("got sni=%q host=%q path=%q", pc.SNI, pc.Host, pc.Path)
	}
	pc = parseOne(t, "vmess://2cd29f9b-38b0-4f14-8e79-fedef9e51acc@198.51.100.52:443?security=tls&type=ws&host=cdn.example.com&sni=edge.example.com#VM")
	if pc.SNI != "edge.example.com" {
		t.Errorf("an explicit sni must win, got %q", pc.SNI)
	}
}

// A trojan:// link without a query implies TLS; libXray applies that default only to
// links with a query, and xray-core refuses plaintext Trojan to public addresses.
func TestParseShareLink_TrojanWithoutQuery(t *testing.T) {
	pc := parseOne(t, "trojan://Q4tFxBY5LMw5@tr.example.com:443#Bare")
	if pc.Protocol != "trojan" || pc.Security != "tls" || pc.Name != "Bare" {
		t.Errorf("got protocol=%q security=%q name=%q", pc.Protocol, pc.Security, pc.Name)
	}
	if got := trojanDefaultTLS("trojan://pw@tr.example.com:443?type=ws#x"); got != "trojan://pw@tr.example.com:443?type=ws#x" {
		t.Errorf("a link with a query must stay as is, got %q", got)
	}
	if got := trojanDefaultTLS("trojan://pw@tr.example.com:443?#x"); got != "trojan://pw@tr.example.com:443?security=tls#x" {
		t.Errorf("an empty query counts as none, got %q", got)
	}
}

func TestNormalizeTransportQuery(t *testing.T) {
	const id = "9c24b220-1ac5-42fb-8191-e26a664e42b0"
	cases := map[string]string{
		"vless://" + id + "@198.51.100.50:443?security=tls&type=ws&host=cdn.example.com&path=%2Fws#WS":          "vless://" + id + "@198.51.100.50:443?host=cdn.example.com&path=%2Fws&security=tls&sni=cdn.example.com&type=ws#WS",
		"vless://" + id + "@198.51.100.50:443?security=tls&type=ws&host=cdn.example.com&sni=#WS":                "vless://" + id + "@198.51.100.50:443?host=cdn.example.com&security=tls&sni=cdn.example.com&type=ws#WS",
		"vless://" + id + "@203.0.113.10:443?type=grpc&serviceName=Pp4TYP7CSS&mode=guna&security=tls#G":         "vless://" + id + "@203.0.113.10:443?security=tls&serviceName=Pp4TYP7CSS&type=grpc#G",
		"vless://" + id + "@203.0.113.10:443?type=grpc&serviceName=Pp4TYP7CSS&mode=multi&security=tls#G":        "vless://" + id + "@203.0.113.10:443?type=grpc&serviceName=Pp4TYP7CSS&mode=multi&security=tls#G",
		"vless://" + id + "@198.51.100.50:443?security=tls&type=ws&host=cdn.example.com&sni=edge.example.com#W": "vless://" + id + "@198.51.100.50:443?security=tls&type=ws&host=cdn.example.com&sni=edge.example.com#W",
	}
	for in, want := range cases {
		if got := normalizeTransportQuery(in); got != want {
			t.Errorf("normalizeTransportQuery(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestShareLinkLabel(t *testing.T) {
	cases := map[string]string{
		"vless://9c24b220-1ac5-42fb-8191-e26a664e42b0@203.0.113.10:443?pbk=k&sid=s#Node": "vless://203.0.113.10:443#Node",
		"ss://YWVzLTI1Ni1nY206QmdvcXMxU3RXT3B2@198.51.100.20:8388":                       "ss://198.51.100.20:8388",
		vmessQR(`{"add":"vm.example.com","port":443,"id":"secret-id"}`):                  "vmess://",
		"hy2://D9hxfGAZhlCn@hy.example.com:20000-30000?sni=hy.example.com":               "hy2://",
		"proxies:": "(not a share link)",
	}
	for in, want := range cases {
		if got := shareLinkLabel(in); got != want {
			t.Errorf("shareLinkLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShareLinkLines(t *testing.T) {
	data := strings.Join([]string{
		"# comment",
		"STATUS=ok",
		"vless://a1da3217-32e7-4301-b862-9e850fbc1d55@info.example.com:1?type=tcp#placeholder",
		"vless://a1da3217-32e7-4301-b862-9e850fbc1d55@203.0.113.10:443?type=tcp#real",
		"  trojan://Q4tFxBY5LMw5@tr.example.com:443?security=tls#t  ",
	}, "\n")
	lines := shareLinkLines([]byte(data))
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "#real") || !strings.HasPrefix(lines[1], "trojan://") {
		t.Errorf("lines = %q", lines)
	}
}

// libXray dropped Clash/Mihomo YAML; such a subscription fails with a clear error
// instead of being fed line by line to libXray (which would log its credentials).
func TestParse_ClashYAMLRejected(t *testing.T) {
	yaml := "proxies:\n  - name: \"n\"\n    type: trojan\n    server: tr.example.com\n    port: 443\n    password: 54gG4Cg3wXzU\n"
	_, err := NewParser().Parse(yaml)
	if err == nil || !strings.Contains(err.Error(), "YAML") {
		t.Fatalf("expected a Clash YAML error, got %v", err)
	}
	if _, err := NewParser().Parse(base64.StdEncoding.EncodeToString([]byte(yaml))); err == nil || !strings.Contains(err.Error(), "YAML") {
		t.Fatalf("expected a Clash YAML error for the base64 form, got %v", err)
	}
}

// Port hopping in JSON subscriptions: the udphop mask of xray-core v26.9+ and the
// older quicParams.udpHop, whose interval is a number or a string.
func TestConvertOutboundJSON_HysteriaHopping(t *testing.T) {
	p := NewParser()
	base := `{"protocol":"hysteria","tag":"H","settings":{"version":2,"address":"hy.example.com","port":443},"streamSettings":{"network":"hysteria","security":"tls","tlsSettings":{"serverName":"hy.example.com"},"hysteriaSettings":{"version":2,"auth":"BZknC9wzd1MG"},"finalmask":%s}}`
	cases := map[string]string{
		"udphop mask":           `{"udp":[{"type":"salamander","settings":{"password":"gTzGgKZhQR"}},{"type":"udphop","settings":{"mode":"intervalLocal,intervalRemote","remotePorts":"20000-30000","interval":"15"}}]}`,
		"legacy udpHop, number": `{"quicParams":{"udpHop":{"ports":"20000-30000","interval":15}}}`,
		"legacy udpHop, string": `{"quicParams":{"udpHop":{"ports":"20000-30000","interval":"15-30"}}}`,
	}
	for name, mask := range cases {
		pc, err := p.convertOutbound([]byte(fmt.Sprintf(base, mask)), 0, nil)
		if err != nil || pc == nil {
			t.Fatalf("%s: convert err=%v pc=%v", name, err, pc)
		}
		if pc.HysteriaPorts != "20000-30000" || pc.HysteriaHopInterval != 15 {
			t.Errorf("%s: ports=%q interval=%d", name, pc.HysteriaPorts, pc.HysteriaHopInterval)
		}
	}
}

// A base64-wrapped JSON subscription takes the JSON path, like a plain one.
func TestParse_Base64WrappedJSON(t *testing.T) {
	body := `[{"remarks":"NL","outbounds":[{"protocol":"trojan","tag":"proxy","settings":{"servers":[{"address":"203.0.113.20","port":443,"password":"njxUp6qTDxkj"}]},"streamSettings":{"network":"tcp","security":"tls","tlsSettings":{"serverName":"nl.example.com"}}}]}]`
	pc := parseOne(t, base64.StdEncoding.EncodeToString([]byte(body)))
	if pc.Name != "NL" || pc.Protocol != "trojan" || pc.Server != "203.0.113.20" || pc.SNI != "nl.example.com" {
		t.Errorf("got name=%q protocol=%q server=%q sni=%q", pc.Name, pc.Protocol, pc.Server, pc.SNI)
	}
}

// Share links keep their subscription order and #fragment names; a link xray-core
// refuses to build (plaintext VLESS to a public host) is left out.
func TestParseShareLinks_OrderAndNames(t *testing.T) {
	links := strings.Join([]string{
		"vless://1e358465-bb5f-4a3b-bdc1-c02e1c30be94@203.0.113.10:443?type=grpc&security=reality&sni=example.com&fp=chrome&pbk=" + testRealityKey + "&sid=b1235cff967d8d6d&serviceName=Pp4TYP7CSS#A",
		"trojan://njxUp6qTDxkj@tr.example.com:443?security=tls&sni=tr.example.com&type=ws&path=%2Ftr&host=tr.example.com#B",
		"ss://YWVzLTI1Ni1nY206QmdvcXMxU3RXT3B2@198.51.100.20:8388#C",
		"vless://1e358465-bb5f-4a3b-bdc1-c02e1c30be94@public.example.com:80?type=ws&security=none#Dropped",
	}, "\n")
	result, err := NewParser().Parse(links)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var names []string
	for _, pc := range result.Configs {
		names = append(names, pc.Name)
	}
	if strings.Join(names, ",") != "A,B,C" {
		t.Fatalf("names = %v, want [A B C]", names)
	}
	if a := result.Configs[0]; a.Type != "grpc" || a.ServiceName != "Pp4TYP7CSS" || a.PublicKey != testRealityKey {
		t.Errorf("A: type=%q service=%q pbk=%q", a.Type, a.ServiceName, a.PublicKey)
	}
	if b := result.Configs[1]; b.Type != "ws" || b.Path != "/tr" || b.Host != "tr.example.com" || b.SNI != "tr.example.com" {
		t.Errorf("B: type=%q path=%q host=%q sni=%q", b.Type, b.Path, b.Host, b.SNI)
	}
	if c := result.Configs[2]; c.Protocol != "shadowsocks" || c.Method != "aes-256-gcm" || c.Password != "Bgoqs1StWOpv" {
		t.Errorf("C: protocol=%q method=%q password=%q", c.Protocol, c.Method, c.Password)
	}
}
