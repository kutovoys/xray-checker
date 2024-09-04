package utils

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"xray-checker/models"

	"golang.org/x/net/proxy"
)

func ParseLink(link string) (*models.ParsedLink, error) {
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
		parsed.Flow = queryParams.Get("flow")
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

func GenerateXrayConfig(parsedLink *models.ParsedLink, templateDir, outputDir string) error {

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

func LogResult(logData models.ConnectionData) {
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
