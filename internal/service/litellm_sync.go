package service

import (
	"fmt"
	"os"
	"strings"
)

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
