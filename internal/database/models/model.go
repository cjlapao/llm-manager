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
	Slug                 string    `gorm:"uniqueIndex;size:128;not null"`
	Type                 string    `gorm:"size:32;not null;index"`
	Name                 string    `gorm:"size:256;not null"`
	HFRepo               string    `gorm:"size:512"`
	YML                  string    `gorm:"type:text"`
	Container            string    `gorm:"size:256"`
	Port                 int       `gorm:"not null"`
	EngineType           string    `gorm:"size:16;default:'vllm'"`
	EnvVars              string    `gorm:"type:text"`
	CommandArgs          string    `gorm:"type:text"`
	InputTokenCost       float64   `gorm:"default:0"`
	OutputTokenCost      float64   `gorm:"default:0"`
	Capabilities         string    `gorm:"type:text"`
	LiteLLMParams        string    `gorm:"type:text"`
	ModelInfo            string    `gorm:"type:text"`
	LitellmModelID       string    `gorm:"type:varchar(36);size:36"` // base model UUID in LiteLLM proxy
	LitellmActiveAliases string    `gorm:"type:text"`                // JSON: {"active": "<uuid>", "active-thinking": "<uuid>"}
	LitellmVariantIDs    string    `gorm:"type:text"`                // JSON: {"thinking": "<uuid>", "instruct": "<uuid>"}
	CreatedAt            time.Time `gorm:"autoCreateTime"`
	UpdatedAt            time.Time `gorm:"autoUpdateTime"`
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
		// Marshal produces "null"; store empty string for clarity.
		if m.LitellmActiveAliases != "" {
			m.LitellmActiveAliases = ""
		}
		return
	}
	b, _ := json.Marshal(aliases)
	m.LitellmActiveAliases = string(b)
}

// getVariantsMap returns the "variants" value from the parsed litellm_params.
// Variants use object/map format where keys are variant names and values are specs.
// Returns the map and true if present, nil and false otherwise.
func (m *Model) getVariantsMap() (map[string]interface{}, bool) {
	block := parseParamBlock(m.LiteLLMParams)
	v, ok := block["variants"].(map[string]interface{})
	return v, ok
}

// HasVariants checks if this model defines any variants in its litellm_params block.
// Returns false when no variants are present or the map is empty.
func (m *Model) HasVariants() bool {
	variants, ok := m.getVariantsMap()
	return ok && len(variants) > 0
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
		// Marshal produces "null"; store empty string for clarity.
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

// parseParamBlock attempts to unmarshal the LiteLLMParams JSON blob.
// Returns an empty map when the blob is unset/bad.
//
// Conversion: when the key "derivates" is present as a JSON array, each
// entry (which carries a "name" + override fields) is converted into a
// "variants" map entry keyed by name. All non-"name" fields from the
// derivates entry are copied into the variant spec preserving their structure
// (e.g., extra_body remains nested). The original "derivates" key is deleted
// so re-parsing is idempotent.
func parseParamBlock(raw string) map[string]interface{} {
	var block map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &block); err != nil || block == nil {
		block = make(map[string]interface{})
	}
	if derivates, hasDerivates := block["derivates"].([]interface{}); hasDerivates && len(derivates) > 0 {
		if _, hasVariants := block["variants"]; !hasVariants {
			block["variants"] = make(map[string]interface{})
		}
		variants := block["variants"].(map[string]interface{})
		for _, entry := range derivates {
			if der, ok := entry.(map[string]interface{}); ok {
				name, _ := der["name"].(string)
				if name == "" {
					continue
				}
				spec := make(map[string]interface{})
				// Copy all fields except "name" (which is used solely as the key)
				for k, v := range der {
					if k == "name" {
						continue
					}
					spec[k] = v
				}
				variants[name] = spec
			}
		}
		delete(block, "derivates")
	}
	return block
}

// DeepMerge deeply merges *src into *dst. Map values are merged recursively;
// leaf values from src overwrite the corresponding destination value. Returns dst.
func DeepMerge(dst, src interface{}) interface{} {
	srcMap, ok := shallowCastToMap(src)
	if !ok {
		// At the leaves simply replace.
		return src
	}
	dstCopy, ok := shallowCastToMap(inheritFrom(dst))
	if !ok {
		dstCopy = make(map[string]interface{})
	}
	for k, v := range srcMap {
		if existing, exists := dstCopy[k]; exists {
			merged := DeepMerge(existing, v)
			dstCopy[k] = merged
		} else {
			dstCopy[k] = inheritFrom(v)
		}
	}
	return dstCopy
}

// shallowCastToMap tries to interpret val as map[string]interface{}.
func shallowCastToMap(val interface{}) (map[string]interface{}, bool) {
	if mv, ok := val.(map[string]interface{}); ok {
		return mv, true
	}
	if mv, ok := val.(map[string]interface{}); ok {
		return mv, true
	}
	return nil, false
}

// inheritFrom ensures only plain string-keyed maps are propagated upward so
// that nested merges operate on consistent data structures (no []*T etc.).
func inheritFrom(val interface{}) interface{} {
	if sv, ok := val.(map[string]interface{}); ok {
		copied := make(map[string]interface{})
		for k, v := range sv {
			copied[k] = inheritFrom(v)
		}
		return copied
	}
	return val
}
