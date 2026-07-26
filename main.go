package main

import (
	"net/http"
	"strings"
	"time"
	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/logger"
	"xray-checker/metrics"
	"xray-checker/models"
	"xray-checker/subscription"
	"xray-checker/web"
	"xray-checker/xray"

	"github.com/go-co-op/gocron"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version   = "unknown"
	startTime = time.Now()
)

func main() {
	config.Parse(version)

	logLevel := logger.ParseLevel(config.CLIConfig.LogLevel)
	logger.SetLevel(logLevel)

	logger.Startup("Xray Checker %s", version)
	if logLevel == logger.LevelNone {
		logger.Startup("Log level: none (silent mode)")
	}
	if config.CLIConfig.Xray.Interface != "" {
		logger.Info("Binding Xray proxy connections to interface %s", config.CLIConfig.Xray.Interface)
	}

	if err := web.InitAssetLoader(config.CLIConfig.Web.CustomAssetsPath); err != nil {
		logger.Fatal("Failed to initialize custom assets: %v", err)
	}

	geoManager := xray.NewGeoFileManager("")
	if err := geoManager.EnsureGeoFiles(); err != nil {
		logger.Fatal("Failed to ensure geo files: %v", err)
	}

	configFile := "xray_config.json"
	proxyConfigs, err := subscription.InitializeConfiguration(configFile, version)
	if err != nil {
		logger.Fatal("Error initializing configuration: %v", err)
	}

	logger.Info("Loaded %d proxy configurations", len(*proxyConfigs))

	if config.CLIConfig.Web.Public {
		if name := subscription.GetSubscriptionName(); name != "" {
			logger.Info("Subscription name for public status page: %s", name)
		}
	} else {
		subNames := web.CollectSubscriptionNames(*proxyConfigs)
		if len(subNames) > 0 {
			logger.Info("Subscriptions: %s", strings.Join(subNames, ", "))
		}
	}

	if logLevel == logger.LevelDebug {
		logger.Debug("=== Parsed Proxy Configurations ===")
		for _, pc := range *proxyConfigs {
			logger.Debug("%s", pc.DebugString())
		}
	}

	xrayRunner := xray.NewRunner(configFile)
	if err := xrayRunner.Start(); err != nil {
		logger.Fatal("Error starting Xray: %v", err)
	}

	defer func() {
		if err := xrayRunner.Stop(); err != nil {
			logger.Error("Error stopping Xray: %v", err)
		}
	}()

	proxyChecker := checker.NewProxyChecker(
		*proxyConfigs,
		config.CLIConfig.Xray.StartPort,
		config.CLIConfig.Proxy.IpCheckUrl,
		config.CLIConfig.Proxy.Timeout,
		config.CLIConfig.Proxy.StatusCheckUrl,
		config.CLIConfig.Proxy.DownloadUrl,
		config.CLIConfig.Proxy.DownloadTimeout,
		config.CLIConfig.Proxy.DownloadMinSize,
		config.CLIConfig.Proxy.CheckMethod,
		config.CLIConfig.Proxy.CheckConcurrency,
	)

	// The collector renders metrics from the checker's current proxy snapshot on
	// each scrape, so custom metricsLabels (#124) can change across subscription
	// updates without resetting other series.
	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.NewCollector(config.CLIConfig.Metrics.Instance, proxyChecker))

	runCheckIteration := func() {
		logger.Info("Starting proxy check iteration")
		start := time.Now()
		proxyChecker.CheckAllProxies()
		elapsed := time.Since(start)

		// Warn if a cycle overruns the interval: with PROXY_CHECK_CONCURRENCY set,
		// a large/slow proxy set can take longer than PROXY_CHECK_INTERVAL, so checks
		// (and metrics) effectively run less often than configured.
		if interval := config.CLIConfig.Proxy.CheckInterval; interval > 0 && elapsed > time.Duration(interval)*time.Second {
			// When a concurrency cap is set, raising it (or the interval) helps. When
			// unlimited (0), the cycle is already as parallel as it gets, so the only
			// useful lever is a longer interval.
			if config.CLIConfig.Proxy.CheckConcurrency > 0 {
				logger.Warn("Check cycle took %s, longer than PROXY_CHECK_INTERVAL=%ds — raise PROXY_CHECK_CONCURRENCY or PROXY_CHECK_INTERVAL", elapsed.Round(time.Second), interval)
			} else {
				logger.Warn("Check cycle took %s, longer than PROXY_CHECK_INTERVAL=%ds — raise PROXY_CHECK_INTERVAL", elapsed.Round(time.Second), interval)
			}
		}

		if config.CLIConfig.Metrics.PushURL != "" {
			pushConfig, err := metrics.ParseURL(config.CLIConfig.Metrics.PushURL)
			if err != nil {
				logger.Error("Error parsing push URL: %v", err)
				return
			}

			if pushConfig != nil {
				if err := metrics.PushMetrics(pushConfig, registry); err != nil {
					logger.Error("Error pushing metrics: %v", err)
				}
			}
		}
	}

	if config.CLIConfig.RunOnce {
		runCheckIteration()
		logger.Info("Check completed")
		return
	}

	checkScheduler := gocron.NewScheduler(time.UTC)
	// SingletonMode: if a check cycle overruns the interval, the next tick is skipped
	// instead of starting a second concurrent cycle. Without a concurrency limit a
	// cycle is bounded by PROXY_TIMEOUT so this rarely triggers, but with
	// PROXY_CHECK_CONCURRENCY a slow cycle degrades to "runs less often" rather than
	// piling up overlapping runs.
	checkScheduler.Every(config.CLIConfig.Proxy.CheckInterval).Seconds().SingletonMode().Do(func() {
		runCheckIteration()
	})
	checkScheduler.StartAsync()

	if config.CLIConfig.Subscription.Update {
		updateScheduler := gocron.NewScheduler(time.UTC)
		updateScheduler.Every(config.CLIConfig.Subscription.UpdateInterval).Seconds().WaitForSchedule().Do(func() {
			logger.Info("Checking subscriptions for updates...")
			newConfigs, err := subscription.ReadFromMultipleSources(config.CLIConfig.Subscription.URLs)
			if err != nil {
				logger.Error("Error fetching subscriptions: %v", err)
				return
			}

			if config.CLIConfig.Proxy.ResolveDomains {
				resolved, err := subscription.ResolveDomainsForConfigs(newConfigs)
				if err != nil {
					logger.Error("Error resolving domains: %v", err)
				} else {
					newConfigs = resolved
				}
			}

			if !xray.IsConfigsEqual(*proxyConfigs, newConfigs) {
				if err := updateConfiguration(newConfigs, proxyConfigs, xrayRunner, proxyChecker); err != nil {
					logger.Error("Error updating configuration: %v", err)
				} else {
					// Immediately re-check the new proxy set so /metrics is repopulated
					// right away instead of staying empty until the next scheduled check
					// (up to PROXY_CHECK_INTERVAL), then drop series for removed proxies.
					runCheckIteration()
					proxyChecker.PruneStaleResults()
				}
			} else {
				logger.Info("Subscriptions checked, no changes")
			}
		})
		updateScheduler.StartAsync()
	}

	mux, err := web.NewPrefixServeMux(config.CLIConfig.Metrics.BasePath)
	if err != nil {
		logger.Fatal("Error creating web server: %v", err)
	}
	mux.Handle("/health", web.HealthHandler())
	mux.Handle("/static/", web.StaticHandler())
	mux.Handle("/api/v1/public/proxies", web.APIPublicProxiesHandler(proxyChecker))

	web.RegisterConfigEndpoints(*proxyConfigs, proxyChecker, config.CLIConfig.Xray.StartPort)

	protectedHandler := http.NewServeMux()
	protectedHandler.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	protectedHandler.Handle("/config/", web.ConfigStatusHandler(proxyChecker))
	protectedHandler.Handle("/api/v1/proxies/", web.APIProxyHandler(proxyChecker, config.CLIConfig.Xray.StartPort))
	protectedHandler.Handle("/api/v1/proxies", web.APIProxiesHandler(proxyChecker, config.CLIConfig.Xray.StartPort))
	protectedHandler.Handle("/api/v1/config", web.APIConfigHandler(proxyChecker))
	protectedHandler.Handle("/api/v1/status", web.APIStatusHandler(proxyChecker))
	protectedHandler.Handle("/api/v1/system/info", web.APISystemInfoHandler(version, startTime))
	protectedHandler.Handle("/api/v1/system/ip", web.APISystemIPHandler(proxyChecker))
	protectedHandler.Handle("/api/v1/docs", web.APIDocsHandler())
	protectedHandler.Handle("/api/v1/openapi.yaml", web.APIOpenAPIHandler())

	if config.CLIConfig.Web.Public {
		mux.Handle("/", web.IndexHandler(version, proxyChecker))
		mux.Handle("/config/", web.ConfigStatusHandler(proxyChecker))
		middlewareHandler := web.BasicAuthMiddleware(
			config.CLIConfig.Metrics.Username,
			config.CLIConfig.Metrics.Password,
		)(protectedHandler)
		mux.Handle("/metrics", middlewareHandler)
		mux.Handle("/api/", middlewareHandler)
	} else if config.CLIConfig.Metrics.Protected {
		protectedHandler.Handle("/", web.IndexHandler(version, proxyChecker))
		middlewareHandler := web.BasicAuthMiddleware(
			config.CLIConfig.Metrics.Username,
			config.CLIConfig.Metrics.Password,
		)(protectedHandler)
		mux.Handle("/", middlewareHandler)
	} else {
		protectedHandler.Handle("/", web.IndexHandler(version, proxyChecker))
		mux.Handle("/", protectedHandler)
	}

	if !config.CLIConfig.RunOnce {
		logger.Info("Server listening on %s:%s%s",
			config.CLIConfig.Metrics.Host,
			config.CLIConfig.Metrics.Port,
			config.CLIConfig.Metrics.BasePath,
		)
		if err := http.ListenAndServe(config.CLIConfig.Metrics.Host+":"+config.CLIConfig.Metrics.Port, mux); err != nil {
			logger.Fatal("Error starting server: %v", err)
		}
	}
}

func updateConfiguration(newConfigs []*models.ProxyConfig, currentConfigs *[]*models.ProxyConfig,
	xrayRunner *xray.Runner, proxyChecker *checker.ProxyChecker) error {

	logger.Info("Subscription changed, updating configuration...")

	xray.PrepareProxyConfigs(newConfigs)

	configFile := "xray_config.json"
	configGenerator := xray.NewConfigGenerator(config.CLIConfig.Xray.Interface)
	validProxies, err := configGenerator.GenerateValidatedConfig(
		newConfigs,
		config.CLIConfig.Xray.StartPort,
		configFile,
		config.CLIConfig.Xray.LogLevel,
	)
	if err != nil {
		return err
	}
	newConfigs = validProxies

	if err := xrayRunner.Stop(); err != nil {
		return err
	}

	if err := xrayRunner.Start(); err != nil {
		return err
	}

	proxyChecker.UpdateProxies(newConfigs)

	*currentConfigs = newConfigs

	web.RegisterConfigEndpoints(newConfigs, proxyChecker, config.CLIConfig.Xray.StartPort)

	logger.Info("Configuration updated: %d proxies", len(newConfigs))
	return nil
}
