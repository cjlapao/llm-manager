package yamlparser

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ValidEngineTypes lists the allowed engine types.
var ValidEngineTypes = []string{"vllm", "sglang", "llama.cpp"}

// ValidTypes is the enum of valid model types.
var ValidTypes = []string{"llm", "rag", "speech", "comfyui", "auto-complete"}

// ValidSubTypes maps a type to its valid subtypes.
var ValidSubTypes = map[string][]string{
	"llm":           {"chat"},
	"rag":           {"embedding", "reranker"},
	"speech":        {"stt", "tts", "omni"},
	"comfyui":       {"image", "3d"},
	"auto-complete": {"chat"},
}

// ValidCapabilities is the enum of known capabilities that map to LiteLLM model_info fields.
var ValidCapabilities = []string{
	"tool-use", "reasoning", "multi-turn", "image", "video", "document",
	"embedding", "reranker", "stt", "tts", "omni",
}

// TypeSubtypeCapability maps a type/subtype pair to its auto-injected capability.
var TypeSubtypeCapability = map[string]string{
	"rag/embedding": "embedding",
	"rag/reranker":  "reranker",
	"speech/stt":    "stt",
	"speech/tts":    "tts",
	"speech/omni":   "omni",
}

// CapabilitiesToModelInfo maps capability names to their corresponding model_info boolean fields.
// tool-use -> supports_function_calling, supports_tool_choice
// reasoning -> supports_reasoning
// multi-turn -> supports_multi_turn
// image -> supports_vision, supports_embedding_image_input (vision encoder for image understanding)
// video -> supports_video
// document -> supports_document (PDF/document understanding via vision encoder)
// embedding -> supports_embedding
// reranker -> supports_reranking
// stt -> supports_stt (speech-to-text)
// tts -> supports_tts (text-to-speech)
// omni -> supports_stt, supports_tts, supports_vision, supports_multimodal
var CapabilitiesToModelInfo = map[string][]string{
	"tool-use":   {"supports_function_calling", "supports_tool_choice"},
	"reasoning":  {"supports_reasoning"},
	"multi-turn": {"supports_multi_turn"},
	"image":      {"supports_vision", "supports_embedding_image_input"},
	"video":      {"supports_video"},
	"document":   {"supports_document"},
	"embedding":  {"supports_embedding"},
	"reranker":   {"supports_reranking"},
	"stt":        {"supports_stt"},
	"tts":        {"supports_tts"},
	"omni":       {"supports_stt", "supports_tts", "supports_vision", "supports_multimodal"},
}

// slugRegex validates that a slug is alphanumeric (with hyphens/underscores/dots), starting with alphanumeric.
var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ModelYAML represents the YAML schema for model import.
type ModelYAML struct {
	Slug            string      `yaml:"slug"`
	Name            string      `yaml:"name"`
	Type            string      `yaml:"type"`
	SubType         string      `yaml:"subtype"`
	Engine          string      `yaml:"engine"`
	EngineVersion   string      `yaml:"engine_version"`
	HFRepo          string      `yaml:"hf_repo"`
	Container       string      `yaml:"container"`
	Port            int         `yaml:"port"`
	EnvVars         map[string]string `yaml:"environment"`
	CommandArgs     []string `yaml:"command"`
	InputTokenCost  *float64    `yaml:"input_token_cost"`
	OutputTokenCost *float64    `yaml:"output_token_cost"`
	Capabilities    []string    `yaml:"capabilities"`
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
		errs = append(errs, fmt.Errorf("slug must match ^[a-z0-9][a-z0-9._-]*$ (got %q)", y.Slug))
	}

	if y.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}

	if y.Type != "" {
		valid := false
		for _, t := range ValidTypes {
			if y.Type == t {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, fmt.Errorf("type must be one of %v (got %q)", ValidTypes, y.Type))
		}
	}

	if y.SubType != "" {
		validSubTypes, ok := ValidSubTypes[y.Type]
		if !ok {
			errs = append(errs, fmt.Errorf("unknown type %q for subtype validation", y.Type))
		} else {
			valid := false
			for _, s := range validSubTypes {
				if y.SubType == s {
					valid = true
					break
				}
			}
			if !valid {
				errs = append(errs, fmt.Errorf("subtype %q is not valid for type %q (must be one of %v)", y.SubType, y.Type, validSubTypes))
			}
		}
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

// InjectCapabilitiesFromTypeSubtype auto-adds capability entries based on a model's
// type and subtype. For example, a rag/embedding model gets "embedding" injected
// into its capabilities slice. Only adds capabilities that are not already present.
func InjectCapabilitiesFromTypeSubtype(y *ModelYAML) {
	key := y.Type + "/" + y.SubType
	if cap, ok := TypeSubtypeCapability[key]; ok {
		// Check if already present
		for _, existing := range y.Capabilities {
			if existing == cap {
				return
			}
		}
		y.Capabilities = append(y.Capabilities, cap)
	}
}

// ValidateNonCapabilities checks the ModelYAML for required fields and valid values,
// except for capabilities. Use this when CLI overrides will replace YAML capabilities.
func ValidateNonCapabilities(y *ModelYAML) []error {
	var errs []error

	if y.Slug == "" {
		errs = append(errs, fmt.Errorf("slug is required"))
	} else if !slugRegex.MatchString(y.Slug) {
		errs = append(errs, fmt.Errorf("slug must match ^[a-z0-9][a-z0-9._-]*$ (got %q)", y.Slug))
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
