package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

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
	specs, err := buildDeploymentSpecs(litellmParams, modelInfo, slug, dbModel.HasThinkingCapability(), litellmParams["variants"], dbModel.InputTokenCost, dbModel.OutputTokenCost, dbModel.CacheCreationInputTokenCost, dbModel.CacheReadInputTokenCost, dbModel.SubType, s.cfg.OpenAIAPIURL, dbModel.Port)
	if err != nil {
		return fmt.Errorf("stage 1 collect: %w", err)
	}

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
			ModelName     string                 `json:"model_name"`
			Info          map[string]interface{} `json:"model_info"`
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
				}
			}
		}
	}

	if deleted > 0 {
		fmt.Printf("Cleaned %d duplicate deployment(s) for %s\n", deleted, slug)
	}

	return lastErr
}
func (s *LiteLLMService) SyncModel(slug string) error {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model %s not found in database: %w", slug, err)
	}

	if dbModel.LitellmModelID == "" {
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
		s.db.UpdateModel(slug, map[string]interface{}{
			"litellm_model_id":       "",
			"litellm_active_aliases": "",
			"litellm_variant_ids":    "",
		})
		return s.AddModel(slug)
	}

	return fmt.Errorf("UpdateModel failed: %w", err)
}

// SyncAll syncs every LLM-type model in the database to LiteLLM.
// It iterates over all models, skips non-LLM types, and continues
// on individual errors (reporting a summary at the end).
func (s *LiteLLMService) SyncAll() error {
	allModels, err := s.db.ListModels()
	if err != nil {
		return fmt.Errorf("failed to list models for sync-all: %w", err)
	}

	llmModels := 0
	skipped := 0
	succeeded := 0
	failed := 0

	fmt.Printf("Syncing %d model(s) to LiteLLM...\n", len(allModels))
	fmt.Println(strings.Repeat("─", 60))

	for _, m := range allModels {
		if m.Type != "llm" && m.Type != "auto-complete" && m.Type != "rag" && !isSpeechType(m.SubType) {
			skipped++
			continue
		}
		llmModels++

		fmt.Printf("\n[%d/%d] %s (%s)... ", llmModels-skipped, llmModels, m.Slug, m.Name)
		if err := s.SyncModel(m.Slug); err != nil {
			fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
			failed++
		} else {
			fmt.Println("OK")
			succeeded++
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Sync complete: %d/%d succeeded, %d failed, %d skipped (non-LLM)\n",
		succeeded, llmModels, failed, skipped)
	if failed > 0 {
		return fmt.Errorf("sync-all: %d model(s) failed", failed)
	}
	return nil
}
