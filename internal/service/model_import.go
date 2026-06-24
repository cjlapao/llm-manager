// === extracted from internal/service/model.go ===

package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/pkg/yamlparser"
)

func (s *ModelService) ImportModel(yamlPath string, overrides ImportOverrides) (*models.Model, error) {
	// 1. Parse YAML
	y, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// 1b. Expand template references (${{ .xxx }}) in string fields
	cfgValues := s.configValues()
	if err := yamlparser.ApplyTemplateVars(y, cfgValues); err != nil {
		return nil, fmt.Errorf("template expansion failed: %w", err)
	}

	// 1c. Auto-inject capabilities from type/subtype (e.g., rag/embedding → "embedding")
	yamlparser.InjectCapabilitiesFromTypeSubtype(y)

	// 1d. Auto-discover profile from HF config if profile block is empty
	if y.Profile == nil {
		if dp, err := DiscoverProfile(y.HFRepo); err == nil {
			y.Profile = MergeProfile(nil, dp)
			for field, src := range dp.Sources {
				fmt.Fprintf(os.Stderr, "  %s: %s\n", field, src)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: profile discovery failed for %s: %v\n", y.HFRepo, err)
		}
	}

	// Handle override — delete existing DB record + LiteLLM deployments before reimport from YAML
	if overrides.Override {
		if existing, dbErr := s.db.GetModel(y.Slug); dbErr == nil && existing != nil {
			fmt.Fprintf(os.Stderr, "Override mode: found existing model %s, deleting...\n", y.Slug)
			if s.litellm != nil {
				if delErr := s.litellm.DeleteModel(y.Slug); delErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to delete from liteLLM: %v\n", delErr)
				} else {
					fmt.Fprintf(os.Stderr, "Deleted from liteLLM ✓\n")
				}
			}
			if delDbErr := s.db.DeleteModel(y.Slug); delDbErr != nil {
				return nil, fmt.Errorf("failed to delete %s from DB: %w", y.Slug, delDbErr)
			}
			fmt.Fprintf(os.Stderr, "Deleted from DB ✓\n")
		}
	}

	// Query DB-sourced engine slugs for duplicate checking (case-sensitive).
	var engineTypeSet map[string]struct{}
	if s.db != nil {
		engineTypes, err := s.db.ListEngineTypes()
		if err == nil {
			engineTypeSet = make(map[string]struct{}, len(engineTypes))
			for _, et := range engineTypes {
				engineTypeSet[et.Slug] = struct{}{}
			}
		}
	}

	// Validate YAML structure (non-capability, basic field checks).
	var validationErrs []error
	hasCliCapsOverride := len(overrides.Capabilities) > 0

	if hasCliCapsOverride {
		baseErrs := yamlparser.ValidateNonCapabilities(y)
		validationErrs = baseErrs
	} else {
		baseErrs := yamlparser.Validate(y)
		validationErrs = baseErrs
	}

	// Additional DB-level validation for engine + engine_version slugs.
	dbErrs := s.validateEngineAndVersion(y)
	validationErrs = append(validationErrs, dbErrs...)

	if len(validationErrs) > 0 {
		var msgParts []string
		for _, e := range validationErrs {
			msgParts = append(msgParts, e.Error())
		}
		return nil, fmt.Errorf("YAML validation failed: %s", strings.Join(msgParts, "; "))
	}

	// Check for duplicate slug (skip override if just cleared) or unknown engine type.
	if _, err := s.db.GetModel(y.Slug); err == nil {
		if !overrides.Override {
			return nil, fmt.Errorf("model %s already exists", y.Slug)
		}
	}

	// LiteLLMParams is optional (e.g. rag/embed/rerank models may not need a proxy).
	// buildLiteLLMParams returns an empty map when no API URL and no YAML api_base
	// are available — ImportModel will store an empty string in the DB.
	litellmParams := s.buildLiteLLMParams(y, s.cfg.OpenAIAPIURL, y.Slug, y.Port)

	// Only LLM-type models need LiteLLM proxy routing (api_base + model name).
	// rag/embedding, rag/reranker, speech/stt, speech/tts, speech/omni, comfyui
	// do not route through LiteLLM — clear any auto-generated params.
	if y.Type != "llm" && y.Type != "auto-complete" && !isSpeechType(y.SubType) {
		litellmParams = map[string]interface{}{}
	}

	// Build model_info map from capabilities (auto-generated) + YAML overrides
	modelInfo := s.buildModelInfo(y)

	// Map ModelYAML → models.Model
	model := &models.Model{
		Slug:                        y.Slug,
		Type:                        y.Type,    // from YAML (defaults to "llm" if empty)
		SubType:                     y.SubType, // from YAML
		Name:                        y.Name,
		HFRepo:                      y.HFRepo,
		Container:                   y.Container,
		Port:                        y.Port,
		EngineType:                  "vllm", // default engine
		EngineVersionSlug:           "",     // resolved by engine service
		InputTokenCost:              0.0,
		OutputTokenCost:             0.0,
		CacheCreationInputTokenCost: 0.0,
		CacheReadInputTokenCost:     0.0,
		Capabilities:                "",
		EnvVars:                     "",
		CommandArgs:                 "",
		Default:                     false,
	}

	// Apply YAML-level optional fields
	if y.Engine != "" {
		model.EngineType = y.Engine
	}
	if y.EngineVersion != "" {
		overrides.EngineVersion = y.EngineVersion
	}

	// Convert maps to JSON strings
	if len(y.EnvVars) > 0 {
		b, err := json.Marshal(y.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal env_vars: %w", err)
		}
		model.EnvVars = string(b)
	}

	if len(y.CommandArgs) > 0 {
		b, err := json.Marshal(y.CommandArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command_args: %w", err)
		}
		model.CommandArgs = string(b)
	}

	// Apply YAML-level optional fields
	if y.InputTokenCost != nil {
		model.InputTokenCost = *y.InputTokenCost
	}
	if y.OutputTokenCost != nil {
		model.OutputTokenCost = *y.OutputTokenCost
	}
	if y.CacheCreationInputTokenCost != nil {
		model.CacheCreationInputTokenCost = *y.CacheCreationInputTokenCost
	}
	if y.CacheReadInputTokenCost != nil {
		model.CacheReadInputTokenCost = *y.CacheReadInputTokenCost
	}
	if len(y.Capabilities) > 0 {
		b, err := json.Marshal(y.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
		}
		model.Capabilities = string(b)
	}

	// Map profile fields from YAML to model
	if y.Profile != nil {
		model.TotalParamsB = y.Profile.TotalParamsB
		model.ActiveParamsB = y.Profile.ActiveParamsB
		model.IsMoe = y.Profile.IsMoe
		model.AttentionLayers = y.Profile.AttentionLayers
		model.GdnLayers = y.Profile.GdnLayers
		model.NumKvHeads = y.Profile.NumKvHeads
		model.HeadDim = y.Profile.HeadDim
		model.SupportsMtp = y.Profile.SupportsMtp
		model.DefaultContext = y.Profile.DefaultContext
		model.MaxContext = y.Profile.MaxContext
		model.QuantBytesPerParam = y.Profile.QuantBytesPerParam
		model.MaxNumSeqs = y.Profile.MaxNumSeqs
		model.MaxNumBatchedTokens = y.Profile.MaxNumBatchedTokens
		model.SpeculativeDecoding = y.Profile.SpeculativeDecoding
		model.NumSpeculativeTokens = y.Profile.NumSpeculativeTokens
		model.SpeculativeModel = y.Profile.SpeculativeModel
		model.GpuMemoryUtilization = y.Profile.GpuMemoryUtilization
	}
	// Wire healthcheck JSON from YAML -> DB.
	model.HealthcheckJSON = y.HealthCheckJSON

	// Marshal litellm_params and model_info to JSON for DB storage
	if len(litellmParams) > 0 {
		b, err := json.Marshal(litellmParams)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal litellm_params: %w", err)
		}
		model.LiteLLMParams = string(b)
	}
	if len(modelInfo) > 0 {
		b, err := json.Marshal(modelInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal model_info: %w", err)
		}
		model.ModelInfo = string(b)
	}

	// 7. Apply CLI overrides
	if overrides.InputCost != nil {
		model.InputTokenCost = *overrides.InputCost
	}
	if overrides.OutputCost != nil {
		model.OutputTokenCost = *overrides.OutputCost
	}
	if len(overrides.Capabilities) > 0 {
		b, err := json.Marshal(overrides.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal override capabilities: %w", err)
		}
		model.Capabilities = string(b)
	}
	if overrides.Type != "" {
		model.Type = overrides.Type
	}
	if overrides.Engine != "" {
		model.EngineType = overrides.Engine
	}
	// Set engine version slug
	if overrides.EngineVersion != "" {
		model.EngineVersionSlug = overrides.EngineVersion
	} else if model.EngineVersionSlug == "" && s.eng != nil {
		// Resolve to default version for the engine type
		defVer, err := s.eng.ResolveDefaultVersion(model.EngineType)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve default engine version for '%s': %w", model.EngineType, err)
		}
		if defVer != nil {
			model.EngineVersionSlug = defVer.Slug
		}
	}

	// 8. Create in database
	if err := s.db.CreateModel(model); err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	// If we did an override import (deleted old LiteLLM deployments),
	// recreate them now so the model is usable immediately without
	// requiring a separate 'litellm sync' step.
	if overrides.Override {
		if s.litellm != nil && (model.Type == "llm" || isSpeechType(model.SubType)) {
			if syncErr := s.litellm.SyncModel(model.Slug); syncErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to sync model to LiteLLM after override import: %v\n", syncErr)
			} else {
				fmt.Fprintf(os.Stderr, "Synced to LiteLLM ✓\n")
			}
		}
	}

	return model, nil
}

// buildLiteLLMParams builds a liteLLM params map from YAML values,
// auto-merging api_base / model name from config. When openAIAURL is empty
// and no api_base was supplied in YAML, the returned map is just the YAML
// contents (plus "model" set to slug) — no error is raised so import stays
// usable without a proxy.
func (s *ModelService) buildLiteLLMParams(y *yamlparser.ModelYAML, openAIAURL string, slug string, port int) map[string]interface{} {
	params := make(map[string]interface{})

	// Start with YAML-provided values
	if y.LiteLLMParams != nil {
		for k, v := range y.LiteLLMParams {
			params[k] = v
		}
	}

	// Remove input/output cost from litellm_params - they're already at root level
	delete(params, "input_cost_per_token")
	delete(params, "output_cost_per_token")

	// Auto-construct api_base from OPENAI_API_URL + PORT/v1 only when
	// api_base is not already set in YAML and we have a config URL.
	// When no API URL is configured and no YAML api_base exists, the map
	// stays as-is (may be empty or YAML-only) — import still succeeds.
	if _, hasAPIBase := params["api_base"]; !hasAPIBase && openAIAURL != "" {
		base := strings.TrimRight(openAIAURL, "/")
		if port > 0 {
			base = fmt.Sprintf("%s:%d/v1", base, port)
		} else {
			base = fmt.Sprintf("%s/v1", base)
		}
		params["api_base"] = base
	}

	// Auto-set model name from slug (if not already set in YAML)
	if _, hasModel := params["model"]; !hasModel {
		params["model"] = slug
	}

	return params
}

// subtypeToMode maps a model type and subtype to the corresponding LiteLLM mode.
func subtypeToMode(modelType, subType string) string {
	switch modelType {
	case "llm", "auto-complete":
		return "chat"
	case "rag":
		if subType == "embedding" {
			return "embedding"
		}
		return "rerank"
	case "speech":
		switch subType {
		case "stt":
			return "audio_transcription"
		case "tts":
			return "audio_speech"
		case "omni":
			return "realtime"
		default:
			return "chat"
		}
	case "comfyui":
		if subType == "image" {
			return "image_generation"
		}
		return "chat"
	default:
		return "chat"
	}
}

// buildModelInfo constructs the model_info map by:
// 1. Starting with YAML-provided model_info values
// 2. Auto-setting boolean fields based on capabilities
// 3. Setting defaults for fields not explicitly provided
func (s *ModelService) buildModelInfo(y *yamlparser.ModelYAML) map[string]interface{} {
	info := make(map[string]interface{})

	// Start with YAML-provided model_info values
	if y.ModelInfo != nil {
		for k, v := range y.ModelInfo {
			info[k] = v
		}
	}

	// Auto-generate support fields from capabilities
	for _, cap := range y.Capabilities {
		if fieldNames, ok := yamlparser.CapabilitiesToModelInfo[cap]; ok {
			for _, fieldName := range fieldNames {
				// Only set if not already provided in YAML
				if _, exists := info[fieldName]; !exists {
					info[fieldName] = true
				}
			}
		}
	}

	// Set defaults for fields not explicitly provided
	if _, exists := info["direct_access"]; !exists {
		info["direct_access"] = false
	}
	if _, exists := info["litellm_provider"]; !exists {
		info["litellm_provider"] = y.Engine // default based on engine type
	}
	if _, exists := info["mode"]; !exists {
		info["mode"] = subtypeToMode(y.Type, y.SubType)
	}
	if y.HealthCheckVoice != nil && *y.HealthCheckVoice != "" {
		info["health_check_voice"] = *y.HealthCheckVoice
	}
	if _, exists := info["supports_vision"]; !exists {
		info["supports_vision"] = false
	}
	if _, exists := info["supports_embedding_image_input"]; !exists {
		info["supports_embedding_image_input"] = false
	}
	if _, exists := info["supports_function_calling"]; !exists {
		info["supports_function_calling"] = false
	}
	if _, exists := info["supports_tool_choice"]; !exists {
		info["supports_tool_choice"] = false
	}
	if _, exists := info["supports_reasoning"]; !exists {
		info["supports_reasoning"] = false
	}
	if _, exists := info["supports_video"]; !exists {
		info["supports_video"] = false
	}
	if _, exists := info["supports_document"]; !exists {
		info["supports_document"] = false
	}
	if _, exists := info["supports_embedding"]; !exists {
		info["supports_embedding"] = false
	}
	if _, exists := info["supports_reranking"]; !exists {
		info["supports_reranking"] = false
	}
	if _, exists := info["supports_stt"]; !exists {
		info["supports_stt"] = false
	}
	if _, exists := info["supports_tts"]; !exists {
		info["supports_tts"] = false
	}
	if _, exists := info["supports_multimodal"]; !exists {
		info["supports_multimodal"] = false
	}

	return info
}
