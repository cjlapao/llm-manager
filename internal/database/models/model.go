// Package models defines the GORM data models for the application.
package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Model represents an LLM model in the registry.
//
//	swag:response
//	@Summary	LLM model definition
//	@Description	An LLM model record stored in the database, including configuration, token pricing, engine settings, and variant information.
type Model struct {
	ID                          uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug                        string    `gorm:"uniqueIndex;size:128;not null;column:slug"`
	Type                        string    `gorm:"size:32;not null;index;column:type"`
	SubType                     string    `gorm:"size:32;column:sub_type"`
	Name                        string    `gorm:"size:256;not null;column:name"`
	HFRepo                      string    `gorm:"size:512;column:hf_repo"`
	YML                         string    `gorm:"type:text;column:yml"`
	Container                   string    `gorm:"size:256;column:container"`
	Port                        int       `gorm:"not null;column:port"`
	EngineType                  string    `gorm:"size:16;default:'vllm';column:engine_type"`
	EnvVars                     string    `gorm:"type:text;column:env_vars"`
	CommandArgs                 string    `gorm:"type:text;column:command_args"`
	InputTokenCost              float64   `gorm:"default:0;column:input_token_cost"`
	OutputTokenCost             float64   `gorm:"default:0;column:output_token_cost"`
	CacheCreationInputTokenCost float64   `gorm:"default:0;column:cache_creation_input_token_cost"`
	CacheReadInputTokenCost     float64   `gorm:"default:0;column:cache_read_input_token_cost"`
	Capabilities                string    `gorm:"type:text;column:capabilities"`
	LiteLLMParams               string    `gorm:"type:text;column:lite_llm_params"`
	ModelInfo                   string    `gorm:"type:text;column:model_info"`
	LitellmModelID              string    `gorm:"type:varchar(36);size:36;column:litellm_model_id"`
	LitellmActiveAliases        string    `gorm:"type:text;column:litellm_active_aliases"`
	LitellmVariantIDs           string    `gorm:"type:text;column:litellm_variant_ids"`
	Default                     bool      `gorm:"type:boolean;default:false;column:default"`
	BaseImageID                 string    `gorm:"size:128;column:base_image_id"`
	EngineVersionSlug           string    `gorm:"size:128;default:'';column:engine_version_slug"`
	TotalParamsB                *float64  `gorm:"column:total_params_b"`
	ActiveParamsB               *float64  `gorm:"column:active_params_b"`
	IsMoe                       *bool     `gorm:"column:is_moe"`
	AttentionLayers             *int      `gorm:"column:attention_layers"`
	GdnLayers                   *int      `gorm:"column:gdn_layers"`
	NumKvHeads                  *int      `gorm:"column:num_kv_heads"`
	HeadDim                     *int      `gorm:"column:head_dim"`
	SupportsMtp                 *bool     `gorm:"column:supports_mtp"`
	DefaultContext              *int      `gorm:"column:default_context"`
	MaxContext                  *int      `gorm:"column:max_context"`
	QuantBytesPerParam          *float64  `gorm:"column:quant_bytes_per_param"`
	MaxNumSeqs                  *int      `gorm:"column:max_num_seqs"`
	MaxNumBatchedTokens         *int      `gorm:"column:max_num_batched_tokens"`
	SpeculativeDecoding         *string   `gorm:"column:speculative_decoding"`
	NumSpeculativeTokens        *int      `gorm:"column:num_speculative_tokens"`
	GpuMemoryUtilization        *float64  `gorm:"column:gpu_memory_utilization"`
	HealthcheckJSON             string    `gorm:"type:text;column:healthcheck_json"`
	CreatedAt                   time.Time `gorm:"autoCreateTime;column:created_at"`
	UpdatedAt                   time.Time `gorm:"autoUpdateTime;column:updated_at"`
}

// TableName returns the database table name for Model.
func (Model) TableName() string { return "models" }

// BeforeCreate generates a UUID for new Model records.
func (m *Model) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// HasThinkingCapability checks if this model should get an active-thinking
// alias. Detects thinking capability support in three places (checked in order):
//  1. Name field for "-think" or "thinking" substrings
//  2. Capabilities field for "thinking" keyword
//  3. litellm_params.extra_body.chat_template_kwargs.enable_thinking == true
func (m *Model) HasThinkingCapability() bool {
	check := strings.ToLower(m.Name) + "," + strings.ToLower(m.Capabilities)
	if strings.Contains(check, "-think") || strings.Contains(check, "thinking") {
		return true
	}
	// Also check lite_llm_params for explicit enable_thinking flag.
	if m.LiteLLMParams != "" {
		var params map[string]interface{}
		if json.Unmarshal([]byte(m.LiteLLMParams), &params) == nil {
			if extraBody, ok := params["extra_body"].(map[string]interface{}); ok {
				if ctKwargs, ok := extraBody["chat_template_kwargs"].(map[string]interface{}); ok {
					if et, ok := ctKwargs["enable_thinking"]; ok {
						if enabled, ok := et.(bool); ok && enabled {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// GetLitellmActiveAliases deserializes the JSON map and returns the active alias
// map keyed by alias name ("active", "active-thinking") mapped to LiteLLM proxy
// row UUIDs. Returns nil if the model has no active aliases yet.
func (m *Model) GetLitellmActiveAliases() map[string]string {
	aliases := make(map[string]string)
	if m.LitellmActiveAliases == "" {
		return nil
	}
	json.Unmarshal([]byte(m.LitellmActiveAliases), &aliases)
	return aliases
}

// SetLitellmActiveAliases serializes the alias map to JSON and stores it in the
// raw text column on this model. Pass nil to clear all active aliases.
func (m *Model) SetLitellmActiveAliases(aliases map[string]string) {
	if aliases == nil || len(aliases) == 0 {
		if m.LitellmActiveAliases != "" {
			m.LitellmActiveAliases = ""
		}
		return
	}
	b, _ := json.Marshal(aliases)
	m.LitellmActiveAliases = string(b)
}

// GetLitellmVariantIDs deserializes the JSON map and returns the variant ids
// keyed by variant name ("thinking", "instruct", …) mapped to LiteLLM proxy
// row UUIDs. Returns nil when the model has no variants yet.
func (m *Model) GetLitellmVariantIDs() map[string]string {
	ids := make(map[string]string)
	if m.LitellmVariantIDs == "" {
		return nil
	}
	json.Unmarshal([]byte(m.LitellmVariantIDs), &ids)
	return ids
}

// SetLitellmVariantIDs serializes the variant id map to JSON and stores it in
// the raw text column on this model. Pass nil to clear all variant ids.
func (m *Model) SetLitellmVariantIDs(ids map[string]string) {
	if ids == nil || len(ids) == 0 {
		if m.LitellmVariantIDs != "" {
			m.LitellmVariantIDs = ""
		}
		return
	}
	b, _ := json.Marshal(ids)
	m.LitellmVariantIDs = string(b)
}

// GetVariantKeys lists the names of all variants defined in this model's variants
// object/map block. Returns a nil slice when no variants exist.
func (m *Model) GetVariantKeys() []string {
	keys := make([]string, 0)
	variants, ok := m.getVariantsMap()
	if !ok {
		return nil
	}
	for k := range variants {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// VariantSpec returns the spec for a named variant. The key maps directly
// to the variant name in the "variants" object/map form.
func (m *Model) VariantSpec(name string) (map[string]interface{}, bool) {
	variantsMap, ok := m.getVariantsMap()
	if !ok {
		return nil, false
	}
	if spec, ok := variantsMap[name].(map[string]interface{}); ok {
		return spec, true
	}
	return nil, false
}

func (m *Model) getVariantsMap() (map[string]interface{}, bool) {
	block := parseParamBlock(m.LiteLLMParams)
	v, ok := block["variants"].(map[string]interface{})
	return v, ok
}

// parseParamBlock parses a JSON-encoded litellm_params string into a map.
// Returns an empty map when the input is empty or invalid JSON.
func parseParamBlock(raw string) map[string]interface{} {
	if raw == "" {
		return make(map[string]interface{})
	}
	var block map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &block); err != nil {
		return make(map[string]interface{})
	}
	return block
}

// HasVariants checks if this model defines any variants in its litellm_params block.
// Returns false when no variants are present or the map is empty.
func (m *Model) HasVariants() bool {
	variants, ok := m.getVariantsMap()
	return ok && len(variants) > 0
}

// GetHealthcheck parses HealthcheckJSON into a map[string]interface{}.
// Returns an empty map (not nil) when the JSON is empty or invalid.
func (m *Model) GetHealthcheck() map[string]interface{} {
	if m.HealthcheckJSON == "" {
		return make(map[string]interface{})
	}
	var hc map[string]interface{}
	if err := json.Unmarshal([]byte(m.HealthcheckJSON), &hc); err != nil {
		return make(map[string]interface{})
	}
	return hc
}

// SetHealthcheck serializes a map[string]interface{} to JSON and stores it in HealthcheckJSON.
func (m *Model) SetHealthcheck(hc map[string]interface{}) error {
	if hc == nil || len(hc) == 0 {
		m.HealthcheckJSON = ""
		return nil
	}
	b, err := json.Marshal(hc)
	if err != nil {
		return err
	}
	m.HealthcheckJSON = string(b)
	return nil
}

// GetContainerName returns the actual Docker container name for this model.
// It is the model type prefixed to the raw container value (e.g., "speech-qwen3-tts-1.7b").
func (m *Model) GetContainerName() string {
	if m.Container == "" {
		return ""
	}
	return fmt.Sprintf("%s-%s", m.Type, m.Container)
}
