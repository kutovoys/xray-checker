package checker

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xray-checker/metrics"
	"xray-checker/models"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// peakTracker records the maximum number of concurrent calls seen.
type peakTracker struct {
	cur, peak int32
	mu        sync.Mutex
}

func (p *peakTracker) enter() {
	n := atomic.AddInt32(&p.cur, 1)
	p.mu.Lock()
	if n > p.peak {
		p.peak = n
	}
	p.mu.Unlock()
}
func (p *peakTracker) leave() { atomic.AddInt32(&p.cur, -1) }

func TestRunBoundedChecks_Concurrency(t *testing.T) {
	proxies := make([]*models.ProxyConfig, 30)
	for i := range proxies {
		proxies[i] = &models.ProxyConfig{Name: "p", Index: i}
	}

	// Bounded to 4: peak concurrency must never exceed 4, and all run.
	var tr peakTracker
	var count int32
	runBoundedChecks(proxies, 4, func(*models.ProxyConfig) {
		tr.enter()
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&count, 1)
		tr.leave()
	})
	if tr.peak > 4 {
		t.Errorf("peak concurrency %d exceeded limit 4", tr.peak)
	}
	if count != 30 {
		t.Errorf("expected all 30 checked, got %d", count)
	}

	// Unlimited (0): checks are not bounded to a small number. Assert the peak far
	// exceeds the limited case's cap of 4 rather than exact equality to 30, which
	// would be timing-sensitive on a loaded/single-CPU runner.
	var tr2 peakTracker
	runBoundedChecks(proxies, 0, func(*models.ProxyConfig) {
		tr2.enter()
		time.Sleep(10 * time.Millisecond)
		tr2.leave()
	})
	if tr2.peak < 10 {
		t.Errorf("unlimited: expected substantial parallelism (peak >= 10), got %d", tr2.peak)
	}
}

func mkProxy(server, name, stableID string) *models.ProxyConfig {
	return &models.ProxyConfig{
		Protocol: "vless",
		Server:   server,
		Port:     443,
		Name:     name,
		SubName:  "sub",
		StableID: stableID,
	}
}

// recordUp mimics what checkProxyInternal stores on a successful check.
func recordUp(pc *ProxyChecker, p *models.ProxyConfig, lat time.Duration) {
	pc.results.Store(proxyMetricKey(p), proxyResult{
		status:    true,
		latency:   lat,
		lastCheck: time.Now(),
	})
}

// statusValue gathers the collector and returns the xray_proxy_status value for a
// given stable_id.
func statusValue(t *testing.T, c prometheus.Collector, stableID string) (float64, bool) {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register collector: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "xray_proxy_status" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "stable_id" && l.GetValue() == stableID {
					return m.GetGauge().GetValue(), true
				}
			}
		}
	}
	return 0, false
}

// Reproduces the #148 scenario under the pull-collector model: a subscription
// update must not blank out a surviving proxy's metric (no blink to 0), removed
// proxies drop out immediately, and newly added proxies appear once checked.
func TestUpdateProxiesNoBlinkAndReflectsCurrentSet(t *testing.T) {
	a := mkProxy("1.1.1.1", "A", "ida")
	b := mkProxy("2.2.2.2", "B", "idb")
	pc := NewProxyChecker([]*models.ProxyConfig{a, b}, 10000, "", 5, "", "", 5, 1, "ip", 0, 1)
	collector := metrics.NewCollector("", pc)

	// Initial check populated A and B.
	recordUp(pc, a, 100*time.Millisecond)
	recordUp(pc, b, 200*time.Millisecond)
	if got := testutil.CollectAndCount(collector, "xray_proxy_status"); got != 2 {
		t.Fatalf("expected 2 status series after initial check, got %d", got)
	}

	// Subscription update: B removed, C added (not yet checked).
	c := mkProxy("3.3.3.3", "C", "idc")
	pc.UpdateProxies([]*models.ProxyConfig{a, c})

	// Surviving A must keep status 1 across the update — never blink to 0.
	if v, ok := statusValue(t, collector, "ida"); !ok || v != 1 {
		t.Fatalf("surviving proxy A must keep status 1 across update, got v=%v ok=%v", v, ok)
	}
	// Only A is emitted now: B is gone (not in current set), C has no result yet.
	if got := testutil.CollectAndCount(collector, "xray_proxy_status"); got != 1 {
		t.Fatalf("after update expected 1 series (A), got %d", got)
	}

	// Immediate post-update check populates C.
	recordUp(pc, c, 300*time.Millisecond)
	if got := testutil.CollectAndCount(collector, "xray_proxy_status"); got != 2 {
		t.Fatalf("after post-update check expected 2 series (A,C), got %d", got)
	}

	// B's stale result lingers in the map but is never emitted; PruneStaleResults
	// removes it for memory hygiene.
	pc.PruneStaleResults()
	if _, ok := pc.results.Load(proxyMetricKey(b)); ok {
		t.Errorf("removed proxy B should be pruned from results")
	}
	for _, p := range []*models.ProxyConfig{a, c} {
		if _, ok := pc.results.Load(proxyMetricKey(p)); !ok {
			t.Errorf("proxy %s should remain in results", p.Name)
		}
	}
}

func TestGetProxyResultLastCheck(t *testing.T) {
	a := mkProxy("1.1.1.1", "A", "ida")
	pc := NewProxyChecker([]*models.ProxyConfig{a}, 10000, "", 5, "", "", 5, 1, "ip", 0, 1)

	// No result yet: not found, lastCheck 0.
	if _, _, lc, found := pc.GetProxyResultByStableID("ida"); found || lc != 0 {
		t.Fatalf("expected no result before check, got found=%v lastCheck=%d", found, lc)
	}

	before := time.Now().Unix()
	recordUp(pc, a, 150*time.Millisecond)
	online, latency, lc, found := pc.GetProxyResultByStableID("ida")
	if !found || !online || latency != 150*time.Millisecond {
		t.Fatalf("unexpected result: online=%v latency=%v found=%v", online, latency, found)
	}
	if lc < before {
		t.Fatalf("lastCheck %d should be >= %d", lc, before)
	}
}

func TestFailureThresholdDefaultsToOne(t *testing.T) {
	pc := NewProxyChecker(nil, 10000, "", 5, "", "", 5, 1, "status", 0, 0)
	if pc.failureThreshold != 1 {
		t.Fatalf("default failure threshold = %d, want 1", pc.failureThreshold)
	}
	pc = NewProxyChecker(nil, 10000, "", 5, "", "", 5, 1, "status", 0, 3)
	if pc.failureThreshold != 3 {
		t.Fatalf("configured threshold = %d, want 3", pc.failureThreshold)
	}
}

func TestCheckProxyRetriesBeforePublishingResult(t *testing.T) {
	for _, tt := range []struct {
		name         string
		results      []bool
		wantAttempts int
		wantOnline   bool
	}{
		{"succeeds before threshold", []bool{false, true}, 2, true},
		{"fails after threshold", []bool{false, false, false}, 3, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxy := mkProxy("proxy.example", "proxy", "id")
			pc := NewProxyChecker([]*models.ProxyConfig{proxy}, 10000, "", 1, "", "", 1, 1, "status", 0, 3)
			pc.retryDelay = 0
			attempts := 0
			pc.checkFn = func(*http.Client) (bool, string, time.Duration, error) {
				success := tt.results[attempts]
				attempts++
				return success, "", 0, nil
			}

			pc.checkProxyInternal(proxy)

			if attempts != tt.wantAttempts {
				t.Fatalf("attempts = %d, want %d", attempts, tt.wantAttempts)
			}
			result, ok := pc.results.Load(proxyMetricKey(proxy))
			if !ok {
				t.Fatal("expected final check result")
			}
			if got := result.(proxyResult).status; got != tt.wantOnline {
				t.Fatalf("final status = %v, want %v", got, tt.wantOnline)
			}
		})
	}
}

// Regression for #172: two proxies with the SAME name but different stable_id must
// resolve to their own result by stable_id, not to the first same-named proxy's.
func TestGetProxyResultByStableID_DuplicateNames(t *testing.T) {
	up := &models.ProxyConfig{Protocol: "socks", Server: "1.1.1.1", Port: 1080, Name: "Dup", StableID: "id-up"}
	down := &models.ProxyConfig{Protocol: "socks", Server: "2.2.2.2", Port: 1080, Name: "Dup", StableID: "id-down"}
	pc := NewProxyChecker([]*models.ProxyConfig{up, down}, 10000, "", 5, "", "", 5, 1, "status", 0, 1)

	// up is healthy, down failed — same name, different stable_id.
	pc.results.Store(proxyMetricKey(up), proxyResult{status: true, latency: 100 * time.Millisecond, lastCheck: time.Now()})
	pc.results.Store(proxyMetricKey(down), proxyResult{status: false, latency: 0, lastCheck: time.Now()})

	if online, _, _, found := pc.GetProxyResultByStableID("id-up"); !found || !online {
		t.Errorf("id-up should be online, got found=%v online=%v", found, online)
	}
	// The bug: name lookup returned the first ("up") result for "down". By stable_id
	// it must be offline.
	if online, _, _, found := pc.GetProxyResultByStableID("id-down"); !found || online {
		t.Errorf("id-down should be offline, got found=%v online=%v", found, online)
	}
}
