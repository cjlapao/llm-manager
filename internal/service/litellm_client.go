// Package service provides LiteLLM proxy integration for model management.
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

const (
	activeAliasName     = "active"
	activeThinkingAlias = "active-thinking"
	ragAliasReranker    = "active-reranker"
	ragAliasEmbeddings  = "active-embeddings"
	speechAliasSTT      = "active-stt"
	speechAliasTTS      = "active-tts"
	speechAliasOmni     = "active-omni"
)

// LiteLLMService handles CRUD operations against the LiteLLM proxy API.
type LiteLLMService struct {
	db        database.DatabaseManager
	cfg       *config.Config
	configSvc *ConfigService
	client    *http.Client
}

// LiteLLMModel represents a deployment in LiteLLM's config.yaml.
type LiteLLMModel struct {
	ModelName     string                 `json:"model_name"`
	LiteLLMParams map[string]interface{} `json:"litellm_params"`
	ModelInfo     map[string]interface{} `json:"model_info"`
}

// LiteLLMUpdate represents a partial update to a LiteLLM model.
type LiteLLMUpdate struct {
	ModelName     *string                 `json:"model_name,omitempty"`
	LiteLLMParams *map[string]interface{} `json:"litellm_params,omitempty"`
	ModelInfo     *map[string]interface{} `json:"model_info,omitempty"`
}

// LiteLLMListResponse is the OpenAI-compatible model list response.
type LiteLLMListResponse struct {
	Data   []LiteLLMModelItem `json:"data"`
	Object string             `json:"object"`
}

// LiteLLMModelItem is a single model in the list response.
type LiteLLMModelItem struct {
	ID         string        `json:"id"`
	Object     string        `json:"object"`
	Created    int64         `json:"created"`
	OwnedBy    string        `json:"owned_by"`
	Permission []interface{} `json:"permission,omitempty"`
}

// LiteLLMCreateResponse is the response from POST /model/new.
type LiteLLMCreateResponse struct {
	ModelID       string                 `json:"model_id"`
	ModelName     string                 `json:"model_name"`
	LiteLLMParams map[string]interface{} `json:"litellm_params"`
	ModelInfo     map[string]interface{} `json:"model_info"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
	CreatedBy     string                 `json:"created_by"`
	UpdatedBy     string                 `json:"updated_by"`
}

// LiteLLMModelInfo represents detailed model metadata for the CLI "litellm get"
// subcommand. It aggregates the flat fields from the /model/info API response
// alongside the nested litellm_params and model_info blocks used by the tool.
type LiteLLMModelInfo struct {
	// Direct API response fields
	ModelName                   string                 `json:"model_name"`
	LiteLLMProvider             string                 `json:"litellm_provider,omitempty"`
	CustomLLMProvider           string                 `json:"custom_llm_provider,omitempty"`
	Mode                        string                 `json:"mode,omitempty"`
	DirectAccess                bool                   `json:"direct_access,omitempty"`
	SupportsVision              *bool                  `json:"supports_vision,omitempty"`
	SupportsFunctionCalling     *bool                  `json:"supports_function_calling,omitempty"`
	SupportsToolChoice          *bool                  `json:"supports_tool_choice,omitempty"`
	SupportsReasoning           *bool                  `json:"supports_reasoning,omitempty"`
	SupportsEmbeddingImageInput *bool                  `json:"supports_embedding_image_input,omitempty"`
	DefaultCachingAPIRequest    *bool                  `json:"default_caching_api_request,omitempty"`
	Filename                    string                 `json:"filename,omitempty"`
	VendorAPIStandard           string                 `json:"vendor_api_standard,omitempty"`
	Requires                    *string                `json:"requires,omitempty"`
	PrefixModel                 string                 `json:"prefix_model,omitempty"`
	Outputs                     []interface{}          `json:"outputs,omitempty"`
	OutputPricePerToken         *float64               `json:"output_price_per_token,omitempty"`
	InputPricePerToken          *float64               `json:"input_price_per_token,omitempty"`
	FunctionCallTypes           []interface{}          `json:"function_call_types,omitempty"`
	ToolChoiceType              string                 `json:"tool_choice_type,omitempty"`
	ToolProvider                string                 `json:"tool_provider,omitempty"`
	InputTokenLimits            []int64                `json:"input_tokens_limits,omitempty"`
	OutputTokenLimits           []int64                `json:"output_token_limits,omitempty"`
	Timeout                     *float64               `json:"timeout,omitempty"`
	Organization                string                 `json:"organization,omitempty"`
	Tags                        map[string]interface{} `json:"tags,omitempty"`
	InternallyDisabled          bool                   `json:"integrated_disabled,omitempty"`
	Usage                       map[string]interface{} `json:"usage,omitempty"`
	Guardrails                  []interface{}          `json:"guardrails,omitempty"`
	ExtraBodyParameters         map[string]interface{} `json:"extra_body_parameters,omitempty"`
	// Nested sub-objects (mirrors the YAML/db layout for display)
	LiteLLMParams map[string]interface{} `json:"litellm_params,omitempty"`
	ModelInfo     map[string]interface{} `json:"model_info,omitempty"`
}

// NewLiteLLMService creates a new LiteLLMService.
func NewLiteLLMService(db database.DatabaseManager, cfg *config.Config, configSvc *ConfigService) *LiteLLMService {
	return &LiteLLMService{
		db:        db,
		cfg:       cfg,
		configSvc: configSvc,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// extractModelID extracts the primary UUID from a POST /model/new response.
// It tries the top-level model_id field first, then falls back to model_info.id.
func (s *LiteLLMService) extractModelID(body []byte) (primaryID, nestedID string, err error) {
	var resp LiteLLMCreateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}
	primaryID = resp.ModelID
	if primaryID == "" && resp.ModelInfo != nil {
		if raw, ok := resp.ModelInfo["id"]; ok {
			if s, ok := raw.(string); ok && s != "" {
				nestedID = s
			}
		}
	}
	return primaryID, nestedID, nil
}

// getAPIKey retrieves the LiteLLM API key, checking env/file first, then DB.
func (s *LiteLLMService) getAPIKey() (string, error) {
	if s.cfg.LiteLLMAPIKey != "" {
		return s.cfg.LiteLLMAPIKey, nil
	}
	if s.configSvc != nil {
		encrypted, err := s.configSvc.GetDecrypted("LITELLM_API_KEY")
		if err != nil {
			return "", fmt.Errorf("failed to get LiteLLM API key from database: %w", err)
		}
		if encrypted != "" {
			return encrypted, nil
		}
	}
	return "", fmt.Errorf("LiteLLM API key not configured")
}

// buildBaseURL constructs the base URL for the LiteLLM API.
func (s *LiteLLMService) buildBaseURL() (string, error) {
	if s.cfg.LiteLLMURL == "" {
		return "", fmt.Errorf("LiteLLM URL not configured")
	}
	base := s.cfg.LiteLLMURL
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base, nil
}

// doRequest executes an HTTP request to the LiteLLM API with authentication.
func (s *LiteLLMService) doRequest(method, path string, body interface{}) ([]byte, error) {
	baseURL, err := s.buildBaseURL()
	if err != nil {
		return nil, err
	}
	url := baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	apiKey, err := s.getAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// syncModelByName finds an existing deployment by name in the full model list
// and returns its UUID, or empty string if not found.
func (s *LiteLLMService) syncModelByName(name string) string {
	models, err := s.ListModels()
	if err != nil {
		return ""
	}
	for _, m := range models {
		if m.ID == name {
			return m.ID
		}
	}
	return ""
}

// getOrCreateDeployment checks whether a deployment with given model_name already
// exists in LiteLLM. If it does (exact ID match), it updates in-place and returns
// the existing UUID (created=false). Otherwise it creates fresh and returns the
// new UUID (created=true). Always persists all IDs returned by the server.
func (s *LiteLLMService) getOrCreateDeployment(name string, params, minfo map[string]interface{}) (uuid string, created bool) {
	existingUUID := s.syncModelByName(name)
	if existingUUID != "" {
		update := LiteLLMUpdate{
			ModelName:     &name,
			LiteLLMParams: &params,
			ModelInfo:     &minfo,
		}
		_, _ = s.doRequest("POST", "/model/update", update)
		return existingUUID, false
	}

	deployment := LiteLLMModel{
		ModelName:     name,
		LiteLLMParams: params,
		ModelInfo:     minfo,
	}
	body, err := s.doRequest("POST", "/model/new", deployment)
	if err != nil {
		return "", true // signal creation failure; caller will handle
	}

	id, _, _ := s.extractModelID(body)
	return id, true
}

// ListModels lists all models from LiteLLM (OpenAI-compatible format).
func (s *LiteLLMService) ListModels() ([]LiteLLMModelItem, error) {
	body, err := s.doRequest("GET", "/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	var resp LiteLLMListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return resp.Data, nil
}

// loadExistingModels scans GET /model/info — ONE call, returns ALL deploy
// records belonging to the given slug as map[displayName -> [internalRowUUID]].
// Base deployments match by model_name == slug.
// Alias deployments match by model_name in {active, active-thinking} AND
// litellm_params.model == slug.
// Variant deployments match by prefix slug- AND litellm_params.model == slug
// with suffix matching known variant names from the variants map.
//
// Unlike the old version that returned map[name]id (dropping duplicates),
// this returns map[name][]id so every duplicate is captured and can be cleaned.
func (s *LiteLLMService) loadExistingModels(slug string, variants interface{}) (map[string][]string, error) {
	body, err := s.doRequest("GET", "/model/info", nil)
	if err != nil {
		return nil, fmt.Errorf("stage 2 scan: %w", err)
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse model info list: %w", err)
	}

	// Build expected variant names for matching
	expectedVariantNames := make(map[string]bool)
	if variantMap, ok := variants.(map[string]interface{}); ok {
		nameIdx := buildVariantSuffixIndex(variantMap)
		for name, suffixes := range nameIdx {
			for _, sfx := range suffixes {
				expectedVariantNames[name+sfx] = true
			}
		}
	}

	// Collect ALL matching deployments, not just one per name
	uuids := make(map[string][]string)
	for _, raw := range resp.Data {
		var item struct {
			ModelName     string                 `json:"model_name"`
			Info          map[string]interface{} `json:"model_info"`
			LiteLLMParams map[string]interface{} `json:"litellm_params"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue // skip malformed entries
		}
		if item.Info == nil {
			continue
		}
		id, ok := item.Info["id"].(string)
		if !ok || id == "" {
			continue
		}

		// Match by deployment type
		if item.ModelName == slug {
			// Base deployment: exact model_name match
			uuids[item.ModelName] = append(uuids[item.ModelName], id)
		} else if item.ModelName == "active" || item.ModelName == "active-thinking" {
			// Alias: check that litellm_params.model points to this slug
			if item.LiteLLMParams != nil {
				if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
					uuids[item.ModelName] = append(uuids[item.ModelName], id)
				}
			}
		} else if item.ModelName == ragAliasReranker || item.ModelName == ragAliasEmbeddings {
			// RAG alias: match by model_name + api_base pointing to this model's port.
			// The alias name is shared ("reranker" / "embeddings"), so we must check
			// that the api_base matches this specific model's port to avoid
			// cross-model alias collisions.
			if item.LiteLLMParams != nil {
				if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
					// Also verify api_base matches — ensures we only delete
					// the alias that points to this model's container.
					if apiBase, ok := item.LiteLLMParams["api_base"].(string); ok && apiBase != "" {
						uuids[item.ModelName] = append(uuids[item.ModelName], id)
					}
				}
			}
		} else if strings.HasPrefix(item.ModelName, slug+"-") {
			// Variant: match by prefix + known suffix, or just by slug pointing
			suffix := strings.TrimPrefix(item.ModelName, slug)
			potentialName := slug + suffix
			if expectedVariantNames[potentialName] {
				if item.LiteLLMParams != nil {
					if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
						uuids[item.ModelName] = append(uuids[item.ModelName], id)
					}
				}
			} else if item.LiteLLMParams != nil {
				// Fallback: any deployment starting with slug+ that points here is ours
				if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
					uuids[item.ModelName] = append(uuids[item.ModelName], id)
				}
			}
		}
	}
	return uuids, nil
}

// variantNameEntry holds the computed suffix for a variant name.
type variantNameEntry struct {
	name   string
	suffix string
}

// buildVariantSuffixIndex builds a map of variant name(s) => their suffix(es)
// from a raw variants parameter map. This handles cases where spec values are
// map[string]interface{} (from JSON parsing) which cannot be cast directly to
