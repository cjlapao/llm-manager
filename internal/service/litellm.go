// Package service provides LiteLLM proxy integration for model management.
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

const (
	activeAliasName     = "active"
	activeThinkingAlias = "active-thinking"
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

// DeepMerge deeply merges src into dst. Map values are merged recursively; leaf
// values from src overwrite the corresponding destination value. Returns dst.
func DeepMerge(dst, src interface{}) interface{} {
	dstCopy, _ := castToMap(copyInterfaceToMapStringInterface(dst))
	srcMap, ok := castToMap(src)
	if !ok {
		return src // at leaves, simply replace
	}
	for k, v := range srcMap {
		if existing, exists := dstCopy[k]; exists {
			dstCopy[k] = DeepMerge(existing, v)
		} else {
			dstCopy[k] = copyInterfaceToMapStringInterface(v)
		}
	}
	return dstCopy
}

// deepObjectMerge deep-merges src into dst in-place. Map values are merged recursively;
// leaf values from src overwrite the corresponding destination value.
func deepObjectMerge(dst, src map[string]interface{}) {
	for k, v := range src {
		if existing, ok := dst[k].(map[string]interface{}); ok {
			if nextSrc, ok := v.(map[string]interface{}); ok && nextSrc != nil {
				deepObjectMerge(existing, nextSrc)
			} else {
				dst[k] = copyInterfaceToMapStringInterface(v)
			}
		} else {
			dst[k] = copyInterfaceToMapStringInterface(v)
		}
	}
}

// castToMap attempts to convert val to map[string]interface{}.
// Returns the map and true if conversion succeeded, nil and false otherwise.
func castToMap(val interface{}) (map[string]interface{}, bool) {
	if m, ok := val.(map[string]interface{}); ok {
		return m, true
	}
	return nil, false
}

// copyInterfaceToMapStringInterface converts val to a deep-copied
// map[string]interface{}. Strings, numbers, booleans and slices pass through.
func copyInterfaceToMapStringInterface(val interface{}) interface{} {
	if sv, ok := val.(map[string]interface{}); ok {
		copied := make(map[string]interface{}, len(sv))
		for k, v := range sv {
			copied[k] = copyInterfaceToMapStringInterface(v)
		}
		return copied
	}
	return val
}

// stripMetadata removes internal-only keys from a variant spec entry before merging
// into LiteLLM deployment params. Keys like "suffix"/"prefix" are used only to derive
// the deployment name and must never reach LiteLLM's config.
func stripMetadata(src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch k {
		case "suffix", "prefix":
			continue // skip - these are only for naming
		default:
			out[k] = copyInterfaceToMapStringInterface(v)
		}
	}
	return out
}

// mapDiffers returns true if two string-to-string maps differ in length or any key-value pair.
func mapDiffers(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || v != bv {
			return true
		}
	}
	return false
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
// Alias deployments match by model_name in {active, active_thinking} AND
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
	totalChecked := 0
	for _, raw := range resp.Data {
		totalChecked++
		var item struct {
			ModelName    string                 `json:"model_name"`
			Info         map[string]interface{} `json:"model_info"`
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
			fmt.Printf("[SCAN] MATCH (base): %s (id=%s)\n", item.ModelName, id)
			uuids[item.ModelName] = append(uuids[item.ModelName], id)
		} else if item.ModelName == "active" || item.ModelName == "active_thinking" {
			// Alias: check that litellm_params.model points to this slug
			if item.LiteLLMParams != nil {
				if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
					fmt.Printf("[SCAN] MATCH (alias): %s (id=%s, params.model=%s)\n", item.ModelName, id, paramsModel)
					uuids[item.ModelName] = append(uuids[item.ModelName], id)
				}
			}
		} else if strings.HasPrefix(item.ModelName, slug+"-") {
			// Variant: match by prefix + known suffix, or just by slug pointing
			suffix := strings.TrimPrefix(item.ModelName, slug)
			potentialName := slug + suffix
			if expectedVariantNames[potentialName] {
				if item.LiteLLMParams != nil {
					if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
						fmt.Printf("[SCAN] MATCH (variant expected): %s (id=%s, params.model=%s)\n", item.ModelName, id, paramsModel)
						uuids[item.ModelName] = append(uuids[item.ModelName], id)
					}
				}
			} else if item.LiteLLMParams != nil {
				// Fallback: any deployment starting with slug+ that points here is ours
				if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
					fmt.Printf("[SCAN] MATCH (variant fallback): %s (id=%s, params.model=%s)\n", item.ModelName, id, paramsModel)
					uuids[item.ModelName] = append(uuids[item.ModelName], id)
				}
			}
		} else {
			// Debug: log non-matching items that have litellm_params.model == slug
			if item.LiteLLMParams != nil {
				if paramsModel, ok := item.LiteLLMParams["model"].(string); ok && paramsModel == slug {
					fmt.Printf("[SCAN] SKIP (no prefix): %s != %s and no prefix match (id=%s)\n", item.ModelName, slug, id)
				}
			}
		}
	}
	fmt.Printf("[SCAN] Checked %d items, matched %d groups\n", totalChecked, len(uuids))
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
// a Go slice — it walks the entry fields looking for arrays or iterable maps.
func buildVariantSuffixIndex(variants interface{}) map[string][]string {
	result := make(map[string][]string)
	vm, ok := variants.(map[string]interface{})
	if !ok {
		return result
	}
	for vname, spec := range vm {
		entry, _ := spec.(map[string]interface{})
		if entry == nil {
			continue
		}
		for _, key := range []string{"suffix", "prefix"} {
			raw := entry[key]
			// Raw YAML gives string (or float64); JSON parse gives float64;
			// expanded template values may have been stored as float64.
			switch v := raw.(type) {
			case string:
				result[vname] = append(result[vname], v)
			case float64:
				result[vname] = append(result[vname], fmt.Sprintf("%v", v))
			}
		}
	}
	return result
}

// DeploymentSpec represents one deployment we intend to have in LiteLLM.
type DeploymentSpec struct {
	Name      string
	Type      string // "base" | "alias" | "variant"
	Params    map[string]interface{}
	ModelInfo map[string]interface{}
}

// ReplicationStep records what happened during Stage 3 replicate-all.
type ReplicationStep struct {
	Spec   DeploymentSpec
	Action string // "deleted-and-recreated" | "fresh-create" | "pruned-only"
	UUID   string // returned from POST /model/new (recreated/fresh steps)
	OldID  string // internal UUID of deleted deployment (recreated/pruned steps)
}

// buildDeploymentSpecs constructs all deployment specs for a slug.
// Each spec is a complete LiteLLM deployment entry — variants are NOT
// nested under a "variants" key. Instead, the base model has all root
// properties (excluding "variants"), and each variant is a separate
// deployment with base properties overridden by variant-specific values.
func buildDeploymentSpecs(params, minfo map[string]interface{},
	slug string, hasThinking bool, variants interface{},
	inputCost, outputCost float64) ([]DeploymentSpec, error) {
	var specs []DeploymentSpec

	// Build base params: root properties only, excluding "variants"
	baseParams := make(map[string]interface{})
	for k, v := range params {
		if k == "variants" {
			continue // variants are internal to llm-manager, not sent to LiteLLM
		}
		baseParams[k] = copyInterfaceToMapStringInterface(v)
	}
	baseParams["model"] = slug
	if inputCost > 0 {
		baseParams["input_cost_per_token"] = inputCost
	}
	if outputCost > 0 {
		baseParams["output_cost_per_token"] = outputCost
	}

	specs = append(specs, DeploymentSpec{
		Name:      slug,
		Type:      "base",
		Params:    baseParams,
		ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
	})

	// Alias: active
	actParams := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
	specs = append(specs, DeploymentSpec{
		Name:      activeAliasName,
		Type:      "alias",
		Params:    actParams,
		ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
	})

	// Alias: active-thinking (if thinking cap present)
	if hasThinking {
		thinkParams := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
		specs = append(specs, DeploymentSpec{
			Name:      activeThinkingAlias,
			Type:      "alias",
			Params:    thinkParams,
			ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
		})
	}

	// Variants: each variant is a separate deployment with base params merged
	// with variant-specific overrides
	if variantMap, ok := params["variants"].(map[string]interface{}); ok {
		for _, specVal := range variantMap {
			entry, ok := specVal.(map[string]interface{})
			if !ok {
				continue
			}
			suffix, _ := entry["prefix"].(string)
			if suffix == "" {
				suffix, _ = entry["suffix"].(string)
			}
			if len(suffix) > 0 && suffix[0] != '-' {
				suffix = "-" + suffix
			}
			// Start with base params, merge variant overrides (strip metadata)
			merged := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
			deepObjectMerge(merged, stripMetadata(entry))

			specs = append(specs, DeploymentSpec{
				Name:      slug + suffix,
				Type:      "variant",
				Params:    merged,
				ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
			})
		}
	}

	return specs, nil
}

// deleteByUUID delegates a POST /model/delete call by internal row UUID.
func (s *LiteLLMService) deleteByUUID(uuid string) error {
	_, err := s.doRequest("POST", "/model/delete", map[string]interface{}{"id": uuid})
	return err
}

// replicateOne compares the spec against existing models, deletes ALL stale
// deployments (including duplicates), then creates a fresh one. Returns ReplicationStep.
func (s *LiteLLMService) replicateOne(spec DeploymentSpec, existing map[string][]string) (ReplicationStep, error) {
	step := ReplicationStep{Spec: spec}
	allIDs, exists := existing[spec.Name]

	if exists && len(allIDs) > 0 {
		// Delete ALL matching deployments (handles duplicates)
		for _, oldID := range allIDs {
			fmt.Printf("  🗑️ [DELETED] %-30s (%s)\n", spec.Name, oldID)
			if err := s.deleteByUUID(oldID); err != nil {
				if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "HTTP 400") {
					fmt.Fprintf(os.Stderr, "    ⚠ Old deployment already gone – proceeding with fresh create\n")
				} else {
					return step, fmt.Errorf("delete old %s (uuid=%s): %w", spec.Name, oldID, err)
				}
			} else if step.OldID == "" {
				step.OldID = oldID // keep first for reporting
			}
		}
	} else {
		fmt.Printf("  ✨ [FRESH]     %-30s\n", spec.Name)
	}

	deployBody := LiteLLMModel{
		ModelName:     spec.Name,
		LiteLLMParams: spec.Params,
		ModelInfo:     spec.ModelInfo,
	}

	// Debug: log the body being sent to LiteLLM
	deployJSON, _ := json.MarshalIndent(deployBody, "", "  ")
	fmt.Printf("  [DEBUG] Sending to LiteLLM for %s:\n%s\n", spec.Name, string(deployJSON))

	resp, err := s.doRequest("POST", "/model/new", deployBody)
	if err != nil {
		action := "fresh-create"
		if exists && len(allIDs) > 0 {
			action = "deleted-and-recreated"
		}
		return ReplicationStep{
			Spec:   spec,
			Action: action,
			OldID:  step.OldID,
		}, fmt.Errorf("create %s: %w", spec.Name, err)
	}

	newID, _, _ := s.extractModelID(resp)
	if exists && len(allIDs) > 0 {
		step.Action = "deleted-and-recreated"
	} else {
		step.Action = "fresh-create"
	}
	step.UUID = newID
	fmt.Printf("  ➜ [CREATED]   %-30s (%s)\n", spec.Name, newID[:10]+"...")

	return step, nil
}

// pruneStale removes deployments that exist in LiteLLM but no longer match
// any spec in our desired set. Best-effort: logs failures but does not abort.
func (s *LiteLLMService) pruneStale(desiredNames []string, existing map[string][]string) {
	want := make(map[string]bool, len(desiredNames))
	for _, n := range desiredNames {
		want[n] = true
	}
	foundExtra := false
	for name, ids := range existing {
		if !want[name] {
			for _, uid := range ids {
				fmt.Printf("  🧹 [PRUNE]     %-30s (%s)\n", name, uid[:10]+"...")
				if err := s.deleteByUUID(uid); err != nil {
					fmt.Fprintf(os.Stderr, "    ⚠ Failed to prune %-30s: %v\n", name, err)
				}
				foundExtra = true
			}
		}
	}
	if !foundExtra {
		fmt.Println("  ✅ No stale deployments to prune.")
	}
}

// updateDBAndPrintSummary persists replication IDs to SQLite and prints a
// formatted ASCII summary table.
func (s *LiteLLMService) updateDBAndPrintSummary(slug string, existingCount int, steps []ReplicationStep) error {
	typeIDs := make(map[string]struct {
		Name string
		ID   string
	}) // alias -> aliasID ; variant -> variantID

	typeCounts := map[string]int{"base": 0, "alias": 0, "variant": 0}

	for _, st := range steps {
		switch st.Spec.Type {
		case "base":
			if st.UUID != "" {
				typeIDs["base"] = struct{ Name, ID string }{st.Spec.Name, st.UUID}
			}
		case "alias":
			typeIDs[st.Spec.Name] = struct{ Name, ID string }{st.Spec.Name, st.UUID}
		case "variant":
			// Extract variant name from slug (everything after first "-")
			idx := strings.LastIndex(st.Spec.Name, "-")
			vName := st.Spec.Name[idx+1:]
			typeIDs[vName] = struct{ Name, ID string }{vName, st.UUID}
		}
		typeCounts[st.Spec.Type]++
	}

	// Build DB payload
	updates := make(map[string]interface{})
	if b, ok := typeIDs["base"]; ok && b.ID != "" {
		updates["litellm_model_id"] = b.ID
	}

	// Collect aliases
	aliasList := []struct{ Name, ID string }{}
	variantList := []struct{ Name, ID string }{}
	for _, st := range steps {
		if st.Spec.Type == "alias" {
			aliasList = append(aliasList, struct{ Name, ID string }{st.Spec.Name, st.UUID})
		}
		if st.Spec.Type == "variant" {
			idx := strings.LastIndex(st.Spec.Name, "-")
			vName := st.Spec.Name[idx+1:]
			variantList = append(variantList, struct{ Name, ID string }{vName, st.UUID})
		}
	}

	if len(aliasList) > 0 {
		am := make(map[string]string)
		for _, a := range aliasList {
			am[a.Name] = a.ID
		}
		b, _ := json.Marshal(am)
		updates["litellm_active_aliases"] = string(b)
	}
	if len(variantList) > 0 {
		vm := make(map[string]string)
		for _, v := range variantList {
			vm[v.Name] = v.ID
		}
		b, _ := json.Marshal(vm)
		updates["litellm_variant_ids"] = string(b)
	}

	if len(updates) == 0 {
		fmt.Println("⚠ DB update skipped – no successful creations.")
		return nil
	}

	if err := s.db.UpdateModel(slug, updates); err != nil {
		return fmt.Errorf("db update for %s: %w", slug, err)
	}

	// ─── Print Summary ───
	fmt.Println()
	totalCreated := 0
	totalRecreated := 0
	totalPruned := 0
	for _, st := range steps {
		switch st.Action {
		case "dedup-and-recreated":
			totalRecreated++
		case "fresh-create":
			totalCreated++
		case "prune-only":
			totalPruned++
		}
	}

	sep := strings.Repeat("─", 62)
	fmt.Println(sep)
	fmt.Printf("  🎯 Synced: %-30s (%d deployments)\n", slug, len(steps))
	fmt.Println(sep)
	fmt.Printf("  📊 Collected: %d base(s), %d alias(es), %d variant(s)\n", typeCounts["base"], typeCounts["alias"], typeCounts["variant"])
	fmt.Printf("  📡 Scanned : %d existing LiteLLM deployment(s)\n", existingCount+totalPruned)
	fmt.Println(sep)
	fmt.Printf("  💾 DB updated ✓\n")
	fmt.Println(sep)
	fmt.Println()

	return nil
}

// GetModelInfo retrieves detailed information for a model using its slug name.
func (s *LiteLLMService) GetModelInfo(slug string) (*LiteLLMModelInfo, error) {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve slug %s: %w", slug, err)
	}
	litellmID := dbModel.LitellmModelID
	if litellmID == "" {
		return nil, fmt.Errorf("cannot query %s — no litellm_model_id; run 'litellm add %s' first", slug, slug)
	}
	apiInfo, err := s.GetModelInfoByUUID(litellmID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	if dbModel.LiteLLMParams != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(dbModel.LiteLLMParams), &params); err == nil {
			apiInfo.LiteLLMParams = params
		}
	}
	if dbModel.ModelInfo != "" {
		var minfo map[string]interface{}
		if err := json.Unmarshal([]byte(dbModel.ModelInfo), &minfo); err == nil {
			apiInfo.ModelInfo = minfo
		}
	}
	return apiInfo, nil
}

// GetModelInfoByUUID retrieves detailed information for a specific model using its
// LiteLLM proxy internal row UUID.
func (s *LiteLLMService) GetModelInfoByUUID(litellmID string) (*LiteLLMModelInfo, error) {
	path := fmt.Sprintf("/model/info?litellm_model_id=%s", litellmID)
	body, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	var result struct {
		Data []LiteLLMModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no model found for litellm id %s", litellmID)
	}
	info := result.Data[0]
	return &info, nil
}

// AddModel adds a model to LiteLLM using a four-stage pipeline:
// 1. COLLECT — parse database record into in-memory deployment specifications
// 2. SCAN    — one GET /model/info call, resolve all internal IDs
// 3. REPLICATE — compare/spec-deployments, delete stale, create fresh
// 4. UPDATE  — persist IDs to SQLite and print a summary table
func (s *LiteLLMService) AddModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in dB: %w", slug, err)
	}

	litellmParams := make(map[string]interface{})
	if dbModel.LiteLLMParams != "" {
		if err := json.Unmarshal([]byte(dbModel.LiteLLMParams), &litellmParams); err != nil {
			return fmt.Errorf("parse litellm_params: %w", err)
		}
	}
	modelInfo := make(map[string]interface{})
	if dbModel.ModelInfo != "" {
		if err := json.Unmarshal([]byte(dbModel.ModelInfo), &modelInfo); err != nil {
			return fmt.Errorf("parse model_info: %w", err)
		}
	}

	// ┌─────────────────────────────────────────┐
	// │ STAGE 1: COLLECT — build specs purely   │
	// │             from in-memory state        │
	// └─────────────────────────────────────────┘
	specs, err := buildDeploymentSpecs(litellmParams, modelInfo, slug, dbModel.HasThinkingCapability(), litellmParams["variants"], dbModel.InputTokenCost, dbModel.OutputTokenCost)
	if err != nil {
		return fmt.Errorf("stage 1 collect: %w", err)
	}

	names := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
	}

	fmt.Printf("[STAGE 1] Collected %d deployment(s):\n", len(specs))
	for _, sp := range specs {
		fmt.Printf("  📋 %-30s (%s)\n", sp.Name, sp.Type)
	}
	fmt.Println()

	// ┌─────────────────────────────────────────┐
	// │ STAGE 2: SCAN — single API call         │
	// │         gets ALL model ids incl         │
	// │         internal row uuids              │
	// └─────────────────────────────────────────┘
	existing, err := s.loadExistingModels(slug, litellmParams["variants"])
	if err != nil {
		return fmt.Errorf("stage 2 scan: %w", err)
	}

	fmt.Printf("[STAGE 2] Scanned LiteLLM – %d existing deployment(s)\n", len(existing))
	fmt.Println()

	// ┌─────────────────────────────────────────┐
	// │ STAGE 3: REPLICATE — compare then       │
	// │         delete+duplicate for each      │
	// │         spec                            │
	// └─────────────────────────────────────────┘
	steps := make([]ReplicationStep, 0, len(specs))
	for _, spec := range specs {
		step, err := s.replicateOne(spec, existing)
		if err != nil {
			return fmt.Errorf("replicate %s: %w", spec.Name, err)
		}
		steps = append(steps, step)
	}

	fmt.Println()

	// ┌─────────────────────────────────────────┐
	// │ STAGE 4: UPDATE — persist IDs + print   │
	// │               nice summary              │
	// └─────────────────────────────────────────┘
	if err := s.updateDBAndPrintSummary(slug, len(existing), steps); err != nil {
		return fmt.Errorf("stage 4 summary: %w", err)
	}

	return nil
}

// UpdateModel works the same 4-stage pipeline as AddModel.
// It re-reads specs from YAML/db, resolves existing IDs,
// replicates every deployment, prunes extras, updates DB, prints summary.
func (s *LiteLLMService) UpdateModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in database: %w", slug, err)
	}
	if dbModel.LitellmModelID == "" {
		return fmt.Errorf("cannot update %s in LiteLLM — run 'litellm add %s' first", slug, slug)
	}

	litellmParams := make(map[string]interface{})
	if dbModel.LiteLLMParams != "" {
		if err := json.Unmarshal([]byte(dbModel.LiteLLMParams), &litellmParams); err != nil {
			return fmt.Errorf("parse litellm_params: %w", err)
		}
	}
	modelInfo := make(map[string]interface{})
	if dbModel.ModelInfo != "" {
		if err := json.Unmarshal([]byte(dbModel.ModelInfo), &modelInfo); err != nil {
			return fmt.Errorf("parse model_info: %w", err)
		}
	}

	// ┌─────────────────────────────────────────┐
	// │ STAGE 1: COLLECT                        │
	// └─────────────────────────────────────────┘
	specs, err := buildDeploymentSpecs(litellmParams, modelInfo, slug, dbModel.HasThinkingCapability(), litellmParams["variants"], dbModel.InputTokenCost, dbModel.OutputTokenCost)
	if err != nil {
		return fmt.Errorf("stage 1 collect: %w", err)
	}

	names := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
	}

	fmt.Printf("[STAGE 1] Updated %d deployment(s):\n", len(specs))
	for _, sp := range specs {
		fmt.Printf("  📋 %-30s (%s)\n", sp.Name, sp.Type)
	}
	fmt.Println()

	// ┌─────────────────────────────────────────┐
	// │ STAGE 2: SCAN                           │
	// └─────────────────────────────────────────┘
	existing, err := s.loadExistingModels(slug, litellmParams["variants"])
	if err != nil {
		return fmt.Errorf("stage 2 scan: %w", err)
	}

	fmt.Printf("[SYNC] Stage 2 scan: found %d deployment groups\n", len(existing))
	for name, ids := range existing {
		fmt.Printf("[SYNC]   %s: %d deployment(s) [", name, len(ids))
		for i, id := range ids {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(id[:10])
		}
		fmt.Println("]")
	}
	fmt.Println()

	// ┌─────────────────────────────────────────┐
	// │ STAGE 3: REPLICATE                      │
	// └─────────────────────────────────────────┘
	steps := make([]ReplicationStep, 0, len(specs))
	for _, spec := range specs {
		step, err := s.replicateOne(spec, existing)
		if err != nil {
			return fmt.Errorf("replicate %s: %w", spec.Name, err)
		}
		steps = append(steps, step)
	}

	fmt.Println()

	// ┌─────────────────────────────────────────┐
	// │ STAGE 4: UPDATE                         │
	// └─────────────────────────────────────────┘
	if err := s.updateDBAndPrintSummary(slug, len(existing), steps); err != nil {
		return fmt.Errorf("stage 4 summary: %w", err)
	}

	return nil
}

// DeleteModel removes a model from LiteLLM along with all aliases and variants.
// It scans LiteLLM for actual deployments belonging to this slug (rather than
// relying solely on stored IDs), deletes them, and only then clears DB state.
func (s *LiteLLMService) DeleteModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in database: %w", slug, err)
	}

	// Collect stored IDs for reporting
	storedAliases := dbModel.GetLitellmActiveAliases()
	storedVariants := dbModel.GetLitellmVariantIDs()
	storedBaseID := dbModel.LitellmModelID

	// Scan LiteLLM for actual deployments belonging to this slug.
	// This handles stale/incorrect IDs in the DB from manual edits or partial syncs.
	// Pass nil for variants — loadExistingModels falls back to checking litellm_params.model == slug.
	found, err := s.loadExistingModels(slug, nil)
	if err != nil {
		// If we can't scan LiteLLM, fall back to stored IDs (best-effort)
		fmt.Fprintf(os.Stderr, "Warning: could not scan LiteLLM for deployments: %v — using stored IDs\n", err)
		found = make(map[string][]string)
		if storedBaseID != "" {
			found[slug] = []string{storedBaseID}
		}
		for name, id := range storedAliases {
			found[name] = []string{id}
		}
		for name, id := range storedVariants {
			found[name] = []string{id}
		}
	}

	if len(found) == 0 {
		// Nothing found in LiteLLM — just clear DB state
		if err := s.db.UpdateModel(slug, map[string]interface{}{
			"litellm_model_id":       "",
			"litellm_active_aliases": "",
			"litellm_variant_ids":    "",
		}); err != nil {
			return fmt.Errorf("cleared litellm ids for %s: %w", slug, err)
		}
		fmt.Printf("ℹ No LiteLLM deployments found for %s — DB state cleared\n", slug)
		return nil
	}

	// Delete all found deployments (iterate over all IDs per name)
	var lastErr error
	totalDeleted := 0
	deployedNames := make([]string, 0, len(found))
	for name, ids := range found {
		for _, id := range ids {
			fmt.Printf("  Deleting deployment: %s (id=%s)\n", name, id)
			if _, dErr := s.doRequest("POST", "/model/delete", map[string]interface{}{"id": id}); dErr != nil {
				lastErr = fmt.Errorf("%w; failed to delete \"%s\" (id=%s): %w", lastErr, name, id, dErr)
			} else {
				totalDeleted++
			}
		}
		if totalDeleted > 0 && len(deployedNames) > 0 && deployedNames[len(deployedNames)-1] != name {
			deployedNames = append(deployedNames, name)
		}
	}

	// Only clear DB state AFTER attempting all deletions
	if err := s.db.UpdateModel(slug, map[string]interface{}{
		"litellm_model_id":       "",
		"litellm_active_aliases": "",
		"litellm_variant_ids":    "",
	}); err != nil {
		if lastErr == nil {
			lastErr = fmt.Errorf("cleared litellm ids for %s: %w", slug, err)
		}
	}

	if lastErr != nil {
		fmt.Fprintf(os.Stderr, "✗ DeleteModel %s: %v\n", slug, lastErr)
		return lastErr
	}

	// Friendly summary
	fmt.Printf("✓ Deleted %s from LiteLLM (%d deployment(s))\n", slug, totalDeleted)
	for _, name := range deployedNames {
		fmt.Printf("  ✓ %s\n", name)
	}

	return nil
}

// CleanDuplicates scans LiteLLM for all deployments where litellm_params.model
// matches the given slug and deletes them. This is used during install with
// --clean to remove stale/duplicate deployments that may have accumulated from
// previous partial syncs, manual edits, or failed operations.
func (s *LiteLLMService) CleanDuplicates(slug string) error {
	fmt.Printf("[CLEAN] Scanning LiteLLM for deployments matching litellm_params.model=%s\n", slug)

	body, err := s.doRequest("GET", "/model/info", nil)
	if err != nil {
		return fmt.Errorf("clean duplicates: failed to list models: %w", err)
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("clean duplicates: parse model info list: %w", err)
	}

	deleted := 0
	var lastErr error

	for _, raw := range resp.Data {
		var item struct {
			ModelName    string                 `json:"model_name"`
			Info         map[string]interface{} `json:"model_info"`
			LiteLLMParams map[string]interface{} `json:"litellm_params"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.Info == nil {
			continue
		}
		id, ok := item.Info["id"].(string)
		if !ok || id == "" {
			continue
		}

		// Check if this deployment belongs to the slug by matching litellm_params.model
		if item.LiteLLMParams != nil {
			paramsModel, ok := item.LiteLLMParams["model"].(string)
			if ok && paramsModel == slug {
				fmt.Printf("  [CLEAN] MATCH: %s (id=%s, params.model=%s)\n", item.ModelName, id, paramsModel)
				if _, dErr := s.doRequest("POST", "/model/delete", map[string]interface{}{"id": id}); dErr != nil {
					lastErr = fmt.Errorf("%w; failed to delete duplicate \"%s\" (id=%s): %w", lastErr, item.ModelName, id, dErr)
				} else {
					deleted++
					fmt.Printf("  [CLEAN] Deleted %s (id=%s)\n", item.ModelName, id)
				}
			}
		}
	}

	if deleted > 0 {
		fmt.Printf("[CLEAN] Cleaned %d duplicate deployment(s) for %s\n", deleted, slug)
	} else {
		fmt.Printf("[CLEAN] No duplicate deployments found for %s (deleted=%d)\n", slug, deleted)
	}

	// Verify: re-scan to confirm nothing remains
	fmt.Printf("[CLEAN] Post-cleanup verification scan for slug=%s\n", slug)
	verifyBody, err := s.doRequest("GET", "/model/info", nil)
	if err != nil {
		fmt.Printf("[CLEAN] Warning: could not verify cleanup: %v\n", err)
	} else {
		var verifyResp struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(verifyBody, &verifyResp); err == nil {
			remaining := 0
			for _, raw := range verifyResp.Data {
				var item struct {
					ModelName    string                 `json:"model_name"`
					Info         map[string]interface{} `json:"model_info"`
					LiteLLMParams map[string]interface{} `json:"litellm_params"`
				}
				if err := json.Unmarshal(raw, &item); err != nil {
					continue
				}
				if item.LiteLLMParams != nil {
					if pm, ok := item.LiteLLMParams["model"].(string); ok && pm == slug {
						remaining++
						if id2, _ := item.Info["id"].(string); id2 != "" {
							fmt.Printf("  [CLEAN] REMAINING after cleanup: %s (id=%s)\n", item.ModelName, id2)
						}
					}
				}
			}
			fmt.Printf("[CLEAN] Post-cleanup: %d deployment(s) still matching litellm_params.model=%s\n", remaining, slug)
		}
	}

	return lastErr
}
func (s *LiteLLMService) SyncModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in database: %w", slug, err)
	}

	fmt.Printf("[SYNC] LitellmModelID='%s' (empty=%v)\n", dbModel.LitellmModelID, dbModel.LitellmModelID == "")
	fmt.Printf("[SYNC] LitellmActiveAliases='%s'\n", dbModel.LitellmActiveAliases)
	fmt.Printf("[SYNC] LitellmVariantIDs='%s'\n", dbModel.LitellmVariantIDs)

	if dbModel.LitellmModelID == "" {
		fmt.Printf("[SYNC] No stored IDs — calling AddModel\n")
		return s.AddModel(slug)
	}

	fmt.Printf("[SYNC] Has stored IDs — calling UpdateModel\n")
	err = s.UpdateModel(slug)
	if err == nil {
		fmt.Printf("[SYNC] UpdateModel succeeded\n")
		fmt.Printf("Model %s updated in LiteLLM (base=%s)\n", slug, dbModel.LitellmModelID)
		return nil
	}

	errMsg := err.Error()
	isRemoteGone := strings.Contains(errMsg, "model not found") ||
		strings.Contains(errMsg, "not found")

	if isRemoteGone {
		fmt.Println("[SYNC] Remote model missing — performing fresh sync")
		s.db.UpdateModel(slug, map[string]interface{}{
			"litellm_model_id":       "",
			"litellm_active_aliases": "",
			"litellm_variant_ids":    "",
		})
		return s.AddModel(slug)
	}

	return fmt.Errorf("UpdateModel failed: %w", err)
}
