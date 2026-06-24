// Package service provides engine configuration builder utilities.
package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
)

// BuildComposeConfig resolves the engine version for a model and builds the
// full EngineComposeConfig with default volumes, env vars, logging, deploy,
// and command args from the engine version. Model-level env vars and command
// args override the engine version equivalents.
func (s *EngineService) BuildComposeConfig(model *models.Model) (*EngineComposeConfig, error) {
	ev, err := s.ResolveVersionForModel(*model)
	if err != nil {
		return nil, fmt.Errorf("resolve engine version for model %s: %w", model.Slug, err)
	}

	// Get provider from engine type
	et, err := s.GetEngineTypeBySlug(ev.EngineTypeSlug)
	if err != nil {
		return nil, fmt.Errorf("get engine type for model %s: %w", model.Slug, err)
	}

	cfg := &EngineComposeConfig{
		Image:       ev.Image,
		Entrypoint:  parseEntrypoint(ev.Entrypoint),
		Provider:    et.Provider,
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
	cfg.LoggingSection = s.BuildLoggingSection(ev.EnableLogging, ev.SyslogAddress, ev.SyslogFacility, model.Name)

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
//	logging:
//	  driver: "syslog"
//	  options:
//	    syslog-address: "{{.Address}}"
//	    syslog-facility: "{{.Facility}}"
//	    tag: "ai-server/model"
func (s *EngineService) BuildLoggingSection(enableLogging bool, address, facility, modelName string) string {
	if !enableLogging {
		return ""
	}
	return fmt.Sprintf(`    logging:
      driver: "syslog"
      options:
        syslog-address: "%s"
        syslog-facility: "%s"
        tag: "ai-server/%s"`, address, facility, modelName)
}

// =============================================================================
// Deploy Config Builder
// =============================================================================

// BuildDeploySection returns a YAML string block for the deploy/resources/reservations/devices section.
// If enableNvidia is false, returns empty string "".
// If enabled, returns:
//
//	deploy:
//	  resources:
//	    reservations:
//	      devices:
//	        - driver: nvidia
//	          count: "{{.Count}}"
//	          capabilities: [gpu]
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

	return fmt.Sprintf(`    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: "%s"
              capabilities: [gpu]`, count)
}

// =============================================================================
// Healthcheck Config Builder
// =============================================================================

// BuildHealthcheckSection renders a YAML healthcheck block from a JSON string.
// If jsonStr is empty, returns "".
// Otherwise parses the JSON into a map and renders each key-value as a
// indented YAML property under `healthcheck:` with 4-space indentation.
func BuildHealthcheckSection(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}

	var hc map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &hc); err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("    healthcheck:\n")
	for k, v := range hc {
		sb.WriteString(fmt.Sprintf("      %s: %s\n", k, formatHealthcheckValue(v)))
	}
	return sb.String()
}

// formatHealthcheckValue formats a single healthcheck value for YAML rendering.
func formatHealthcheckValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case float64:
		// JSON unmarshals all numbers as float64
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%.0f", val)
	case []interface{}:
		parts := make([]string, len(val))
		for i, item := range val {
			parts[i] = fmt.Sprintf("%q", item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// =============================================================================
// Ulimits Config Builder
// =============================================================================

// BuildUlimitsSection renders a YAML ulimits block from a JSON string.
// If jsonStr is empty, returns "".
// Otherwise parses the JSON into a map and renders each key-value as
// `key: value` under `ulimits:` with 4-space indentation.
func BuildUlimitsSection(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}

	var ul map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &ul); err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("    ulimits:\n")
	for k, v := range ul {
		sb.WriteString(fmt.Sprintf("      %s: %s\n", k, formatUlimitsValue(v)))
	}
	return sb.String()
}

// formatUlimitsValue formats a single ulimits value for YAML rendering.
func formatUlimitsValue(v interface{}) string {
	switch val := v.(type) {
	case float64:
		if val == -1 {
			return "-1"
		}
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%.0f", val)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", v)
	}
}
