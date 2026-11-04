package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type ProxyConfig struct {
	Protocol             string
	Server               string
	Port                 int
	Name                 string
	Security             string
	Type                 string
	UUID                 string
	Flow                 string
	Encryption           string
	HeaderType           string
	Path                 string
	Host                 string
	SNI                  string
	Fingerprint          string
	PublicKey            string
	ShortID              string
	Mode                 string
	Username             string
	Password             string
	Method               string
	Level                int
	AlterId              int
	VMessAid             int
	MultiMode            bool
	ServiceName          string
	IdleTimeout          int
	WindowsSize          int
	AllowInsecure        bool
	PinnedPeerCertSha256 string
	VerifyPeerCertByName string
	ALPN                 []string
	Index                int
	Settings             map[string]string
	StableID             string
	RawXhttpSettings     string
	RawKcpSettings       string
	// RawFinalMaskTcp is streamSettings.finalmask.tcp as received (JSON array).
	// Not part of GenerateStableID, so adding masks to a config keeps its history.
	RawFinalMaskTcp string
	SubName         string
	GroupName       string

	// MetricsLabels holds operator-defined static labels parsed from a JSON
	// outbound's "metricsLabels" object. They are exported as extra Prometheus
	// labels and in the API, but are deliberately NOT part of GenerateStableID
	// (they are metadata, not connection identity).
	MetricsLabels map[string]string

	// Hysteria2 fields
	HysteriaAuth         string
	HysteriaUp           string
	HysteriaDown         string
	HysteriaPorts        string
	HysteriaHopInterval  int32
	HysteriaObfs         string
	HysteriaObfsPassword string

	// WireGuard fields (Server/Port hold the peer endpoint).
	WGPrivateKey    string
	WGPeerPublicKey string
	WGPreSharedKey  string
	WGAddresses     []string
	WGAllowedIPs    []string
	WGMTU           int
	WGKeepalive     int
	WGDNS           []string
}

func (pc *ProxyConfig) Validate() error {
	if pc.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}
	if pc.Server == "" {
		return fmt.Errorf("server is required")
	}
	if pc.Port <= 0 || pc.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", pc.Port)
	}

	switch pc.Protocol {
	case "vless", "vmess":
		if pc.UUID == "" {
			return fmt.Errorf("UUID is required for %s", pc.Protocol)
		}
	case "trojan":
		if pc.Password == "" {
			return fmt.Errorf("password is required for Trojan")
		}
	case "shadowsocks":
		if pc.Password == "" || pc.Method == "" {
			return fmt.Errorf("password and method are required for Shadowsocks")
		}
	case "hysteria":
		if pc.HysteriaAuth == "" {
			return fmt.Errorf("auth is required for Hysteria2")
		}
	case "socks", "http":
		// Forward proxies need only server/port (checked above); credentials
		// are optional.
	case "wireguard":
		if pc.WGPrivateKey == "" || pc.WGPeerPublicKey == "" {
			return fmt.Errorf("private key and peer public key are required for WireGuard")
		}
	default:
		return fmt.Errorf("unsupported protocol: %s", pc.Protocol)
	}

	return nil
}

// GenerateStableID returns a 16-hex content hash that identifies a proxy by its
// connection parameters. Every field that affects the actual connection is included
// so endpoints differing only in transport details (path, host, fp, shortId, flow,
// alpn, ...) get distinct IDs. Name/SubName are deliberately excluded so that
// renaming a server in the panel does NOT change its ID; same-connection configs
// that differ only by name are separated afterwards by AssignStableIDs.
func (pc *ProxyConfig) GenerateStableID() string {
	h := sha256.New()
	// length-framed components so values containing the delimiter stay unambiguous
	write := func(key, val string) {
		fmt.Fprintf(h, "%s=%d:%s;", key, len(val), val)
	}

	write("protocol", pc.Protocol)
	write("server", pc.Server)
	write("port", strconv.Itoa(pc.Port))

	// Low-entropy, human-chosen secrets (trojan/shadowsocks password, hysteria auth)
	// are deliberately NOT hashed: stableID is a PUBLIC identifier (API, badges) and a
	// 64-bit truncated hash of a weak password alongside otherwise-known fields could
	// be brute-forced offline. The UUID is kept: it is a high-entropy (122-bit) random
	// identifier, so its hash is not brute-forceable, and it is a meaningful
	// discriminator when one endpoint serves several users/routes that differ only by
	// UUID (e.g. vless route-id setups). The AssignStableIDs tiebreaker separates any
	// configs that still collide after excluding the low-entropy secrets.
	switch pc.Protocol {
	case "vless", "vmess":
		write("uuid", pc.UUID)
		if pc.Protocol == "vmess" {
			write("alterId", strconv.Itoa(pc.GetAlterId()))
		}
	case "shadowsocks":
		write("method", pc.Method)
	case "hysteria":
		write("ports", pc.HysteriaPorts)
		write("obfs", pc.HysteriaObfs)
	case "wireguard":
		// Peer public key and tunnel address are public and distinguish configs;
		// the private key (a secret) is deliberately excluded.
		write("wgpub", pc.WGPeerPublicKey)
		write("wgaddr", strings.Join(pc.WGAddresses, ","))
	}

	write("encryption", pc.Encryption)
	write("flow", pc.Flow)
	write("security", pc.Security)
	write("sni", pc.SNI)
	write("fp", pc.Fingerprint)
	write("pbk", pc.PublicKey)
	write("sid", pc.ShortID)

	alpn := append([]string(nil), pc.ALPN...)
	sort.Strings(alpn)
	write("alpn", strings.Join(alpn, ","))

	write("net", pc.Type)
	write("headerType", pc.HeaderType)
	write("host", pc.Host)
	write("path", pc.Path)
	write("serviceName", pc.ServiceName)
	write("mode", pc.Mode)
	write("rawXhttp", pc.RawXhttpSettings)
	write("rawKcp", pc.RawKcpSettings)

	return hex.EncodeToString(h.Sum(nil))[:16]
}

// AssignStableIDs sets the final StableID for every proxy in the set. Proxies with
// identical connection parameters (same GenerateStableID) are separated with a
// deterministic suffix derived from a stable ordering (Name, then SubName, then
// Index), so the resulting IDs are unique and stable across subscription reordering.
// The first member of each colliding group keeps the bare hash, so single configs
// (the common case) are unaffected.
func AssignStableIDs(proxies []*ProxyConfig) {
	groups := make(map[string][]*ProxyConfig)
	order := make([]string, 0)
	for _, p := range proxies {
		base := p.GenerateStableID()
		if _, seen := groups[base]; !seen {
			order = append(order, base)
		}
		groups[base] = append(groups[base], p)
	}

	for _, base := range order {
		group := groups[base]
		if len(group) == 1 {
			group[0].StableID = base
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Name != group[j].Name {
				return group[i].Name < group[j].Name
			}
			if group[i].SubName != group[j].SubName {
				return group[i].SubName < group[j].SubName
			}
			return group[i].Index < group[j].Index
		})
		for n, p := range group {
			if n == 0 {
				p.StableID = base
				continue
			}
			sum := sha256.Sum256([]byte(base + "::" + strconv.Itoa(n)))
			p.StableID = hex.EncodeToString(sum[:])[:16]
		}
	}
}

func (pc *ProxyConfig) GetTransportType() string {
	if pc.Type == "" {
		return "tcp"
	}
	return pc.Type
}

func (pc *ProxyConfig) GetSecurityType() string {
	if pc.Security == "" {
		return "none"
	}
	return pc.Security
}

func (pc *ProxyConfig) GetAlterId() int {
	if pc.AlterId == 0 {
		return pc.VMessAid
	}
	return pc.AlterId
}

func (pc *ProxyConfig) GetVMessSecurity() string {
	if pc.Security == "" {
		return "auto"
	}
	return pc.Security
}

func (pc *ProxyConfig) GetUserLevel() int {
	if pc.Level == 0 {
		return 0
	}
	return pc.Level
}

func (pc *ProxyConfig) GetServiceName() string {
	if pc.ServiceName == "" {
		return ""
	}
	return pc.ServiceName
}

func (pc *ProxyConfig) DebugString() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("  [%d] %s\n", pc.Index, pc.Name))
	sb.WriteString(fmt.Sprintf("      Protocol: %s\n", pc.Protocol))
	sb.WriteString(fmt.Sprintf("      Server:   %s:%d\n", pc.Server, pc.Port))

	switch pc.Protocol {
	case "vless", "vmess":
		sb.WriteString(fmt.Sprintf("      UUID:     %s\n", pc.UUID))
		if pc.Protocol == "vmess" {
			sb.WriteString(fmt.Sprintf("      AlterId:  %d\n", pc.GetAlterId()))
		}
		if pc.Flow != "" {
			sb.WriteString(fmt.Sprintf("      Flow:     %s\n", pc.Flow))
		}
		if pc.Encryption != "" {
			sb.WriteString(fmt.Sprintf("      Encryption: %s\n", pc.Encryption))
		}
	case "trojan":
		sb.WriteString(fmt.Sprintf("      Password: %s\n", maskSecret(pc.Password)))
		if pc.Flow != "" {
			sb.WriteString(fmt.Sprintf("      Flow:     %s\n", pc.Flow))
		}
	case "shadowsocks":
		sb.WriteString(fmt.Sprintf("      Method:   %s\n", pc.Method))
		sb.WriteString(fmt.Sprintf("      Password: %s\n", maskSecret(pc.Password)))
	case "hysteria":
		sb.WriteString(fmt.Sprintf("      Auth:     %s\n", maskSecret(pc.HysteriaAuth)))
		if pc.HysteriaUp != "" {
			sb.WriteString(fmt.Sprintf("      Up:       %s\n", pc.HysteriaUp))
		}
		if pc.HysteriaDown != "" {
			sb.WriteString(fmt.Sprintf("      Down:     %s\n", pc.HysteriaDown))
		}
		if pc.HysteriaPorts != "" {
			sb.WriteString(fmt.Sprintf("      Ports:    %s\n", pc.HysteriaPorts))
		}
		if pc.HysteriaObfs != "" {
			sb.WriteString(fmt.Sprintf("      Obfs:     %s\n", pc.HysteriaObfs))
		}
	}

	transport := pc.GetTransportType()
	sb.WriteString(fmt.Sprintf("      Transport: %s\n", transport))

	if transport == "ws" || transport == "httpupgrade" || transport == "splithttp" || transport == "xhttp" || transport == "h2" || transport == "http" {
		if pc.Path != "" {
			sb.WriteString(fmt.Sprintf("      Path:     %s\n", pc.Path))
		}
		if pc.Host != "" {
			sb.WriteString(fmt.Sprintf("      Host:     %s\n", pc.Host))
		}
		if pc.Mode != "" {
			sb.WriteString(fmt.Sprintf("      Mode:     %s\n", pc.Mode))
		}
		if pc.RawXhttpSettings != "" {
			sb.WriteString("      RawSettings: (present)\n")
		}
	}

	if transport == "grpc" {
		sb.WriteString(fmt.Sprintf("      ServiceName: %s\n", pc.GetServiceName()))
		if pc.MultiMode {
			sb.WriteString("      MultiMode:   true\n")
		}
	}

	if transport == "tcp" && pc.HeaderType != "" && pc.HeaderType != "none" {
		sb.WriteString(fmt.Sprintf("      HeaderType: %s\n", pc.HeaderType))
		if pc.HeaderType == "http" {
			if pc.Host != "" {
				sb.WriteString(fmt.Sprintf("      Host:     %s\n", pc.Host))
			}
			if pc.Path != "" {
				sb.WriteString(fmt.Sprintf("      Path:     %s\n", pc.Path))
			}
		}
	}

	security := pc.GetSecurityType()
	sb.WriteString(fmt.Sprintf("      Security: %s\n", security))

	if security == "tls" {
		if pc.SNI != "" {
			sb.WriteString(fmt.Sprintf("      SNI:      %s\n", pc.SNI))
		}
		if pc.Fingerprint != "" {
			sb.WriteString(fmt.Sprintf("      Fingerprint: %s\n", pc.Fingerprint))
		}
		if len(pc.ALPN) > 0 {
			sb.WriteString(fmt.Sprintf("      ALPN:     %s\n", strings.Join(pc.ALPN, ",")))
		}
		if pc.AllowInsecure {
			sb.WriteString("      AllowInsecure: true\n")
		}
	}

	if security == "reality" {
		if pc.SNI != "" {
			sb.WriteString(fmt.Sprintf("      SNI:       %s\n", pc.SNI))
		}
		if pc.Fingerprint != "" {
			sb.WriteString(fmt.Sprintf("      Fingerprint: %s\n", pc.Fingerprint))
		}
		if pc.PublicKey != "" {
			sb.WriteString(fmt.Sprintf("      PublicKey: %s\n", pc.PublicKey))
		}
		if pc.ShortID != "" {
			sb.WriteString(fmt.Sprintf("      ShortID:   %s\n", pc.ShortID))
		}
	}

	sb.WriteString(fmt.Sprintf("      StableID: %s\n", pc.StableID))

	return sb.String()
}

func maskSecret(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}
