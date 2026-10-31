package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"xray-checker/checker"
	"xray-checker/config"
	"xray-checker/logger"
	"xray-checker/metrics"
	"xray-checker/models"
	"xray-checker/probecore"
	"xray-checker/subscription"
	"xray-checker/web"
	"xray-checker/xray"

	"github.com/go-co-op/gocron"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version               = "unknown"
	startTime             = time.Now()
	errBackendUnavailable = errors.New("probe backend could not be restored")
)

func main() {
	if err := run(); err != nil {
		logger.Fatal("%v", err)
	}
}

func run() (runErr error) {
	config.Parse(version)
	if err := config.CLIConfig.Validate(); err != nil {
		return err
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancelCore := context.WithCancelCause(signalCtx)
	defer cancelCore(nil)
	var cycleMu sync.Mutex

	logLevel := logger.ParseLevel(config.CLIConfig.LogLevel)
	logger.SetLevel(logLevel)

	logger.Startup("Xray Checker %s", version)
	if logLevel == logger.LevelNone {
		logger.Startup("Log level: none (silent mode)")
	}

	if err := web.InitAssetLoader(config.CLIConfig.Web.CustomAssetsPath); err != nil {
		return fmt.Errorf("initialize custom assets: %w", err)
	}

	if !probecore.IsMihomo() {
		geoManager := xray.NewGeoFileManager("")
		if err := geoManager.EnsureGeoFiles(); err != nil {
			return fmt.Errorf("ensure geo files: %w", err)
		}
	}

	configFile := probecore.ConfigFile()
	proxyConfigs, err := subscription.InitializeConfigurationContext(ctx, configFile, version)
	if err != nil {
		return fmt.Errorf("initialize configuration: %w", err)
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

	coreRunner := probecore.NewRunner(configFile)
	if ctx.Err() != nil {
		return nil
	}
	if err := coreRunner.Start(); err != nil {
		return fmt.Errorf("start probe backend: %w", err)
	}
	var failures <-chan error
	if supervised, ok := coreRunner.(probecore.SupervisedRunner); ok {
		failures = supervised.Failures()
	}
	var coreFailure error
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case coreFailure = <-failures:
			cancelCore(coreFailure)
		case <-ctx.Done():
		}
	}()
	defer func() {
		cancelCore(nil)
		<-watchDone
		// Stop may discover a dead child before its watcher has run. Also retain
		// a failure consumed concurrently with cancellation during normal cleanup.
		if coreFailure == nil {
			select {
			case coreFailure = <-failures:
			default:
			}
		}
		if coreFailure != nil && signalCtx.Err() == nil {
			runErr = errors.Join(runErr, coreFailure)
		}
		if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(runErr, cause) {
			runErr = errors.Join(runErr, cause)
		}
	}()

	defer func() {
		cancelCore(nil)
		cycleMu.Lock()
		defer cycleMu.Unlock()
		if err := coreRunner.Stop(); err != nil {
			logger.Error("Error stopping probe backend: %v", err)
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
	proxyChecker.SetContext(ctx)

	// The collector renders metrics from the checker's current proxy snapshot on
	// each scrape, so custom metricsLabels (#124) can change across subscription
	// updates without resetting other series.
	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.NewCollector(config.CLIConfig.Metrics.Instance, proxyChecker))

	// The caller holds cycleMu for this entire cycle: a core refresh must not
	// reassign a SOCKS port while an old probe still attributes results to it.
	checkIteration := func() {
		if ctx.Err() != nil {
			return
		}
		logger.Info("Starting proxy check iteration")
		start := time.Now()
		proxyChecker.CheckAllProxies()
		if ctx.Err() != nil {
			return
		}
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
	runCheckIteration := func() {
		cycleMu.Lock()
		defer cycleMu.Unlock()
		checkIteration()
	}

	if config.CLIConfig.RunOnce {
		runCheckIteration()
		if ctx.Err() == nil {
			logger.Info("Check completed")
		}
		return nil
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
	defer checkScheduler.Stop()

	if config.CLIConfig.Subscription.Update {
		updateScheduler := gocron.NewScheduler(time.UTC)
		updateScheduler.Every(config.CLIConfig.Subscription.UpdateInterval).Seconds().SingletonMode().WaitForSchedule().Do(func() {
			logger.Info("Checking subscriptions for updates...")
			newConfigs, err := subscription.ReadFromMultipleSourcesContext(ctx, config.CLIConfig.Subscription.URLs)
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

			cycleMu.Lock()
			defer cycleMu.Unlock()
			if ctx.Err() != nil {
				return
			}
			if !xray.IsConfigsEqual(*proxyConfigs, newConfigs) {
				if err := updateConfiguration(newConfigs, proxyConfigs, coreRunner, proxyChecker); err != nil {
					logger.Error("Error updating configuration: %v", err)
					if errors.Is(err, errBackendUnavailable) {
						cancelCore(err)
					}
				} else {
					// Immediately re-check the new proxy set so /metrics is repopulated
					// right away instead of staying empty until the next scheduled check
					// (up to PROXY_CHECK_INTERVAL), then drop series for removed proxies.
					checkIteration()
					proxyChecker.PruneStaleResults()
				}
			} else {
				logger.Info("Subscriptions checked, no changes")
			}
		})
		updateScheduler.StartAsync()
		defer updateScheduler.Stop()
	}
	// Run before scheduler cleanup on any exit, including an HTTP bind failure.
	defer cancelCore(nil)

	mux, err := web.NewPrefixServeMux(config.CLIConfig.Metrics.BasePath)
	if err != nil {
		return fmt.Errorf("create web server: %w", err)
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

	logger.Info("Server listening on %s:%s%s",
		config.CLIConfig.Metrics.Host,
		config.CLIConfig.Metrics.Port,
		config.CLIConfig.Metrics.BasePath,
	)
	server := &http.Server{Addr: config.CLIConfig.Metrics.Host + ":" + config.CLIConfig.Metrics.Port, Handler: mux}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shut down HTTP: %w", err)
		}
	}
	return nil
}

func updateConfiguration(newConfigs []*models.ProxyConfig, currentConfigs *[]*models.ProxyConfig,
	coreRunner probecore.Runner, proxyChecker *checker.ProxyChecker) error {

	logger.Info("Subscription changed, updating configuration...")

	xray.PrepareProxyConfigs(newConfigs)

	configFile := probecore.ConfigFile()
	return replaceConfiguration(newConfigs, currentConfigs, coreRunner, proxyChecker, configFile, probecore.NewGenerator())
}

// replaceConfiguration stages a complete validated candidate before touching the
// running backend. The caller serializes this operation with probe cycles.
func replaceConfiguration(newConfigs []*models.ProxyConfig, currentConfigs *[]*models.ProxyConfig,
	coreRunner probecore.Runner, proxyChecker *checker.ProxyChecker, configFile string, configGenerator probecore.Generator) error {
	staged, err := os.CreateTemp(filepath.Dir(configFile), ".checker-config-*")
	if err != nil {
		return fmt.Errorf("stage configuration: %w", err)
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return err
	}
	defer os.Remove(stagedPath)
	validProxies, err := configGenerator.GenerateValidatedConfig(
		newConfigs,
		config.CLIConfig.Xray.StartPort,
		stagedPath,
		config.CLIConfig.Xray.LogLevel,
	)
	if err != nil {
		return err
	}
	newConfigs = validProxies

	previous, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("read previous configuration: %w", err)
	}
	if err := coreRunner.Stop(); err != nil {
		return err
	}

	rollback := func(cause error) error {
		_ = coreRunner.Stop()
		if err := os.WriteFile(configFile, previous, 0600); err != nil {
			return errors.Join(errBackendUnavailable, cause, fmt.Errorf("restore previous configuration: %w", err))
		}
		if err := coreRunner.Start(); err != nil {
			return errors.Join(errBackendUnavailable, cause, fmt.Errorf("restart previous backend: %w", err))
		}
		return cause
	}
	if err := os.Rename(stagedPath, configFile); err != nil {
		return rollback(fmt.Errorf("install configuration: %w", err))
	}
	if err := coreRunner.Start(); err != nil {
		return rollback(fmt.Errorf("start updated backend: %w", err))
	}

	proxyChecker.UpdateProxies(newConfigs)

	*currentConfigs = newConfigs

	web.RegisterConfigEndpoints(newConfigs, proxyChecker, config.CLIConfig.Xray.StartPort)

	logger.Info("Configuration updated: %d proxies", len(newConfigs))
	return nil
}
