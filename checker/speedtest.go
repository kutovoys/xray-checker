package checker

import (
	"context"
	"fmt"
	"time"

	"xray-checker/logger"
	"xray-checker/metrics"
	"xray-checker/models"

	"github.com/showwin/speedtest-go/speedtest"
)

const (
	speedtestServerFetchTimeout = 15 * time.Second
	speedtestDownloadTimeout    = 60 * time.Second
	speedtestUploadTimeout      = 60 * time.Second
)

type SpeedtestResult struct {
	DownloadBps float64
	UploadBps   float64
}

func (pc *ProxyChecker) checkProxySpeedtest(proxy *models.ProxyConfig) {
	if proxy.StableID == "" {
		proxy.StableID = proxy.GenerateStableID()
	}

	metricKey := fmt.Sprintf("%s|%s:%d|%s|%s|%s",
		proxy.Protocol,
		proxy.Server,
		proxy.Port,
		proxy.Name,
		proxy.SubName,
		proxy.StableID,
	)

	proxyAddr := fmt.Sprintf("socks5://127.0.0.1:%d", pc.startPort+proxy.Index)

	client := speedtest.New(speedtest.WithUserConfig(&speedtest.UserConfig{
		Proxy: proxyAddr,
	}))

	fetchCtx, cancelFetch := context.WithTimeout(context.Background(), speedtestServerFetchTimeout)
	servers, err := client.FetchServerListContext(fetchCtx)
	cancelFetch()
	if err != nil || len(servers) == 0 {
		logger.Error("%s | Speedtest: failed to fetch servers: %v", proxy.Name, err)
		return
	}

	targets, err := servers.FindServer(nil)
	if err != nil || len(targets) == 0 {
		logger.Error("%s | Speedtest: failed to select server: %v", proxy.Name, err)
		return
	}
	server := targets[0]

	downloadCtx, cancelDownload := context.WithTimeout(context.Background(), speedtestDownloadTimeout)
	err = server.DownloadTestContext(downloadCtx)
	cancelDownload()
	if err != nil {
		logger.Error("%s | Speedtest: download test failed: %v", proxy.Name, err)
		return
	}

	uploadCtx, cancelUpload := context.WithTimeout(context.Background(), speedtestUploadTimeout)
	err = server.UploadTestContext(uploadCtx)
	cancelUpload()
	if err != nil {
		logger.Error("%s | Speedtest: upload test failed: %v", proxy.Name, err)
		return
	}

	if server.DLSpeed <= 0 || server.ULSpeed <= 0 {
		logger.Error("%s | Speedtest: invalid result (download=%s upload=%s)", proxy.Name, server.DLSpeed, server.ULSpeed)
		return
	}

	downloadBps := float64(server.DLSpeed) * 8
	uploadBps := float64(server.ULSpeed) * 8

	logger.Result("%s | Speedtest | Server: %s | Download: %.1f Mbps | Upload: %.1f Mbps",
		proxy.Name, server.Name, downloadBps/1e6, uploadBps/1e6)

	metrics.RecordProxySpeedtestDownload(
		proxy.Protocol,
		fmt.Sprintf("%s:%d", proxy.Server, proxy.Port),
		proxy.Name,
		proxy.SubName,
		downloadBps,
	)
	metrics.RecordProxySpeedtestUpload(
		proxy.Protocol,
		fmt.Sprintf("%s:%d", proxy.Server, proxy.Port),
		proxy.Name,
		proxy.SubName,
		uploadBps,
	)

	pc.speedtestMetrics.Store(metricKey, SpeedtestResult{DownloadBps: downloadBps, UploadBps: uploadBps})
}

// RunSpeedtests runs a download/upload speedtest for every proxy, one at a
// time, so results aren't skewed by proxies competing for host bandwidth.
func (pc *ProxyChecker) RunSpeedtests() {
	pc.mu.RLock()
	proxiesToCheck := make([]*models.ProxyConfig, len(pc.proxies))
	copy(proxiesToCheck, pc.proxies)
	pc.mu.RUnlock()

	for _, proxy := range proxiesToCheck {
		pc.checkProxySpeedtest(proxy)
	}
}

func (pc *ProxyChecker) GetProxySpeedtest(name string) (downloadBps float64, uploadBps float64, tested bool) {
	pc.mu.RLock()
	var metricKey string
	for _, proxy := range pc.proxies {
		if proxy.Name == name {
			if proxy.StableID == "" {
				proxy.StableID = proxy.GenerateStableID()
			}
			metricKey = fmt.Sprintf("%s|%s:%d|%s|%s|%s",
				proxy.Protocol,
				proxy.Server,
				proxy.Port,
				proxy.Name,
				proxy.SubName,
				proxy.StableID,
			)
			break
		}
	}
	pc.mu.RUnlock()

	if metricKey == "" {
		return 0, 0, false
	}

	value, ok := pc.speedtestMetrics.Load(metricKey)
	if !ok {
		return 0, 0, false
	}

	result := value.(SpeedtestResult)
	return result.DownloadBps, result.UploadBps, true
}
