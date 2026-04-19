// Package service provides a Docker Compose file generator for LLM models.
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/user/llm-manager/internal/database/models"
)

// composeTemplateData holds the data passed to the docker-compose template.
type composeTemplateData struct {
	ServiceName string            // "vllm-node" or "sglang-node"
	Container   string            // e.g. "llm-qwen3-next"
	Port        int               // e.g. 8017
	EnvVars     map[string]string // from model.EnvVars JSON
	CommandArgs map[string]string // from model.CommandArgs JSON
}

// composeTemplate is the docker-compose YAML template.
const composeTemplate = `services:
  llm:
    extends:
      file: base-pgx-llm.yml
      service: {{.ServiceName}}
    container_name: {{.Container}}
    ports:
      - "{{.Port}}:8000"
    environment:
      - HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}
{{- range $k, $v := .EnvVars}}
      - {{ $k }}={{ $v }}
{{- end}}
    command: >
{{- range $k, $v := .CommandArgs}}
      --{{ $k }} {{ $v }}{{ end }}
`

// ComposeGenerator generates docker-compose YAML from a model record.
type ComposeGenerator struct {
	vllmTemplate *template.Template
	sglangTemplate *template.Template
}

// NewComposeGenerator creates a new ComposeGenerator with pre-parsed templates.
func NewComposeGenerator() (*ComposeGenerator, error) {
	vllmTmpl, err := template.New("compose").Parse(composeTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose template: %w", err)
	}

	sglangTmpl, err := template.New("compose-sglang").Parse(composeTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose template: %w", err)
	}

	return &ComposeGenerator{
		vllmTemplate:   vllmTmpl,
		sglangTemplate: sglangTmpl,
	}, nil
}

// parseJSONField parses a JSON-encoded field from the model.
func parseJSONField(s string, target interface{}) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), target)
}

// GenerateVLLM generates a docker-compose YAML for a vLLM model.
func (g *ComposeGenerator) GenerateVLLM(model *models.Model) (string, error) {
	envVars := map[string]string{}
	if err := parseJSONField(model.EnvVars, &envVars); err != nil {
		return "", fmt.Errorf("failed to parse env_vars: %w", err)
	}

	commandArgs := map[string]string{}
	if err := parseJSONField(model.CommandArgs, &commandArgs); err != nil {
		return "", fmt.Errorf("failed to parse command_args: %w", err)
	}

	data := composeTemplateData{
		ServiceName: "vllm-node",
		Container:   model.Container,
		Port:        model.Port,
		EnvVars:     envVars,
		CommandArgs: commandArgs,
	}

	var buf bytes.Buffer
	if err := g.vllmTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render compose template: %w", err)
	}

	return buf.String(), nil
}

// GenerateSGLang generates a docker-compose YAML for an SGLang model.
func (g *ComposeGenerator) GenerateSGLang(model *models.Model) (string, error) {
	envVars := map[string]string{}
	if err := parseJSONField(model.EnvVars, &envVars); err != nil {
		return "", fmt.Errorf("failed to parse env_vars: %w", err)
	}

	commandArgs := map[string]string{}
	if err := parseJSONField(model.CommandArgs, &commandArgs); err != nil {
		return "", fmt.Errorf("failed to parse command_args: %w", err)
	}

	data := composeTemplateData{
		ServiceName: "sglang-node",
		Container:   model.Container,
		Port:        model.Port,
		EnvVars:     envVars,
		CommandArgs: commandArgs,
	}

	var buf bytes.Buffer
	if err := g.sglangTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render compose template: %w", err)
	}

	return buf.String(), nil
}

// Generate generates a docker-compose YAML based on the model's engine_type.
func (g *ComposeGenerator) Generate(model *models.Model) (string, error) {
	switch model.EngineType {
	case "sglang":
		return g.GenerateSGLang(model)
	default:
		return g.GenerateVLLM(model)
	}
}
