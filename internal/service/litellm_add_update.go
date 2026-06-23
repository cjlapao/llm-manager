package service

import (
	"encoding/json"
	"fmt"
)

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
