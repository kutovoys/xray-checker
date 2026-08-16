package checker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
	"time"

	"xray-checker/logger"
	"xray-checker/metrics"
	"xray-checker/models"
)

type ProxyChecker struct {
	proxies          []*models.ProxyConfig
	startPort        int
	ipCheck          string
	currentIP        string
	httpClient       *http.Client
	results          sync.Map // proxyMetricLabels -> proxyResult
	ipInitialized    bool
	ipCheckTimeout   int
	genMethodURL     string
	downloadURL      string
	downloadTimeout  int
	downloadMinSize  int64
	checkMethod      string
	checkConcurrency int // max proxies checked in parallel per cycle; 0 = unlimited
	mu               sync.RWMutex
	failureThreshold int
	retryDelay       time.Duration
	checkFn          func(*http.Client) (bool, string, time.Duration, error)
}

// proxyResult is the latest check outcome for one proxy. Metrics are rendered from
// these at scrape time (a pull model), so there is no separate metric state to keep
// in sync and no series to delete — the metrics collector simply reflects whatever
// results exist for the current proxy set.
type proxyResult struct {
	status    bool
	latency   time.Duration
	lastCheck time.Time
}

func NewProxyChecker(proxies []*models.ProxyConfig, startPort int, ipCheckURL string, ipCheckTimeout int, genMethodURL string, downloadURL string, downloadTimeout int, downloadMinSize int64, checkMethod string, checkConcurrency, failureThreshold int) *ProxyChecker {
	if failureThreshold < 1 {
		failureThreshold = 1
	}

	return &ProxyChecker{
		proxies:   proxies,
		startPort: startPort,
		ipCheck:   ipCheckURL,
		httpClient: &http.Client{
			Timeout: time.Second * time.Duration(ipCheckTimeout),
		},
		ipCheckTimeout:   ipCheckTimeout,
		genMethodURL:     genMethodURL,
		downloadURL:      downloadURL,
		downloadTimeout:  downloadTimeout,
		downloadMinSize:  downloadMinSize,
		checkMethod:      checkMethod,
		checkConcurrency: checkConcurrency,
		failureThreshold: failureThreshold,
		retryDelay:       5 * time.Second,
	}
}

func (pc *ProxyChecker) GetCurrentIP() (string, error) {
	if pc.ipInitialized && pc.currentIP != "" {
		return pc.currentIP, nil
	}

	resp, err := pc.httpClient.Get(pc.ipCheck)
	if err != nil {
		return "", fmt.Errorf("error getting current IP: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	pc.currentIP = string(body)
	pc.ipInitialized = true
	return pc.currentIP, nil
}

func (pc *ProxyChecker) CheckProxy(proxy *models.ProxyConfig) {
	pc.checkProxyInternal(proxy)
}

// proxyMetricLabels is the full Prometheus label set for a proxy. It doubles as the
// in-memory map key, so series can be deleted exactly from their labels without
// parsing a packed string (proxy names may legitimately contain any character).
type proxyMetricLabels struct {
	protocol  string
	address   string
	name      string
	subName   string
	stableID  string
	groupName string
}

func proxyMetricKey(proxy *models.ProxyConfig) proxyMetricLabels {
	return proxyMetricLabels{
		protocol:  proxy.Protocol,
		address:   fmt.Sprintf("%s:%d", proxy.Server, proxy.Port),
		name:      proxy.Name,
		subName:   proxy.SubName,
		stableID:  proxy.StableID,
		groupName: proxy.GroupName,
	}
}

func (pc *ProxyChecker) checkProxyInternal(proxy *models.ProxyConfig) {
	if proxy.StableID == "" {
		proxy.StableID = proxy.GenerateStableID()
	}

	metricKey := proxyMetricKey(proxy)

	storeResult := func(status bool, latency time.Duration) {
		pc.results.Store(metricKey, proxyResult{
			status:    status,
			latency:   latency,
			lastCheck: time.Now(),
		})
	}

	setFailed := func() {
		storeResult(false, 0)
	}

	proxyURL := fmt.Sprintf("socks5://127.0.0.1:%d", pc.startPort+proxy.Index)
	proxyURLParsed, err := url.Parse(proxyURL)
	if err != nil {
		logger.Error("Error parsing proxy URL %s: %v", proxyURL, err)
		setFailed()

		return
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURLParsed),
			DisableKeepAlives: true,
		},
		Timeout: time.Second * time.Duration(pc.ipCheckTimeout),
	}

	for attempt := 1; attempt <= pc.failureThreshold; attempt++ {
		var checkSuccess bool
		var checkErr error
		var logMessage string
		var latency time.Duration

		if pc.checkFn != nil {
			checkSuccess, logMessage, latency, checkErr = pc.checkFn(client)
		} else {
			switch pc.checkMethod {
			case "ip":
				checkSuccess, logMessage, latency, checkErr = pc.checkByIP(client)
			case "status":
				checkSuccess, logMessage, latency, checkErr = pc.checkByGen(client)
			case "download":
				checkSuccess, logMessage, latency, checkErr = pc.checkByDownload(client)
			default:
				logger.Error("Invalid check method: %s", pc.checkMethod)
				return
			}
		}

		if checkErr == nil && checkSuccess {
			logger.Result("%s | Success | %s | Latency: %s", proxy.Name, logMessage, latency)
			storeResult(true, latency)
			return
		}

		if attempt < pc.failureThreshold {
			logger.Warn("%s | Check attempt %d/%d failed; retrying in 5 seconds", proxy.Name, attempt, pc.failureThreshold)
			time.Sleep(pc.retryDelay)
			continue
		}

		if checkErr != nil {
			logger.Error("%s | %v", proxy.Name, checkErr)
		} else {
			logger.Error("%s | Failed | %s | Latency: %s", proxy.Name, logMessage, latency)
		}
		setFailed()
	}
}

func (pc *ProxyChecker) checkByIP(client *http.Client) (bool, string, time.Duration, error) {
	req, err := http.NewRequest("GET", pc.ipCheck, nil)
	if err != nil {
		return false, "", 0, err
	}

	var ttfb time.Duration
	start := time.Now()
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), trace))

	resp, err := client.Do(req)
	if err != nil {
		return false, "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", ttfb, err
	}

	proxyIP := string(body)
	logMessage := fmt.Sprintf("Source IP: %s | Proxy IP: %s", pc.currentIP, proxyIP)
	return proxyIP != pc.currentIP, logMessage, ttfb, nil
}

func (pc *ProxyChecker) checkByGen(client *http.Client) (bool, string, time.Duration, error) {
	req, err := http.NewRequest("GET", pc.genMethodURL, nil)
	if err != nil {
		return false, "", 0, err
	}

	var ttfb time.Duration
	start := time.Now()
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), trace))

	resp, err := client.Do(req)
	if err != nil {
		return false, "", 0, err
	}
	defer resp.Body.Close()

	logMessage := fmt.Sprintf("Status: %d", resp.StatusCode)
	return resp.StatusCode >= 200 && resp.StatusCode < 300, logMessage, ttfb, nil
}

func (pc *ProxyChecker) checkByDownload(client *http.Client) (bool, string, time.Duration, error) {
	if pc.downloadURL == "" {
		return false, "Download URL not configured", 0, fmt.Errorf("download URL not configured")
	}

	req, err := http.NewRequest("GET", pc.downloadURL, nil)
	if err != nil {
		return false, "", 0, err
	}

	var ttfb time.Duration
	start := time.Now()
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), trace))

	downloadClient := &http.Client{
		Transport: client.Transport,
		Timeout:   time.Second * time.Duration(pc.downloadTimeout),
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return false, "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("HTTP status: %d", resp.StatusCode), ttfb, nil
	}

	totalBytes := int64(0)
	buffer := make([]byte, 8192)

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			totalBytes += int64(n)
		}

		if totalBytes >= pc.downloadMinSize {
			break
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Sprintf("Download error after %d bytes: %v", totalBytes, err), ttfb, nil
		}
	}

	success := totalBytes >= pc.downloadMinSize
	logMessage := fmt.Sprintf("Downloaded: %d bytes (min: %d)", totalBytes, pc.downloadMinSize)

	return success, logMessage, ttfb, nil
}

// UpdateProxies swaps in a new proxy set. Metrics are rendered from the current
// set at scrape time, so surviving proxies keep their last result (no blink to 0,
// the #148 regression) and removed proxies simply stop being emitted on the next
// scrape — no metric deletion needed. The caller should run an immediate check to
// populate the new proxies and may call PruneStaleResults to drop cached results
// for removed proxies.
func (pc *ProxyChecker) UpdateProxies(newProxies []*models.ProxyConfig) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.proxies = newProxies
}

// PruneStaleResults drops cached results for proxies no longer in the current set.
// This is memory hygiene only: stale results are never emitted (the snapshot
// iterates the current set), but pruning keeps the results map from growing across
// many subscription changes.
func (pc *ProxyChecker) PruneStaleResults() {
	pc.mu.RLock()
	currentKeys := make(map[proxyMetricLabels]struct{}, len(pc.proxies))
	for _, proxy := range pc.proxies {
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}
		currentKeys[proxyMetricKey(proxy)] = struct{}{}
	}
	pc.mu.RUnlock()

	pc.results.Range(func(key, _ interface{}) bool {
		if _, ok := currentKeys[key.(proxyMetricLabels)]; !ok {
			pc.results.Delete(key)
		}
		return true
	})
}

// MetricsSnapshot returns one ProxyMetric per current proxy that has a check
// result, attaching its custom metricsLabels. The metrics collector renders these
// at scrape time, so the exported series always match the current proxy set.
func (pc *ProxyChecker) MetricsSnapshot() []metrics.ProxyMetric {
	pc.mu.RLock()
	proxies := make([]*models.ProxyConfig, len(pc.proxies))
	copy(proxies, pc.proxies)
	pc.mu.RUnlock()

	out := make([]metrics.ProxyMetric, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}
		key := proxyMetricKey(proxy)
		v, ok := pc.results.Load(key)
		if !ok {
			// Not checked yet: no series until the first result, matching prior behavior.
			continue
		}
		r := v.(proxyResult)
		out = append(out, metrics.ProxyMetric{
			Protocol:     key.protocol,
			Address:      key.address,
			Name:         key.name,
			SubName:      key.subName,
			StableID:     key.stableID,
			GroupName:    key.groupName,
			CustomLabels: proxy.MetricsLabels,
			Online:       r.status,
			LatencyMs:    float64(r.latency.Milliseconds()),
		})
	}
	return out
}

func (pc *ProxyChecker) CheckAllProxies() {
	if _, err := pc.GetCurrentIP(); err != nil {
		logger.Warn("Error getting current IP: %v", err)
		return
	}

	pc.mu.RLock()
	proxiesToCheck := make([]*models.ProxyConfig, len(pc.proxies))
	copy(proxiesToCheck, pc.proxies)
	pc.mu.RUnlock()

	runBoundedChecks(proxiesToCheck, pc.checkConcurrency, pc.checkProxyInternal)
}

// runBoundedChecks runs check(p) for every proxy concurrently. concurrency == 0
// keeps the original behavior (all at once); a positive value bounds how many run
// simultaneously via a semaphore, so large subscriptions don't open thousands of
// connections in one burst. Local socks ports are unchanged — each check still
// dials its own fixed port; this only throttles how many run at a time.
func runBoundedChecks(proxies []*models.ProxyConfig, concurrency int, check func(*models.ProxyConfig)) {
	var sem chan struct{}
	if concurrency > 0 {
		sem = make(chan struct{}, concurrency)
	}

	var wg sync.WaitGroup
	for _, proxy := range proxies {
		wg.Add(1)
		go func(p *models.ProxyConfig) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			check(p)
		}(proxy)
	}
	wg.Wait()
}

// GetProxyResultByStableID returns the latest check outcome for the proxy with the
// given stable_id: online status, latency, last-check time as a Unix timestamp in
// seconds (0 if never checked), and whether a result was found.
//
// Lookup is by the unique stable_id (and the full metric key it maps to), the same
// key /metrics uses — so proxies that share a display name are never confused. A
// previous name-based lookup returned the first same-named proxy's result, making
// /config/{id}, the dashboard and the JSON API disagree with /metrics (issue #172).
func (pc *ProxyChecker) GetProxyResultByStableID(stableID string) (bool, time.Duration, int64, bool) {
	pc.mu.RLock()
	var metricKey proxyMetricLabels
	found := false
	for _, proxy := range pc.proxies {
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}
		if proxy.StableID == stableID {
			metricKey = proxyMetricKey(proxy)
			found = true
			break
		}
	}
	pc.mu.RUnlock()

	if !found {
		return false, 0, 0, false
	}

	v, ok := pc.results.Load(metricKey)
	if !ok {
		return false, 0, 0, false
	}

	r := v.(proxyResult)
	var lastCheck int64
	if !r.lastCheck.IsZero() {
		lastCheck = r.lastCheck.Unix()
	}
	return r.status, r.latency, lastCheck, true
}

func (pc *ProxyChecker) GetProxyByStableID(stableID string) (*models.ProxyConfig, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	for _, proxy := range pc.proxies {
		if proxy.StableID == "" {
			proxy.StableID = proxy.GenerateStableID()
		}

		if proxy.StableID == stableID {
			return proxy, true
		}
	}
	return nil, false
}

func (pc *ProxyChecker) GetProxies() []*models.ProxyConfig {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	result := make([]*models.ProxyConfig, len(pc.proxies))
	copy(result, pc.proxies)
	return result
}
