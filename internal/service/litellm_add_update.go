package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// filterOutActiveAliases removes "active", "active-thinking", and all
// speech/RAG alias specs from the deployment list. These are managed
// separately by ActivateModel/ActivateSpeechRAGModel on llm start,
// not during import or sync-all.
func filterOutActiveAliases(specs []DeploymentSpec) []DeploymentSpec {
	filtered := make([]DeploymentSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == activeAliasName || spec.Name == activeThinkingAlias {
			continue
		}
		if spec.Name == speechAliasSTT || spec.Name == speechAliasTTS ||
			spec.Name == speechAliasOmni || spec.Name == ragAliasReranker ||
			spec.Name == ragAliasEmbeddings {
			continue
		}
		filtered = append(filtered, spec)
	}
	return filtered
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
	specs, err := buildDeploymentSpecs(litellmParams, modelInfo, slug, dbModel.HasThinkingCapability(), litellmParams["variants"], dbModel.InputTokenCost, dbModel.OutputTokenCost, dbModel.CacheCreationInputTokenCost, dbModel.CacheReadInputTokenCost, dbModel.SubType, s.cfg.OpenAIAPIURL, dbModel.Port)
	if err != nil {
		return fmt.Errorf("stage 1 collect: %w", err)
	}

	// Filter out active/active-thinking aliases — those are managed by ActivateModel
	// on llm start, not during import/sync.
	specs = filterOutActiveAliases(specs)

	names := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
	}

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

// ActivateModel creates only the "active" and "active-thinking" alias deployments
// for the given model. It first deactivates all existing active aliases (from any
// previously-started model), then creates fresh aliases pointing to the requested
// model. This ensures only one model owns the active aliases at a time.
func (s *LiteLLMService) ActivateModel(slug string) error {
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

	fmt.Println()
	fmt.Println(strings.Repeat("═", 56))
	fmt.Printf("  Activating model: %s\n", slug)
	fmt.Println(strings.Repeat("═", 56))

	// Step 1: Deactivate all existing active aliases (from any model)
	if err := s.DeactivateAll(); err != nil {
		return fmt.Errorf("deactivate all: %w", err)
	}

	// Step 2: Build only the active alias specs
	specs := buildActiveSpecs(slug, litellmParams, modelInfo, dbModel.HasThinkingCapability())

	if len(specs) == 0 {
		fmt.Println("No active aliases to create for this model.")
		return nil
	}

	// Step 3: Scan existing active aliases (should be empty after deactivation, but handle stale)
	existing, err := s.loadExistingModels(slug, litellmParams["variants"])
	if err != nil {
		return fmt.Errorf("stage 2 scan: %w", err)
	}

	// Step 4: Replicate each alias spec
	typeIDs := make(map[string]struct {
		Name string
		ID   string
	})
	for _, spec := range specs {
		step, err := s.replicateOne(spec, existing)
		if err != nil {
			return fmt.Errorf("replicate %s: %w", spec.Name, err)
		}
		if step.UUID != "" {
			typeIDs[spec.Name] = struct{ Name, ID string }{spec.Name, step.UUID}
		}
	}

	// Step 5: Persist alias IDs to DB
	if len(typeIDs) > 0 {
		am := make(map[string]string)
		for name, entry := range typeIDs {
			am[name] = entry.ID
		}
		b, _ := json.Marshal(am)
		if err := s.db.UpdateModel(slug, map[string]interface{}{
			"litellm_active_aliases": string(b),
		}); err != nil {
			return fmt.Errorf("db update for %s: %w", slug, err)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("─", 56))
	fmt.Printf("  ✅ Activated: %-30s\n", slug)
	fmt.Printf("  Aliases created: %d\n", len(typeIDs))
	for name, entry := range typeIDs {
		fmt.Printf("    - %-15s → %s\n", name, entry.ID[:10]+"...")
	}
	fmt.Println(strings.Repeat("─", 56))
	fmt.Println()

	return nil
}

// ActivateSpeechRAGModel creates the speech/RAG alias deployment for the given
// model. It first deactivates all existing speech/RAG aliases (from any model),
// then creates a fresh alias pointing to the requested model. This ensures only
// running models have speech/RAG aliases at any time.
func (s *LiteLLMService) ActivateSpeechRAGModel(slug string, subType string) error {
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

	fmt.Println()
	fmt.Println(strings.Repeat("═", 56))
	fmt.Printf("  Activating %s alias: %s\n", subType, slug)
	fmt.Println(strings.Repeat("═", 56))

	// Step 1: Determine which alias name(s) to create
	var aliasNames []string
	switch subType {
	case "stt":
		aliasNames = []string{speechAliasSTT}
	case "tts":
		aliasNames = []string{speechAliasTTS}
	case "omni":
		aliasNames = []string{speechAliasOmni}
	case "reranker":
		aliasNames = []string{ragAliasReranker}
	case "embedding":
		aliasNames = []string{ragAliasEmbeddings}
	default:
		aliasNames = []string{fmt.Sprintf("active-%s", subType)}
	}

	// Step 2: Deactivate existing deployments for each alias name
	for _, name := range aliasNames {
		if err := s.DeactivateAliases(name); err != nil {
			return fmt.Errorf("deactivate %s: %w", name, err)
		}
	}

	// Step 3: Build the speech/RAG alias spec
	specs := buildSpeechRAGSpecs(slug, litellmParams, modelInfo, subType, s.cfg.OpenAIAPIURL, dbModel.Port)

	if len(specs) == 0 {
		fmt.Println("No speech/RAG aliases to create for this model.")
		return nil
	}

	// Step 4: Scan existing (should be empty after deactivation, but handle stale)
	existing, err := s.loadExistingModels(slug, litellmParams["variants"])
	if err != nil {
		return fmt.Errorf("stage 2 scan: %w", err)
	}

	// Step 4: Replicate the alias spec
	typeIDs := make(map[string]struct {
		Name string
		ID   string
	})
	for _, spec := range specs {
		step, err := s.replicateOne(spec, existing)
		if err != nil {
			return fmt.Errorf("replicate %s: %w", spec.Name, err)
		}
		if step.UUID != "" {
			typeIDs[spec.Name] = struct{ Name, ID string }{spec.Name, step.UUID}
		}
	}

	// Step 5: Persist alias IDs to DB
	if len(typeIDs) > 0 {
		am := make(map[string]string)
		for name, entry := range typeIDs {
			am[name] = entry.ID
		}
		b, _ := json.Marshal(am)
		if err := s.db.UpdateModel(slug, map[string]interface{}{
			"litellm_active_aliases": string(b),
		}); err != nil {
			return fmt.Errorf("db update for %s: %w", slug, err)
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("─", 56))
	fmt.Printf("  ✅ Activated: %-30s\n", slug)
	fmt.Printf("  Aliases created: %d\n", len(typeIDs))
	for name, entry := range typeIDs {
		fmt.Printf("    - %-15s → %s\n", name, entry.ID[:10]+"...")
	}
	fmt.Println(strings.Repeat("─", 56))
	fmt.Println()

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
	specs, err := buildDeploymentSpecs(litellmParams, modelInfo, slug, dbModel.HasThinkingCapability(), litellmParams["variants"], dbModel.InputTokenCost, dbModel.OutputTokenCost, dbModel.CacheCreationInputTokenCost, dbModel.CacheReadInputTokenCost, dbModel.SubType, s.cfg.OpenAIAPIURL, dbModel.Port)
	if err != nil {
		return fmt.Errorf("stage 1 collect: %w", err)
	}

	// Filter out active/active-thinking aliases — those are managed by ActivateModel
	// on llm start, not during update.
	specs = filterOutActiveAliases(specs)

	names := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
	}

	// ┌─────────────────────────────────────────┐
	// │ STAGE 2: SCAN                           │
	// └─────────────────────────────────────────┘
	existing, err := s.loadExistingModels(slug, litellmParams["variants"])
	if err != nil {
		return fmt.Errorf("stage 2 scan: %w", err)
	}

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
