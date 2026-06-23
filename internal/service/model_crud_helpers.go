// === extracted from internal/service/model.go ===

package service

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/pkg/yamlparser"
)

type LiteLLMModeler interface {
	DeleteModel(slug string) error
	SyncModel(slug string) error
	SyncAll() error
}

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
				fmt.Printf("ℹ Port %d already in use\n", requestedPort)
				fmt.Printf("→ Using free port %d instead.\n", candidate)
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
	InputCost     *float64
	OutputCost    *float64
	Capabilities  []string
	Type          string // llm, rag, speech, comfyui (defaults to "llm")
	Engine        string // vllm, sglang, llama.cpp (defaults to "vllm")
	EngineVersion string // engine version slug (optional)
	Override      bool   // if true, delete existing DB record + LiteLLM deployments, then re-import from YAML
}

// validateEngineAndVersion checks that model's Engine and EngineVersion fields match
// entries registered in the database. Returns a slice of error strings (empty means valid).
// EngineType falls back to built-in defaults; engine version is validated only when
// the parent engine type already exists in the DB (avoids import-order issues).
func (s *ModelService) validateEngineAndVersion(y *yamlparser.ModelYAML) []error {
	var errs []error

	if s.db == nil {
		return errs // No DB available — skip validation entirely (backward compat).
	}

	engineTypes, err := s.db.ListEngineTypes()
	if err != nil || y.Engine == "" {
		return errs
	}

	found := false
	for _, et := range engineTypes {
		if et.Slug == y.Engine {
			found = true
			break
		}
	}
	if !found {
		slugList := make([]string, len(engineTypes))
		for i, et := range engineTypes {
			slugList[i] = et.Slug
		}
		errs = append(errs, fmt.Errorf("engine %q not found in known engines: %v", y.Engine, slugList))
	}

	// Validate engine version only if it's provided AND the parent engine type
	// is already in the DB (import-order-safe).
	if y.EngineVersion != "" && foundInDB(y.Engine, engineTypes) {
		allVersions, _ := s.db.ListEngineVersions()
		if len(allVersions) > 0 {
			versionFound := false
			for _, ev := range allVersions {
				if ev.Slug == y.EngineVersion {
					versionFound = true
					break
				}
			}
			if !versionFound {
				verList := make([]string, len(allVersions))
				for i, ev := range allVersions {
					verList[i] = ev.Slug
				}
				errs = append(errs, fmt.Errorf("engine_version %q not found in known versions: %v", y.EngineVersion, verList))
			}
		}
	}

	return errs
}

// foundInDB checks if a given slug is present in a list of EngineType records.
func foundInDB(slug string, types []models.EngineType) bool {
	for _, t := range types {
		if t.Slug == slug {
			return true
		}
	}
	return false
}

// configValues builds a flat map of uppercase ENV keys -> values, used to resolve
// ${{ .config.XXX }} references in model YAML during import. Keys come from both
// environment variables and the loaded config struct; env vars always win.
func (s *ModelService) configValues() map[string]string {
	cfg := s.cfg
	v := make(map[string]string, 12)
	// Config-derived values -- stored at the top level so they're easily discoverable.
	if cfg.OpenAIAPIURL != "" {
		v["OPENAI_API_URL"] = cfg.OpenAIAPIURL
	}
	if cfg.LiteLLMURL != "" {
		v["LITELLM_URL"] = cfg.LiteLLMURL
	}
	if cfg.HfToken != "" {
		v["HF_TOKEN"] = cfg.HfToken
	}
	// Environment overrides -- always checked last so they take priority over config file values.
	for _, k := range []string{"OPENAI_API_URL", "LITELLM_URL", "HF_TOKEN",
		"LITELLM_API_KEY", "VLLM_HOST", "DOCKER_HOST"} {
		if val := os.Getenv(k); val != "" {
			v[k] = val
		}
	}
	return v
}

// ImportModel imports a model from a YAML file into the database.
// It parses the YAML, validates it, checks for duplicates, maps to models.Model,
// applies CLI overrides, and creates the model record.
// LiteLLM params are auto-merged: api_base is constructed from config URL + model port,
// and model name is derived from the slug.
// model_info is auto-generated from capabilities.
