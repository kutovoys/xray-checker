package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"
)

//go:embed templates/*.html
var content embed.FS

var indexTmpl *template.Template

func init() {
	var err error
	funcMap := template.FuncMap{
		"formatLatency": func(d time.Duration) string {
			if d == 0 {
				return "n/a"
			}
			return fmt.Sprintf("%dms", d.Milliseconds())
		},
	}

	indexTmpl, err = template.New("index.html").Funcs(funcMap).ParseFS(content, "templates/*.html")
	if err != nil {
		panic(err)
	}
}

type PageData struct {
	Version                    string
	Host                       string
	Port                       string
	CheckInterval              int
	IPCheckUrl                 string
	SimulateLatency            bool
	CheckMethod                string
	StatusCheckUrl             string
	DownloadUrl                string
	Timeout                    int
	SubscriptionUpdate         bool
	SubscriptionUpdateInterval int
	StartPort                  int
	Instance                   string
	PushUrl                    string
	Endpoints                  []EndpointInfo
	ShowServerDetails          bool
	IsPublic                   bool
	SubscriptionName           string
	SpeedtestEnabled           bool
	SpeedtestInterval          int
}

func RenderIndex(w io.Writer, data PageData) error {
	loader := GetAssetLoader()

	var tmpl *template.Template
	if loader != nil && loader.HasCustomTemplate() {
		tmpl = loader.GetCustomTemplate()
	} else {
		tmpl = indexTmpl
	}

	if loader == nil || !loader.HasCustomCSS() {
		return tmpl.Execute(w, data)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	html := buf.String()
	customCSSLink := `<link rel="stylesheet" href="/static/custom.css">`
	html = strings.Replace(html, "</head>", customCSSLink+"\n  </head>", 1)

	_, err := io.WriteString(w, html)
	return err
}
