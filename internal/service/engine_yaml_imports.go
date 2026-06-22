// Package service provides YAML engine imports and quick parsing utilities.
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/user/llm-manager/internal/database/models"
)

// =============================================================================
// Engine YAML Import
// =============================================================================

// yamlFile represents the top-level structure of an engine import YAML file.
type yamlFile struct {
	Engine   yamlEngine    `yaml:"engine"`
	Versions []yamlVersion `yaml:"versions"`
}

// yamlEngine represents an engine type definition.
type yamlEngine struct {
	Slug        string `yaml:"slug"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Provider    string `yaml:"provider"` // optional, defaults to "custom"
}

// yamlVersion represents an engine version definition.
type yamlVersion struct {
	Slug          string                 `yaml:"slug"`
	Version       string                 `yaml:"version"`
	ContainerName string                 `yaml:"container_name"`
	Image         string                 `yaml:"image"`
	Entrypoint    []string               `yaml:"entrypoint"`
	Default       bool                   `yaml:"default"`
	Latest        bool                   `yaml:"latest"`
	Volumes       map[string]string      `yaml:"volumes"`
	Environment   map[string]string      `yaml:"environment"`
	Logging       yamlLogging            `yaml:"logging"`
	Nvidia        bool                   `yaml:"nvidia"`
	GPUCount      string                 `yaml:"gpu_count"`
	CommandArgs   []string               `yaml:"command_args"`
	Healthcheck   map[string]interface{} `yaml:"healthcheck"`
	Ulimits       map[string]interface{} `yaml:"ulimits"`
	IPC           string                 `yaml:"ipc"`
	Port          int                    `yaml:"port"`
}

// yamlLogging represents logging configuration.
type yamlLogging struct {
	Enable   bool   `yaml:"enable"`
	Address  string `yaml:"address"`
	Facility string `yaml:"facility"`
}

// ImportEngineFile parses an engine YAML file and creates engine type + versions.
// Returns (created, updated, skipped, error) where created=inserted count, updated=existing records updated, skipped=invalid entries skipped.
func (s *EngineService) ImportEngineFile(yamlPath string, overrides EngineImportOverrides) (created, updated, skipped int, err error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read file: %w", err)
	}

	var yf yamlFile
	if err := yaml.Unmarshal(data, &yf); err != nil {
		return 0, 0, 0, fmt.Errorf("parse YAML: %w", err)
	}

	if yf.Engine.Slug == "" {
		return 0, 0, 0, fmt.Errorf("engine slug is required")
	}

	// Create or skip / update engine type
	provider := yf.Engine.Provider
	if provider == "" {
		provider = "custom"
	}
	if !isValidProvider(provider) {
		return 0, 0, 0, fmt.Errorf("invalid provider %q for engine %s: must be one of vllm, sglang, llama.cpp, custom", provider, yf.Engine.Slug)
	}

	et := &models.EngineType{
		Slug:        yf.Engine.Slug,
		Name:        yf.Engine.Name,
		Description: yf.Engine.Description,
		Provider:    provider,
	}

	if overrides.Overwrite {
		// Update existing engine type or create if new
		existing, getErr := s.GetEngineTypeBySlug(yf.Engine.Slug)
		if getErr == nil && existing != nil {
			// Update existing — use UpdateEngineType with a map
			updates := map[string]interface{}{
				"name":        yf.Engine.Name,
				"description": yf.Engine.Description,
				"provider":    provider,
			}
			if err := s.UpdateEngineType(yf.Engine.Slug, updates); err != nil {
				return 0, 0, 0, fmt.Errorf("engine type %s: update failed: %w", yf.Engine.Slug, err)
			}
			updated++
			fmt.Fprintf(os.Stderr, "  Updated engine type: %s\n", yf.Engine.Slug)
		} else {
			// Create new
			if err := s.CreateEngineType(et); err != nil {
				return 0, 0, 0, fmt.Errorf("engine type %s: %w", yf.Engine.Slug, err)
			}
			created++
			fmt.Fprintf(os.Stderr, "  Created engine type: %s\n", yf.Engine.Slug)
		}
	} else {
		// Original create-or-skip behavior
		_, err = s.CreateOrSkipEngineType(et)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("engine type %s: %w", yf.Engine.Slug, err)
		}
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

		// Build healthcheck JSON
		hcJSON := ""
		if len(v.Healthcheck) > 0 {
			b, _ := json.Marshal(v.Healthcheck)
			hcJSON = string(b)
		}

		// Build ulimits JSON
		ulJSON := ""
		if len(v.Ulimits) > 0 {
			b, _ := json.Marshal(v.Ulimits)
			ulJSON = string(b)
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
			HealthcheckJSON:    hcJSON,
			UlimitsJSON:        ulJSON,
			IPC:                v.IPC,
		}

		var verCreated, verUpdated int
		if overrides.Overwrite {
			// Upsert: update if exists, insert if new
			existingVer, getErr := s.GetEngineVersionByTypeAndSlug(yf.Engine.Slug, v.Slug)
			if getErr == nil && existingVer != nil {
				// Update existing version — build updates map
				updates := map[string]interface{}{
					"version":              v.Version,
					"container_name":       v.ContainerName,
					"image":                v.Image,
					"entrypoint":           ep,
					"is_default":           isDefault,
					"is_latest":            isLatest,
					"environment_json":     envJSON,
					"volumes_json":         volJSON,
					"enable_logging":       v.Logging.Enable,
					"syslog_address":       v.Logging.Address,
					"syslog_facility":      v.Logging.Facility,
					"deploy_enable_nvidia": v.Nvidia,
					"deploy_gpu_count":     v.GPUCount,
					"command_args":         cmdJSON,
					"healthcheck_json":     hcJSON,
					"ulimits_json":         ulJSON,
					"ipc":                  v.IPC,
				}
				if err := s.UpdateEngineVersion(v.Slug, updates); err != nil {
					return created, updated, skipped, fmt.Errorf("engine version %s/%s: update failed: %w", yf.Engine.Slug, v.Slug, err)
				}
				verUpdated = 1
				fmt.Fprintf(os.Stderr, "  Updated engine version: %s/%s\n", yf.Engine.Slug, v.Slug)
			} else {
				// Insert new version
				if err := s.CreateEngineVersion(ev); err != nil {
					return created, updated, skipped, fmt.Errorf("engine version %s/%s: create failed: %w", yf.Engine.Slug, v.Slug, err)
				}
				verCreated = 1
				fmt.Fprintf(os.Stderr, "  Created engine version: %s/%s\n", yf.Engine.Slug, v.Slug)
			}
		} else {
			// Original create-or-skip behavior
			var createErr error
			createdBool, createErr := s.CreateOrSkipEngineVersion(ev)
			if createErr != nil {
				return created, updated, skipped, fmt.Errorf("engine version %s/%s: %w", yf.Engine.Slug, v.Slug, createErr)
			}
			if createdBool {
				verCreated = 1
			}
		}
		if verCreated > 0 {
			created++
		}
		if verUpdated > 0 {
			updated++
		}
		if !overrides.Overwrite && verCreated == 0 {
			skipped++
		}
	}

	return created, updated, skipped, nil
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
