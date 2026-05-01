package service

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/user/llm-manager/internal/database/models"
)

// mergeProfileFlags takes existing command args and, if the model has profile
// data (total_params_b != nil), computes a MemoryResult, generates smart
// command flags, and merges them into the existing args.
// If no profile data exists the existing args are returned unchanged.
// The generated command array is computed at runtime during compose
// generation and is NOT persisted to the database.
func mergeProfileFlags(model *models.Model, existingCmds []string) []string {
	if model == nil || model.TotalParamsB == nil || model.QuantBytesPerParam == nil {
		return existingCmds
	}

	// Build profile from DB fields
	profile := ModelProfile{
		TotalParamsB:       *model.TotalParamsB,
		ActiveParamsB:      derefOrZero(model.ActiveParamsB),
		IsMoe:              derefOrFalse(model.IsMoe),
		AttentionLayers:    derefOrZero(model.AttentionLayers),
		GdnLayers:          derefOrZero(model.GdnLayers),
		NumKvHeads:         derefOrZero(model.NumKvHeads),
		HeadDim:            derefOrZero(model.HeadDim),
		SupportsMtp:        derefOrFalse(model.SupportsMtp),
		DefaultContext:     derefOrZero(model.DefaultContext),
		MaxContext:         derefOrZero(model.MaxContext),
		QuantBytesPerParam: *model.QuantBytesPerParam,
	}

	// Compute memory result for the profile
	memResult, err := CalculateMemory(profile, *model.QuantBytesPerParam, profile.DefaultContext, 1, 0)
	if err != nil {
		// If memory calculation fails, fall back to existing args
		return existingCmds
	}

	// Generate smart command flags
	genFlags := GenerateFlags(profile, memResult, profile.DefaultContext, 1)

	// Merge generated flags with existing args
	return MergeFlags(existingCmds, genFlags)
}

// derefOrZero returns the dereferenced value of a *T or zero for *int/*float64.
func derefOrZero[T ~int | ~float64](p *T) T {
	if p == nil {
		return 0
	}
	return *p
}

// derefOrFalse returns the dereferenced value of a *bool or false.
func derefOrFalse(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

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
	if model == nil {
		return "", fmt.Errorf("model is required for composition generation")
	}

	// Merge profile-derived flags into command args
	commandArgs := mergeProfileFlags(model, cfg.CommandArgs)

	data := ComposeTemplateData{
		ServiceName: fmt.Sprintf("%s-%s", model.Type, model.Slug),
		Container:      model.Container,
		Port:           model.Port,
		Image:          cfg.Image,
		Entrypoint:     cfg.Entrypoint,
		EnvVars:        cfg.EnvVars,
		Volumes:        cfg.Volumes,
		CommandArgs:    commandArgs,
		LoggingSection: cfg.LoggingSection,
		DeploySection:  cfg.DeploySection,
	}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render compose template: %w", err)
	}
	return buf.String(), nil
}
