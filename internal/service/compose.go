package service

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/user/llm-manager/internal/database/models"
)

// mergeProfileFlagsWithOptions is like mergeProfileFlags but accepts CLI
// overrides that replace auto-calculated values.
func mergeProfileFlagsWithOptions(model *models.Model, existingCmds []string, overrides StartOverrides) []string {
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
		SupportsVision:     strings.Contains(model.CommandArgs, "mm-processor-cache-type"),
		DefaultContext:     derefOrZero(model.DefaultContext),
		MaxContext:         derefOrZero(model.MaxContext),
		QuantBytesPerParam: *model.QuantBytesPerParam,
		MaxNumSeqs:         derefOrZero(model.MaxNumSeqs),
	}

	// Apply CLI overrides — non-zero values replace auto-calculated defaults
	contextLen := profile.DefaultContext
	if overrides.MaxModelLen > 0 {
		contextLen = overrides.MaxModelLen
	}

	// numSequences: CLI override > profile > hardcoded default (1)
	numSequences := 1
	if profile.MaxNumSeqs > 0 {
		numSequences = profile.MaxNumSeqs
	}
	if overrides.MaxNumSeqs > 0 {
		numSequences = overrides.MaxNumSeqs
	}

	// Profile-based MaxNumBatchedTokens override (yields to CLI override)
	profileBatchedTokens := derefOrZero(model.MaxNumBatchedTokens)

	// Extract MTP tokens from command args (same logic as checkGPUMemory)
	mtpTokens := 0
	if model.CommandArgs != "" {
		idx := strings.Index(model.CommandArgs, "num_speculative_tokens")
		if idx >= 0 {
			rest := model.CommandArgs[idx+len("num_speculative_tokens"):]
			for i, ch := range rest {
				if ch >= '0' && ch <= '9' {
					for j := i; j < len(rest); j++ {
						c := rest[j]
						if c >= '0' && c <= '9' {
							mtpTokens = mtpTokens*10 + int(c-'0')
						} else {
							break
						}
					}
					break
				}
			}
		}
	}

	// Compute memory result for the profile (with MTP tokens)
	freeGPUmb := ReadFreeGPUMemory()
	memResult, err := CalculateMemory(profile, *model.QuantBytesPerParam, contextLen, numSequences, mtpTokens, freeGPUmb)
	if err != nil {
		return existingCmds
	}

	// Generate smart command flags
	genFlags := GenerateFlags(profile, memResult, contextLen, numSequences)

	// Debug: print what we're generating
	fmt.Fprintf(os.Stderr, "  mergeProfileFlags: contextLen=%d numSeqs=%d MTP=%d\n", contextLen, numSequences, mtpTokens)
	fmt.Fprintf(os.Stderr, "  genFlags: maxModelLen=%s maxNumBatchedTokens=%s maxNumSeqs=%s gpuMemUtil=%s\n",
		genFlags.MaxModelLen, genFlags.MaxNumBatchedTokens, genFlags.MaxNumSeqs, genFlags.GPUMemoryUtil)

	// Apply MaxNumBatchedTokens override if provided
	if overrides.MaxNumBatchedTokens > 0 {
		genFlags.MaxNumBatchedTokens = strconv.Itoa(overrides.MaxNumBatchedTokens)
		fmt.Fprintf(os.Stderr, "  genFlags[override]: maxNumBatchedTokens=%s\n", genFlags.MaxNumBatchedTokens)
	} else if profileBatchedTokens > 0 {
		// Profile-based MaxNumBatchedTokens override (yields to CLI override)
		genFlags.MaxNumBatchedTokens = strconv.Itoa(profileBatchedTokens)
		fmt.Fprintf(os.Stderr, "  genFlags[profile]: maxNumBatchedTokens=%s\n", genFlags.MaxNumBatchedTokens)
	}

	// Merge generated flags with existing args
	result := MergeFlags(existingCmds, genFlags)

	// Combine any remaining standalone "--flag" + value pairs into combined
	// strings so the compose template renders them on a single line.
	result = combineFlagPairs(result)

	// Remove any existing --speculative-config to prevent collisions
	// when the profile overrides it.
	result = removeSpeculativeConfigFlag(result)

	// Inject --speculative-config if profile specifies it and model supports MTP
	if model.SpeculativeDecoding != nil && *model.SpeculativeDecoding != "" &&
		model.NumSpeculativeTokens != nil && *model.NumSpeculativeTokens > 0 {

		// Only inject if model supports MTP (NVFP4 does not support speculative decoding)
		if model.SupportsMtp != nil && *model.SupportsMtp && model.QuantBytesPerParam != nil && *model.QuantBytesPerParam != 0.5 {
			// Inject as a single combined string with single quotes around the JSON
			// so the compose template renders it on one line: --speculative-config '{...}'
			specConfig := fmt.Sprintf("'{\"method\":\"%s\",\"num_speculative_tokens\":%d}'", *model.SpeculativeDecoding, *model.NumSpeculativeTokens)
			result = append(result, "--speculative-config "+specConfig)
			fmt.Fprintf(os.Stderr, "  [speculative] --speculative-config %s\n", specConfig)
		} else if !(model.SupportsMtp != nil && *model.SupportsMtp) {
			fmt.Fprintf(os.Stderr, "  [warning] speculative_decoding set but model does not support MTP\n")
		} else if model.QuantBytesPerParam != nil && *model.QuantBytesPerParam == 0.5 {
			fmt.Fprintf(os.Stderr, "  [warning] speculative_decoding set but NVFP4 does not support speculative decoding\n")
		}
	}

	fmt.Fprintf(os.Stderr, "  merge result: %v\n", result)
	return result
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
	tmpl       *template.Template
	comfyUITmpl *template.Template
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

	comfyUITmpl, err := template.New("comfyui").Funcs(funcs).Parse(comfyUITemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ComfyUI compose template: %w", err)
	}

	return &ComposeGenerator{tmpl: tmpl, comfyUITmpl: comfyUITmpl}, nil
}

// Generate produces a complete docker-compose YAML string.
func (g *ComposeGenerator) Generate(model *models.Model, cfg EngineComposeConfig) (string, error) {
	return g.GenerateWithOptions(model, cfg, StartOverrides{})
}

// GenerateWithOptions produces a complete docker-compose YAML string with
// optional CLI overrides applied to auto-calculated flags.
func (g *ComposeGenerator) GenerateWithOptions(model *models.Model, cfg EngineComposeConfig, overrides StartOverrides) (string, error) {
	if model == nil {
		return "", fmt.Errorf("model is required for composition generation")
	}

	// Merge profile-derived flags into command args (with overrides)
	commandArgs := mergeProfileFlagsWithOptions(model, cfg.CommandArgs, overrides)

	data := ComposeTemplateData{
		ServiceName:    fmt.Sprintf("%s-%s", model.Type, model.Slug),
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

// ComfyUIComposeTemplateData carries the data needed to render a ComfyUI
// Docker Compose YAML with GPU passthrough.
type ComfyUIComposeTemplateData struct {
	ImageName     string
	ImageTag      string
	HostPort      int
	VolumePath    string
	ContainerName string
}

const comfyUITemplate = `x-gpu-service: &gpu-service
  runtime: nvidia
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: all
            capabilities: [gpu]

services:
  comfyui:
    image: {{ .ImageName }}:{{ .ImageTag }}
    container_name: {{ .ContainerName }}
    restart: unless-stopped
    <<: *gpu-service
    ports:
      - "{{ .HostPort }}:8188"
    volumes:
      - {{ .VolumePath }}:/home/runner/ComfyUI/models
    environment:
      - CLI_ARGS=--listen 0.0.0.0
`

// GenerateComfyUICompose renders a Docker Compose YAML string for a ComfyUI
// service with GPU passthrough enabled via the NVIDIA runtime.
func (g *ComposeGenerator) GenerateComfyUICompose(data ComfyUIComposeTemplateData) (string, error) {
	if data.ImageName == "" {
		return "", fmt.Errorf("ImageName is required for ComfyUI composition")
	}
	if data.ContainerName == "" {
		return "", fmt.Errorf("ContainerName is required for ComfyUI composition")
	}

	var buf bytes.Buffer
	if err := g.comfyUITmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render ComfyUI compose template: %w", err)
	}
	return buf.String(), nil
}
