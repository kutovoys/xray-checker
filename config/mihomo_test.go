package config

import (
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestValidateProbeEngine(t *testing.T) {
	for _, tc := range []struct {
		name           string
		engine         string
		binary         string
		jsonFormat     bool
		resolveDomains bool
		wantError      string
	}{
		{name: "legacy zero value"},
		{name: "existing Xray JSON", engine: "xray", jsonFormat: true},
		{name: "Mihomo subscription", engine: "mihomo", binary: "mihomo"},
		{name: "unknown backend", engine: "unknown", wantError: "--proxy-engine"},
		{name: "incompatible JSON request", engine: "mihomo", binary: "mihomo", jsonFormat: true, wantError: "--subscription-json-format"},
		{name: "missing Mihomo executable", engine: "mihomo", wantError: "--mihomo-binary"},
		{name: "existing Xray domain expansion", engine: "xray", resolveDomains: true},
		{name: "Mihomo must retain implicit SNI", engine: "mihomo", binary: "mihomo", resolveDomains: true, wantError: "--proxy-resolve-domains"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cli CLI
			cli.Proxy.Engine = tc.engine
			cli.Mihomo.Binary = tc.binary
			cli.Subscription.JSONFormat = tc.jsonFormat
			cli.Proxy.ResolveDomains = tc.resolveDomains
			err := cli.Validate()
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want a message containing %q", err, tc.wantError)
			}
		})
	}
}

func TestProbeEngineCLIDefaultsRemainXray(t *testing.T) {
	for _, name := range []string{"PROXY_ENGINE", "MIHOMO_BINARY"} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
	var cli CLI
	parser, err := kong.New(&cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"--subscription-url=file:///unused.json"}); err != nil {
		t.Fatal(err)
	}
	if cli.Proxy.Engine != "xray" {
		t.Fatalf("default backend = %q, want xray", cli.Proxy.Engine)
	}
	if cli.Mihomo.Binary != "mihomo" {
		t.Fatalf("default optional executable = %q, want mihomo", cli.Mihomo.Binary)
	}
}
