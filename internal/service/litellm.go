// Package service provides LiteLLM proxy integration for model management.
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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
// records from LiteLLM as map[displayName -> internalRowUUID]. If any entry
// lacks an ID or other required field it is silently skipped.
func (s *LiteLLMService) loadExistingModels() (map[string]string, error) {
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

	uuids := make(map[string]string)
	for _, raw := range resp.Data {
		var item struct {
			ModelName string                 `json:"model_name"`
			Info      map[string]interface{} `json:"model_info"`
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
		uuids[item.ModelName] = id
	}
	return uuids, nil
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
func buildDeploymentSpecs(params, minfo map[string]interface{},
	slug string, hasThinking bool, variants interface{},
	inputCost, outputCost float64) ([]DeploymentSpec, error) {
	var specs []DeploymentSpec

	// Base
	baseParams := copyInterfaceToMapStringInterface(params).(map[string]interface{})
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

	// Variants
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
			merged := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
			DeepMerge(merged, entry)

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

// replicateOne compares the spec against existing models, deletes the stale
// deployment if present, then creates a fresh one. Returns ReplicationStep.
func (s *LiteLLMService) replicateOne(spec DeploymentSpec, existing map[string]string) (ReplicationStep, error) {
	step := ReplicationStep{Spec: spec}
	oldID, exists := existing[spec.Name]

	if exists {
		fmt.Printf("  🗑️ [DELETED] %-30s (%s)\n", spec.Name, oldID)
		if err := s.deleteByUUID(oldID); err != nil {
			return step, fmt.Errorf("delete old %s (uuid=%s): %w", spec.Name, oldID, err)
		}
		step.OldID = oldID
	} else {
		fmt.Printf("  ✨ [FRESH]     %-30s\n", spec.Name)
	}

	deployBody := LiteLLMModel{
		ModelName:     spec.Name,
		LiteLLMParams: spec.Params,
		ModelInfo:     spec.ModelInfo,
	}
	resp, err := s.doRequest("POST", "/model/new", deployBody)
	if err != nil {
		action := "fresh-create"
		if exists {
			action = "deleted-and-recreated"
		}
		return ReplicationStep{
			Spec:   spec,
			Action: action,
			OldID:  oldID,
		}, fmt.Errorf("create %s: %w", spec.Name, err)
	}

	newID, _, _ := s.extractModelID(resp)
	if exists {
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
func (s *LiteLLMService) pruneStale(desiredNames []string, existing map[string]string) {
	want := make(map[string]bool, len(desiredNames))
	for _, n := range desiredNames {
		want[n] = true
	}
	foundExtra := false
	for name, uid := range existing {
		if !want[name] {
			fmt.Printf("  🧹 [PRUNE]     %-30s (%s)\n", name, uid[:10]+"...")
			if err := s.deleteByUUID(uid); err != nil {
				fmt.Fprintf(os.Stderr, "    ⚠ Failed to prune %-30s: %v\n", name, err)
			}
			foundExtra = true
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
	existing, err := s.loadExistingModels()
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
	existing, err := s.loadExistingModels()
	if err != nil {
		return fmt.Errorf("stage 2 scan: %w", err)
	}

	fmt.Printf("[STAGE 2] Scanned LiteLLM – %d existing deployment(s)\n", len(existing))
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
func (s *LiteLLMService) DeleteModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in database: %w", slug, err)
	}

	// Collect all IDs before deleting anything
	aliases := dbModel.GetLitellmActiveAliases()
	variants := dbModel.GetLitellmVariantIDs()
	baseID := dbModel.LitellmModelID
	var lastErr error

	// Best-effort: delete aliases first
	for aliasName, aliasID := range aliases {
		if _, dErr := s.doRequest("POST", "/model/delete", map[string]interface{}{"id": aliasID}); dErr != nil {
			lastErr = fmt.Errorf("%w; failed to delete \"%s\" alias (litellm_id=%s): %w", lastErr, aliasName, aliasID, dErr)
		}
	}

	// Then delete variants
	for variantName, variantID := range variants {
		if _, dErr := s.doRequest("POST", "/model/delete", map[string]interface{}{"id": variantID}); dErr != nil {
			lastErr = fmt.Errorf("%w; failed to delete \"%s\" variant (litellm_id=%s): %w", lastErr, variantName, variantID, dErr)
		}
	}

	// Finally delete base model
	if baseID != "" {
		if _, err := s.doRequest("POST", "/model/delete", map[string]interface{}{"id": baseID}); err != nil {
			lastErr = fmt.Errorf("%w; DeleteModel base for %s (litellm_id=%s): %w", lastErr, slug, baseID, err)
		}
	}

	// Clear ALL DB state
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
		return lastErr
	}

	// Friendly output
	msg := fmt.Sprintf("✓ Deleted %s from LiteLLM", slug)
	count := 0
	if baseID != "" {
		msg += fmt.Sprintf(" (base=%s)", baseID)
		count++
	}
	if aliases != nil && len(aliases) > 0 {
		count += len(aliases)
	}
	if variants != nil && len(variants) > 0 {
		count += len(variants)
	}
	if count > 0 {
		msg = msg + ", " + strconv.Itoa(count) + " deployment(s) removed"
	}
	fmt.Println(msg)

	return nil
}

// SyncModel adds or updates a model in LiteLLM plus its aliases and variants.
func (s *LiteLLMService) SyncModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in database: %w", slug, err)
	}

	if dbModel.LitellmModelID == "" {
		fmt.Printf("Model %s has never been synced — creating fresh\n", slug)
		return s.AddModel(slug)
	}

	err = s.UpdateModel(slug)
	if err == nil {
		fmt.Printf("Model %s updated in LiteLLM (base=%s)\n", slug, dbModel.LitellmModelID)
		return nil
	}

	errMsg := err.Error()
	isRemoteGone := strings.Contains(errMsg, "model not found") ||
		strings.Contains(errMsg, "not found")

	if isRemoteGone {
		fmt.Println("Remote model missing in LiteLLM — performing fresh sync")
		s.db.UpdateModel(slug, map[string]interface{}{
			"litellm_model_id":       "",
			"litellm_active_aliases": "",
			"litellm_variant_ids":    "",
		})
		return s.AddModel(slug)
	}

	return fmt.Errorf("UpdateModel failed: %w", err)
}
