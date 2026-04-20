package yamlparser

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ValidEngineTypes lists the allowed engine types.
var ValidEngineTypes = []string{"vllm", "sglang"}

// ValidCapabilities is the enum of known capabilities that map to LiteLLM model_info fields.
var ValidCapabilities = []string{"vision", "tool-use", "reasoning", "multi-turn"}

// CapabilitiesToModelInfo maps capability names to their corresponding model_info boolean fields.
// vision -> supports_vision, supports_embedding_image_input
// tool-use -> supports_function_calling, supports_tool_choice
// reasoning -> supports_reasoning
var CapabilitiesToModelInfo = map[string][]string{
	"vision":    {"supports_vision", "supports_embedding_image_input"},
	"tool-use":  {"supports_function_calling", "supports_tool_choice"},
	"reasoning": {"supports_reasoning"},
}

// slugRegex validates that a slug is alphanumeric (with hyphens/underscores), starting with alphanumeric.
var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ModelYAML represents the YAML schema for model import.
type ModelYAML struct {
	Slug            string            `yaml:"slug"`
	Name            string            `yaml:"name"`
	Engine          string            `yaml:"engine"`
	HFRepo          string            `yaml:"hf_repo"`
	Container       string            `yaml:"container"`
	Port            int               `yaml:"port"`
	EnvVars         map[string]string `yaml:"environment"`
	CommandArgs     map[string]string `yaml:"command"`
	InputTokenCost  *float64          `yaml:"input_token_cost"`
	OutputTokenCost *float64          `yaml:"output_token_cost"`
	Capabilities    []string          `yaml:"capabilities"`
	// LiteLLM parameters - optional, supports mixed types (float, int, string, bool, nested maps, arrays).
	// The system will auto-construct api_base (from config URL + port) and model (from slug) during import.
	LiteLLMParams map[string]interface{} `yaml:"litellm_params"`
	// Model metadata from LiteLLM's model registry.
	ModelInfo map[string]interface{} `yaml:"model_info"`
}

// ParseYAML reads and parses a YAML file into a ModelYAML struct.
func ParseYAML(path string) (*ModelYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", path, err)
	}

	var y ModelYAML
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file %s: %w", path, err)
	}

	return &y, nil
}

// Validate checks the ModelYAML for required fields and valid values.
// Returns a slice of error strings (empty means valid).
func Validate(y *ModelYAML) []error {
	var errs []error

	if y.Slug == "" {
		errs = append(errs, fmt.Errorf("slug is required"))
	} else if !slugRegex.MatchString(y.Slug) {
		errs = append(errs, fmt.Errorf("slug must match ^[a-z0-9][a-z0-9_-]*$ (got %q)", y.Slug))
	}

	if y.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}

	if y.Engine == "" {
		errs = append(errs, fmt.Errorf("engine is required"))
	} else {
		valid := false
		for _, e := range ValidEngineTypes {
			if y.Engine == e {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, fmt.Errorf("engine must be one of %v (got %q)", ValidEngineTypes, y.Engine))
		}
	}

	if y.Port < 1 || y.Port > 65535 {
		errs = append(errs, fmt.Errorf("port must be between 1 and 65535 (got %d)", y.Port))
	}

	if y.InputTokenCost != nil && *y.InputTokenCost < 0 {
		errs = append(errs, fmt.Errorf("input_token_cost must be >= 0 (got %s)", formatCost(*y.InputTokenCost)))
	}

	if y.OutputTokenCost != nil && *y.OutputTokenCost < 0 {
		errs = append(errs, fmt.Errorf("output_token_cost must be >= 0 (got %s)", formatCost(*y.OutputTokenCost)))
	}

	// Validate capabilities against known enum
	if len(y.Capabilities) > 0 {
		for _, cap := range y.Capabilities {
			valid := false
			for _, known := range ValidCapabilities {
				if cap == known {
					valid = true
					break
				}
			}
			if !valid {
				errs = append(errs, fmt.Errorf("invalid capability %q (must be one of %v)", cap, ValidCapabilities))
			}
		}
	}

	return errs
}

// ValidateNonCapabilities checks the ModelYAML for required fields and valid values,
// except for capabilities. Use this when CLI overrides will replace YAML capabilities.
func ValidateNonCapabilities(y *ModelYAML) []error {
	var errs []error

	if y.Slug == "" {
		errs = append(errs, fmt.Errorf("slug is required"))
	} else if !slugRegex.MatchString(y.Slug) {
		errs = append(errs, fmt.Errorf("slug must match ^[a-z0-9][a-z0-9_-]*$ (got %q)", y.Slug))
	}

	if y.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}

	if y.Engine == "" {
		errs = append(errs, fmt.Errorf("engine is required"))
	} else {
		valid := false
		for _, e := range ValidEngineTypes {
			if y.Engine == e {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, fmt.Errorf("engine must be one of %v (got %q)", ValidEngineTypes, y.Engine))
		}
	}

	if y.Port < 1 || y.Port > 65535 {
		errs = append(errs, fmt.Errorf("port must be between 1 and 65535 (got %d)", y.Port))
	}

	if y.InputTokenCost != nil && *y.InputTokenCost < 0 {
		errs = append(errs, fmt.Errorf("input_token_cost must be >= 0 (got %s)", formatCost(*y.InputTokenCost)))
	}

	if y.OutputTokenCost != nil && *y.OutputTokenCost < 0 {
		errs = append(errs, fmt.Errorf("output_token_cost must be >= 0 (got %s)", formatCost(*y.OutputTokenCost)))
	}

	return errs
}

// formatCost formats a cost value for display in error messages.
func formatCost(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if s == "-0" {
		s = "0"
	}
	return s
}
