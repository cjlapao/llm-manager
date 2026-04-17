package models

import (
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
			typ:  "embed",
			nm:   "Qwen3-Embedding 0.6B",
			hf:   "Qwen/Qwen3-Embedding-0.6B",
			ct:   "llm-embed",
			port: 8020,
		},
		{
			name: "rerank model",
			slug: "qwen3-reranker",
			typ:  "rerank",
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
