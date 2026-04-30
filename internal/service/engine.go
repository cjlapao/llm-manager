package service

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"gopkg.in/yaml.v3"
)

// EngineDefaults holds hardcoded default volumes and environment variables
// for a specific engine type.
type EngineDefaults struct {
	Volumes     map[string]string
	Environment map[string]string
}

// vllmDefaults are the hardcoded defaults for the vLLM engine type.
var vllmDefaults = EngineDefaults{
	Volumes: map[string]string{
		"../models":          "/root/.cache/huggingface",
		"../vllm-cache":      "/root/.cache/vllm",
		"../llm-templates":   "/app/templates",
		"../triton-cache":    "/root/.cache/triton",
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

// ShowComposition generates a docker-compose YAML for a model+engine version.
func (s *EngineService) ShowComposition(model *models.Model, ev *models.EngineVersion) (string, error) {
	generator, err := NewComposeGenerator()
	if err != nil {
		return "", fmt.Errorf("failed to create compose generator: %w", err)
	}

	cfg := EngineComposeConfig{
		Image:       ev.Image,
		Entrypoint:  parseEntrypoint(ev.Entrypoint),
		EnvVars:     ev.GetEnvironment(),
		CommandArgs: ev.GetCommandArgs(),
	}

	// Convert volumes to strings
	volumes := ev.GetVolumes()
	for host, cont := range volumes {
		cfg.Volumes = append(cfg.Volumes, host+":"+cont)
	}

	// Build logging section
	cfg.LoggingSection = s.BuildLoggingSection(ev.EnableLogging, ev.SyslogAddress, ev.SyslogFacility)

	// Build deploy section
	cfg.DeploySection = s.BuildDeploySection(ev.DeployEnableNvidia, ev.DeployGPUCount)

	return generator.Generate(model, cfg)
}

// BuildComposeConfig resolves the engine version for a model and builds the
// full EngineComposeConfig with default volumes, env vars, logging, deploy,
// and command args from the engine version. Model-level env vars and command
// args override the engine version equivalents.
func (s *EngineService) BuildComposeConfig(model *models.Model) (*EngineComposeConfig, error) {
	ev, err := s.ResolveVersionForModel(*model)
	if err != nil {
		return nil, fmt.Errorf("resolve engine version for model %s: %w", model.Slug, err)
	}

	cfg := &EngineComposeConfig{
		Image:       ev.Image,
		Entrypoint:  parseEntrypoint(ev.Entrypoint),
		EnvVars:     ev.GetEnvironment(),
		CommandArgs: ev.GetCommandArgs(),
	}

	// Convert version volumes to strings
	volumes := ev.GetVolumes()
	for host, cont := range volumes {
		cfg.Volumes = append(cfg.Volumes, host+":"+cont)
	}

	// Merge model-level env vars on top of engine version env vars
	if model.EnvVars != "" {
		modelEnv := parseJSONToMap(model.EnvVars)
		for k, v := range modelEnv {
			cfg.EnvVars[k] = v
		}
	}

	// Merge model-level command args on top of engine version command args
	if model.CommandArgs != "" {
		modelCmd := parseJSONToArray(model.CommandArgs)
		if len(modelCmd) > 0 {
			cfg.CommandArgs = modelCmd
		}
	}

	// Build logging section
	cfg.LoggingSection = s.BuildLoggingSection(ev.EnableLogging, ev.SyslogAddress, ev.SyslogFacility)

	// Build deploy section
	cfg.DeploySection = s.BuildDeploySection(ev.DeployEnableNvidia, ev.DeployGPUCount)

	return cfg, nil
}

// parseEntrypoint parses an entrypoint string into a slice of strings.
// Handles both space-separated and JSON array formats.
func parseEntrypoint(raw string) []string {
	if raw == "" {
		return nil
	}
	// Try JSON array first
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return arr
	}
	// Fall back to space-separated
	return strings.Fields(raw)
}

// =============================================================================
// Validation Functions
// =============================================================================

// slugRegex validates slugs: lowercase letters, numbers, hyphens, 1-128 chars.
var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{0,127}$`)

// ValidateSlug validates that a slug is a valid identifier (lowercase letters,
// numbers, hyphens, no spaces, 1-128 chars, must start with alphanumeric).
func (s *EngineService) ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}
	if len(slug) > 128 {
		return fmt.Errorf("slug must be at most 128 characters (got %d)", len(slug))
	}
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens (got %q)", slug)
	}
	return nil
}

// ValidateImage validates a Docker image string.
// Must be non-empty, contain at least one "/" or ":".
func (s *EngineService) ValidateImage(image string) error {
	if image == "" {
		return fmt.Errorf("image cannot be empty")
	}
	if !strings.Contains(image, "/") && !strings.Contains(image, ":") {
		return fmt.Errorf("image must be a valid Docker image reference (e.g. registry/image:tag or image:tag)")
	}
	return nil
}

// ValidateEnvKey validates an environment variable key.
// Must be alphanumeric, underscores, hyphens, 1-128 chars.
func (s *EngineService) ValidateEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("env key cannot be empty")
	}
	if len(key) > 128 {
		return fmt.Errorf("env key must be at most 128 characters (got %d)", len(key))
	}
	envKeyRegex := regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)
	if !envKeyRegex.MatchString(key) {
		return fmt.Errorf("env key must contain only alphanumeric characters, underscores, and hyphens (got %q)", key)
	}
	return nil
}

// ParseVolumeMapping parses a volume mapping string in the format:
// host_path:container_path[:ro|rw]
// Returns (hostPath, containerPath, readonly, error).
func (s *EngineService) ParseVolumeMapping(input string) (hostPath, containerPath string, readonly bool, err error) {
	if input == "" {
		return "", "", false, fmt.Errorf("volume mapping cannot be empty")
	}

	parts := strings.Split(input, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", false, fmt.Errorf("invalid volume format %q: expected host:container[:ro|rw]", input)
	}

	hostPath = parts[0]
	containerPath = parts[1]

	if hostPath == "" || containerPath == "" {
		return "", "", false, fmt.Errorf("volume mapping host and container paths cannot be empty")
	}

	if len(parts) == 3 {
		mode := strings.ToLower(parts[2])
		if mode == "ro" {
			readonly = true
		} else if mode == "rw" {
			readonly = false
		} else {
			return "", "", false, fmt.Errorf("invalid volume mode %q: must be ro or rw", mode)
		}
	}

	return hostPath, containerPath, readonly, nil
}

// ParseEnvKV parses an environment variable string in the format KEY=VALUE.
// Returns (key, value, error).
func (s *EngineService) ParseEnvKV(input string) (key, value string, err error) {
	if input == "" {
		return "", "", fmt.Errorf("env kv cannot be empty")
	}

	idx := strings.Index(input, "=")
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid env format %q: expected KEY=VALUE", input)
	}

	key = input[:idx]
	value = input[idx+1:]

	if err := s.ValidateEnvKey(key); err != nil {
		return "", "", err
	}

	return key, value, nil
}

// =============================================================================
// Merge/Pipeline Functions
// =============================================================================

// MergeEnvironments merges multiple environment maps with priority:
// versionEnv → modelEnv → cliOverrides (last wins on conflict).
// Returns a new map; never returns nil if called (always returns an empty map for zero inputs).
func MergeEnvironments(versionEnv, modelEnv, cliOverrides map[string]string) map[string]string {
	merged := make(map[string]string)

	// Start with version env
	for k, v := range versionEnv {
		merged[k] = v
	}
	// Overlay model env
	for k, v := range modelEnv {
		merged[k] = v
	}
	// Overlay CLI overrides (highest priority)
	for k, v := range cliOverrides {
		merged[k] = v
	}

	return merged
}

// MergeVolumes merges multiple volume maps with priority:
// defaultVolumes → versionVolumes → userVolumes (last wins on conflict, dedup by host path).
// Returns a new map; never returns nil if called (always returns an empty map for zero inputs).
func MergeVolumes(defaultVolumes, versionVolumes, userVolumes map[string]string) map[string]string {
	merged := make(map[string]string)

	// Start with hardcoded defaults
	for k, v := range defaultVolumes {
		merged[k] = v
	}
	// Overlay version volumes
	for k, v := range versionVolumes {
		merged[k] = v
	}
	// Overlay user volumes (highest priority)
	for k, v := range userVolumes {
		merged[k] = v
	}

	return merged
}

// ResolveDefaultVersion finds the default version for the given engine type.
// Returns nil, nil if no default exists (not an error — just no default set yet).
func (s *EngineService) ResolveDefaultVersion(engineTypeSlug string) (*models.EngineVersion, error) {
	return s.db.FindDefaultVersionByType(engineTypeSlug)
}

// ResolveLatestVersion finds the latest version for the given engine type.
// Returns error if not found.
func (s *EngineService) ResolveLatestVersion(engineTypeSlug string) (*models.EngineVersion, error) {
	ev, err := s.db.FindLatestVersionByType(engineTypeSlug)
	if err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, fmt.Errorf("no latest version found for engine type %s", engineTypeSlug)
	}
	return ev, nil
}

// ResolveVersionForModel resolves the effective EngineVersion for a model.
// If the model has an explicit engine_version_slug, finds that specific version.
// Otherwise, resolves to the default version for the model's engine type.
// Returns error if no version can be resolved.
func (s *EngineService) ResolveVersionForModel(model models.Model) (*models.EngineVersion, error) {
	if model.EngineVersionSlug != "" {
		// Explicit version specified — find by type+slug
		ev, err := s.db.GetEngineVersionBySlugAndType(model.EngineType, model.EngineVersionSlug)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve engine version %s for model %s: %w", model.EngineVersionSlug, model.Slug, err)
		}
		return ev, nil
	}

	// No explicit version — resolve to default for engine type
	ev, err := s.db.FindDefaultVersionByType(model.EngineType)
	if err != nil {
		return nil, fmt.Errorf("failed to find default version for engine type %s: %w", model.EngineType, err)
	}
	if ev == nil {
		return nil, fmt.Errorf("engine type %s has no default version (model %s has no explicit engine_version_slug)", model.EngineType, model.Slug)
	}
	return ev, nil
}

// SetAsDefault sets the given version as the default for its engine type.
// First clears is_default for ALL versions of the type, then sets the target.
func (s *EngineService) SetAsDefault(engineTypeSlug, versionSlug string) error {
	if err := s.db.ClearIsDefaultForType(engineTypeSlug); err != nil {
		return fmt.Errorf("failed to clear existing defaults: %w", err)
	}
	if err := s.db.UpdateEngineVersion(versionSlug, map[string]interface{}{"is_default": true}); err != nil {
		return fmt.Errorf("failed to set %s as default: %w", versionSlug, err)
	}
	return nil
}

// =============================================================================
// Logging Config Builder
// =============================================================================

// BuildLoggingSection returns a YAML string block for the logging configuration.
// If enableLogging is false, returns empty string "".
// If enabled, returns:
//
//		logging:
//		  driver: "syslog"
//		  options:
//		    syslog-address: "{{.Address}}"
//		    syslog-facility: "{{.Facility}}"
//		    tag: "ai-server/{{.Name}}"
func (s *EngineService) BuildLoggingSection(enableLogging bool, address, facility string) string {
	if !enableLogging {
		return ""
	}
	return fmt.Sprintf(`  logging:
    driver: "syslog"
    options:
      syslog-address: "%s"
      syslog-facility: "%s"
      tag: "ai-server/{{.Name}}"`, address, facility)
}

// =============================================================================
// Deploy Config Builder
// =============================================================================

// BuildDeploySection returns a YAML string block for the deploy/resources/reservations/devices section.
// If enableNvidia is false, returns empty string "".
// If enabled, returns:
//
//		deploy:
//		  resources:
//		    reservations:
//		      devices:
//		        - driver: nvidia
//		          count: "{{.Count}}"
//		          capabilities: [gpu]
//
// If gpuCount is empty, uses "all" as the count value.
func (s *EngineService) BuildDeploySection(enableNvidia bool, gpuCount string) string {
	if !enableNvidia {
		return ""
	}

	count := gpuCount
	if count == "" {
		count = "all"
	}

	return fmt.Sprintf(`  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: "%s"
            capabilities: [gpu]`, count)
}

// =============================================================================
// Engine YAML Import
// =============================================================================

// yamlFile represents the top-level structure of an engine import YAML file.
type yamlFile struct {
	Engine   yamlEngine   `yaml:"engine"`
	Versions []yamlVersion `yaml:"versions"`
}

// yamlEngine represents an engine type definition.
type yamlEngine struct {
	Slug        string `yaml:"slug"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// yamlVersion represents an engine version definition.
type yamlVersion struct {
	Slug          string              `yaml:"slug"`
	Version       string              `yaml:"version"`
	ContainerName string              `yaml:"container_name"`
	Image         string              `yaml:"image"`
	Entrypoint    []string            `yaml:"entrypoint"`
	Default       bool                `yaml:"default"`
	Latest        bool                `yaml:"latest"`
	Volumes       map[string]string   `yaml:"volumes"`
	Environment   map[string]string   `yaml:"environment"`
	Logging       yamlLogging         `yaml:"logging"`
	Nvidia        bool                `yaml:"nvidia"`
	GPUCount      string              `yaml:"gpu_count"`
	CommandArgs   []string            `yaml:"command_args"`
	Port          int                 `yaml:"port"`
}

// yamlLogging represents logging configuration.
type yamlLogging struct {
	Enable   bool   `yaml:"enable"`
	Address  string `yaml:"address"`
	Facility string `yaml:"facility"`
}

// ImportEngineFile parses an engine YAML file and creates engine type + versions.
// Returns (created, skipped, error) where created=inserted count, skipped=duplicate count.
func (s *EngineService) ImportEngineFile(yamlPath string) (created, skipped int, err error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return 0, 0, fmt.Errorf("read file: %w", err)
	}

	var yf yamlFile
	if err := yaml.Unmarshal(data, &yf); err != nil {
		return 0, 0, fmt.Errorf("parse YAML: %w", err)
	}

	if yf.Engine.Slug == "" {
		return 0, 0, fmt.Errorf("engine slug is required")
	}

	// Create or skip engine type
	et := &models.EngineType{
		Slug:        yf.Engine.Slug,
		Name:        yf.Engine.Name,
		Description: yf.Engine.Description,
	}
	_, err = s.CreateOrSkipEngineType(et)
	if err != nil {
		return 0, 0, fmt.Errorf("engine type %s: %w", yf.Engine.Slug, err)
	}

	// Process versions
	firstDefault := true
	for _, v := range yf.Versions {
		if v.Slug == "" {
			skipped++
			continue
		}

		// Parse entrypoint
		ep := ""
		if len(v.Entrypoint) > 0 {
			ep = strings.Join(v.Entrypoint, " ")
		}

		// Build env JSON
		envJSON := ""
		if len(v.Environment) > 0 {
			b, _ := json.Marshal(v.Environment)
			envJSON = string(b)
		}

		// Build volumes JSON
		volJSON := ""
		if len(v.Volumes) > 0 {
			b, _ := json.Marshal(v.Volumes)
			volJSON = string(b)
		}

		// Build command args JSON
		cmdJSON := ""
		if len(v.CommandArgs) > 0 {
			b, _ := json.Marshal(v.CommandArgs)
			cmdJSON = string(b)
		}

		isDefault := v.Default && firstDefault
		if isDefault {
			firstDefault = false
		}
		isLatest := v.Latest

		ev := &models.EngineVersion{
			Slug:               v.Slug,
			EngineTypeSlug:     yf.Engine.Slug,
			Version:            v.Version,
			ContainerName:      v.ContainerName,
			Image:              v.Image,
			Entrypoint:         ep,
			IsDefault:          isDefault,
			IsLatest:           isLatest,
			EnvironmentJSON:    envJSON,
			VolumesJSON:        volJSON,
			EnableLogging:      v.Logging.Enable,
			SyslogAddress:      v.Logging.Address,
			SyslogFacility:     v.Logging.Facility,
			DeployEnableNvidia: v.Nvidia,
			DeployGPUCount:     v.GPUCount,
			CommandArgs:        cmdJSON,
		}

		created2, err := s.CreateOrSkipEngineVersion(ev)
		if err != nil {
			return created, skipped, fmt.Errorf("engine version %s/%s: %w", yf.Engine.Slug, v.Slug, err)
		}
		if created2 {
			created++
		} else {
			skipped++
		}
	}

	return created, skipped, nil
}

// IsEngineYAML checks whether the given YAML data represents an engine configuration.
// Returns true if the top-level "engine" key with a "slug" sub-key exists.
func IsEngineYAML(data []byte) bool {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}
	engine, ok := raw["engine"].(map[string]interface{})
	if !ok {
		return false
	}
	_, hasSlug := engine["slug"]
	return hasSlug
}

// ParseQuickYAML unmarshals YAML data into the target interface.
// Used for quick top-level key inspection without full struct binding.
func ParseQuickYAML(data []byte, target interface{}) error {
	return yaml.Unmarshal(data, target)
}
