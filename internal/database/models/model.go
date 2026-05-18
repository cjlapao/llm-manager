// Package models defines the GORM data models for the application.
package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Model represents an LLM model in the registry.
type Model struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug                 string    `gorm:"uniqueIndex;size:128;not null;column:slug"`
	Type                 string    `gorm:"size:32;not null;index;column:type"`
	SubType              string    `gorm:"size:32;column:sub_type"`
	Name                 string    `gorm:"size:256;not null;column:name"`
	HFRepo               string    `gorm:"size:512;column:hf_repo"`
	YML                  string    `gorm:"type:text;column:yml"`
	Container            string    `gorm:"size:256;column:container"`
	Port                 int       `gorm:"not null;column:port"`
	EngineType           string    `gorm:"size:16;default:'vllm';column:engine_type"`
	EnvVars              string    `gorm:"type:text;column:env_vars"`
	CommandArgs          string    `gorm:"type:text;column:command_args"`
	InputTokenCost       float64   `gorm:"default:0;column:input_token_cost"`
	OutputTokenCost      float64   `gorm:"default:0;column:output_token_cost"`
	Capabilities         string    `gorm:"type:text;column:capabilities"`
	LiteLLMParams        string    `gorm:"type:text;column:lite_llm_params"`
	ModelInfo            string    `gorm:"type:text;column:model_info"`
	LitellmModelID       string    `gorm:"type:varchar(36);size:36;column:litellm_model_id"`
	LitellmActiveAliases string    `gorm:"type:text;column:litellm_active_aliases"`
	LitellmVariantIDs    string    `gorm:"type:text;column:litellm_variant_ids"`
	Default              bool      `gorm:"type:boolean;default:false;column:default"`
	BaseImageID          string    `gorm:"size:128;column:base_image_id"`
	EngineVersionSlug    string    `gorm:"size:128;default:'';column:engine_version_slug"`
	TotalParamsB         *float64  `gorm:"column:total_params_b"`
	ActiveParamsB        *float64  `gorm:"column:active_params_b"`
	IsMoe                *bool     `gorm:"column:is_moe"`
	AttentionLayers      *int      `gorm:"column:attention_layers"`
	GdnLayers            *int      `gorm:"column:gdn_layers"`
	NumKvHeads           *int      `gorm:"column:num_kv_heads"`
	HeadDim              *int      `gorm:"column:head_dim"`
	SupportsMtp          *bool     `gorm:"column:supports_mtp"`
	DefaultContext       *int      `gorm:"column:default_context"`
	MaxContext           *int      `gorm:"column:max_context"`
	QuantBytesPerParam   *float64  `gorm:"column:quant_bytes_per_param"`
	CreatedAt            time.Time `gorm:"autoCreateTime;column:created_at"`
	UpdatedAt            time.Time `gorm:"autoUpdateTime;column:updated_at"`
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
// alias. Detects thinking capability support in the name or capabilities list.
func (m *Model) HasThinkingCapability() bool {
	check := strings.ToLower(m.Name) + "," + strings.ToLower(m.Capabilities)
	return strings.Contains(check, "-think") || strings.Contains(check, "thinking")
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
