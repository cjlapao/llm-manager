// Package api provides the HTTP API server for llm-manager.
package api

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// FieldWhiteList defines the set of allowed fields for a given model type.
type FieldWhiteList struct {
	// Columns is the list of allowed database column names (as used in GORM
	// column tags or struct field names).
	Columns []string
}

// FieldWhiteLists holds the field white-lists keyed by model table name.
// This map is exported so new model types can register their field sets.
var FieldWhiteLists = map[string]FieldWhiteList{
	// Model table: "models"
	"models": {
		Columns: []string{
			"id", "slug", "type", "sub_type", "name", "hf_repo", "yml",
			"container", "port", "engine_type", "env_vars", "command_args",
			"input_token_cost", "output_token_cost", "capabilities",
			"lite_llm_params", "model_info", "litellm_model_id",
			"litellm_active_aliases", "litellm_variant_ids", "default",
			"base_image_id", "engine_version_slug", "total_params_b",
			"active_params_b", "is_moe", "attention_layers", "gdn_layers",
			"num_kv_heads", "head_dim", "supports_mtp", "default_context",
			"max_context", "quant_bytes_per_param", "max_num_seqs",
			"max_num_batched_tokens", "speculative_decoding",
			"num_speculative_tokens", "created_at", "updated_at",
		},
	},
	// Container table: "containers"
	"containers": {
		Columns: []string{
			"id", "slug", "name", "status", "port", "gpu_used",
			"created_at", "updated_at",
		},
	},
	// BaseImage table: "base_images"
	"base_images": {
		Columns: []string{
			"id", "slug", "name", "engine_type", "docker_image",
			"entrypoint", "environment_json", "volumes_json",
			"composed_yml_file", "created_at", "updated_at",
		},
	},
	// Hotspot table: "hotspots"
	"hotspots": {
		Columns: []string{
			"id", "model_slug", "active", "created_at", "updated_at",
		},
	},
	// Config table: "config"
	"config": {
		Columns: []string{
			"id", "key", "value", "created_at", "updated_at",
		},
	},
	// EngineType table: "engine_types"
	"engine_types": {
		Columns: []string{
			"id", "slug", "name", "description", "created_at", "updated_at",
		},
	},
	// EngineVersion table: "engine_versions"
	"engine_versions": {
		Columns: []string{
			"id", "slug", "engine_type_slug", "version", "is_default",
			"is_latest", "created_at", "updated_at",
		},
	},
}

// ValidateFields checks that every requested field is in the white-list for
// the given model table. It returns the validated field list or an error
// listing all unknown fields.
//
// An empty requestedFields slice returns nil — callers should treat this as
// "no projection needed" (select all columns).
func ValidateFields(modelTable string, requestedFields []string) ([]string, error) {
	if len(requestedFields) == 0 {
		return nil, nil
	}

	whiteList, ok := FieldWhiteLists[modelTable]
	if !ok {
		return nil, fmt.Errorf("no field white-list defined for model table %q", modelTable)
	}

	// Build a set for O(1) lookup
	allowed := make(map[string]struct{}, len(whiteList.Columns))
	for _, col := range whiteList.Columns {
		allowed[col] = struct{}{}
	}

	unknown := make([]string, 0, len(requestedFields))
	for _, field := range requestedFields {
		if _, ok := allowed[field]; !ok {
			unknown = append(unknown, field)
		}
	}

	if len(unknown) > 0 {
		return nil, fmt.Errorf("unsupported field(s): %s (allowed: %s)",
			strings.Join(unknown, ", "),
			strings.Join(whiteList.Columns, ", "))
	}

	return requestedFields, nil
}

// ApplyFieldProjection applies a GORM .Select() with the validated field list.
// If fields is nil or empty, the query is returned unchanged (select all columns).
func ApplyFieldProjection(db *gorm.DB, fields []string) *gorm.DB {
	if len(fields) == 0 {
		return db
	}
	return db.Select(fields)
}
