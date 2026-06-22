// Package service provides engine version/type management operations.
package service

import (
	"fmt"
	"strings"

	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

// EngineDefaults holds hardcoded default volumes and environment variables
// for a specific engine type.
type EngineDefaults struct {
	Volumes     map[string]string
	Environment map[string]string
}

// EngineImportOverrides holds CLI argument overrides for engine import.
type EngineImportOverrides struct {
	Overwrite bool // if true, update existing engine type + upsert versions instead of skip
}

// vllmDefaults are the hardcoded defaults for the vLLM engine type.
var vllmDefaults = EngineDefaults{
	Volumes: map[string]string{
		"../models":           "/root/.cache/huggingface",
		"../vllm-cache":       "/root/.cache/vllm",
		"../llm-templates":    "/app/templates",
		"../triton-cache":     "/root/.cache/triton",
		"../flashinfer-cache": "/root/.cache/flashinfer",
	},
	Environment: map[string]string{
		"HF_HUB_OFFLINE":              "0",
		"VLLM_LOAD_FORMAT_DEVICE":     "cuda",
		"VLLM_NVFP4_GEMM_BACKEND":     "marlin",
		"VLLM_USE_FLASHINFER_MOE_FP4": "0",
		"VLLM_TEST_FORCE_FP8_MARLIN":  "1",
	},
}

// GetEngineDefaults returns the hardcoded defaults for the given engine type.
// Currently only supports "vllm"; returns empty defaults for unknown types.
func GetEngineDefaults(engineType string) EngineDefaults {
	switch strings.ToLower(engineType) {
	case "vllm":
		return vllmDefaults
	default:
		return EngineDefaults{
			Volumes:     make(map[string]string),
			Environment: make(map[string]string),
		}
	}
}

// isValidProvider checks whether a provider string is one of the supported values.
func isValidProvider(p string) bool {
	return p == "vllm" || p == "sglang" || p == "llama.cpp" || p == "custom"
}

// EngineService handles business logic for engine types and versions.
type EngineService struct {
	db database.DatabaseManager
}

// NewEngineService creates a new EngineService.
func NewEngineService(db database.DatabaseManager) *EngineService {
	return &EngineService{db: db}
}

// =============================================================================
// EngineType CRUD Wrappers
// =============================================================================

// ListAllEngineTypes returns all engine types sorted by slug.
func (s *EngineService) ListAllEngineTypes() ([]models.EngineType, error) {
	return s.db.ListEngineTypes()
}

// GetEngineTypeBySlug returns an engine type by its slug.
func (s *EngineService) GetEngineTypeBySlug(slug string) (*models.EngineType, error) {
	return s.db.GetEngineTypeBySlug(slug)
}

// DeleteEngineType removes an engine type by slug.
func (s *EngineService) DeleteEngineType(slug string) error {
	return s.db.DeleteEngineType(slug)
}

// CreateEngineType creates a new engine type, validating the slug first.
func (s *EngineService) CreateEngineType(et *models.EngineType) error {
	if err := s.ValidateSlug(et.Slug); err != nil {
		return err
	}
	return s.db.CreateEngineType(et)
}

// CreateOrSkipEngineType creates an engine type if it doesn't already exist.
// Returns (created, error) where created=true means a new record was inserted.
func (s *EngineService) CreateOrSkipEngineType(et *models.EngineType) (bool, error) {
	exists, err := s.db.EngineTypeExists(et.Slug)
	if err != nil {
		return false, fmt.Errorf("check existence of engine type %s: %w", et.Slug, err)
	}
	if exists {
		return false, nil
	}
	if err := s.CreateEngineType(et); err != nil {
		return false, fmt.Errorf("create engine type %s: %w", et.Slug, err)
	}
	return true, nil
}

// UpdateEngineType updates an existing engine type by slug with the provided field updates.
func (s *EngineService) UpdateEngineType(slug string, updates map[string]interface{}) error {
	return s.db.UpdateEngineType(slug, updates)
}

// ListModelsByEngineVersion returns models linked to the given engine version slug.
func (s *EngineService) ListModelsByEngineVersion(engineVersionSlug string) ([]models.Model, error) {
	return s.db.ListModelsByEngineVersion(engineVersionSlug)
}

// =============================================================================
// EngineVersion CRUD Wrappers
// =============================================================================

// ListAllEngineVersions returns all engine versions sorted by created_at desc.
func (s *EngineService) ListAllEngineVersions() ([]models.EngineVersion, error) {
	return s.db.ListEngineVersions()
}

// ListEngineVersionsByType returns engine versions for a given type, sorted by created_at desc.
func (s *EngineService) ListEngineVersionsByType(typeSlug string) ([]models.EngineVersion, error) {
	all, err := s.db.ListEngineVersions()
	if err != nil {
		return nil, err
	}
	var filtered []models.EngineVersion
	for _, v := range all {
		if v.EngineTypeSlug == typeSlug {
			filtered = append(filtered, v)
		}
	}
	return filtered, nil
}

// GetEngineVersionByTypeAndSlug returns a specific engine version.
func (s *EngineService) GetEngineVersionByTypeAndSlug(typeSlug, slug string) (*models.EngineVersion, error) {
	return s.db.GetEngineVersionBySlugAndType(typeSlug, slug)
}

// DeleteEngineVersionByTypeAndSlug removes an engine version by type+slug.
func (s *EngineService) DeleteEngineVersionByTypeAndSlug(typeSlug, slug string) error {
	if _, err := s.GetEngineVersionByTypeAndSlug(typeSlug, slug); err != nil {
		return err
	}
	return s.db.DeleteEngineVersion(slug)
}

// CreateEngineVersion creates a new engine version, validating inputs first.
func (s *EngineService) CreateEngineVersion(ev *models.EngineVersion) error {
	if err := s.ValidateSlug(ev.Slug); err != nil {
		return err
	}
	if err := s.ValidateSlug(ev.EngineTypeSlug); err != nil {
		return err
	}
	if err := s.ValidateImage(ev.Image); err != nil {
		return err
	}
	return s.db.CreateEngineVersion(ev)
}

// CreateOrSkipEngineVersion creates an engine version if it doesn't already exist.
// Returns (created, error) where created=true means a new record was inserted.
func (s *EngineService) CreateOrSkipEngineVersion(ev *models.EngineVersion) (bool, error) {
	exists, err := s.db.EngineVersionExistsByTypeAndSlug(ev.EngineTypeSlug, ev.Slug)
	if err != nil {
		return false, fmt.Errorf("check existence of engine version %s/%s: %w", ev.EngineTypeSlug, ev.Slug, err)
	}
	if exists {
		return false, nil
	}
	if err := s.CreateEngineVersion(ev); err != nil {
		return false, fmt.Errorf("create engine version %s/%s: %w", ev.EngineTypeSlug, ev.Slug, err)
	}
	return true, nil
}

// UpdateEngineVersion updates an existing engine version by slug with the provided field updates.
func (s *EngineService) UpdateEngineVersion(slug string, updates map[string]interface{}) error {
	return s.db.UpdateEngineVersion(slug, updates)
}

// ShowComposition generates a docker-compose YAML for a model+engine version.
func (s *EngineService) ShowComposition(model *models.Model, ev *models.EngineVersion) (string, error) {
	if ev == nil {
		return "", fmt.Errorf("engine version is required for composition generation")
	}
	generator, err := NewComposeGenerator()
	if err != nil {
		return "", fmt.Errorf("failed to create compose generator: %w", err)
	}

	// Get provider from engine type
	et, etErr := s.GetEngineTypeBySlug(ev.EngineTypeSlug)
	if etErr != nil {
		return "", fmt.Errorf("get engine type for model %s: %w", model.Slug, etErr)
	}

	cfg := EngineComposeConfig{
		Image:       ev.Image,
		Entrypoint:  parseEntrypoint(ev.Entrypoint),
		Provider:    et.Provider,
		EnvVars:     ev.GetEnvironment(),
		CommandArgs: ev.GetCommandArgs(),
	}

	// Convert volumes to strings
	volumes := ev.GetVolumes()
	for host, cont := range volumes {
		cfg.Volumes = append(cfg.Volumes, host+":"+cont)
	}

	// Build logging section
	modelName := ""
	if model != nil {
		modelName = model.Name
	}
	cfg.LoggingSection = s.BuildLoggingSection(ev.EnableLogging, ev.SyslogAddress, ev.SyslogFacility, modelName)

	// Build deploy section
	cfg.DeploySection = s.BuildDeploySection(ev.DeployEnableNvidia, ev.DeployGPUCount)

	// Build healthcheck section (custom overrides auto-injected)
	cfg.HealthCheckSection = BuildHealthcheckSection(ev.HealthcheckJSON)

	// Pass model healthcheck JSON for model-level override in GenerateWithOptions
	cfg.ModelHealthcheckJSON = model.HealthcheckJSON

	// Build ulimits section
	cfg.UlimitsSection = BuildUlimitsSection(ev.UlimitsJSON)

	// Set IPC override (non-empty value replaces hardcoded "host")
	if ev.IPC != "" {
		cfg.IPCOverride = ev.IPC
	}

	return generator.Generate(model, cfg)
}
