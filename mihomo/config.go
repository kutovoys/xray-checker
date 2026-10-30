// Package mihomo runs checks through an explicitly selected, separately installed
// Mihomo core. Only proxy definitions are imported from a subscription: listeners,
// routing, DNS and control interfaces are owned by the checker.
package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"xray-checker/models"

	"gopkg.in/yaml.v3"
)

const validationTimeout = 30 * time.Second

type ConfigGenerator struct {
	binary string
}

func NewConfigGenerator(binary string) *ConfigGenerator {
	return &ConfigGenerator{binary: binary}
}

type listenerConfig struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Listen string `yaml:"listen"`
	Port   int    `yaml:"port"`
	UDP    bool   `yaml:"udp"`
	Proxy  string `yaml:"proxy"`
}

type generatedConfig struct {
	Mode               string           `yaml:"mode"`
	LogLevel           string           `yaml:"log-level"`
	AllowLAN           bool             `yaml:"allow-lan"`
	BindAddress        string           `yaml:"bind-address"`
	ExternalController string           `yaml:"external-controller"`
	IPv6               bool             `yaml:"ipv6"`
	DNS                map[string]any   `yaml:"dns"`
	TUN                map[string]any   `yaml:"tun"`
	Profile            map[string]any   `yaml:"profile"`
	Listeners          []listenerConfig `yaml:"listeners"`
	Proxies            []map[string]any `yaml:"proxies"`
	Rules              []string         `yaml:"rules"`
}

// GenerateConfig preserves each proxy's native options, including smux and
// reality-opts. A private SOCKS listener targets each proxy directly, so health
// checks cannot fall back through a subscription's selection groups or rules.
func (g *ConfigGenerator) GenerateConfig(proxies []*models.ProxyConfig, startPort int, logLevel string) ([]byte, error) {
	if len(proxies) == 0 {
		return nil, fmt.Errorf("Mihomo requires at least one proxy")
	}
	config := generatedConfig{
		Mode:               "rule",
		LogLevel:           mihomoLogLevel(logLevel),
		AllowLAN:           false,
		BindAddress:        "127.0.0.1",
		ExternalController: "",
		IPv6:               true,
		DNS:                map[string]any{"enable": false},
		TUN:                map[string]any{"enable": false},
		Profile:            map[string]any{"store-selected": false, "store-fake-ip": false},
		Rules:              []string{"MATCH,REJECT"},
	}
	used := make(map[int]bool, len(proxies))
	for i, proxy := range proxies {
		if proxy == nil || len(proxy.Mihomo) == 0 {
			return nil, fmt.Errorf("proxy %d has no native Mihomo definition; use a Mihomo subscription with this engine", i+1)
		}
		if proxy.Index < 0 || startPort < 1 || startPort > 65535 || proxy.Index > 65535-startPort {
			return nil, fmt.Errorf("proxy %d has an invalid SOCKS listener port", i+1)
		}
		port := startPort + proxy.Index
		if used[port] {
			return nil, fmt.Errorf("proxy %d has a duplicate SOCKS listener port", i+1)
		}
		used[port] = true

		// Round-tripping through JSON makes a deep copy even after domain expansion
		// cloned a ProxyConfig with its raw map shared. Do not modify the source.
		encoded, err := json.Marshal(proxy.Mihomo)
		if err != nil {
			return nil, fmt.Errorf("proxy %d contains an unsupported Mihomo value", i+1)
		}
		var raw map[string]any
		if err := json.Unmarshal(encoded, &raw); err != nil {
			return nil, fmt.Errorf("proxy %d contains an unsupported Mihomo value", i+1)
		}
		if dependency, present := raw["dialer-proxy"]; present && dependency != nil && dependency != "" {
			return nil, fmt.Errorf("proxy %d uses dialer-proxy; proxy chains and group references are not supported by isolated Mihomo checks", i+1)
		}
		name := "checker-proxy-" + strconv.Itoa(proxy.Index)
		raw["name"] = name
		raw["server"] = proxy.Server
		raw["port"] = proxy.Port
		config.Proxies = append(config.Proxies, raw)
		config.Listeners = append(config.Listeners, listenerConfig{
			Name:   "checker-socks-" + strconv.Itoa(proxy.Index),
			Type:   "socks",
			Listen: "127.0.0.1",
			Port:   port,
			UDP:    false,
			Proxy:  name,
		})
	}
	return yaml.Marshal(config)
}

func mihomoLogLevel(level string) string {
	switch strings.ToLower(level) {
	case "none", "silent", "":
		return "silent"
	case "debug", "info", "warning", "error":
		return strings.ToLower(level)
	default:
		return "warning"
	}
}

// GenerateValidatedConfig validates in a disposable private directory before
// replacing filename. It fails as a unit instead of silently skipping a node or
// retrying with options (such as multiplexing) removed.
func (g *ConfigGenerator) GenerateValidatedConfig(proxies []*models.ProxyConfig, startPort int, filename, logLevel string) ([]*models.ProxyConfig, error) {
	data, err := g.GenerateConfig(proxies, startPort, logLevel)
	if err != nil {
		return nil, err
	}
	binary, err := resolveBinary(g.binary)
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp("", "xray-checker-mihomo-validate-")
	if err != nil {
		return nil, fmt.Errorf("create Mihomo validation directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	privateConfig := filepath.Join(workDir, "config.yaml")
	if err := os.WriteFile(privateConfig, data, 0o600); err != nil {
		return nil, fmt.Errorf("write Mihomo validation configuration: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-t", "-f", privateConfig, "-d", workDir)
	cmd.Dir = workDir
	cmd.Env = childEnvironment()
	guard, err := newProcessGuard(cmd)
	if err != nil {
		return nil, fmt.Errorf("could not prepare Mihomo validation process supervision")
	}
	defer guard.close()
	// Linux Pdeathsig follows the thread that creates the child. Keep that
	// thread alive until validation has exited, even if the Go caller migrates.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Mihomo validation errors can quote proxy passwords or other raw fields.
	// Keep its stdout/stderr private; return only an exit status to the caller.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start Mihomo configuration validation")
	}
	if err := guard.attach(cmd); err != nil {
		// A short-lived validator may already have exited before Windows can
		// attach its job. Wait still retrieves its result; if it is alive and
		// cannot be supervised, terminate it instead of leaving an orphan.
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("Mihomo configuration validation timed out")
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("Mihomo rejected the generated configuration (exit %d); check the installed core version and proxy options", exitErr.ExitCode())
		}
		return nil, fmt.Errorf("could not run Mihomo configuration validation")
	}
	if err := writePrivateFile(filename, data); err != nil {
		return nil, err
	}
	return proxies, nil
}

func writePrivateFile(filename string, data []byte) error {
	dir := filepath.Dir(filename)
	file, err := os.CreateTemp(dir, ".mihomo-config-*")
	if err != nil {
		return fmt.Errorf("create Mihomo configuration: %w", err)
	}
	tmpName := file.Name()
	defer os.Remove(tmpName)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write Mihomo configuration: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Mihomo configuration: %w", err)
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("replace Mihomo configuration: %w", err)
	}
	return nil
}

func resolveBinary(binary string) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("Mihomo executable not found; install it and set --mihomo-binary")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not resolve Mihomo executable path")
	}
	return path, nil
}

// A checker process may share a shell/service environment with an existing
// Mihomo installation. Its overrides (including post-up shell hooks) must never
// alter the generated checker configuration or activate a controller/TUN device.
func childEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		key = strings.ToUpper(key)
		if strings.HasPrefix(key, "CLASH_") || strings.HasPrefix(key, "MIHOMO_") || key == "SAFE_PATHS" || key == "SKIP_SAFE_PATH_CHECK" {
			continue
		}
		env = append(env, value)
	}
	return env
}
