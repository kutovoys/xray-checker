package config

import (
	"fmt"

	"github.com/alecthomas/kong"
)

var CLIConfig CLI
var Version string

func Parse(version string) {
	Version = version
	ctx := kong.Parse(&CLIConfig,
		kong.Name("xray-checker"),
		kong.Description("Xray Checker: A Prometheus exporter for monitoring Xray proxies"),
		kong.Vars{
			"version": version,
		},
	)
	_ = ctx
}

type CLI struct {
	Subscription struct {
		URLs           []string `name:"subscription-url" help:"URL(s) of the subscription (can be specified multiple times)" required:"true" env:"SUBSCRIPTION_URL"`
		Update         bool     `name:"subscription-update" help:"Whether to recheck the subscription" default:"true" env:"SUBSCRIPTION_UPDATE"`
		UpdateInterval int      `name:"subscription-update-interval" help:"Interval for subscription updates in seconds" default:"300" env:"SUBSCRIPTION_UPDATE_INTERVAL"`
	} `embed:"" prefix:""`

	Proxy struct {
		CheckInterval   int    `name:"proxy-check-interval" help:"Interval for proxy checks in seconds" default:"300" env:"PROXY_CHECK_INTERVAL"`
		CheckMethod     string `name:"proxy-check-method" help:"Method for checking proxy, ip, status or download" default:"ip" env:"PROXY_CHECK_METHOD"`
		IpCheckUrl      string `name:"proxy-ip-check-url" help:"Service URL for IP checking" default:"https://api.ipify.org?format=text" env:"PROXY_IP_CHECK_URL"`
		StatusCheckUrl  string `name:"proxy-status-check-url" help:"Response status generator, used by check-method=status" default:"http://cp.cloudflare.com/generate_204" env:"PROXY_STATUS_CHECK_URL"`
		DownloadUrl     string `name:"proxy-download-url" help:"URL for file download checking, used by check-method=download" default:"https://proof.ovh.net/files/1Mb.dat" env:"PROXY_DOWNLOAD_URL"`
		DownloadTimeout int    `name:"proxy-download-timeout" help:"Timeout for download checking in seconds" default:"60" env:"PROXY_DOWNLOAD_TIMEOUT"`
		DownloadMinSize int64  `name:"proxy-download-min-size" help:"Minimum bytes to download for successful check" default:"51200" env:"PROXY_DOWNLOAD_MIN_SIZE"`
		Timeout         int    `name:"proxy-timeout" help:"Timeout for IP checking in seconds" default:"30" env:"PROXY_TIMEOUT"`
		SimulateLatency bool   `name:"simulate-latency" help:"Whether to add latency to the response" default:"true" env:"SIMULATE_LATENCY"`
		ResolveDomains  bool   `name:"proxy-resolve-domains" help:"Resolve proxy server domains into IPs and expand configs" env:"PROXY_RESOLVE_DOMAINS"`
	} `embed:"" prefix:""`

	Xray struct {
		StartPort int    `name:"xray-start-port" help:"Start port for proxy configuration" default:"10000" env:"XRAY_START_PORT"`
		LogLevel  string `name:"xray-log-level" help:"Xray log level (debug|info|warning|error|none)" default:"none" env:"XRAY_LOG_LEVEL"`
	} `embed:"" prefix:""`

	Speedtest struct {
		Enabled  bool `name:"speedtest-enabled" help:"Enable periodic download/upload speedtest through each proxy" default:"false" env:"SPEEDTEST_ENABLED"`
		Interval int  `name:"speedtest-interval" help:"Interval for speedtest checks in minutes" default:"240" env:"SPEEDTEST_INTERVAL"`
	} `embed:"" prefix:""`

	Metrics struct {
		Host      string `name:"metrics-host" help:"Host to listen on" default:"0.0.0.0" env:"METRICS_HOST"`
		Port      string `name:"metrics-port" help:"Port to listen on" default:"2112" env:"METRICS_PORT"`
		Protected bool   `name:"metrics-protected" help:"Whether metrics are protected by basic auth" default:"false" env:"METRICS_PROTECTED"`
		Username  string `name:"metrics-username" help:"Username for metrics if protected by basic auth" default:"metricsUser" env:"METRICS_USERNAME"`
		Password  string `name:"metrics-password" help:"Password for metrics if protected by basic auth" default:"MetricsVeryHardPassword" env:"METRICS_PASSWORD"`
		Instance  string `name:"metrics-instance" help:"Instance label for metrics" default:"" env:"METRICS_INSTANCE"`
		PushURL   string `name:"metrics-push-url" help:"Prometheus pushgateway URL (e.g. https://user:pass@host:port)" default:"" env:"METRICS_PUSH_URL"`
		BasePath  string `name:"metrics-base-path" help:"URL path to metrics (e.g. /xray/metrics)" default:"" env:"METRICS_BASE_PATH"`
	} `embed:"" prefix:""`

	Web struct {
		ShowServerDetails bool   `name:"web-show-details" help:"Show server IP addresses and ports in web UI" default:"false" env:"WEB_SHOW_DETAILS"`
		Public            bool   `name:"web-public" help:"Make dashboard public (requires --metrics-protected)" default:"false" env:"WEB_PUBLIC"`
		CustomAssetsPath  string `name:"web-custom-assets-path" help:"Path to custom assets directory (logo.svg, favicon.ico, custom.css, index.html)" default:"" env:"WEB_CUSTOM_ASSETS_PATH"`
	} `embed:"" prefix:""`

	Version  VersionFlag `name:"version" help:"Print version information and quit"`
	RunOnce  bool        `name:"run-once" help:"Run one check cycle and exit" default:"false" env:"RUN_ONCE"`
	LogLevel string      `name:"log-level" help:"Log level (debug|info|warn|error|none)" default:"info" env:"LOG_LEVEL"`
}

func (c *CLI) Validate() error {
	if c.Web.Public && !c.Metrics.Protected {
		return fmt.Errorf("--web-public requires --metrics-protected to be enabled")
	}
	return nil
}

type VersionFlag string

func (v VersionFlag) Decode(ctx *kong.DecodeContext) error { return nil }
func (v VersionFlag) IsBool() bool                         { return true }
func (v VersionFlag) BeforeApply(app *kong.Kong, vars kong.Vars) error {
	fmt.Println("Xray Checker: A Prometheus exporter for monitoring Xray proxies")
	fmt.Printf("Version:\t %s\n", vars["version"])
	fmt.Printf("GitHub: https://github.com/kutovoys/xray-checker\n")
	app.Exit(0)
	return nil
}
