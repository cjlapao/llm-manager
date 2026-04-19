package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/pkg/yamlparser"
)

// ImportOverrides holds CLI argument overrides for model import.
type ImportOverrides struct {
	InputCost    *float64
	OutputCost   *float64
	Capabilities []string
}

// ImportModel imports a model from a YAML file into the database.
// It parses the YAML, validates it, checks for duplicates, maps to models.Model,
// applies CLI overrides, and creates the model record.
func (s *ModelService) ImportModel(yamlPath string, overrides ImportOverrides) (*models.Model, error) {
	// 1. Parse YAML
	y, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// 2. Validate
	if validationErrs := yamlparser.Validate(y); len(validationErrs) > 0 {
		var msgParts []string
		for _, e := range validationErrs {
			msgParts = append(msgParts, e.Error())
		}
		return nil, fmt.Errorf("YAML validation failed: %s", strings.Join(msgParts, "; "))
	}

	// 3. Check for duplicate slug
	if _, err := s.db.GetModel(y.Slug); err == nil {
		return nil, fmt.Errorf("model %s already exists", y.Slug)
	}

	// 4. Map ModelYAML → models.Model
	model := &models.Model{
		Slug:            y.Slug,
		Type:            "llm",
		Name:            y.Name,
		HFRepo:          y.HFRepo,
		Container:       y.Container,
		Port:            y.Port,
		EngineType:      y.Engine,
		InputTokenCost:  0.0,
		OutputTokenCost: 0.0,
		Capabilities:    "",
		EnvVars:         "",
		CommandArgs:     "",
	}

	// Convert maps to JSON strings
	if len(y.EnvVars) > 0 {
		b, err := json.Marshal(y.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal env_vars: %w", err)
		}
		model.EnvVars = string(b)
	}

	if len(y.CommandArgs) > 0 {
		b, err := json.Marshal(y.CommandArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command_args: %w", err)
		}
		model.CommandArgs = string(b)
	}

	// Apply YAML-level optional fields
	if y.InputTokenCost != nil {
		model.InputTokenCost = *y.InputTokenCost
	}
	if y.OutputTokenCost != nil {
		model.OutputTokenCost = *y.OutputTokenCost
	}
	if len(y.Capabilities) > 0 {
		b, err := json.Marshal(y.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
		}
		model.Capabilities = string(b)
	}

	// 5. Apply CLI overrides
	if overrides.InputCost != nil {
		model.InputTokenCost = *overrides.InputCost
	}
	if overrides.OutputCost != nil {
		model.OutputTokenCost = *overrides.OutputCost
	}
	if len(overrides.Capabilities) > 0 {
		b, err := json.Marshal(overrides.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal override capabilities: %w", err)
		}
		model.Capabilities = string(b)
	}

	// 6. Create in database
	if err := s.db.CreateModel(model); err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	return model, nil
}

// ExportModel exports a model from the database to a ModelYAML struct.
func (s *ModelService) ExportModel(slug string) (*yamlparser.ModelYAML, error) {
	// 1. Get model from DB
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("model %s not found: %w", slug, err)
	}

	// 2. Convert JSON strings back to maps/slices
	y := &yamlparser.ModelYAML{
		Slug:         model.Slug,
		Name:         model.Name,
		Engine:       model.EngineType,
		HFRepo:       model.HFRepo,
		Container:    model.Container,
		Port:         model.Port,
		EnvVars:      map[string]string{},
		CommandArgs:  map[string]string{},
		Capabilities: []string{},
	}

	// Parse env_vars JSON string
	if model.EnvVars != "" {
		var envVars map[string]string
		if err := json.Unmarshal([]byte(model.EnvVars), &envVars); err == nil {
			y.EnvVars = envVars
		}
	}

	// Parse command_args JSON string
	if model.CommandArgs != "" {
		var cmdArgs map[string]string
		if err := json.Unmarshal([]byte(model.CommandArgs), &cmdArgs); err == nil {
			y.CommandArgs = cmdArgs
		}
	}

	// Parse capabilities JSON string
	if model.Capabilities != "" {
		var caps []string
		if err := json.Unmarshal([]byte(model.Capabilities), &caps); err == nil {
			y.Capabilities = caps
		}
	}

	// Set optional cost fields as pointers
	if model.InputTokenCost > 0 {
		y.InputTokenCost = &model.InputTokenCost
	}
	if model.OutputTokenCost > 0 {
		y.OutputTokenCost = &model.OutputTokenCost
	}

	return y, nil
}
