// === extracted from internal/service/model.go ===

package service

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/user/llm-manager/pkg/yamlparser"
)

// ExportModel exports a model from the database to a ModelYAML struct.
func (s *ModelService) ExportModel(slug string) (*yamlparser.ModelYAML, error) {
	// 1. Get model from DB
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("model %s not found: %w", slug, err)
	}

	// 2. Convert JSON strings back to maps/slices
	y := &yamlparser.ModelYAML{
		Slug:          model.Slug,
		Name:          model.Name,
		Engine:        model.EngineType,
		HFRepo:        model.HFRepo,
		Container:     model.Container,
		Port:          model.Port,
		EnvVars:       map[string]string{},
		CommandArgs:   []string{},
		Capabilities:  []string{},
		LiteLLMParams: map[string]interface{}{},
		ModelInfo:     map[string]interface{}{},
	}

	// Parse env_vars JSON string
	if model.EnvVars != "" {
		var envVars map[string]string
		if err := json.Unmarshal([]byte(model.EnvVars), &envVars); err == nil {
			y.EnvVars = envVars
		}
	}

	// Parse command_args JSON string
	if model.CommandArgs != "" {
		if err := json.Unmarshal([]byte(model.CommandArgs), &y.CommandArgs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal command_args: %w", err)
		}
	}

	// Parse capabilities JSON string
	if model.Capabilities != "" {
		var caps []string
		if err := json.Unmarshal([]byte(model.Capabilities), &caps); err == nil {
			y.Capabilities = caps
		}
	}

	// Parse litellm_params JSON string
	if model.LiteLLMParams != "" {
		var litellmParams map[string]interface{}
		if err := json.Unmarshal([]byte(model.LiteLLMParams), &litellmParams); err == nil {
			y.LiteLLMParams = litellmParams
		}
	}

	// Parse model_info JSON string
	if model.ModelInfo != "" {
		var modelInfo map[string]interface{}
		if err := json.Unmarshal([]byte(model.ModelInfo), &modelInfo); err == nil {
			y.ModelInfo = modelInfo
		}
	}

	// Set optional cost fields as pointers
	if model.InputTokenCost > 0 {
		y.InputTokenCost = &model.InputTokenCost
	}
	if model.OutputTokenCost > 0 {
		y.OutputTokenCost = &model.OutputTokenCost
	}
	if model.CacheCreationInputTokenCost > 0 {
		y.CacheCreationInputTokenCost = &model.CacheCreationInputTokenCost
	}
	if model.CacheReadInputTokenCost > 0 {
		y.CacheReadInputTokenCost = &model.CacheReadInputTokenCost
	}

	// Export profile fields
	if model.TotalParamsB != nil {
		y.Profile = &yamlparser.ModelProfile{
			TotalParamsB:         model.TotalParamsB,
			ActiveParamsB:        model.ActiveParamsB,
			IsMoe:                model.IsMoe,
			AttentionLayers:      model.AttentionLayers,
			GdnLayers:            model.GdnLayers,
			NumKvHeads:           model.NumKvHeads,
			HeadDim:              model.HeadDim,
			SupportsMtp:          model.SupportsMtp,
			DefaultContext:       model.DefaultContext,
			MaxContext:           model.MaxContext,
			QuantBytesPerParam:   model.QuantBytesPerParam,
			MaxNumSeqs:           model.MaxNumSeqs,
			MaxNumBatchedTokens:  model.MaxNumBatchedTokens,
			SpeculativeDecoding:  model.SpeculativeDecoding,
			NumSpeculativeTokens: model.NumSpeculativeTokens,
			SpeculativeModel:     model.SpeculativeModel,
			GpuMemoryUtilization: model.GpuMemoryUtilization,
		}
	}

	// Export healthcheck JSON from DB -> YAML.
	if model.HealthcheckJSON != "" {
		y.HealthCheckJSON = model.HealthcheckJSON
	}

	return y, nil
}

// formatTypedValue converts a typed JSON value back to a string for YAML export.
// Unlike fmt.Sprintf("%v", v), this handles nil, nested maps, and arrays safely.
func formatTypedValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers are float64; format without unnecessary decimals
		s := strconv.FormatFloat(val, 'f', -1, 64)
		return s
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		// For nested maps/arrays, serialize to JSON string
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
