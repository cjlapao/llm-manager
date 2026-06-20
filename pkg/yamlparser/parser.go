package yamlparser

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ValidEngineTypes lists the allowed core engine types. These are fallback defaults
// when no database is available. Custom engines imported via YAML files will
// register their own slugs (e.g., "qwen-voice") which must be passed explicitly.
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

// ModelProfile holds architecture-specific constants for GPU memory calculation.
type ModelProfile struct {
	TotalParamsB       *float64 `yaml:"total_params_b"`
	ActiveParamsB      *float64 `yaml:"active_params_b"`
	IsMoe              *bool    `yaml:"is_moe"`
	AttentionLayers    *int     `yaml:"attention_layers"`
	GdnLayers          *int     `yaml:"gdn_layers"`
	NumKvHeads         *int     `yaml:"num_kv_heads"`
	HeadDim            *int     `yaml:"head_dim"`
	SupportsMtp        *bool    `yaml:"supports_mtp"`
	DefaultContext     *int     `yaml:"default_context"`
	MaxContext         *int     `yaml:"max_context"`
	QuantBytesPerParam *float64 `yaml:"quant_bytes_per_param"`
	// New fields for runtime tuning
	MaxNumSeqs           *int    `yaml:"max_num_seqs"`
	MaxNumBatchedTokens  *int    `yaml:"max_num_batched_tokens"`
	SpeculativeDecoding  *string `yaml:"speculative_decoding"` // e.g., "mtp", "dflash"
	NumSpeculativeTokens *int    `yaml:"num_speculative_tokens"`
	SpeculativeModel     *string `yaml:"speculative_model"`
	// GpuMemoryUtilization is an optional override for gpu_memory_utilization.
	// When set, the auto-calculated memory utilization is bypassed and this
	// value is used directly. Value must be in (0, 1).
	GpuMemoryUtilization *float64 `yaml:"gpu_memory_utilization"`
}

// ModelYAML represents the YAML schema for model import.
type ModelYAML struct {
	Slug            string            `yaml:"slug"`
	Name            string            `yaml:"name"`
	Type            string            `yaml:"type"`
	SubType         string            `yaml:"subtype"`
	Engine          string            `yaml:"engine"`
	EngineVersion   string            `yaml:"engine_version"`
	HFRepo          string            `yaml:"hf_repo"`
	Container       string            `yaml:"container"`
	Port            int               `yaml:"port"`
	EnvVars         map[string]string `yaml:"environment"`
	CommandArgs     []string          `yaml:"command"`
	InputTokenCost  *float64          `yaml:"input_token_cost"`
	OutputTokenCost *float64          `yaml:"output_token_cost"`
	// Cache-aware pricing fields (LiteLLM style)
	CacheCreationInputTokenCost *float64 `yaml:"cache_creation_input_token_cost"`
	CacheReadInputTokenCost     *float64 `yaml:"cache_read_input_token_cost"`
	Capabilities                []string `yaml:"capabilities"`
	// LiteLLM parameters - optional, supports mixed types (float, int, string, bool, nested maps, arrays).
	// The system will auto-construct api_base (from config URL + port) and model (from slug) during import.
	LiteLLMParams map[string]interface{} `yaml:"litellm_params"`
	// Model metadata from LiteLLM's model registry.
	ModelInfo map[string]interface{} `yaml:"model_info"`
	// Model architecture profile for GPU memory calculation — optional.
	Profile *ModelProfile `yaml:"profile"`
	// Optional health check URL for TTS voice verification.
	HealthCheckVoice *string `yaml:"health_check_voice"`
	// Optional healthcheck configuration block for container health checks.
	// Populated by ParseYAML from the top-level "healthcheck:" YAML key
	// (not automatically captured by standard yaml.Unmarshal).
	HealthCheckJSON string `json:"health_check_json,omitempty"`

	// Raw healthcheck block as a yaml.Node — allows direct child extraction
	// into flat JSON without the wrapper-key nesting problem.
	HealthCheckNode yaml.Node `yaml:"healthcheck,omitempty"`
}

// knownKeys is the set of top-level YAML keys that ModelYAML handles directly.
// Anything not in here is treated as an extra block (e.g. "healthcheck:")
// and serialized into HealthCheckJSON.
var knownKeys = map[string]struct{}{
	"slug": {}, "name": {}, "type": {}, "subtype": {}, "engine": {},
	"engine_version": {}, "hf_repo": {}, "container": {}, "port": {},
	"environment": {}, "command": {}, "input_token_cost": {},
	"output_token_cost": {}, "cache_creation_input_token_cost": {},
	"cache_read_input_token_cost": {}, "capabilities": {},
	"litellm_params": {}, "model_info": {}, "profile": {},
	"health_check_voice": {},
}

// ParseYAML reads and parses a YAML file into a ModelYAML struct.
func ParseYAML(path string) (*ModelYAML, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", path, err)
	}

	var y ModelYAML
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file: %w", err)
	}

	// Extract extra top-level keys (e.g., healthcheck:) not captured
	// by known struct tags. Uses yaml.Node traversal for accuracy.
	//
	// Note: "healthcheck:" is handled specially — if HealthCheckNode was populated
	// by the struct tag above, its children are extracted directly into flat JSON
	// so that BuildHealthcheckSection receives the expected format (no wrapper
	// key around the inner map).
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err == nil && len(rootNode.Content) > 0 {
		top := rootNode.Content[0]
		if top.Kind == yaml.MappingNode {
			// Extract the "healthcheck:" block as flat JSON (no wrapper key).
			// Iterate over top-level nodes; when we hit the "healthcheck" key,
			// unmarshal the YAML node into a Go value, then json.Marshal it.
			for i := 0; i < len(top.Content); i += 2 {
				k := top.Content[i].Value
				if k == "healthcheck" {
					valBytes, _ := yaml.Marshal(top.Content[i+1])
					var val interface{}
					if yaml.Unmarshal(valBytes, &val) == nil {
						if b, err := json.Marshal(val); err == nil {
							y.HealthCheckJSON = string(b)
						}
					}
				}
			}

			// Then: handle remaining unknown keys (not "healthcheck":).
			extra := make(map[string]interface{})
			for i := 0; i < len(top.Content); i += 2 {
				k := top.Content[i].Value
				// Skip healthcheck — already handled above via HealthCheckNode
				if k == "healthcheck" {
					continue
				}
				valBytes, _ := yaml.Marshal(top.Content[i+1])
				if _, known := knownKeys[k]; known {
					continue // skip keys handled by ModelYAML struct fields
				}
				var val interface{}
				if e := yaml.Unmarshal(valBytes, &val); e == nil {
					extra[k] = val
				}
			}
			if len(extra) > 0 {
				b, e := json.Marshal(extra)
				if e == nil {
					// Only set if HC wasn't found via HealthCheckNode,
					// or merge additional keys. For simplicity, append
					// extra (non-healthcheck) keys as additional JSON in model_info.
					if y.HealthCheckJSON == "" {
						y.HealthCheckJSON = string(b)
					}
				}
			}
		}
	}

	return &y, nil
}

// Validate checks the ModelYAML for required fields and valid values.
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

	// Validate profile fields
	if y.Profile != nil {
		errs = append(errs, validateProfile(y.Profile)...)
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
// except for capabilities.
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

	// Validate profile fields
	if y.Profile != nil {
		errs = append(errs, validateProfile(y.Profile)...)
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

// validQuantBytesPerParam lists the allowed quantization bytes-per-parameter values.
var validQuantBytesPerParam = map[float64]struct{}{
	0.5: {},
	1.0: {},
	2.0: {},
}

// validateProfile validates all fields in a ModelProfile, returning a slice of
// error strings (empty means valid). Each numeric field is checked for positive
// values where applicable; quant_bytes_per_param must be one of 0.5, 1.0, 2.0.
func validateProfile(p *ModelProfile) []error {
	var errs []error

	if p.TotalParamsB != nil && *p.TotalParamsB <= 0 {
		errs = append(errs, fmt.Errorf("profile.total_params_b must be > 0 (got %s)", formatCost(*p.TotalParamsB)))
	}
	if p.ActiveParamsB != nil && *p.ActiveParamsB <= 0 {
		// -1.0 is a sentinel value meaning "active params equals total params"
		// (used for non-MoE models where all params are active). Allow it.
		if *p.ActiveParamsB < -1.0 {
			errs = append(errs, fmt.Errorf("profile.active_params_b must be > 0 or -1.0 sentinel (got %s)", formatCost(*p.ActiveParamsB)))
		}
	}
	if p.AttentionLayers != nil && *p.AttentionLayers < 0 {
		errs = append(errs, fmt.Errorf("profile.attention_layers must be >= 0 (got %d)", *p.AttentionLayers))
	}
	if p.GdnLayers != nil && *p.GdnLayers < 0 {
		errs = append(errs, fmt.Errorf("profile.gdn_layers must be >= 0 (got %d)", *p.GdnLayers))
	}
	if p.NumKvHeads != nil && *p.NumKvHeads < 0 {
		errs = append(errs, fmt.Errorf("profile.num_kv_heads must be >= 0 (got %d)", *p.NumKvHeads))
	}
	if p.HeadDim != nil && *p.HeadDim < 0 {
		errs = append(errs, fmt.Errorf("profile.head_dim must be >= 0 (got %d)", *p.HeadDim))
	}
	if p.DefaultContext != nil && *p.DefaultContext <= 0 {
		errs = append(errs, fmt.Errorf("profile.default_context must be > 0 (got %d)", *p.DefaultContext))
	}
	if p.MaxContext != nil && *p.MaxContext <= 0 {
		errs = append(errs, fmt.Errorf("profile.max_context must be > 0 (got %d)", *p.MaxContext))
	}
	if p.QuantBytesPerParam != nil {
		if *p.QuantBytesPerParam <= 0 {
			errs = append(errs, fmt.Errorf("profile.quant_bytes_per_param must be > 0 (got %s)", formatCost(*p.QuantBytesPerParam)))
		} else if _, ok := validQuantBytesPerParam[*p.QuantBytesPerParam]; !ok {
			errs = append(errs, fmt.Errorf("profile.quant_bytes_per_param must be one of 0.5, 1.0, 2.0 (got %s)", formatCost(*p.QuantBytesPerParam)))
		}
	}

	// Validate new runtime-tuning fields
	if p.MaxNumSeqs != nil && *p.MaxNumSeqs <= 0 {
		errs = append(errs, fmt.Errorf("profile.max_num_seqs must be > 0 (got %d)", *p.MaxNumSeqs))
	}
	if p.MaxNumBatchedTokens != nil && *p.MaxNumBatchedTokens <= 0 {
		errs = append(errs, fmt.Errorf("profile.max_num_batched_tokens must be > 0 (got %d)", *p.MaxNumBatchedTokens))
	}
	if p.SpeculativeDecoding != nil {
		switch *p.SpeculativeDecoding {
		case "mtp", "dflash":
			// valid
		case "":
			// empty is fine (disabled)
		default:
			errs = append(errs, fmt.Errorf("profile.speculative_decoding must be one of mtp, dflash (got %q)", *p.SpeculativeDecoding))
		}
	}
	if p.NumSpeculativeTokens != nil && *p.NumSpeculativeTokens <= 0 {
		errs = append(errs, fmt.Errorf("profile.num_speculative_tokens must be > 0 (got %d)", *p.NumSpeculativeTokens))
	}
	if p.GpuMemoryUtilization != nil {
		if *p.GpuMemoryUtilization <= 0 || *p.GpuMemoryUtilization > 1.0 {
			errs = append(errs, fmt.Errorf("profile.gpu_memory_utilization must be in (0, 1] (got %s)", formatCost(*p.GpuMemoryUtilization)))
		}
	}

	return errs
}
