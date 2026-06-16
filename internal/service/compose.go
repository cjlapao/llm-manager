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
		TotalParamsB:              *model.TotalParamsB,
		ActiveParamsB:             derefOrZero(model.ActiveParamsB),
		IsMoe:                     derefOrFalse(model.IsMoe),
		AttentionLayers:           derefOrZero(model.AttentionLayers),
		GdnLayers:                 derefOrZero(model.GdnLayers),
		NumKvHeads:                derefOrZero(model.NumKvHeads),
		HeadDim:                   derefOrZero(model.HeadDim),
		SupportsMtp:               derefOrFalse(model.SupportsMtp),
		SupportsVision:            strings.Contains(model.CommandArgs, "mm-processor-cache-type"),
		DefaultContext:            derefOrZero(model.DefaultContext),
		MaxContext:                derefOrZero(model.MaxContext),
		QuantBytesPerParam:        *model.QuantBytesPerParam,
		MaxNumSeqs:                derefOrZero(model.MaxNumSeqs),
		SubType:                   model.SubType,
		KvCacheOverheadMultiplier: 1.0,
	}

	// Detect hybrid (GDN) models and apply KV cache overhead multiplier.
	// Dense hybrid models with MTP need a higher multiplier because the draft
	// model's speculative tokens also consume KV cache entries, and GDN
	// attention kernels allocate additional page buffers.
	if profile.GdnLayers > 0 {
		if profile.SupportsMtp && !profile.IsMoe {
			profile.KvCacheOverheadMultiplier = 2.00
		} else {
			profile.KvCacheOverheadMultiplier = 1.44
		}
		fmt.Fprintf(os.Stderr, "  [kv-override] hybrid gdn=%d model, kv_cache_x%.2f\n", profile.GdnLayers, profile.KvCacheOverheadMultiplier)
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

	// MTP tokens: CLI override > DB profile.
	// If CLI sets --speculative-tokens 0, MTP is disabled even if DB has it configured.
	mtpTokens := 0
	if overrides.NumSpeculativeTokens != nil {
		mtpTokens = *overrides.NumSpeculativeTokens
	} else if model.NumSpeculativeTokens != nil && *model.NumSpeculativeTokens > 0 {
		mtpTokens = *model.NumSpeculativeTokens
	}

	// ── Determine GPU memory utilization ────────────────────────────────
	// Priority: CLI override > DB profile value > auto-calculated
	var gpuMemUtil float64
	var utilSource string // "cli", "db", or "calc"

	if overrides.GPUMemoryUtil != nil {
		gpuMemUtil = *overrides.GPUMemoryUtil
		utilSource = "cli"
	} else if model.GpuMemoryUtilization != nil && *model.GpuMemoryUtilization > 0 {
		gpuMemUtil = *model.GpuMemoryUtilization
		utilSource = "db"
	}

	// Compute memory result for the profile (with MTP tokens) only when
	// there is no override. When an override is present we still need the
	// breakdown for other flags (max-num-batched-tokens etc.) but will
	// replace the utilization afterwards.
	freeMB := 0
	if fm, err := readMemAvailableMB(); err == nil {
		freeMB = fm
	}
	memResult, err := CalculateMemory(profile, getKVDtypeBytes(model.CommandArgs), contextLen, numSequences, mtpTokens, freeMB)
	if err != nil {
		return existingCmds
	}

	// Override utilization if CLI or DB value is set
	if utilSource != "" {
		fmt.Fprintf(os.Stderr, "  [gpu-memory] using %s override: %.2f\n", utilSource, gpuMemUtil)
		memResult.GPUMemoryUtilization = gpuMemUtil
	} else {
		fmt.Fprintf(os.Stderr, "  [mem] weights=%d CUDA=%d prefix=%d totalRealistic=%d\n",
			memResult.Breakdown.WeightsMB,
			memResult.Breakdown.CUDAContextMB,
			memResult.Breakdown.PrefixCacheMB,
			memResult.TotalRealisticMB)
		fmt.Fprintf(os.Stderr, "  [mem] utilization=%.4f (%d / %d)",
			float64(memResult.TotalRealisticMB)/float64(TotalGPUMB),
			memResult.TotalRealisticMB, TotalGPUMB)
		if freeMB > 0 {
			fmt.Fprintf(os.Stderr, " (scaled from free RAM %d MB)", freeMB)
		}
		fmt.Fprintf(os.Stderr, "\n")
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

	// Determine speculative method: CLI override > DB profile.
	specMethod := ""
	if overrides.SpeculativeDecoding != nil && *overrides.SpeculativeDecoding != "" {
		specMethod = *overrides.SpeculativeDecoding
	} else if model.SpeculativeDecoding != nil {
		specMethod = *model.SpeculativeDecoding
	}

	// Inject --speculative-config only when we have both method AND tokens > 0.
	// If operator provides both via CLI, we trust them (no supports_mtp gate).
	if specMethod != "" && mtpTokens > 0 {
		specConfig := fmt.Sprintf("'{\"method\":\"%s\",\"num_speculative_tokens\":%d}'", specMethod, mtpTokens)
		result = append(result, "--speculative-config "+specConfig)
		fmt.Fprintf(os.Stderr, "  [speculative] --speculative-config %s\n", specConfig)
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
	Image              string
	Entrypoint         []string
	EnvVars            map[string]string
	Volumes            []string
	CommandArgs        []string
	LoggingSection     string
	DeploySection      string
	HealthCheckSection string
	UlimitsSection     string
	IPCOverride        string
}

// ComposeTemplateData is what gets passed to the Go template.
type ComposeTemplateData struct {
	ServiceName        string
	Container          string
	Port               int
	Image              string
	Entrypoint         []string
	EnvVars            map[string]string
	Volumes            []string
	CommandArgs        []string
	LoggingSection     string
	DeploySection      string
	HealthCheckSection string
	UlimitsSection     string
	IPCOverride        string
}

const composeTemplate = `services:
  {{.ServiceName}}:
    image: {{.Image}}
    container_name: {{.Container}}
{{- if .IPCOverride}}
    ipc: {{.IPCOverride}}
{{- else}}
    ipc: host
{{- end}}
    entrypoint: [{{range $i, $e := .Entrypoint}}{{if $i}}, {{end}}\"{{$e}}\"{{end}}]
    ports:
      - \"{{.Port}}:8000\"
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
{{- if .UlimitsSection}}
{{.UlimitsSection}}
{{- end}}
{{- if .LoggingSection}}
{{.LoggingSection}}
{{- end}}
{{- if .DeploySection}}
{{.DeploySection}}
{{- end}}
{{- if .HealthCheckSection}}
{{.HealthCheckSection}}
{{- end}}
`

// ComposeGenerator generates docker-compose YAML from model + engine config.
type ComposeGenerator struct {
	tmpl        *template.Template
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
		ServiceName:        fmt.Sprintf("%s-%s", model.Type, model.Slug),
		Container:          model.Container,
		Port:               model.Port,
		Image:              cfg.Image,
		Entrypoint:         cfg.Entrypoint,
		EnvVars:            cfg.EnvVars,
		Volumes:            cfg.Volumes,
		CommandArgs:        commandArgs,
		LoggingSection:     cfg.LoggingSection,
		DeploySection:      cfg.DeploySection,
		HealthCheckSection: cfg.HealthCheckSection,
		UlimitsSection:     cfg.UlimitsSection,
		IPCOverride:        cfg.IPCOverride,
	}

	// Add healthcheck for chat-type LLM models (only if no custom healthcheck was provided)
	if data.HealthCheckSection == "" && model.Type == "llm" && model.SubType == "chat" {
		data.HealthCheckSection = `    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
      interval: 30s
      timeout: 10s
      retries: 10
      start_period: 180s`
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
