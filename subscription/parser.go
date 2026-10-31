package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"xray-checker/config"
	"xray-checker/logger"
	"xray-checker/models"

	libXray "github.com/xtls/libxray"
)

const (
	// subscriptionHWID is a fixed device id sent with subscription requests. It is
	// intentionally constant (not per-request) so panels that enforce HWID/device
	// limits register a single device for the checker.
	subscriptionHWID = "0JLQq9Ca0JvQrtCn0Jgg0JHQm9Cp0KLQrCBIV0lE"
	// subscriptionJSONUserAgent impersonates an app whose responses are full JSON
	// configs (used when --subscription-json-format is enabled).
	subscriptionJSONUserAgent = "Happ/1.0"
)

type Parser struct {
	ctx context.Context
}

type fetchResult struct {
	Content []byte
	Name    string
}

func NewParser() *Parser {
	return NewParserWithContext(context.Background())
}

func NewParserWithContext(ctx context.Context) *Parser {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Parser{ctx: ctx}
}

type libXrayResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

type libXrayOutbound struct {
	Protocol       string                 `json:"protocol"`
	SendThrough    string                 `json:"sendThrough"`
	Tag            string                 `json:"tag"`
	Settings       *libXraySettings       `json:"settings"`
	StreamSettings *libXrayStreamSettings `json:"streamSettings"`
}

type libXraySettings struct {
	Address    string `json:"address"`
	Port       int    `json:"port"`
	Level      int    `json:"level"`
	ID         string `json:"id"`
	Flow       string `json:"flow"`
	Encryption string `json:"encryption"`
	AlterId    int    `json:"alterId"`
	Security   string `json:"security"`
	Password   string `json:"password"`
	Method     string `json:"method"`
	Version    int32  `json:"version"`
	Auth       string `json:"auth"`
	User       string `json:"user"`
	Pass       string `json:"pass"`
}

type libXrayStreamSettings struct {
	Network             string                      `json:"network"`
	Security            string                      `json:"security"`
	TlsSettings         *libXrayTlsSettings         `json:"tlsSettings"`
	RealitySettings     *libXrayRealitySettings     `json:"realitySettings"`
	RawSettings         *libXrayRawSettings         `json:"rawSettings"`
	WsSettings          *libXrayWsSettings          `json:"wsSettings"`
	GrpcSettings        *libXrayGrpcSettings        `json:"grpcSettings"`
	HttpSettings        *libXrayHttpSettings        `json:"httpSettings"`
	HttpupgradeSettings *libXrayHttpupgradeSettings `json:"httpupgradeSettings"`
	XhttpSettings       json.RawMessage             `json:"xhttpSettings"`
	SplithttpSettings   json.RawMessage             `json:"splithttpSettings"`
	KcpSettings         json.RawMessage             `json:"kcpSettings"`
	HysteriaSettings    *libXrayHysteriaSettings    `json:"hysteriaSettings"`
	Sockopt             *libXraySockopt             `json:"sockopt"`
	FinalMask           *libXrayFinalMask           `json:"finalMask"`
}

type libXrayTlsSettings struct {
	ServerName           string   `json:"serverName"`
	AllowInsecure        bool     `json:"allowInsecure"`
	Fingerprint          string   `json:"fingerprint"`
	Alpn                 []string `json:"alpn"`
	PinnedPeerCertSha256 string   `json:"pinnedPeerCertSha256"`
	VerifyPeerCertByName string   `json:"verifyPeerCertByName"`
}

type libXrayRealitySettings struct {
	ServerName  string `json:"serverName"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"publicKey"`
	ShortId     string `json:"shortId"`
}

type libXrayRawSettings struct {
	Header *struct {
		Type    string `json:"type"`
		Request *struct {
			Path    []string `json:"path"`
			Headers *struct {
				Host []string `json:"Host"`
			} `json:"headers"`
		} `json:"request"`
	} `json:"header"`
}

type libXrayWsSettings struct {
	Path    string `json:"path"`
	Headers *struct {
		Host string `json:"Host"`
	} `json:"headers"`
	Host string `json:"host"`
}

type libXrayGrpcSettings struct {
	ServiceName string `json:"serviceName"`
	MultiMode   bool   `json:"multiMode"`
}

type libXrayHttpSettings struct {
	Path string   `json:"path"`
	Host []string `json:"host"`
}

type libXrayHttpupgradeSettings struct {
	Path string `json:"path"`
	Host string `json:"host"`
}

type libXrayXhttpSettings struct {
	Path string `json:"path"`
	Host string `json:"host"`
	Mode string `json:"mode"`
}

type libXrayHysteriaSettings struct {
	Version int32  `json:"version"`
	Auth    string `json:"auth"`
}

type libXraySockopt struct {
	FinalMask *libXrayFinalMask `json:"finalMask"`
}

type libXrayFinalMask struct {
	QuicParams *libXrayQuicParams `json:"quicParams"`
	Udp        []libXrayMask      `json:"udp"`
}

type libXrayQuicParams struct {
	Congestion string         `json:"congestion"`
	BrutalUp   string         `json:"brutalUp"`
	BrutalDown string         `json:"brutalDown"`
	UdpHop     *libXrayUdpHop `json:"udpHop"`
}

type libXrayUdpHop struct {
	// libXray serializes xray-core's conf.UdpHop, whose port list key is "ports".
	PortList json.RawMessage    `json:"ports"`
	Interval *libXrayInt32Range `json:"interval"`
}

type libXrayInt32Range struct {
	From int32 `json:"from"`
	To   int32 `json:"to"`
}

type libXrayMask struct {
	Type     string           `json:"type"`
	Settings *json.RawMessage `json:"settings"`
}

type libXraySalamander struct {
	Password string `json:"password"`
}

type originalLinkData struct {
	Name          string
	Encryption    string
	Type          string
	Path          string
	Host          string
	AllowInsecure bool
}

type parsedLink struct {
	Server        string
	Port          int
	Name          string
	Encryption    string
	Type          string
	Path          string
	Host          string
	AllowInsecure bool
}

type xrayStandardSettings struct {
	Vnext []struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
		Users   []struct {
			ID         string `json:"id"`
			Flow       string `json:"flow"`
			Encryption string `json:"encryption"`
			AlterId    int    `json:"alterId"`
			Security   string `json:"security"`
			Level      int    `json:"level"`
		} `json:"users"`
	} `json:"vnext"`
	Servers []struct {
		Address  string `json:"address"`
		Port     int    `json:"port"`
		Password string `json:"password"`
		Method   string `json:"method"`
		Flow     string `json:"flow"`
		Users    []struct {
			User string `json:"user"`
			Pass string `json:"pass"`
		} `json:"users"`
	} `json:"servers"`
}

type ParseResult struct {
	Configs []*models.ProxyConfig
	Name    string
}

func (p *Parser) Parse(subscriptionData string) (*ParseResult, error) {
	if p.ctx != nil {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
	}
	sourceType := p.detectSourceType(subscriptionData)
	logger.Debug("Detected source type: %s", sourceType)

	var rawData []byte
	var subName string
	var err error

	switch sourceType {
	case "url":
		result, fetchErr := p.fetchURLContent(subscriptionData)
		if fetchErr != nil {
			return nil, fmt.Errorf("failed to fetch URL content: %w", fetchErr)
		}
		rawData = result.Content
		subName = result.Name
	case "folder":
		folderPath := strings.TrimPrefix(subscriptionData, "folder://")
		configs, folderErr := p.parseFolder(folderPath)
		if folderErr != nil {
			return nil, folderErr
		}
		return &ParseResult{Configs: configs, Name: ""}, nil
	case "file":
		filePath := strings.TrimPrefix(subscriptionData, "file://")
		rawData, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %v", err)
		}
	case "base64":
		encoded := strings.TrimPrefix(subscriptionData, "base64://")
		encoded = strings.TrimPrefix(encoded, "data:text/plain;base64,")
		rawData, err = p.decodeBase64(strings.TrimSpace(encoded))
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 subscription: %w", err)
		}
	default:
		rawData = []byte(subscriptionData)
	}

	trimmedData := strings.TrimSpace(string(rawData))
	if config.CLIConfig.Proxy.Engine == "mihomo" {
		configs, parseErr := parseMihomoSubscription(rawData)
		if parseErr != nil {
			return nil, parseErr
		}
		return &ParseResult{Configs: configs, Name: subName}, nil
	}
	if isMihomoSubscription(rawData) {
		return nil, fmt.Errorf("Mihomo subscription requires --proxy-engine=mihomo")
	}
	if strings.HasPrefix(trimmedData, "[") {
		logger.Debug("Detected JSON array format")
		configs, jsonErr := p.parseJSONConfigs(rawData)
		if jsonErr != nil {
			return nil, jsonErr
		}
		return &ParseResult{Configs: configs, Name: subName}, nil
	}

	if strings.HasPrefix(trimmedData, "{") {
		logger.Debug("Detected single JSON object format")
		configs, jsonErr := p.parseSingleJSONConfig(rawData)
		if jsonErr != nil {
			return nil, jsonErr
		}
		return &ParseResult{Configs: configs, Name: subName}, nil
	}

	originalData := p.parseOriginalLinks(rawData)

	cleanedData := p.cleanEmptyLines(rawData)

	// Pull out socks/http/https forward-proxy URIs first: libXray cannot parse
	// them, so we handle them directly and pass the rest to the normal path.
	directConfigs, remaining := p.extractDirectProxyLines(cleanedData)
	if len(directConfigs) > 0 {
		logger.Info("Parsed %d direct proxy line(s) (socks/http/wireguard)", len(directConfigs))
	}

	var proxyConfigs []*models.ProxyConfig
	if len(strings.TrimSpace(string(remaining))) > 0 {
		// Try batch parsing first
		proxyConfigs = p.parseViaLibXray(remaining, originalData)

		// If batch parsing failed, fall back to line-by-line parsing
		if len(proxyConfigs) == 0 {
			logger.Warn("Batch parsing failed or returned no configs, trying line-by-line parsing")
			proxyConfigs = p.parseLineByLine(remaining, originalData)
		}
	}

	proxyConfigs = append(proxyConfigs, directConfigs...)

	if len(proxyConfigs) == 0 {
		return nil, fmt.Errorf("no valid proxy configurations found")
	}

	// Re-index after merging direct + libXray-parsed configs.
	for i, cfg := range proxyConfigs {
		cfg.Index = i
	}

	return &ParseResult{Configs: proxyConfigs, Name: subName}, nil
}

// extractDirectProxyLines pulls socks/http/https forward-proxy URIs out of the
// raw subscription content (which libXray cannot parse) and returns the parsed
// configs together with the remaining lines for the normal libXray path.
func (p *Parser) extractDirectProxyLines(cleanedData []byte) ([]*models.ProxyConfig, []byte) {
	lines := strings.Split(string(cleanedData), "\n")
	var configs []*models.ProxyConfig
	remaining := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if pc := parseWireGuardURI(trimmed); pc != nil {
				configs = append(configs, pc)
				continue
			}
			if pc := parseProxyURI(trimmed); pc != nil {
				configs = append(configs, pc)
				continue
			}
		}
		remaining = append(remaining, line)
	}

	return configs, []byte(strings.Join(remaining, "\n"))
}

// parseWireGuardURI parses a wg://<base64(conf)> line, where the base64 payload
// is a standard WireGuard .conf (INI text). An optional #name fragment sets the
// display name. Returns nil if the line is not a usable WireGuard config.
func parseWireGuardURI(line string) *models.ProxyConfig {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "wg://") {
		return nil
	}
	line = line[len("wg://"):]

	name := ""
	if i := strings.IndexByte(line, '#'); i >= 0 {
		name = line[i+1:]
		line = line[:i]
	}
	line = strings.TrimSpace(line)

	conf, ok := decodeFlexibleBase64(line)
	if !ok {
		return nil
	}

	pc := parseWireGuardConf(string(conf))
	if pc == nil {
		return nil
	}
	pc.Protocol = "wireguard"
	if name != "" {
		if un, err := url.PathUnescape(name); err == nil {
			name = un
		}
		pc.Name = name
	}
	if pc.Name == "" {
		pc.Name = fmt.Sprintf("wireguard-%s", pc.Server)
	}
	return pc
}

// decodeFlexibleBase64 decodes a base64 string trying standard and URL-safe
// alphabets, with and without padding.
func decodeFlexibleBase64(s string) ([]byte, bool) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, true
		}
	}
	return nil, false
}

// parseWireGuardConf parses a WireGuard .conf (INI) into a ProxyConfig.
// Server/Port hold the first peer's endpoint. Returns nil if the minimum
// required fields (private key, peer public key, endpoint) are missing.
func parseWireGuardConf(text string) *models.ProxyConfig {
	pc := &models.ProxyConfig{}
	section := ""

	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, ";") {
			continue
		}
		if strings.HasPrefix(ln, "[") && strings.HasSuffix(ln, "]") {
			section = strings.ToLower(strings.TrimSpace(ln[1 : len(ln)-1]))
			continue
		}
		eq := strings.IndexByte(ln, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(ln[:eq]))
		val := strings.TrimSpace(ln[eq+1:])

		switch section {
		case "interface":
			switch key {
			case "privatekey":
				pc.WGPrivateKey = val
			case "address":
				pc.WGAddresses = splitCSVList(val)
			case "dns":
				pc.WGDNS = splitCSVList(val)
			case "mtu":
				pc.WGMTU = atoiOrZero(val)
			}
		case "peer":
			switch key {
			case "publickey":
				pc.WGPeerPublicKey = val
			case "presharedkey":
				pc.WGPreSharedKey = val
			case "endpoint":
				if host, portStr, err := net.SplitHostPort(val); err == nil {
					pc.Server = host
					pc.Port = atoiOrZero(portStr)
				}
			case "allowedips":
				pc.WGAllowedIPs = splitCSVList(val)
			case "persistentkeepalive":
				pc.WGKeepalive = atoiOrZero(val)
			}
		}
	}

	if pc.WGPrivateKey == "" || pc.WGPeerPublicKey == "" || pc.Server == "" || pc.Port == 0 {
		return nil
	}
	return pc
}

func splitCSVList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// parseProxyURI parses a socks/http/https forward-proxy URI into a ProxyConfig.
// Supported forms (optional #name fragment on any):
//
//	socks://base64(user:pass)@host:port      (standard subscription form)
//	socks5://user:pass@host:port             (socks5h:// also accepted)
//	http://user:pass@host:port
//	https://user:pass@host:port              (TLS to the proxy itself)
//
// Optional query params: sni=<name>, allowInsecure=true|1 (alias insecure=).
// Returns nil if the line is not a usable forward proxy (so other URI schemes
// and plain web URLs fall through to the normal parsing path).
func parseProxyURI(line string) *models.ProxyConfig {
	line = strings.TrimSpace(line)

	var scheme string
	for _, s := range []string{"socks5h", "socks5", "socks", "https", "http"} {
		if strings.HasPrefix(line, s+"://") {
			scheme = s
			line = line[len(s)+3:]
			break
		}
	}
	if scheme == "" {
		return nil
	}

	// Fragment -> name.
	name := ""
	if i := strings.IndexByte(line, '#'); i >= 0 {
		name = line[i+1:]
		line = line[:i]
	}
	// Query params.
	var query url.Values
	if i := strings.IndexByte(line, '?'); i >= 0 {
		query, _ = url.ParseQuery(line[i+1:])
		line = line[:i]
	}
	// Path: forward proxies have none; a real path means this is a web URL.
	if i := strings.IndexByte(line, '/'); i >= 0 {
		path := line[i:]
		line = line[:i]
		if (scheme == "http" || scheme == "https") && path != "/" {
			return nil
		}
	}

	// userinfo@host:port (last '@'; base64 userinfo never contains '@').
	userinfo := ""
	hostport := line
	if i := strings.LastIndexByte(line, '@'); i >= 0 {
		userinfo = line[:i]
		hostport = line[i+1:]
	}

	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil || host == "" {
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil
	}

	pc := &models.ProxyConfig{Server: host, Port: port, Type: "tcp"}
	switch scheme {
	case "socks", "socks5", "socks5h":
		pc.Protocol = "socks"
	case "http":
		pc.Protocol = "http"
	case "https":
		pc.Protocol = "http"
		pc.Security = "tls"
	}

	// Credentials: explicit "user:pass", else base64(user:pass), else user only.
	if userinfo != "" {
		if i := strings.IndexByte(userinfo, ':'); i >= 0 {
			pc.Username, _ = url.QueryUnescape(userinfo[:i])
			pc.Password, _ = url.QueryUnescape(userinfo[i+1:])
		} else if decoded, derr := base64.StdEncoding.DecodeString(userinfo); derr == nil && strings.Contains(string(decoded), ":") {
			parts := strings.SplitN(string(decoded), ":", 2)
			pc.Username, pc.Password = parts[0], parts[1]
		} else {
			pc.Username, _ = url.QueryUnescape(userinfo)
		}
	}

	if sni := query.Get("sni"); sni != "" {
		pc.SNI = sni
	}
	if pc.Security == "tls" {
		if pc.SNI == "" {
			pc.SNI = host
		}
		// xray-core removed allowInsecure; pin a (self-signed) cert by its
		// sha256 instead, or verify against a specific name.
		if v := query.Get("pinnedPeerCertSha256"); v != "" {
			pc.PinnedPeerCertSha256 = v
		} else if v := query.Get("pcs"); v != "" {
			pc.PinnedPeerCertSha256 = v
		}
		if v := query.Get("verifyPeerCertByName"); v != "" {
			pc.VerifyPeerCertByName = v
		} else if v := query.Get("vcn"); v != "" {
			pc.VerifyPeerCertByName = v
		}
	}

	if name != "" {
		if un, derr := url.PathUnescape(name); derr == nil {
			pc.Name = un
		} else {
			pc.Name = name
		}
	} else {
		pc.Name = fmt.Sprintf("%s:%d", host, port)
	}

	return pc
}

// parseViaLibXray attempts to parse all configs at once via libXray.
// Returns parsed configs or nil if parsing fails.
func (p *Parser) parseViaLibXray(cleanedData []byte, originalData map[string]*originalLinkData) []*models.ProxyConfig {
	base64Data := base64.StdEncoding.EncodeToString(cleanedData)

	resultBase64 := libXray.ConvertShareLinksToXrayJson(base64Data)

	resultBytes, err := base64.StdEncoding.DecodeString(resultBase64)
	if err != nil {
		logger.Debug("Failed to decode libXray response: %v", err)
		return nil
	}

	var response libXrayResponse
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		logger.Debug("Failed to parse libXray response: %v", err)
		return nil
	}

	if !response.Success {
		logger.Debug("libXray batch parsing returned success=false")
		return nil
	}

	return p.extractOutbounds(response.Data, originalData)
}

// parseLineByLine parses each config line individually, skipping broken ones.
func (p *Parser) parseLineByLine(cleanedData []byte, originalData map[string]*originalLinkData) []*models.ProxyConfig {
	lines := strings.Split(string(cleanedData), "\n")
	var allConfigs []*models.ProxyConfig
	skippedCount := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lineBase64 := base64.StdEncoding.EncodeToString([]byte(line))
		resultBase64 := libXray.ConvertShareLinksToXrayJson(lineBase64)

		resultBytes, err := base64.StdEncoding.DecodeString(resultBase64)
		if err != nil {
			logger.Warn("Skipping invalid config line (decode error): %.50s...", line)
			skippedCount++
			continue
		}

		var response libXrayResponse
		if err := json.Unmarshal(resultBytes, &response); err != nil {
			logger.Warn("Skipping invalid config line (parse error): %.50s...", line)
			skippedCount++
			continue
		}

		if !response.Success {
			logger.Warn("Skipping invalid config line (libXray error): %.50s...", line)
			skippedCount++
			continue
		}

		configs := p.extractOutbounds(response.Data, originalData)
		allConfigs = append(allConfigs, configs...)
	}

	if skippedCount > 0 {
		logger.Warn("Skipped %d invalid config line(s) during parsing", skippedCount)
	}

	// Re-index configs
	for i, cfg := range allConfigs {
		cfg.Index = i
	}

	return allConfigs
}

// extractOutbounds extracts proxy configs from libXray response data.
func (p *Parser) extractOutbounds(data json.RawMessage, originalData map[string]*originalLinkData) []*models.ProxyConfig {
	var xrayConfig struct {
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(data, &xrayConfig); err != nil {
		logger.Debug("Failed to parse libXray config data: %v", err)
		return nil
	}

	logger.Debug("Parsed %d outbounds", len(xrayConfig.Outbounds))

	var proxyConfigs []*models.ProxyConfig
	configIndex := 0
	for _, outboundRaw := range xrayConfig.Outbounds {
		proxyConfig, err := p.convertOutbound(outboundRaw, configIndex, originalData)
		if err != nil {
			logger.Debug("Skipping outbound: %v", err)
			continue
		}
		if proxyConfig != nil {
			proxyConfigs = append(proxyConfigs, proxyConfig)
			configIndex++
		}
	}

	return proxyConfigs
}

// nameGroupedProxies assigns display names to the proxies parsed from a single JSON
// config (one `remarks` group). A single-node group takes the group name; a
// multi-node group (a balancer) names each node "<group> | <node>" so the nodes are
// tracked individually. Each proxy's Name is expected to already hold its outbound
// tag (set by convertOutbound); server:port is used as a fallback and to
// disambiguate nodes that share a tag.
func nameGroupedProxies(remarks string, group []*models.ProxyConfig) {
	if len(group) == 0 {
		return
	}
	if len(group) == 1 {
		if remarks != "" {
			group[0].Name = remarks
		}
		return
	}

	nodeLabel := func(pc *models.ProxyConfig) string {
		if pc.Name != "" {
			return pc.Name
		}
		return fmt.Sprintf("%s:%d", pc.Server, pc.Port)
	}
	labelCounts := make(map[string]int, len(group))
	for _, pc := range group {
		labelCounts[nodeLabel(pc)]++
	}
	for _, pc := range group {
		node := nodeLabel(pc)
		if labelCounts[node] > 1 {
			node = fmt.Sprintf("%s (%s:%d)", node, pc.Server, pc.Port)
		}
		if remarks != "" {
			pc.Name = fmt.Sprintf("%s | %s", remarks, node)
			pc.GroupName = remarks
		} else {
			pc.Name = node
		}
	}
}

func (p *Parser) parseJSONConfigs(data []byte) ([]*models.ProxyConfig, error) {
	var configs []struct {
		Remarks   string            `json:"remarks"`
		Outbounds []json.RawMessage `json:"outbounds"`
	}

	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to parse JSON configs: %v", err)
	}

	logger.Debug("Parsed %d JSON configs", len(configs))

	var proxyConfigs []*models.ProxyConfig
	configIndex := 0

	for _, config := range configs {
		var group []*models.ProxyConfig
		for _, outboundRaw := range config.Outbounds {
			proxyConfig, err := p.convertOutbound(outboundRaw, configIndex, nil)
			if err != nil || proxyConfig == nil {
				continue
			}
			group = append(group, proxyConfig)
			configIndex++
		}
		nameGroupedProxies(config.Remarks, group)
		proxyConfigs = append(proxyConfigs, group...)
	}

	if len(proxyConfigs) == 0 {
		return nil, fmt.Errorf("no valid proxy configurations found in JSON")
	}

	return proxyConfigs, nil
}

func (p *Parser) parseSingleJSONConfig(data []byte) ([]*models.ProxyConfig, error) {
	var config struct {
		Remarks   string            `json:"remarks"`
		Outbounds []json.RawMessage `json:"outbounds"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse single JSON config: %v", err)
	}

	logger.Debug("Parsed single JSON config with %d outbounds", len(config.Outbounds))

	var proxyConfigs []*models.ProxyConfig
	configIndex := 0

	for _, outboundRaw := range config.Outbounds {
		proxyConfig, err := p.convertOutbound(outboundRaw, configIndex, nil)
		if err != nil || proxyConfig == nil {
			continue
		}
		proxyConfigs = append(proxyConfigs, proxyConfig)
		configIndex++
	}

	nameGroupedProxies(config.Remarks, proxyConfigs)

	if len(proxyConfigs) == 0 {
		return nil, fmt.Errorf("no valid proxy configurations found in single JSON config")
	}

	return proxyConfigs, nil
}

func (p *Parser) cleanEmptyLines(data []byte) []byte {
	decoded := p.tryDecodeBase64(data)

	lines := strings.Split(string(decoded), "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return []byte(strings.Join(cleanLines, "\n"))
}

func (p *Parser) detectSourceType(source string) string {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return "url"
	}
	if strings.HasPrefix(source, "folder://") {
		return "folder"
	}
	if strings.HasPrefix(source, "file://") {
		return "file"
	}
	if strings.HasPrefix(source, "base64://") || strings.HasPrefix(source, "data:text/plain;base64,") {
		return "base64"
	}
	return "raw"
}

func (p *Parser) fetchURLContent(source string) (*fetchResult, error) {
	cleanURL, fragmentName := p.extractURLFragment(source)

	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", cleanURL, nil)
	if err != nil {
		return nil, err
	}

	sub := config.CLIConfig.Subscription
	req.Header.Set("Accept", "*/*")
	switch {
	case sub.UserAgent != "":
		// Explicit override: the user controls exactly which client to impersonate.
		req.Header.Set("User-Agent", sub.UserAgent)
	case config.CLIConfig.Proxy.Engine == "mihomo":
		req.Header.Set("User-Agent", "mihomo")
		req.Header.Set("X-Hwid", subscriptionHWID)
	case sub.JSONFormat:
		// App-like headers so panels (e.g. Remnawave) return full JSON configs with
		// individual grouped/balancer nodes instead of collapsed base64 share links.
		req.Header.Set("User-Agent", subscriptionJSONUserAgent)
		req.Header.Set("X-Hwid", subscriptionHWID)
	default:
		req.Header.Set("User-Agent", "Xray-Checker")
		req.Header.Set("X-Device-OS", "CheckerOS")
		req.Header.Set("X-Ver-OS", config.Version)
		req.Header.Set("X-Device-Model", "Xray-Checker Pro Max")
		req.Header.Set("X-Hwid", subscriptionHWID)
	}

	// User-supplied headers are applied last so they can override any of the above.
	for _, h := range sub.Headers {
		key, value, ok := strings.Cut(h, ":")
		if !ok {
			logger.Warn("Ignoring malformed subscription header (want 'Key: Value'): %s", h)
			continue
		}
		req.Header.Set(strings.TrimSpace(key), strings.TrimSpace(value))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	name := fragmentName
	if name == "" {
		name = p.extractNameFromHeader(resp.Header.Get("profile-title"))
	}

	return &fetchResult{
		Content: content,
		Name:    name,
	}, nil
}

func (p *Parser) extractURLFragment(source string) (cleanURL string, name string) {
	if idx := strings.LastIndex(source, "#"); idx != -1 {
		name = strings.TrimSpace(source[idx+1:])
		cleanURL = source[:idx]
		if decoded, err := url.QueryUnescape(name); err == nil {
			name = decoded
		}
		return cleanURL, name
	}
	return source, ""
}

func (p *Parser) extractNameFromHeader(headerValue string) string {
	if headerValue == "" {
		return ""
	}

	headerValue = strings.TrimSpace(headerValue)

	if strings.HasPrefix(headerValue, "base64:") {
		encoded := strings.TrimPrefix(headerValue, "base64:")
		if decoded, err := p.decodeBase64(encoded); err == nil {
			return strings.TrimSpace(string(decoded))
		}
		return ""
	}

	if decoded, err := p.decodeBase64(headerValue); err == nil {
		decodedStr := string(decoded)
		if p.isPrintableString(decodedStr) {
			return strings.TrimSpace(decodedStr)
		}
	}

	return headerValue
}

func (p *Parser) isPrintableString(s string) bool {
	for _, r := range s {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

func (p *Parser) parseOriginalLinks(rawData []byte) map[string]*originalLinkData {
	result := make(map[string]*originalLinkData)

	decoded := p.tryDecodeBase64(rawData)

	lines := strings.Split(string(decoded), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		data := p.parseShareLink(line)
		if data != nil {
			key := fmt.Sprintf("%s:%d", data.Server, data.Port)
			result[key] = &originalLinkData{
				Name:          data.Name,
				Encryption:    data.Encryption,
				Type:          data.Type,
				Path:          data.Path,
				Host:          data.Host,
				AllowInsecure: data.AllowInsecure,
			}
		}
	}

	return result
}

func (p *Parser) parseShareLink(link string) *parsedLink {
	if strings.HasPrefix(link, "vmess://") {
		return p.parseVMessLink(link)
	}

	u, err := url.Parse(link)
	if err != nil {
		return nil
	}

	result := &parsedLink{
		Name: u.Fragment,
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port == 0 {
		return nil
	}
	result.Server = host
	result.Port = port

	query := u.Query()
	result.Type = query.Get("type")
	result.Encryption = query.Get("encryption")
	result.Path = query.Get("path")
	result.Host = query.Get("host")
	result.AllowInsecure = query.Get("allowInsecure") == "1" || query.Get("allowInsecure") == "true"

	return result
}

func (p *Parser) parseVMessLink(link string) *parsedLink {
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := p.decodeBase64(encoded)
	if err != nil {
		return nil
	}

	var vmess map[string]interface{}
	if err := json.Unmarshal(decoded, &vmess); err != nil {
		return nil
	}

	result := &parsedLink{}

	if ps, ok := vmess["ps"].(string); ok {
		result.Name = ps
	}
	if add, ok := vmess["add"].(string); ok {
		result.Server = add
	}

	switch port := vmess["port"].(type) {
	case float64:
		result.Port = int(port)
	case string:
		if p, err := strconv.Atoi(port); err == nil {
			result.Port = p
		}
	}

	if result.Port == 0 {
		return nil
	}

	if net, ok := vmess["net"].(string); ok {
		result.Type = net
	}
	if host, ok := vmess["host"].(string); ok {
		result.Host = host
	}
	if path, ok := vmess["path"].(string); ok {
		result.Path = path
	}

	return result
}

// convertWireGuardOutbound fills a ProxyConfig from an xray "wireguard" outbound's
// settings: secretKey/address/mtu plus the first peer's publicKey/endpoint/
// allowedIPs/keepAlive/preSharedKey. Server/Port hold the peer endpoint.
func (p *Parser) convertWireGuardOutbound(settings json.RawMessage, pc *models.ProxyConfig) (*models.ProxyConfig, error) {
	var wg struct {
		SecretKey string   `json:"secretKey"`
		Address   []string `json:"address"`
		MTU       int      `json:"mtu"`
		Peers     []struct {
			PublicKey    string   `json:"publicKey"`
			PreSharedKey string   `json:"preSharedKey"`
			Endpoint     string   `json:"endpoint"`
			KeepAlive    int      `json:"keepAlive"`
			AllowedIPs   []string `json:"allowedIPs"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(settings, &wg); err != nil {
		return nil, fmt.Errorf("failed to parse wireguard settings: %v", err)
	}
	if len(wg.Peers) == 0 {
		return nil, fmt.Errorf("no wireguard peers found")
	}
	peer := wg.Peers[0]
	host, portStr, err := net.SplitHostPort(peer.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid wireguard endpoint %q: %v", peer.Endpoint, err)
	}
	pc.Server = host
	pc.Port = atoiOrZero(portStr)
	pc.WGPrivateKey = wg.SecretKey
	pc.WGAddresses = wg.Address
	pc.WGMTU = wg.MTU
	pc.WGPeerPublicKey = peer.PublicKey
	pc.WGPreSharedKey = peer.PreSharedKey
	pc.WGAllowedIPs = peer.AllowedIPs
	pc.WGKeepalive = peer.KeepAlive

	if pc.WGPrivateKey == "" || pc.WGPeerPublicKey == "" || pc.Server == "" || pc.Port == 0 {
		return nil, fmt.Errorf("incomplete wireguard config (missing secretKey/publicKey/endpoint)")
	}
	if pc.Name == "" {
		pc.Name = fmt.Sprintf("wireguard-%s", pc.Server)
	}
	return pc, nil
}

// sanitizeMetricsLabels trims label keys and drops entries with an empty key or
// value. It returns nil for an empty result so proxies without custom labels keep
// a nil map. Prometheus-name validity is enforced later in the metrics collector.
func sanitizeMetricsLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" || v == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *Parser) convertOutbound(raw json.RawMessage, index int, originalData map[string]*originalLinkData) (*models.ProxyConfig, error) {
	var baseOutbound struct {
		Protocol       string                 `json:"protocol"`
		Tag            string                 `json:"tag"`
		SendThrough    string                 `json:"sendThrough"`
		Settings       json.RawMessage        `json:"settings"`
		StreamSettings *libXrayStreamSettings `json:"streamSettings"`
		MetricsLabels  map[string]string      `json:"metricsLabels"`
	}
	if err := json.Unmarshal(raw, &baseOutbound); err != nil {
		return nil, err
	}

	if baseOutbound.Protocol == "freedom" || baseOutbound.Protocol == "blackhole" || baseOutbound.Protocol == "dns" {
		return nil, nil
	}

	pc := &models.ProxyConfig{
		Index:         index,
		Name:          baseOutbound.SendThrough,
		Protocol:      baseOutbound.Protocol,
		MetricsLabels: sanitizeMetricsLabels(baseOutbound.MetricsLabels),
	}

	if pc.Name == "" {
		pc.Name = baseOutbound.Tag
	}

	// WireGuard outbounds have a distinct settings shape (secretKey/address/peers)
	// and no streamSettings, so handle them up front.
	if baseOutbound.Protocol == "wireguard" {
		return p.convertWireGuardOutbound(baseOutbound.Settings, pc)
	}

	var flatSettings libXraySettings
	if err := json.Unmarshal(baseOutbound.Settings, &flatSettings); err == nil && flatSettings.Address != "" {
		pc.Server = flatSettings.Address
		pc.Port = flatSettings.Port

		switch baseOutbound.Protocol {
		case "vless":
			pc.UUID = flatSettings.ID
			pc.Flow = flatSettings.Flow
			pc.Encryption = flatSettings.Encryption
			pc.Level = flatSettings.Level
		case "vmess":
			pc.UUID = flatSettings.ID
			pc.AlterId = flatSettings.AlterId
			pc.Security = flatSettings.Security
			pc.Level = flatSettings.Level
		case "trojan":
			pc.Password = flatSettings.Password
		case "shadowsocks":
			pc.Password = flatSettings.Password
			pc.Method = flatSettings.Method
		case "hysteria":
			pc.HysteriaAuth = flatSettings.Auth
		case "socks", "http":
			pc.Username = flatSettings.User
			pc.Password = flatSettings.Pass
		}
	} else {
		var stdSettings xrayStandardSettings
		if err := json.Unmarshal(baseOutbound.Settings, &stdSettings); err != nil {
			return nil, fmt.Errorf("failed to parse settings: %v", err)
		}

		switch baseOutbound.Protocol {
		case "vless", "vmess":
			if len(stdSettings.Vnext) == 0 || len(stdSettings.Vnext[0].Users) == 0 {
				return nil, fmt.Errorf("no vnext/users found")
			}
			pc.Server = stdSettings.Vnext[0].Address
			pc.Port = stdSettings.Vnext[0].Port
			user := stdSettings.Vnext[0].Users[0]
			pc.UUID = user.ID
			pc.Flow = user.Flow
			pc.Encryption = user.Encryption
			pc.AlterId = user.AlterId
			pc.Level = user.Level
			if baseOutbound.Protocol == "vmess" {
				pc.Security = user.Security
			}
		case "trojan", "shadowsocks":
			if len(stdSettings.Servers) == 0 {
				return nil, fmt.Errorf("no servers found")
			}
			srv := stdSettings.Servers[0]
			pc.Server = srv.Address
			pc.Port = srv.Port
			pc.Password = srv.Password
			pc.Method = srv.Method
			pc.Flow = srv.Flow
		case "socks", "http":
			if len(stdSettings.Servers) == 0 {
				return nil, fmt.Errorf("no servers found")
			}
			srv := stdSettings.Servers[0]
			pc.Server = srv.Address
			pc.Port = srv.Port
			if len(srv.Users) > 0 {
				pc.Username = srv.Users[0].User
				pc.Password = srv.Users[0].Pass
			}
		case "hysteria":
			var hySettings struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
				Version int32  `json:"version"`
				Auth    string `json:"auth"`
			}
			if err := json.Unmarshal(baseOutbound.Settings, &hySettings); err != nil {
				return nil, fmt.Errorf("failed to parse hysteria settings: %v", err)
			}
			pc.Server = hySettings.Address
			pc.Port = hySettings.Port
			pc.HysteriaAuth = hySettings.Auth
		default:
			return nil, fmt.Errorf("unsupported protocol: %s", baseOutbound.Protocol)
		}
	}

	if pc.Server == "" || pc.Port == 0 {
		return nil, fmt.Errorf("failed to parse server/port")
	}

	if pc.Port == 0 || pc.Port == 1 {
		return nil, nil
	}

	if baseOutbound.StreamSettings != nil {
		ss := baseOutbound.StreamSettings
		pc.Type = ss.Network
		pc.Security = ss.Security

		if ss.TlsSettings != nil {
			pc.SNI = ss.TlsSettings.ServerName
			pc.AllowInsecure = ss.TlsSettings.AllowInsecure
			pc.Fingerprint = ss.TlsSettings.Fingerprint
			pc.ALPN = ss.TlsSettings.Alpn
			pc.PinnedPeerCertSha256 = ss.TlsSettings.PinnedPeerCertSha256
			pc.VerifyPeerCertByName = ss.TlsSettings.VerifyPeerCertByName
		}

		if ss.RealitySettings != nil {
			pc.SNI = ss.RealitySettings.ServerName
			pc.Fingerprint = ss.RealitySettings.Fingerprint
			pc.PublicKey = ss.RealitySettings.PublicKey
			pc.ShortID = ss.RealitySettings.ShortId
		}

		if ss.Network == "raw" {
			pc.Type = "tcp"
		}

		if ss.RawSettings != nil && ss.RawSettings.Header != nil {
			pc.HeaderType = ss.RawSettings.Header.Type
			if ss.RawSettings.Header.Request != nil {
				if len(ss.RawSettings.Header.Request.Path) > 0 {
					pc.Path = ss.RawSettings.Header.Request.Path[0]
				}
				if ss.RawSettings.Header.Request.Headers != nil && len(ss.RawSettings.Header.Request.Headers.Host) > 0 {
					pc.Host = ss.RawSettings.Header.Request.Headers.Host[0]
				}
			}
		}

		if ss.WsSettings != nil {
			pc.Path = ss.WsSettings.Path
			if ss.WsSettings.Headers != nil {
				pc.Host = ss.WsSettings.Headers.Host
			}
			if pc.Host == "" {
				pc.Host = ss.WsSettings.Host
			}
		}

		if ss.GrpcSettings != nil {
			pc.ServiceName = ss.GrpcSettings.ServiceName
			pc.MultiMode = ss.GrpcSettings.MultiMode
		}

		if ss.HttpSettings != nil {
			pc.Path = ss.HttpSettings.Path
			if len(ss.HttpSettings.Host) > 0 {
				pc.Host = strings.Join(ss.HttpSettings.Host, ",")
			}
		}

		if ss.HttpupgradeSettings != nil {
			pc.Type = "httpupgrade"
			pc.Path = ss.HttpupgradeSettings.Path
			pc.Host = ss.HttpupgradeSettings.Host
		}

		if (ss.Network == "kcp" || ss.Network == "mkcp") && len(ss.KcpSettings) > 0 {
			pc.RawKcpSettings = string(ss.KcpSettings)
		}

		if ss.Network == "xhttp" || ss.Network == "splithttp" {
			pc.Type = ss.Network

			var rawSettings json.RawMessage
			if len(ss.XhttpSettings) > 0 {
				rawSettings = ss.XhttpSettings
			} else if len(ss.SplithttpSettings) > 0 {
				rawSettings = ss.SplithttpSettings
			}

			if len(rawSettings) > 0 {
				pc.RawXhttpSettings = string(rawSettings)
				var parsed libXrayXhttpSettings
				if err := json.Unmarshal(rawSettings, &parsed); err == nil {
					pc.Path = parsed.Path
					pc.Host = parsed.Host
					pc.Mode = parsed.Mode
				}
			}
		}

		// Hysteria stream settings
		if ss.HysteriaSettings != nil {
			if pc.HysteriaAuth == "" {
				pc.HysteriaAuth = ss.HysteriaSettings.Auth
			}
		}

		// Extract QuicParams and Salamander from FinalMask
		// FinalMask can be at streamSettings level or inside sockopt
		finalMask := ss.FinalMask
		if finalMask == nil && ss.Sockopt != nil {
			finalMask = ss.Sockopt.FinalMask
		}
		if finalMask != nil {
			if finalMask.QuicParams != nil {
				qp := finalMask.QuicParams
				pc.HysteriaUp = qp.BrutalUp
				pc.HysteriaDown = qp.BrutalDown
				if qp.UdpHop != nil {
					if qp.UdpHop.PortList != nil {
						var ports string
						if err := json.Unmarshal(qp.UdpHop.PortList, &ports); err == nil {
							pc.HysteriaPorts = ports
						} else {
							logger.Debug("Failed to parse UdpHop portList as string: %v", err)
						}
					}
					if qp.UdpHop.Interval != nil {
						pc.HysteriaHopInterval = qp.UdpHop.Interval.From
					}
				}
			}
			if len(finalMask.Udp) > 0 {
				mask := finalMask.Udp[0]
				if mask.Type == "salamander" && mask.Settings != nil {
					pc.HysteriaObfs = "salamander"
					var sal libXraySalamander
					if err := json.Unmarshal(*mask.Settings, &sal); err == nil {
						pc.HysteriaObfsPassword = sal.Password
					} else {
						logger.Debug("Failed to parse salamander settings: %v", err)
					}
				}
			}
		}
	}

	key := fmt.Sprintf("%s:%d", pc.Server, pc.Port)
	if orig, ok := originalData[key]; ok {
		if pc.Encryption == "" || pc.Encryption == "none" {
			if orig.Encryption != "" {
				pc.Encryption = orig.Encryption
			}
		}
		if orig.AllowInsecure {
			pc.AllowInsecure = true
		}
	}

	if err := pc.Validate(); err != nil {
		return nil, err
	}

	pc.StableID = pc.GenerateStableID()

	return pc, nil
}

func (p *Parser) tryDecodeBase64(data []byte) []byte {
	text := strings.TrimSpace(string(data))

	if strings.HasPrefix(text, "vless://") || strings.HasPrefix(text, "vmess://") ||
		strings.HasPrefix(text, "trojan://") || strings.HasPrefix(text, "ss://") ||
		strings.HasPrefix(text, "hysteria2://") || strings.HasPrefix(text, "hy2://") ||
		strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return data
	}

	decoded, err := p.decodeBase64(text)
	if err != nil {
		return data
	}

	return decoded
}

func (p *Parser) decodeBase64(text string) ([]byte, error) {
	text = strings.ReplaceAll(text, "-", "+")
	text = strings.ReplaceAll(text, "_", "/")

	if m := len(text) % 4; m != 0 {
		text += strings.Repeat("=", 4-m)
	}

	return base64.StdEncoding.DecodeString(text)
}

func (p *Parser) parseFolder(folderPath string) ([]*models.ProxyConfig, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read folder: %v", err)
	}

	var allConfigs []*models.ProxyConfig
	configIndex := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		ext := strings.ToLower(filepath.Ext(fileName))
		if ext != ".json" && !(config.CLIConfig.Proxy.Engine == "mihomo" && (ext == ".yaml" || ext == ".yml")) {
			continue
		}

		filePath := filepath.Join(folderPath, fileName)
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("Failed to read file %s: %v", fileName, err)
			continue
		}

		configs, err := p.parseSingleConfigFile(data, configIndex)
		if err != nil {
			logger.Warn("Failed to parse file %s: %v", fileName, err)
			continue
		}

		for _, cfg := range configs {
			cfg.Index = configIndex
			allConfigs = append(allConfigs, cfg)
			configIndex++
		}

		logger.Debug("Parsed %d configs from %s", len(configs), fileName)
	}

	if len(allConfigs) == 0 {
		return nil, fmt.Errorf("no valid proxy configurations found in folder")
	}

	logger.Debug("Total configs from folder: %d", len(allConfigs))
	return allConfigs, nil
}

func (p *Parser) parseSingleConfigFile(data []byte, startIndex int) ([]*models.ProxyConfig, error) {
	if config.CLIConfig.Proxy.Engine == "mihomo" {
		return parseMihomoSubscription(data)
	}
	if isMihomoSubscription(data) {
		return nil, fmt.Errorf("Mihomo subscription requires --proxy-engine=mihomo")
	}
	trimmedData := strings.TrimSpace(string(data))

	if strings.HasPrefix(trimmedData, "[") {
		return p.parseJSONConfigs(data)
	}

	if strings.HasPrefix(trimmedData, "{") {
		var config struct {
			Remarks   string            `json:"remarks"`
			Outbounds []json.RawMessage `json:"outbounds"`
		}

		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %v", err)
		}

		var proxyConfigs []*models.ProxyConfig
		for _, outboundRaw := range config.Outbounds {
			proxyConfig, err := p.convertOutbound(outboundRaw, startIndex, nil)
			if err != nil || proxyConfig == nil {
				continue
			}
			proxyConfigs = append(proxyConfigs, proxyConfig)
		}

		nameGroupedProxies(config.Remarks, proxyConfigs)

		if len(proxyConfigs) == 0 {
			return nil, fmt.Errorf("no valid proxy configurations found")
		}

		return proxyConfigs, nil
	}

	return nil, fmt.Errorf("unsupported config format")
}
