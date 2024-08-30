package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
	"golang.org/x/net/proxy"
)

type Config struct {
	Log      map[string]interface{} `json:"log"`
	Inbounds []struct {
		Listen   string `json:"listen"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		Sniffing struct {
			Enabled      bool     `json:"enabled"`
			DestOverride []string `json:"destOverride"`
			RouteOnly    bool     `json:"routeOnly"`
		} `json:"sniffing"`
	} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Webhook   string                   `json:"webhook"`
}

type LogData struct {
	ConfigFile   string
	SourceIP     string
	VPNIP        string
	WebhookURL   string
	ProxyAddress string
	Status       string
	Error        error
}

func getIP(url string, client *http.Client) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ip)), nil
}

func runXray(configPath string) (*exec.Cmd, error) {
	cmd := exec.Command("xray", "-c", configPath)
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func killXray(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func createProxyClient(proxyAddress string) (*http.Client, error) {
	proxyURL, err := url.Parse(proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("неправильный формат прокси: %v", err)
	}

	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания прокси-диалера: %v", err)
	}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	client := &http.Client{
		Transport: transport,
	}

	return client, nil
}

func processConfigFile(configPath string) {
	logData := LogData{ConfigFile: configPath}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		logData.Error = fmt.Errorf("ошибка чтения конфигурационного файла: %v", err)
		logResult(logData)
		return
	}

	var config Config
	err = json.Unmarshal(configData, &config)
	if err != nil {
		logData.Error = fmt.Errorf("ошибка парсинга конфигурационного файла: %v", err)
		logResult(logData)
		return
	}

	logData.WebhookURL = config.Webhook
	if logData.WebhookURL == "" {
		logData.Error = fmt.Errorf("webhook URL не найден в конфигурационном файле")
		logResult(logData)
		return
	}

	ipCheckURL := "https://ifconfig.io"
	logData.SourceIP, err = getIP(ipCheckURL, &http.Client{})
	if err != nil {
		logData.Error = fmt.Errorf("ошибка получения исходного IP: %v", err)
		logResult(logData)
		return
	}

	listen := config.Inbounds[0].Listen
	port := config.Inbounds[0].Port
	logData.ProxyAddress = fmt.Sprintf("socks5://%s:%d", listen, port)

	cmd, err := runXray(configPath)
	if err != nil {
		logData.Error = fmt.Errorf("ошибка запуска Xray: %v", err)
		logResult(logData)
		return
	}
	defer killXray(cmd)
	time.Sleep(2 * time.Second)

	proxyClient, err := createProxyClient(logData.ProxyAddress)
	if err != nil {
		logData.Error = fmt.Errorf("ошибка создания прокси-клиента: %v", err)
		logResult(logData)
		return
	}

	logData.VPNIP, err = getIP(ipCheckURL, proxyClient)
	if err != nil {
		logData.Error = fmt.Errorf("ошибка получения VPN IP через прокси: %v", err)
		logResult(logData)
		return
	}

	if logData.VPNIP != logData.SourceIP {
		_, err = http.Get(logData.WebhookURL)
		if err != nil {
			logData.Error = fmt.Errorf("ошибка отправки статуса: %v", err)
			logData.Status = "Не удалось отправить статус"
		} else {
			logData.Status = "Статус отправлен успешно"
		}
	} else {
		logData.Status = "IP-адреса совпадают, статус не отправлен"
	}

	logResult(logData)
}

func logResult(logData LogData) {
	var logMsg string

	if logData.Error != nil {
		logMsg = fmt.Sprintf("Error: %v | Config: %s | Source IP: %s | VPN IP: %s",
			logData.Error, logData.ConfigFile, logData.SourceIP, logData.VPNIP)
	} else {
		logMsg = fmt.Sprintf("Status: %s | Config: %s | Source IP: %s | VPN IP: %s",
			logData.Status, logData.ConfigFile, logData.SourceIP, logData.VPNIP)
	}

	log.Println(logMsg)
}

func scheduleConfigs(configDir string, scheduler *gocron.Scheduler) {
	files, err := os.ReadDir(configDir)
	if err != nil {
		fmt.Println("Ошибка чтения директории:", err)
		return
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			configPath := filepath.Join(configDir, file.Name())
			// Запускаем задачу в шедулере каждые 10 секунд
			scheduler.Every(40).Seconds().Do(func() {
				processConfigFile(configPath)
			})
		}
	}
}

func main() {
	configDir := "./configs" // директория с конфигурационными файлами
	var wg sync.WaitGroup

	// Создаем новый шедулер
	scheduler := gocron.NewScheduler(time.UTC)

	// Планируем обработку конфигурационных файлов
	scheduleConfigs(configDir, scheduler)

	// Запускаем шедулер в отдельной горутине
	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.StartBlocking()
	}()

	// Ожидаем завершения работы
	wg.Wait()
}
