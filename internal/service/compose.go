package service

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/user/llm-manager/internal/database/models"
)

// EngineComposeConfig carries pre-merged compose data from the caller.
type EngineComposeConfig struct {
	Image          string
	Entrypoint     []string
	EnvVars        map[string]string
	Volumes        []string
	CommandArgs    []string
	LoggingSection string
	DeploySection  string
}

// ComposeTemplateData is what gets passed to the Go template.
type ComposeTemplateData struct {
	ServiceName    string
	Container      string
	Port           int
	Image          string
	Entrypoint     []string
	EnvVars        map[string]string
	Volumes        []string
	CommandArgs    []string
	LoggingSection string
	DeploySection  string
}

const composeTemplate = `services:
  {{.ServiceName}}:
    image: {{.Image}}
    container_name: {{.Container}}
    ipc: host
    entrypoint: [{{range $i, $e := .Entrypoint}}{{if $i}}, {{end}}"{{$e}}"{{end}}]
    ports:
      - "{{.Port}}:8000"
    environment:
      - HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}
      - HF_TOKEN=${HF_TOKEN}
{{- range $k, $v := .EnvVars}}
      - {{$k}}={{$v}}
{{- end}}
{{- if .Volumes}}
    volumes:
{{- range .Volumes}}
      - {{.}}
{{- end}}
{{- end}}
{{- if .CommandArgs}}
    command: >
{{- range .CommandArgs}}
      {{.}} {{end}}
{{- end}}
{{- if .LoggingSection}}
{{.LoggingSection}}
{{- end}}
{{- if .DeploySection}}
{{.DeploySection}}
{{- end}}
`

// ComposeGenerator generates docker-compose YAML from model + engine config.
type ComposeGenerator struct {
	tmpl *template.Template
}

// NewComposeGenerator creates a new ComposeGenerator.
func NewComposeGenerator() (*ComposeGenerator, error) {
	funcs := template.FuncMap{
		"join": strings.Join,
	}
	tmpl, err := template.New("compose").Funcs(funcs).Parse(composeTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose template: %w", err)
	}
	return &ComposeGenerator{tmpl: tmpl}, nil
}

// Generate produces a complete docker-compose YAML string.
func (g *ComposeGenerator) Generate(model *models.Model, cfg EngineComposeConfig) (string, error) {
	data := ComposeTemplateData{
		ServiceName: fmt.Sprintf("%s-%s", model.Type, model.Slug),
		Container:      model.Container,
		Port:           model.Port,
		Image:          cfg.Image,
		Entrypoint:     cfg.Entrypoint,
		EnvVars:        cfg.EnvVars,
		Volumes:        cfg.Volumes,
		CommandArgs:    cfg.CommandArgs,
		LoggingSection: cfg.LoggingSection,
		DeploySection:  cfg.DeploySection,
	}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render compose template: %w", err)
	}
	return buf.String(), nil
}
