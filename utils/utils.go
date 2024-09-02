package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"xray-checker/models"

	"golang.org/x/net/proxy"
)

func ProviderFactory(providerType string, data json.RawMessage) (models.Provider, error) {
	switch providerType {
	case "uptime-kuma":
		var uptimeKuma models.UptimeKuma
		err := json.Unmarshal(data, &uptimeKuma)
		if err != nil {
			return nil, err
		}
		return &uptimeKuma, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerType)
	}
}

func GetIP(url string, client *http.Client) (string, error) {
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

func RunXray(configPath string) (*exec.Cmd, error) {
	cmd := exec.Command("xray", "-c", configPath)
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func KillXray(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func CreateProxyClient(proxyAddress string) (*http.Client, error) {
	proxyURL, err := url.Parse(proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy format: %v", err)
	}

	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("error creating proxy dialer: %v", err)
	}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	client := &http.Client{
		Transport: transport,
	}

	return client, nil
}

func LogResult(logData models.LogData) {
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
