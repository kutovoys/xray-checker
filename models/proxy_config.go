package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type ProxyConfig struct {
	Protocol         string
	Server           string
	Port             int
	Name             string
	Security         string
	Type             string
	UUID             string
	Flow             string
	Encryption       string
	HeaderType       string
	Path             string
	Host             string
	SNI              string
	Fingerprint      string
	PublicKey        string
	ShortID          string
	Mode             string
	Password         string
	Method           string
	Level            int
	AlterId          int
	VMessAid         int
	MultiMode        bool
	ServiceName      string
	IdleTimeout      int
	WindowsSize      int
	AllowInsecure    bool
	ALPN             []string
	Index            int
	Settings         map[string]string
	StableID         string
	RawXhttpSettings string
	SubName          string
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
	default:
		return fmt.Errorf("unsupported protocol: %s", pc.Protocol)
	}

	return nil
}

func (pc *ProxyConfig) GenerateStableID() string {
	var idComponents []string

	idComponents = append(idComponents, pc.Protocol)

	idComponents = append(idComponents, pc.Server)
	idComponents = append(idComponents, fmt.Sprintf("%d", pc.Port))

	switch pc.Protocol {
	case "vless", "vmess":
		if pc.UUID != "" {
			idComponents = append(idComponents, pc.UUID)
		}
	case "trojan", "shadowsocks":
		if pc.Password != "" {
			idComponents = append(idComponents, pc.Password)
		}
		if pc.Protocol == "shadowsocks" && pc.Method != "" {
			idComponents = append(idComponents, pc.Method)
		}
	}

	if pc.SNI != "" {
		idComponents = append(idComponents, pc.SNI)
	}

	if pc.Type != "" {
		idComponents = append(idComponents, pc.Type)
	}

	if pc.Security != "" {
		idComponents = append(idComponents, pc.Security)
	}

	if pc.PublicKey != "" {
		idComponents = append(idComponents, pc.PublicKey)
	}

	if pc.Path != "" {
		idComponents = append(idComponents, pc.Path)
	}

	if pc.Host != "" {
		idComponents = append(idComponents, pc.Host)
	}

	if pc.ServiceName != "" {
		idComponents = append(idComponents, pc.ServiceName)
	}

	if pc.ShortID != "" {
		idComponents = append(idComponents, pc.ShortID)
	}

	idString := strings.Join(idComponents, "|")

	hash := sha256.Sum256([]byte(idString))

	return hex.EncodeToString(hash[:])[:16]
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
