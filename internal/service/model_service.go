// Package service provides business logic services that wrap the database layer.
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/pkg/yamlparser"
)

// Health check constants — mirrored from cmd/health.go to avoid cross-package deps.
const (
	healthCheckTimeout       = 180 * time.Second
	healthCheckInterval      = 3 * time.Second
	healthCheckClientTimeout = 5 * time.Second
)

// OpenCodeModelEntry represents a model entry in opencode's provider.models.
type OpenCodeModelEntry struct {
	Name       string                 `json:"name,omitempty"`
	Limit      *OpenCodeLimit         `json:"limit,omitempty"`
	Cost       *OpenCodeCost          `json:"cost,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
	Variants   map[string]interface{} `json:"variants,omitempty"`
	Modalities map[string][]string    `json:"modalities,omitempty"`
}

// OpenCodeLimit represents context and output token limits.
type OpenCodeLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// OpenCodeCost represents per-1-million-tokens pricing.
type OpenCodeCost struct {
	Input  *float64 `json:"input,omitempty"`
	Output *float64 `json:"output,omitempty"`
}

// variantEntry holds a variant name and its merged parameters.
type variantEntry struct {
	Name   string
	Params map[string]interface{}
}

// ModelService handles business logic for LLM model operations.
type ModelService struct {
	db      database.DatabaseManager
	cfg     *config.Config
	litellm LiteLLMModeler
	eng     *EngineService
}

// NewModelService creates a new ModelService.
func NewModelService(db database.DatabaseManager, cfg *config.Config) *ModelService {
	return &ModelService{db: db, cfg: cfg}
}

// SetEngineService sets the optional EngineService for engine version resolution.
func (s *ModelService) SetEngineService(svc *EngineService) {
	s.eng = svc
}

// SetLiteLLMService sets the optional LiteLLM manager for delete+reimport mode.
func (s *ModelService) SetLiteLLMService(l LiteLLMModeler) {
	s.litellm = l
}

// ListModels returns all models from the database.
func (s *ModelService) ListModels() ([]models.Model, error) {
	return s.db.ListModels()
}

// GetModel returns a single model by slug.
func (s *ModelService) GetModel(slug string) (*models.Model, error) {
	return s.db.GetModel(slug)
}

// CreateModel creates a new model record.
func (s *ModelService) CreateModel(model *models.Model) error {
	return s.db.CreateModel(model)
}

// UpdateModel updates a model by slug.
func (s *ModelService) UpdateModel(slug string, updates map[string]interface{}) error {
	return s.db.UpdateModel(slug, updates)
}

// DeleteModel deletes a model by slug. If the deleted model was tracked as
// the latest-started model (LATEST_MODEL), that reference is cleared to avoid
// a stale pointer.
func (s *ModelService) DeleteModel(slug string) error {
	if err := s.db.DeleteModel(slug); err != nil {
		return err
	}
	configSvc := NewConfigService(s.db)
	if latest, err := configSvc.GetLatestModel(); err == nil && latest == slug {
		if err := configSvc.UnsetLatestModel(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clear latest model: %v\n", err)
		}
	}
	return nil
}

// UpdateModelWithYAML updates a model's fields using values from a YAML file.
// Unlike ImportModel, this does NOT check for duplicate slug.
func (s *ModelService) UpdateModelWithYAML(slug string, yamlPath string) (*models.Model, error) {
	y, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Expand template references (${{ .xxx }}) before validation.
	cfgValues := s.configValues()
	if err := yamlparser.ApplyTemplateVars(y, cfgValues); err != nil {
		return nil, fmt.Errorf("template expansion failed: %w", err)
	}

	baseErrs := yamlparser.Validate(y)
	extraErrs := s.validateEngineAndVersion(y)
	allErrs := append(baseErrs, extraErrs...)
	if len(allErrs) > 0 {
		var msg strings.Builder
		msg.WriteString("validation errors:\n")
		for _, e := range allErrs {
			fmt.Fprintf(&msg, "  - %s\n", e)
		}
		return nil, fmt.Errorf("invalid model YAML:%s", msg.String())
	}

	// Build updates map
	updates := map[string]interface{}{}
	if y.Name != "" {
		updates["name"] = y.Name
	}
	if y.HFRepo != "" {
		updates["hf_repo"] = y.HFRepo
	}
	if y.Container != "" {
		updates["container"] = y.Container
	}
	if y.Port > 0 {
		updates["port"] = y.Port
	}
	if y.Engine != "" {
		updates["engine_type"] = y.Engine
	}

	// Convert maps to JSON strings
	if len(y.EnvVars) > 0 {
		envVarsJSON, err := json.Marshal(y.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal env_vars: %w", err)
		}
		updates["env_vars"] = string(envVarsJSON)
	}
	if len(y.CommandArgs) > 0 {
		commandArgsStr, err := json.Marshal(y.CommandArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command_args: %w", err)
		}
		updates["command_args"] = string(commandArgsStr)
	}
	if len(y.Capabilities) > 0 {
		capabilitiesJSON, err := json.Marshal(y.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
		}
		updates["capabilities"] = string(capabilitiesJSON)
	}
	if y.InputTokenCost != nil {
		updates["input_token_cost"] = *y.InputTokenCost
	}
	if y.OutputTokenCost != nil {
		updates["output_token_cost"] = *y.OutputTokenCost
	}
	if y.CacheCreationInputTokenCost != nil {
		updates["cache_creation_input_token_cost"] = *y.CacheCreationInputTokenCost
	}
	if y.CacheReadInputTokenCost != nil {
		updates["cache_read_input_token_cost"] = *y.CacheReadInputTokenCost
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update from YAML")
	}

	if err := s.db.UpdateModel(slug, updates); err != nil {
		return nil, fmt.Errorf("failed to update model %s: %w", slug, err)
	}

	return s.db.GetModel(slug)
}

// GenerateOpenCodeModel generates opencode-compatible model entry for a single model.
func (s *ModelService) GenerateOpenCodeModel(slug string) ([]byte, error) {
	m, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get model %s: %w", slug, err)
	}

	modelsMap := make(map[string]*OpenCodeModelEntry)
	modelsMap[m.Slug] = s.buildOpenCodeEntry(m)
	return json.MarshalIndent(modelsMap, "", "  ")
}

// isGenerateExcluded returns true if the model should be excluded from
// generate opencode/pi output (RAG embeddings, rerankers, speech models).
func isGenerateExcluded(m *models.Model) bool {
	// Exclude RAG models
	if m.Type == "rag" {
		return true
	}
	// Exclude embedding models (type or subtype)
	if m.Type == "embed" || m.SubType == "embedding" || m.SubType == "embed" {
		return true
	}
	// Exclude reranker models (type or subtype)
	if m.Type == "rerank" || m.SubType == "reranker" || m.SubType == "rerank" {
		return true
	}
	// Exclude speech models
	if m.SubType == "stt" || m.SubType == "tts" || m.SubType == "omni" {
		return true
	}
	return false
}

// GenerateOpenCodeModels generates opencode-compatible model entries from all
// models registered in the database. It returns a JSON object of model entries
// keyed by slug, suitable for pasting directly into a provider's models section.
//
// For each model it:
//   - Includes the base model entry with name, limit, options, and variants
//   - Includes all variants (from litellm_params.variants) but NOT the active alias
//   - Includes input/output token costs if available
//   - Excludes top_k/top_p from base params so they can be set at provider level
//   - Excludes RAG embeddings, rerankers, and speech models
func (s *ModelService) GenerateOpenCodeModels() ([]byte, error) {
	models, err := s.db.ListModels()
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	modelsMap := make(map[string]*OpenCodeModelEntry)
	for _, m := range models {
		if isGenerateExcluded(&m) {
			continue
		}
		entry := s.buildOpenCodeEntry(&m)
		if entry != nil {
			modelsMap[m.Slug] = entry
		}
	}

	return json.MarshalIndent(modelsMap, "", "  ")
}

// buildOpenCodeEntry builds a single OpenCode model entry from a model record.
func (s *ModelService) buildOpenCodeEntry(m *models.Model) *OpenCodeModelEntry {
	oc := &OpenCodeModelEntry{}

	displayName := m.Name
	if displayName == "" {
		displayName = m.Slug
	}
	oc.Name = displayName

	oc.Options = map[string]interface{}{
		"model": m.Slug,
		"provider": map[string]interface{}{
			"model": m.Slug,
		},
	}

	contextLimit := 262144    // default context window
	outputLimit := uint(32768) // default output limit

	// Try to extract limits from model_info
	if m.ModelInfo != "" {
		var minfo map[string]interface{}
		if err := json.Unmarshal([]byte(m.ModelInfo), &minfo); err == nil {
			if inputTokens, ok := minfo["input_tokens_limits"].([]interface{}); ok && len(inputTokens) > 0 {
				if v, ok := inputTokens[0].(float64); ok {
					contextLimit = int(v)
				}
			}
			if outputTokens, ok := minfo["output_token_limits"].([]interface{}); ok && len(outputTokens) > 0 {
				if v, ok := outputTokens[0].(float64); ok {
					outputLimit = uint(v)
				}
			}
		}
	}

	oc.Limit = &OpenCodeLimit{
		Context: contextLimit,
		Output:  int(outputLimit),
	}

	// Cost: per-1-million-tokens pricing (multiply raw per-token cost by 1,000,000)
	if m.InputTokenCost > 0 || m.OutputTokenCost > 0 {
		oc.Cost = &OpenCodeCost{}
		if m.InputTokenCost > 0 {
			inputCostPerM := m.InputTokenCost * 1_000_000
			oc.Cost.Input = &inputCostPerM
		}
		if m.OutputTokenCost > 0 {
			outputCostPerM := m.OutputTokenCost * 1_000_000
			oc.Cost.Output = &outputCostPerM
		}
	}

	variants := s.extractVariants(*m)

	// Find the coder base variant: prefer "coder", fall back to "coder-fast"
	coderVariant := ""
	for _, v := range variants {
		if strings.EqualFold(v.Name, "coder") {
			coderVariant = v.Name
			break
		}
	}
	if coderVariant == "" {
		for _, v := range variants {
			if strings.EqualFold(v.Name, "coder-fast") {
				coderVariant = v.Name
				break
			}
		}
	}

	// If no coder variant exists, skip this model
	if coderVariant == "" {
		return nil
	}

	// Update options to use coder variant as base
	oc.Options["model"] = m.Slug + "-" + coderVariant
	if provider, ok := oc.Options["provider"].(map[string]interface{}); ok {
		provider["model"] = m.Slug + "-" + coderVariant
	}

	var caps []string
	json.Unmarshal([]byte(m.Capabilities), &caps)
	hasReasoning := false
	for _, c := range caps {
		if c == "reasoning" {
			hasReasoning = true
		}
	}

	// Build modalities based on capabilities
	oc.Modalities = map[string][]string{
		"input":  {"text"},
		"output": {"text"},
	}
	for _, c := range caps {
		switch c {
		case "image":
			oc.Modalities["input"] = append(oc.Modalities["input"], "image")
		case "video":
			oc.Modalities["input"] = append(oc.Modalities["input"], "video")
		case "document":
			oc.Modalities["input"] = append(oc.Modalities["input"], "pdf")
		}
	}

	// Only include coder-* variants
	oc.Variants = make(map[string]interface{})
	for _, v := range variants {
		// Skip the base coder variant
		if strings.EqualFold(v.Name, coderVariant) {
			continue
		}
		// Only include coder-* variants
		if !strings.HasPrefix(strings.ToLower(v.Name), "coder") {
			continue
		}
		vEntry := map[string]interface{}{
			"model": m.Slug + "-" + v.Name,
		}
		if hasReasoning {
			if strings.Contains(strings.ToLower(v.Name), "think") {
				vEntry["thinking"] = map[string]interface{}{
					"type":         "enabled",
					"budgetTokens": 16000,
				}
			}
		}
		oc.Variants[v.Name] = vEntry
	}

	return oc
}

// extractVariants parses litellm_params from a model record and returns
// a list of variant entries with their names and specs.
// Excludes top_k and top_p from base params so they can be set at provider level.
func (s *ModelService) extractVariants(m models.Model) []variantEntry {
	var variants []variantEntry

	if m.LiteLLMParams == "" {
		return variants
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(m.LiteLLMParams), &params); err != nil {
		return variants
	}

	variantsMap, ok := params["variants"].(map[string]interface{})
	if !ok || len(variantsMap) == 0 {
		return variants
	}

	// Build base params (without variants key, top_k, top_p)
	baseParams := make(map[string]interface{})
	for k, v := range params {
		if k != "variants" && k != "top_k" && k != "top_p" {
			baseParams[k] = v
		}
	}

	for name, spec := range variantsMap {
		vEntry := variantEntry{
			Name:   name,
			Params: make(map[string]interface{}),
		}

		if specMap, ok := spec.(map[string]interface{}); ok {
			// Merge base params with variant overrides
			for k, v := range baseParams {
				vEntry.Params[k] = v
			}
			// Override with variant-specific values (excluding top_k/top_p)
			for k, v := range specMap {
				if k == "top_k" || k == "top_p" {
					continue
				}
				vEntry.Params[k] = v
			}
		}

		variants = append(variants, vEntry)
	}

	return variants
}

// GenerateCompose generates a docker-compose YAML for the model using the given generator.
// It resolves the engine version from the model's DB record to build the full compose config.
func (s *ModelService) GenerateCompose(slug string, generator *ComposeGenerator, cfg EngineComposeConfig) (string, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return "", fmt.Errorf("model %s not found: %w", slug, err)
	}

	// If caller provided a config, merge it on top of the engine-resolved config.
	// Engine-resolved takes priority; caller overrides are layered on top.
	if cfg.Image == "" {
		resolved, err := s.resolveComposeConfig(model)
		if err != nil {
			return "", err
		}
		cfg = *resolved
	}

	// Apply caller-provided overrides
	// cfg.Image and cfg.EnvVars are already set from resolved config

	composeYAML, err := generator.Generate(model, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate compose file for %s: %w", slug, err)
	}

	return composeYAML, nil
}

// resolveComposeConfig resolves the engine version for a model and returns the
// full EngineComposeConfig with image, entrypoint, env vars, volumes, logging, deploy.
func (s *ModelService) resolveComposeConfig(model *models.Model) (*EngineComposeConfig, error) {
	if s.eng == nil {
		return &EngineComposeConfig{}, nil
	}
	return s.eng.BuildComposeConfig(model)
}

// ──────────────────────────────────────────────────────────────────────
// Pi-compatible model list generation
// ──────────────────────────────────────────────────────────────────────

// PiModelEntry represents a single model entry in Pi-compatible format.
type PiModelEntry struct {
	ID            string           `json:"id"`
	Reasoning     bool             `json:"reasoning"`
	Name          string           `json:"name"`
	Input         []string         `json:"input"`
	ContextWindow int              `json:"contextWindow"`
	MaxTokens     int              `json:"maxTokens"`
	Cost          *PiCostEntry     `json:"cost,omitempty"`
}

// PiCostEntry represents cost information per 1M tokens.
type PiCostEntry struct {
	Input     *float64 `json:"input,omitempty"`
	Output    *float64 `json:"output,omitempty"`
	CacheRead *float64 `json:"cacheRead,omitempty"`
	CacheWrite *float64 `json:"cacheWrite,omitempty"`
}

// GeneratePiModel generates Pi-compatible model entries for a single model.
func (s *ModelService) GeneratePiModel(slug string) ([]byte, error) {
	m, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get model %s: %w", slug, err)
	}

	entries := s.buildPiEntriesForModel(m)
	if len(entries) == 0 {
		return json.MarshalIndent([]PiModelEntry{}, "", "  ")
	}
	return json.MarshalIndent(entries, "", "  ")
}

// GeneratePiModels generates Pi-compatible model entries from all models
// registered in the database.
// Excludes RAG embeddings, rerankers, and speech models.
// For models with coder variants, creates separate entries for each variant.
func (s *ModelService) GeneratePiModels() ([]byte, error) {
	models, err := s.db.ListModels()
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	entries := make([]PiModelEntry, 0, len(models))
	for _, m := range models {
		if isGenerateExcluded(&m) {
			continue
		}
		variantEntries := s.buildPiEntriesForModel(&m)
		entries = append(entries, variantEntries...)
	}

	return json.MarshalIndent(entries, "", "  ")
}

// buildPiEntriesForModel creates Pi entries for a model, including all coder variants.
func (s *ModelService) buildPiEntriesForModel(m *models.Model) []PiModelEntry {
	variants := s.extractVariants(*m)

	// Find the coder base variant: prefer "coder", fall back to "coder-fast"
	coderVariant := ""
	for _, v := range variants {
		if strings.EqualFold(v.Name, "coder") {
			coderVariant = v.Name
			break
		}
	}
	if coderVariant == "" {
		for _, v := range variants {
			if strings.EqualFold(v.Name, "coder-fast") {
				coderVariant = v.Name
				break
			}
		}
	}

	// If no coder variant exists, skip this model
	if coderVariant == "" {
		return nil
	}

	// Get the base name from the model
	baseName := m.Name
	if baseName == "" {
		baseName = m.Slug
	}

	// Create entry for the base coder variant
	baseEntry := s.buildPiEntry(m)
	baseEntry.ID = m.Slug + "-" + coderVariant
	entries := []PiModelEntry{baseEntry}

	// Create entries for other coder-* variants
	for _, v := range variants {
		if strings.EqualFold(v.Name, coderVariant) {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(v.Name), "coder") {
			continue
		}

		// Extract suffix from variant name (e.g., "thinking" from "coder-thinking")
		suffix := extractVariantSuffix(v.Name)
		variantName := baseName + " " + suffix

		variantEntry := s.buildPiEntry(m)
		variantEntry.ID = m.Slug + "-" + v.Name
		variantEntry.Name = variantName
		entries = append(entries, variantEntry)
	}

	return entries
}

// extractVariantSuffix extracts the suffix from a variant name.
// For "coder-thinking" → "Thinking", for "coder-fast" → "Fast"
func extractVariantSuffix(variantName string) string {
	// Find the last "-" in the variant name
	lastDash := strings.LastIndex(variantName, "-")
	if lastDash == -1 {
		return variantName
	}

	suffix := variantName[lastDash+1:]
	// Replace underscores with spaces
	suffix = strings.ReplaceAll(suffix, "_", " ")
	// Capitalize first letter of each word
	words := strings.Fields(suffix)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// buildPiEntry builds a single Pi-compatible model entry from a model record.
func (s *ModelService) buildPiEntry(m *models.Model) PiModelEntry {
	entry := PiModelEntry{}

	// ID: use slug
	entry.ID = m.Slug

	// Name: display name or slug
	entry.Name = m.Name
	if entry.Name == "" {
		entry.Name = m.Slug
	}

	// Reasoning: check capabilities and name for thinking/reasoning
	entry.Reasoning = m.HasThinkingCapability()

	// Context window: default 262144
	entry.ContextWindow = 262144
	if m.ModelInfo != "" {
		var minfo map[string]interface{}
		if err := json.Unmarshal([]byte(m.ModelInfo), &minfo); err == nil {
			if inputTokens, ok := minfo["input_tokens_limits"].([]interface{}); ok && len(inputTokens) > 0 {
				if v, ok := inputTokens[0].(float64); ok {
					entry.ContextWindow = int(v)
				}
			}
		}
	}

	// Max tokens (output limit): default 32768
	entry.MaxTokens = 32768
	if m.ModelInfo != "" {
		var minfo map[string]interface{}
		if err := json.Unmarshal([]byte(m.ModelInfo), &minfo); err == nil {
			if outputTokens, ok := minfo["output_token_limits"].([]interface{}); ok && len(outputTokens) > 0 {
				if v, ok := outputTokens[0].(float64); ok {
					entry.MaxTokens = int(v)
				}
			}
		}
	}

	// Input modalities: only text and image
	entry.Input = []string{"text"}
	var caps []string
	json.Unmarshal([]byte(m.Capabilities), &caps)
	for _, c := range caps {
		if c == "image" {
			entry.Input = append(entry.Input, "image")
			break
		}
	}

	// Cost: per 1M tokens
	if m.InputTokenCost > 0 || m.OutputTokenCost > 0 || m.CacheReadInputTokenCost > 0 || m.CacheCreationInputTokenCost > 0 {
		entry.Cost = &PiCostEntry{}
		if m.InputTokenCost > 0 {
			v := m.InputTokenCost * 1_000_000
			entry.Cost.Input = &v
		}
		if m.OutputTokenCost > 0 {
			v := m.OutputTokenCost * 1_000_000
			entry.Cost.Output = &v
		}
		if m.CacheReadInputTokenCost > 0 {
			v := m.CacheReadInputTokenCost * 1_000_000
			entry.Cost.CacheRead = &v
		}
		if m.CacheCreationInputTokenCost > 0 {
			v := m.CacheCreationInputTokenCost * 1_000_000
			entry.Cost.CacheWrite = &v
		}
	}

	return entry
}
