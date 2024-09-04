package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xray-checker/models"
	uptimekuma "xray-checker/providers/uptime-kuma"
	"xray-checker/utils"

	"github.com/go-co-op/gocron"
)

func loadProgramConfig(configPath string) (models.Provider, error) {
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading program configuration file: %v", err)
	}

	var rawProvider json.RawMessage
	var temp struct {
		Provider json.RawMessage `json:"provider"`
	}
	err = json.Unmarshal(configFile, &temp)
	if err != nil {
		return nil, fmt.Errorf("error parsing program configuration file: %v", err)
	}
	rawProvider = temp.Provider

	var providerType struct {
		Name string `json:"name"`
	}
	err = json.Unmarshal(rawProvider, &providerType)
	if err != nil {
		return nil, fmt.Errorf("error determining provider type: %v", err)
	}

	provider, err := uptimekuma.ProviderFactory(providerType.Name, rawProvider)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func processConfigFile(configPath string, provider models.Provider) {
	logData := models.ConnectionData{ConfigFile: configPath}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		logData.Error = fmt.Errorf("error reading xray config: %v", err)
		utils.LogResult(logData)
		return
	}

	var config models.XrayConfig
	err = json.Unmarshal(configData, &config)
	if err != nil {
		logData.Error = fmt.Errorf("error parsing xray config: %v", err)
		utils.LogResult(logData)
		return
	}

	logData.WebhookURL = config.Webhook
	if logData.WebhookURL == "" {
		logData.Error = fmt.Errorf("webhook URL not found in xray config")
		utils.LogResult(logData)
		return
	}

	ipCheckURL := "https://ifconfig.io"
	logData.SourceIP, err = utils.GetIP(ipCheckURL, &http.Client{})
	if err != nil {
		logData.Error = fmt.Errorf("error getting source IP: %v", err)
		utils.LogResult(logData)
		return
	}

	// Запускаем Xray и проверяем VPN IP
	listen := config.Inbounds[0].Listen
	port := config.Inbounds[0].Port
	logData.ProxyAddress = fmt.Sprintf("socks5://%s:%d", listen, port)

	cmd, err := utils.RunXray(configPath)
	if err != nil {
		logData.Error = fmt.Errorf("error starting Xray: %v", err)
		utils.LogResult(logData)
		return
	}
	defer utils.KillXray(cmd)
	time.Sleep(2 * time.Second)

	// Ждем и проверяем IP через прокси
	proxyClient, err := utils.CreateProxyClient(logData.ProxyAddress)
	if err != nil {
		logData.Error = fmt.Errorf("error creating proxy client: %v", err)
		utils.LogResult(logData)
		return
	}

	logData.VPNIP, err = utils.GetIP(ipCheckURL, proxyClient)
	if err != nil {
		logData.Error = fmt.Errorf("error getting VPN IP through proxy: %v", err)
		utils.LogResult(logData)
		return
	}

	// Отправляем результат провайдеру (Uptime-Kuma или другому)
	err = provider.ProcessResults(logData)
	if err != nil {
		logData.Error = fmt.Errorf("error processing results: %v", err)
	}
}

func scheduleConfigs(configDir string, scheduler *gocron.Scheduler, interval int, provider models.Provider) {
	files, err := os.ReadDir(configDir)
	if err != nil {
		fmt.Println("error reading directory:", err)
		return
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			configPath := filepath.Join(configDir, file.Name())
			scheduler.Every(interval).Seconds().Do(func() {
				processConfigFile(configPath, provider)
			})
		}
	}
}

func main() {
	configDir := "./configs"
	programConfigPath := "./config.json"
	templateDir := "./templates"

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		err := os.Mkdir(configDir, os.ModePerm)
		if err != nil {
			return
		}
	}

	var wg sync.WaitGroup

	// Загружаем конфигурацию провайдера
	provider, err := loadProgramConfig(programConfigPath)
	if err != nil {
		fmt.Println("error loading program configuration:", err)
		return
	}

	// Процесс генерации конфигов для каждого подключения
	for i, config := range provider.GetConfigs() {
		parsedLink, err := utils.ParseLink(config.Link)
		if err != nil {
			fmt.Println("error parsing link:", err)
			continue
		}

		parsedLink.MonitorLink = config.MonitorLink
		parsedLink.RandomPort = provider.GetProxyStartPort() + i

		err = utils.GenerateXrayConfig(parsedLink, templateDir, configDir)
		if err != nil {
			fmt.Println("error generating Xray config:", err)
			continue
		}

		fmt.Printf("Xray config generated: %s-%s.json\n", parsedLink.Protocol, parsedLink.Server)
	}

	// Планируем выполнение тестов
	scheduler := gocron.NewScheduler(time.UTC)
	scheduleConfigs(configDir, scheduler, provider.GetInterval(), provider)

	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.StartBlocking()
	}()

	wg.Wait()
}
