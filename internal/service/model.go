package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/pkg/yamlparser"
)

// resolvePortCollision scans existing models for any conflict with the requested port.
// It returns (resolvedPort, changed bool) where changed=true means the user's explicit
// value was bumped because another model already claimed that slot. If no collisions
// exist within a reasonable search window the original port stays untouched.
func (s *ModelService) resolvePortCollision(requestedPort int) (int, bool) {
    allModels, err := s.db.ListModels()
    if err != nil || len(allModels) == 0 { // Nothing in DB yet
        return requestedPort, false
    }

    usedPortSet := make(map[int]struct{})
    for _, m := range allModels {
        if m.Port > 0 {
            usedPortSet[m.Port] = struct{}{}
        }
    }

    candidate := requestedPort
    const maxShift = 256 // Sufficiently large cap; realistically < 100 models
    for shift := 0; shift <= maxShift; shift++ {
        if _, occupied := usedPortSet[candidate]; !occupied {
                if shift != 0 { // Bumped away from user-requested slot
                     fmt.Printf("ℹ Port %d already in use\n",requestedPort)
                  fmt.Printf ("→ Using free port %d instead.\n,",candidate )  
               }
              return candidate, shift != 0
        }
        candidate++
    }
    
    // All slots blocked up to boundary; fail gracefully
    return requestedPort, false
	}
	
// ImportOverrides holds CLI argument overrides for model import.
type ImportOverrides struct {
	InputCost    *float64
	OutputCost   *float64
	Capabilities []string
}
	Capabilities []string
}

// ImportModel imports a model from a YAML file into the database.
// It parses the YAML, validates it, checks for duplicates, maps to models.Model,
// applies CLI overrides, and creates the model record.
// LiteLLM params are auto-merged: api_base is constructed from config URL + model port,
// and model name is derived from the slug.
// model_info is auto-generated from capabilities.
func (s *ModelService) ImportModel(yamlPath string, overrides ImportOverrides) (*models.Model, error) {
	// 1. Parse YAML
	y, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// 2. Validate — skip capability validation when CLI overrides are provided
	// (the overrides will replace YAML capabilities entirely)
	var validationErrs []error
	if len(overrides.Capabilities) > 0 {
		// Validate only non-capability fields when overrides are present
		validationErrs = yamlparser.ValidateNonCapabilities(y)
	} else {
		validationErrs = yamlparser.Validate(y)
	}
	if len(validationErrs) > 0 {
		var msgParts []string
		for _, e := range validationErrs {
			msgParts = append(msgParts, e.Error())
		}
		return nil, fmt.Errorf("YAML validation failed: %s", strings.Join(msgParts, "; "))
	}

	// 3. Check for duplicate slug
	if _, err := s.db.GetModel(y.Slug); err == nil {
		return nil, fmt.Errorf("model %s already exists", y.Slug)
	}

	// 4. Build litellm_params map from YAML, auto-merging system values
	litellmParams, err := s.buildLiteLLMParams(y, s.cfg.OpenAIAPIURL, y.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed to build litellm params: %w", err)
	}

	// 5. Build model_info map from capabilities (auto-generated) + YAML overrides
	modelInfo := s.buildModelInfo(y)

	// 6. Map ModelYAML → models.Model
	model := &models.Model{
		Slug:            y.Slug,
		Type:            "llm",
		Name:            y.Name,
		HFRepo:          y.HFRepo,
		Container:       y.Container,
		Port:            y.Port,
		EngineType:      y.Engine,
		InputTokenCost:  0.0,
		OutputTokenCost: 0.0,
		Capabilities:    "",
		EnvVars:         "",
		CommandArgs:     "",
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
		typedArgs := yamlparser.ParseTypedCommandArgs(y.CommandArgs)
		b, err := yamlparser.CommandArgsToJSON(typedArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command_args: %w", err)
		}
		model.CommandArgs = b
	}

	// Apply YAML-level optional fields
	if y.InputTokenCost != nil {
		model.InputTokenCost = *y.InputTokenCost
	}
	if y.OutputTokenCost != nil {
		model.OutputTokenCost = *y.OutputTokenCost
	}
	if len(y.Capabilities) > 0 {
		b, err := json.Marshal(y.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
		}
		model.Capabilities = string(b)
	}

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

	// 8. Create in database
	if err := s.db.CreateModel(model); err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	return model, nil
}

// buildLiteLLMParams constructs the litellm_params map from YAML values,
// auto-merging system-provided values (api_base, model name) and excluding
// values already present at the YAML root level (input_cost_per_token,
// output_cost_per_token).
// Requires openaiAPIURL to be set for api_base construction; returns an error
// if openaiAPIURL is empty and no api_base is provided in YAML.
// System values take precedence when not explicitly set in YAML.
func (s *ModelService) buildLiteLLMParams(y *yamlparser.ModelYAML, openaiAPIURL string, slug string) (map[string]interface{}, error) {
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

	// Auto-construct api_base from OPENAI_API_URL
	// If api_base is already set in YAML, keep it (user override)
	if _, hasAPIBase := params["api_base"]; !hasAPIBase {
		if openaiAPIURL == "" {
			return nil, fmt.Errorf("OPENAI_API_URL must be set to construct api_base for model creation/update")
		}
		// Append /v1 if not already present
		base := openaiAPIURL
		if !strings.HasSuffix(base, "/v1") {
			base = strings.TrimRight(base, "/") + "/v1"
		}
		params["api_base"] = base
	}

	// Auto-set model name from slug (if not already set in YAML)
	if _, hasModel := params["model"]; !hasModel {
		params["model"] = slug
	}

	return params, nil
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
		info["mode"] = "chat"
	}
	if _, exists := info["supports_vision"]; !exists {
		info["supports_vision"] = false
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
	if _, exists := info["supports_embedding_image_input"]; !exists {
		info["supports_embedding_image_input"] = false
	}

	return info
}

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
		CommandArgs:   map[string]string{},
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
		var cmdArgs map[string]interface{}
		if err := json.Unmarshal([]byte(model.CommandArgs), &cmdArgs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal command_args: %w", err)
		}
		y.CommandArgs = make(map[string]string, len(cmdArgs))
		for k, v := range cmdArgs {
			y.CommandArgs[k] = formatTypedValue(v)
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
