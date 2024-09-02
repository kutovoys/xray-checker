package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
	"xray-checker/models"
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

	provider, err := utils.ProviderFactory(providerType.Name, rawProvider)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func parseLink(link string) (*models.ParsedLink, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("error parsing link: %v", err)
	}

	protocol := strings.Split(u.Scheme, "://")[0]
	userInfo := u.User
	hostPort := strings.Split(u.Host, ":")
	queryParams := u.Query()

	parsed := &models.ParsedLink{
		Protocol: protocol,
		Server:   hostPort[0],
		Port:     hostPort[1],
		Name:     u.Fragment,
	}

	switch protocol {
	case "vless":
		parsed.UID = userInfo.Username()
		parsed.Security = queryParams.Get("security")
		parsed.Type = queryParams.Get("type")
		parsed.HeaderType = queryParams.Get("headerType")
		parsed.Path = queryParams.Get("path")
		parsed.Host = queryParams.Get("host")
		parsed.SNI = queryParams.Get("sni")
		parsed.FP = queryParams.Get("fp")
		parsed.PBK = queryParams.Get("pbk")
		parsed.SID = queryParams.Get("sid")

	case "trojan":
		parsed.UID = userInfo.Username()
		parsed.Security = queryParams.Get("security")
		parsed.Type = queryParams.Get("type")
		parsed.HeaderType = queryParams.Get("headerType")
		parsed.Path = queryParams.Get("path")
		parsed.Host = queryParams.Get("host")
		parsed.SNI = queryParams.Get("sni")
		parsed.FP = queryParams.Get("fp")

	case "ss":
		decodedUserInfo, err := base64.StdEncoding.DecodeString(userInfo.Username())
		if err != nil {
			return nil, fmt.Errorf("error decoding base64: %v", err)
		}
		parts := strings.Split(string(decodedUserInfo), ":")
		parsed.Method = parts[0]
		parsed.UID = parts[1]
		parsed.Protocol = "shadowsocks"
	}

	return parsed, nil
}

func generateXrayConfig(parsedLink *models.ParsedLink, templateDir, outputDir string) error {

	templatePath := filepath.Join(templateDir, fmt.Sprintf("%s.json.tmpl", parsedLink.Protocol))
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("error loading template: %v", err)
	}

	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s-%s.json", parsedLink.Protocol, parsedLink.Server))
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating xray config: %v", err)
	}
	defer outputFile.Close()

	err = tmpl.Execute(outputFile, parsedLink)
	if err != nil {
		return fmt.Errorf("error generating xray config: %v", err)
	}

	return nil
}

func processConfigFile(configPath string) {
	logData := models.LogData{ConfigFile: configPath}

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

	if logData.VPNIP != logData.SourceIP {
		_, err = http.Get(logData.WebhookURL)
		if err != nil {
			logData.Error = fmt.Errorf("error sending status: %v", err)
			logData.Status = "Error"
		} else {
			logData.Status = "Success"
		}
	} else {
		logData.Status = "IP addresses match, status not sent"
	}

	utils.LogResult(logData)
}

func scheduleConfigs(configDir string, scheduler *gocron.Scheduler, interval int) {
	files, err := os.ReadDir(configDir)
	if err != nil {
		fmt.Println("error reading directory:", err)
		return
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			configPath := filepath.Join(configDir, file.Name())
			scheduler.Every(interval).Seconds().Do(func() {
				processConfigFile(configPath)
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

	provider, err := loadProgramConfig(programConfigPath)
	if err != nil {
		fmt.Println("error loading program configuration:", err)
		return
	}

	for i, config := range provider.GetConfigs() {
		parsedLink, err := parseLink(config.Link)
		if err != nil {
			fmt.Println("error parsing link:", err)
			continue
		}

		parsedLink.MonitorLink = config.MonitorLink
		parsedLink.RandomPort = provider.GetProxySrartPort() + i

		err = generateXrayConfig(parsedLink, templateDir, configDir)
		if err != nil {
			fmt.Println("error generating Xray config:", err)
			continue
		}

		fmt.Printf("Xray config generated: %s-%s.json\n", parsedLink.Protocol, parsedLink.Server)
	}

	scheduler := gocron.NewScheduler(time.UTC)

	scheduleConfigs(configDir, scheduler, provider.GetInterval())

	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.StartBlocking()
	}()

	wg.Wait()
}
