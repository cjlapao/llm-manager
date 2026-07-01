package service

import (
	"fmt"
	"os"
	"strings"
)

// ensureCustomLLMProvider ensures custom_llm_provider is set in params,
// defaulting to "hosted_vllm" if not already present.
func ensureCustomLLMProvider(params map[string]interface{}) {
	if _, has := params["custom_llm_provider"]; !has {
		params["custom_llm_provider"] = "hosted_vllm"
	}
}

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

// setThinkingConfig configures the thinking-related keys inside
// litellm_params.extra_body.chat_template_kwargs for a deployment spec.
// It creates the nested maps if they don't exist, and always overwrites
// enable_thinking and preserve_thinking with the given values.
// This ensures the active alias has thinking disabled and the
// active-thinking alias has thinking enabled.
func setThinkingConfig(params map[string]interface{}, enableThinking, preserveThinking bool) {
	// Ensure extra_body exists
	if params["extra_body"] == nil {
		params["extra_body"] = make(map[string]interface{})
	}
	extraBody, ok := params["extra_body"].(map[string]interface{})
	if !ok {
		// extra_body is not a map — replace it
		extraBody = make(map[string]interface{})
		params["extra_body"] = extraBody
	}

	// Ensure chat_template_kwargs exists
	if extraBody["chat_template_kwargs"] == nil {
		extraBody["chat_template_kwargs"] = make(map[string]interface{})
	}
	kt, ok := extraBody["chat_template_kwargs"].(map[string]interface{})
	if !ok {
		// chat_template_kwargs is not a map — replace it
		kt = make(map[string]interface{})
		extraBody["chat_template_kwargs"] = kt
	}

	kt["enable_thinking"] = enableThinking
	kt["preserve_thinking"] = preserveThinking
}

// buildDeploymentSpecs constructs all deployment specs for a slug.
// Each spec is a complete LiteLLM deployment entry — variants are NOT
// nested under a "variants" key. Instead, the base model has all root
// properties (excluding "variants"), and each variant is a separate
// deployment with base properties overridden by variant-specific values.
func buildDeploymentSpecs(params, minfo map[string]interface{},
	slug string, hasThinking bool, variants interface{},
	inputCost, outputCost, cacheCreationCost, cacheReadCost float64, subType string, openAIAURL string, port int) ([]DeploymentSpec, error) {
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
	ensureCustomLLMProvider(baseParams)
	if inputCost > 0 {
		baseParams["input_cost_per_token"] = inputCost
	}
	if outputCost > 0 {
		baseParams["output_cost_per_token"] = outputCost
	}
	if cacheCreationCost > 0 {
		baseParams["cache_creation_input_token_cost"] = cacheCreationCost
	}
	if cacheReadCost > 0 {
		baseParams["cache_read_input_token_cost"] = cacheReadCost
	}



	// Base deployment: only for non-RAG models
	specs = append(specs, DeploymentSpec{
		Name:      slug,
		Type:      "base",
		Params:    baseParams,
		ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
	})

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

// buildActiveSpecs returns only the "active" and "active-thinking" alias specs
// for a given model. This is used by ActivateModel to create a single pair of
// aliases pointing to the currently-started model.
func buildActiveSpecs(slug string, params, minfo map[string]interface{}, hasThinking bool) []DeploymentSpec {
	// Build base params (same as buildDeploymentSpecs, minus variants)
	baseParams := make(map[string]interface{})
	for k, v := range params {
		if k == "variants" {
			continue
		}
		baseParams[k] = copyInterfaceToMapStringInterface(v)
	}
	baseParams["model"] = slug
	ensureCustomLLMProvider(baseParams)

	var specs []DeploymentSpec

	// Alias: active — always created with thinking disabled
	actParams := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
	setThinkingConfig(actParams, false, false)
	specs = append(specs, DeploymentSpec{
		Name:      activeAliasName,
		Type:      "alias",
		Params:    actParams,
		ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
	})

	// Alias: active-thinking — only for thinking-capable models, with thinking enabled
	if hasThinking {
		thinkParams := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
		setThinkingConfig(thinkParams, true, true)
		specs = append(specs, DeploymentSpec{
			Name:      activeThinkingAlias,
			Type:      "alias",
			Params:    thinkParams,
			ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
		})
	}

	return specs
}

// buildSpeechRAGSpecs creates both the base deployment and alias specs for a
// speech/RAG model. These are created on 'speech start' or 'rag start' so only
// running models have deployments in LiteLLM.
// Returns two specs:
//   - base: the model slug (e.g., qwen3-asr-1.7b) with api_base, model, custom_llm_provider
//   - alias: active-stt/tts/omni/reranker/embeddings pointing to the base
func buildSpeechRAGSpecs(slug string, params, minfo map[string]interface{}, subType string, openAIAURL string, port int) []DeploymentSpec {
	if !isSpeechType(subType) && subType != "reranker" && subType != "embedding" {
		return nil
	}

	// Build base params: copy from input, exclude variants, add endpoint info
	baseParams := make(map[string]interface{})
	for k, v := range params {
		if k == "variants" {
			continue
		}
		baseParams[k] = copyInterfaceToMapStringInterface(v)
	}
	baseParams["model"] = slug
	ensureCustomLLMProvider(baseParams)

	if port > 0 && openAIAURL != "" {
		base := strings.TrimRight(openAIAURL, "/")
		baseParams["api_base"] = fmt.Sprintf("%s:%d/v1", base, port)
	} else {
		fmt.Fprintf(os.Stderr, "  [WARN] Speech/RAG base api_base NOT SET: port=%d openAIAURL=%q\n", port, openAIAURL)
	}

	// Build alias params: same base, but model points to slug
	aliasParams := copyInterfaceToMapStringInterface(baseParams).(map[string]interface{})
	aliasParams["model"] = slug

	// Determine alias name
	var aliasName string
	switch strings.ToLower(subType) {
	case "reranker":
		aliasName = ragAliasReranker
	case "embedding":
		aliasName = ragAliasEmbeddings
	case "stt":
		aliasName = speechAliasSTT
	case "tts":
		aliasName = speechAliasTTS
	case "omni":
		aliasName = speechAliasOmni
	default:
		aliasName = fmt.Sprintf("active-%s", subType)
	}

	return []DeploymentSpec{
		// Base deployment
		{
			Name:      slug,
			Type:      "base",
			Params:    baseParams,
			ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
		},
		// Alias deployment
		{
			Name:      aliasName,
			Type:      "alias",
			Params:    aliasParams,
			ModelInfo: copyInterfaceToMapStringInterface(minfo).(map[string]interface{}),
		},
	}
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
