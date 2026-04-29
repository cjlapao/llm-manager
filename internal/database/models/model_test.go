package models

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestModelTableName(t *testing.T) {
	m := Model{}
	if got := m.TableName(); got != "models" {
		t.Errorf("Model.TableName() = %q, want %q", got, "models")
	}
}

func TestModelBeforeCreate_GeneratesUUID(t *testing.T) {
	m := &Model{
		Slug: "test-model",
		Type: "llm",
		Name: "Test Model",
		Port: 8080,
	}

	err := m.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if m.ID == uuid.Nil {
		t.Error("BeforeCreate() did not generate a UUID")
	}
}

func TestModelBeforeCreate_PreservesExistingUUID(t *testing.T) {
	existingID := uuid.New()
	m := &Model{
		ID:   existingID,
		Slug: "test-model",
		Type: "llm",
		Name: "Test Model",
		Port: 8080,
	}

	err := m.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if m.ID != existingID {
		t.Errorf("BeforeCreate() changed existing UUID: got %v, want %v", m.ID, existingID)
	}
}

func TestModelBeforeCreate_NilTx(t *testing.T) {
	m := &Model{
		Slug: "test-model",
		Type: "llm",
		Name: "Test Model",
		Port: 8080,
	}

	// Should not panic with nil tx
	err := m.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate(nil) returned error: %v", err)
	}
}

func TestModelFields(t *testing.T) {
	tests := []struct {
		name string
		slug string
		typ  string
		nm   string
		hf   string
		yml  string
		ct   string
		port int
	}{
		{
			name: "llm model",
			slug: "qwen3_6",
			typ:  "llm",
			nm:   "Qwen3.6-35B-A3B",
			hf:   "Qwen/Qwen3.6-35B-A3B",
			yml:  "qwen3_6.yml",
			ct:   "llm-qwen36-35b-a3b",
			port: 8025,
		},
		{
			name: "embed model",
			slug: "qwen3-embedding",
			typ:  "embedding",
			nm:   "Qwen3-Embedding 0.6B",
			hf:   "Qwen/Qwen3-Embedding-0.6B",
			ct:   "llm-embed",
			port: 8020,
		},
		{
			name: "rerank model",
			slug: "qwen3-reranker",
			typ:  "reranker",
			nm:   "Qwen3-Reranker 0.6B",
			hf:   "Qwen/Qwen3-Reranker-0.6B",
			ct:   "llm-rerank",
			port: 8021,
		},
		{
			name: "autocomplete model",
			slug: "autocomplete",
			typ:  "autocomplete",
			nm:   "Autocomplete",
			hf:   "Qwen/Qwen2.5-Coder-1.5B",
			ct:   "llm-autocomplete",
			port: 8081,
		},
		{
			name: "draft model",
			slug: "qwen3_5-2b",
			typ:  "draft",
			nm:   "Qwen3.5-2B (draft)",
			hf:   "Qwen/Qwen3.5-2B",
			port: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				Slug:      tt.slug,
				Type:      tt.typ,
				Name:      tt.nm,
				HFRepo:    tt.hf,
				YML:       tt.yml,
				Container: tt.ct,
				Port:      tt.port,
			}

			if m.Slug != tt.slug {
				t.Errorf("Slug = %q, want %q", m.Slug, tt.slug)
			}
			if m.Type != tt.typ {
				t.Errorf("Type = %q, want %q", m.Type, tt.typ)
			}
			if m.Name != tt.nm {
				t.Errorf("Name = %q, want %q", m.Name, tt.nm)
			}
			if m.Port != tt.port {
				t.Errorf("Port = %d, want %d", m.Port, tt.port)
			}
		})
	}
}

func TestModelBeforeCreate_WithGormDB(t *testing.T) {
	m := &Model{
		Slug: "test-model",
		Type: "llm",
		Name: "Test Model",
		Port: 8080,
	}

	// Create a mock *gorm.DB — we only need it to not be nil for the test
	db, err := gorm.Open(nil, nil)
	if err != nil {
		t.Skipf("Skipping test: could not open mock DB: %v", err)
	}

	err = m.BeforeCreate(db)
	if err != nil {
		t.Fatalf("BeforeCreate() returned error: %v", err)
	}

	if m.ID == uuid.Nil {
		t.Error("BeforeCreate() did not generate a UUID")
	}
}

// ---------------------------------------------------------------------------
// Variant feature regression tests.
// ---------------------------------------------------------------------------

func TestModel_HasVariants(t *testing.T) {
	// No variants key → false
	m := &Model{Slug: "test-basic", LiteLLMParams: `{"temperature": 0.7}`}
	if m.HasVariants() {
		t.Fatal("expected HasVariants to be false for model without variants")
	}

	// Valid variants map → true
	sampleJSON := `{
      "temperature": 1.0,
      "top_p": 0.95,
      "variants": {
        "thinking": {"suffix": "-thinking", "temperature": 0.7},
        "instruct": {"suffix": "-instruct", "temperature": 0.9}
      }
    }`
	m2 := &Model{Slug: "test-with-variants", LiteLLMParams: sampleJSON}
	if !m2.HasVariants() {
		t.Fatal("expected HasVariants to be true for model with variants")
	}

	// Empty variants map → false
	m3 := &Model{Slug: "test-empty-variants", LiteLLMParams: `{"variants": {}}`}
	if m3.HasVariants() {
		t.Fatal("expected HasVariants to be false for empty variants map")
	}
}

func TestModel_GetLitellmVariantIDs(t *testing.T) {
	// Valid JSON deserializes correctly
	m := &Model{Slug: "test", LitellmVariantIDs: `{"thinking":"uuid-1","instruct":"uuid-2"}`}
	ids := m.GetLitellmVariantIDs()
	if ids == nil || len(ids) != 2 {
		t.Fatalf("expected 2 variant IDs, got %d", len(ids))
	}
	if ids["thinking"] != "uuid-1" {
		t.Errorf("expected thinking->uuid-1, got %s", ids["thinking"])
	}
	if ids["instruct"] != "uuid-2" {
		t.Errorf("expected instruct->uuid-2, got %s", ids["instruct"])
	}

	// Empty field returns nil
	m2 := &Model{Slug: "test-empty", LitellmVariantIDs: ""}
	ids2 := m2.GetLitellmVariantIDs()
	if ids2 != nil {
		t.Fatalf("expected nil for empty field, got %v", ids2)
	}

	// Invalid JSON — Unmarshal ignores errors on maps; returns empty non-nil map
	m3 := &Model{Slug: "test-invalid", LitellmVariantIDs: "not json"}
	ids3 := m3.GetLitellmVariantIDs()
	if ids3 == nil || len(ids3) != 0 {
		t.Fatalf("expected empty non-nil map for invalid JSON, got %v", ids3)
	}
}

func TestModel_SetLitellmVariantIDs(t *testing.T) {
	m := &Model{Slug: "test-set"}

	// Set valid map
	m.SetLitellmVariantIDs(map[string]string{"thinking": "uuid-a", "instruct": "uuid-b"})

	// Round-trip check
	ids := m.GetLitellmVariantIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs after set, got %d", len(ids))
	}
	if ids["thinking"] != "uuid-a" || ids["instruct"] != "uuid-b" {
		t.Errorf("round-trip mismatch: %+v", ids)
	}

	// Verify DB field contains valid JSON
	if m.LitellmVariantIDs == "" || !strings.Contains(m.LitellmVariantIDs, `"thinking"`) {
		t.Errorf("LitellmVariantIDs field should contain JSON: %q", m.LitellmVariantIDs)
	}

	// Setting nil clears to empty
	m2 := &Model{Slug: "test-clear"}
	m2.SetLitellmVariantIDs(nil)
	if m2.LitellmVariantIDs != "" {
		t.Errorf("expected empty string after nil set, got %q", m2.LitellmVariantIDs)
	}

	// Setting empty map also clears
	m3 := &Model{Slug: "test-clear-empty-map"}
	m3.SetLitellmVariantIDs(map[string]string{})
	if m3.LitellmVariantIDs != "" {
		t.Errorf("expected empty string after empty-map set, got %q", m3.LitellmVariantIDs)
	}
}

func TestModel_VariantSpec(t *testing.T) {
	sampleJSON := `{
      "temperature": 1.0,
      "top_p": 0.95,
      "variants": {
        "thinking": {
          "prefix": "-thinking",
          "temperature": 0.7,
          "extra_body": {
            "chat_template_kwargs": {
              "enable_thinking": true
            }
          }
        },
        "instruct": {
          "prefix": "-instruct",
          "temperature": 0.9,
          "top_p": 0.9
        }
      }
    }`
	m := &Model{Slug: "test-vspec", LiteLLMParams: sampleJSON}

	// existing variant → non-nil, correct overrides
	spec, ok := m.VariantSpec("thinking")
	if !ok || spec == nil {
		t.Fatal("VariantSpec returned nil for thinking")
	}
	if spec["temperature"] != 0.7 {
		t.Errorf("thinking temperature = %v, want 0.7", spec["temperature"])
	}
	if spec["prefix"] != "-thinking" {
		t.Errorf("thinking prefix = %v, want \"-thinking\"", spec["prefix"])
	}
	// extra_body chat_template_kwargs should be preserved
	ebr, ebrOk := spec["extra_body"].(map[string]interface{})
	if !ebrOk {
		t.Fatal("thinking spec missing extra_body map")
	}
	ctk, ctkOk := ebr["chat_template_kwargs"].(map[string]interface{})
	if !ctkOk {
		t.Fatal("extra_body missing chat_template_kwargs")
	}
	if ctk["enable_thinking"] != true {
		t.Errorf("enable_thinking = %v, want true", ctk["enable_thinking"])
	}

	// Also verify we get null back for nonexistent variant
	missing, hasMissing := m.VariantSpec("nonexistent")
	if hasMissing || missing != nil {
		t.Error("expected nil/false for missing variant")
	}
}

func TestModel_GetVariantKeys(t *testing.T) {
	sampleJSON := `{
      "temperature": 1.0,
      "variants": {
        "thinking": {"suffix": "-thinking"},
        "instruct": {"suffix": "-instruct"}
      }
    }`
	m := &Model{Slug: "test-keys", LiteLLMParams: sampleJSON}

	keys := m.GetVariantKeys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 variant keys, got %d", len(keys))
	}
	foundThinking := false
	foundInstruct := false
	for _, k := range keys {
		if k == "thinking" {
			foundThinking = true
		}
		if k == "instruct" {
			foundInstruct = true
		}
	}
	if !foundThinking || !foundInstruct {
		t.Errorf("expected both 'thinking' and 'instruct', got %v", keys)
	}

	// No variants → nil slice
	m2 := &Model{Slug: "test-no-keys", LiteLLMParams: `{"model": "foo"}`}
	nilKeys := m2.GetVariantKeys()
	if nilKeys != nil && len(nilKeys) > 0 {
		t.Errorf("expected nil for no variants, got %v", nilKeys)
	}
}
