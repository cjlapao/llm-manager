package api

import (
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func TestValidateFields_ValidFields(t *testing.T) {
	tests := []struct {
		name       string
		modelTable string
		fields     []string
		wantFields []string
		wantErr    bool
	}{
		{
			name:       "Model: single field",
			modelTable: "models",
			fields:     []string{"slug", "name"},
			wantFields: []string{"slug", "name"},
			wantErr:    false,
		},
		{
			name:       "Model: all fields",
			modelTable: "models",
			fields: []string{
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
			wantFields: []string{
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
			wantErr: false,
		},
		{
			name:       "Container: fields",
			modelTable: "containers",
			fields:     []string{"slug", "status", "port"},
			wantFields: []string{"slug", "status", "port"},
			wantErr:    false,
		},
		{
			name:       "BaseImage: fields",
			modelTable: "base_images",
			fields:     []string{"slug", "name", "docker_image"},
			wantFields: []string{"slug", "name", "docker_image"},
			wantErr:    false,
		},
		{
			name:       "empty fields returns nil (select all)",
			modelTable: "models",
			fields:     []string{},
			wantFields: nil,
			wantErr:    false,
		},
		{
			name:       "nil fields returns nil (select all)",
			modelTable: "models",
			fields:     nil,
			wantFields: nil,
			wantErr:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateFields(tc.modelTable, tc.fields)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.wantFields) {
				t.Fatalf("got %d fields, want %d: got=%v want=%v", len(got), len(tc.wantFields), got, tc.wantFields)
			}
			for i, f := range tc.wantFields {
				if got[i] != f {
					t.Errorf("[%d] got %q, want %q", i, got[i], f)
				}
			}
		})
	}
}

func TestValidateFields_UnknownFields(t *testing.T) {
	tests := []struct {
		name        string
		modelTable  string
		fields      []string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Model: unknown field",
			modelTable:  "models",
			fields:      []string{"slug", "nonexistent_field"},
			wantErr:     true,
			errContains: "nonexistent_field",
		},
		{
			name:        "Model: multiple unknown fields",
			modelTable:  "models",
			fields:      []string{"slug", "fake1", "fake2"},
			wantErr:     true,
			errContains: "fake1",
		},
		{
			name:        "Container: unknown field",
			modelTable:  "containers",
			fields:      []string{"slug", "unknown_col"},
			wantErr:     true,
			errContains: "unknown_col",
		},
		{
			name:        "Unknown model table",
			modelTable:  "unknown_table",
			fields:      []string{"any_field"},
			wantErr:     true,
			errContains: "no field white-list defined",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateFields(tc.modelTable, tc.fields)
			if !tc.wantErr {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if tc.errContains != "" && !containsSubstring(err.Error(), tc.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
			}
		})
	}
}

func TestValidateFields_ErrorMessages(t *testing.T) {
	_, err := ValidateFields("models", []string{"slug", "fake_field"})
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	msg := err.Error()
	if !containsSubstring(msg, "unsupported field(s)") {
		t.Errorf("error missing 'unsupported field(s)': %s", msg)
	}
	if !containsSubstring(msg, "fake_field") {
		t.Errorf("error missing unknown field name: %s", msg)
	}
	if !containsSubstring(msg, "allowed:") {
		t.Errorf("error missing 'allowed:' list: %s", msg)
	}
}

func TestValidateFields_CaseSensitivity(t *testing.T) {
	// Field names are case-sensitive — "Slug" should not match "slug"
	_, err := ValidateFields("models", []string{"Slug"})
	if err == nil {
		t.Error("expected error for uppercase field name 'Slug'")
	}
}

func TestValidateFields_AllModelTables(t *testing.T) {
	// Verify all registered tables have non-empty white-lists
	for tableName, wl := range FieldWhiteLists {
		if len(wl.Columns) == 0 {
			t.Errorf("FieldWhiteLists[%q] has empty column list", tableName)
		}
	}
}

func TestApplyFieldProjection_NoFields(t *testing.T) {
	db, err := gorm.Open(&noopDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	result := ApplyFieldProjection(db, nil)
	if result != db {
		t.Error("ApplyFieldProjection with nil should return same db")
	}

	result = ApplyFieldProjection(db, []string{})
	if result != db {
		t.Error("ApplyFieldProjection with empty slice should return same db")
	}
}

func TestApplyFieldProjection_WithFields(t *testing.T) {
	db, err := gorm.Open(&noopDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	result := ApplyFieldProjection(db, []string{"slug", "name"})
	// The result should have a Select clause set
	if result.Statement == nil {
		t.Fatal("result.Statement is nil")
	}
	if len(result.Statement.Selects) != 2 {
		t.Errorf("expected 2 selects, got %d: %v", len(result.Statement.Selects), result.Statement.Selects)
	}
	for i, expected := range []string{"slug", "name"} {
		if result.Statement.Selects[i] != expected {
			t.Errorf("select[%d]: got %q, want %q", i, result.Statement.Selects[i], expected)
		}
	}
}

func TestFieldWhiteListsExported(t *testing.T) {
	// Verify FieldWhiteLists is accessible and has expected keys
	expectedTables := []string{"models", "containers", "base_images", "hotspots", "config", "engine_types", "engine_versions"}
	for _, table := range expectedTables {
		if _, ok := FieldWhiteLists[table]; !ok {
			t.Errorf("FieldWhiteLists missing key %q", table)
		}
	}
}

func TestFieldWhiteList_Columns(t *testing.T) {
	// Verify Model white-list has the expected number of columns
	modelWL, ok := FieldWhiteLists["models"]
	if !ok {
		t.Fatal("models white-list not found")
	}
	if len(modelWL.Columns) != 40 {
		t.Errorf("models white-list has %d columns, expected 40", len(modelWL.Columns))
	}

	// Verify Container white-list
	containerWL, ok := FieldWhiteLists["containers"]
	if !ok {
		t.Fatal("containers white-list not found")
	}
	if len(containerWL.Columns) != 8 {
		t.Errorf("containers white-list has %d columns, expected 8", len(containerWL.Columns))
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// noopDialector is a minimal GORM dialector for testing without a real database.
type noopDialector struct{}

func (noopDialector) Name() string                                          { return "noop" }
func (noopDialector) Initialize(*gorm.DB) error                             { return nil }
func (noopDialector) BindVarTo(clause.Writer, *gorm.Statement, interface{}) {}
func (noopDialector) DataTypeOf(*schema.Field) string                       { return "" }
func (noopDialector) DefaultValueOf(*schema.Field) clause.Expression        { return nil }
func (noopDialector) QuoteTo(clause.Writer, string)                         {}
func (noopDialector) Explain(string, ...interface{}) string                 { return "" }
func (noopDialector) Migrator(*gorm.DB) gorm.Migrator                       { return nil }
