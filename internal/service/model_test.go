package service

import (
	"os"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/pkg/yamlparser"
)

func newTestModelService(t *testing.T, openaiAPIURL string) *ModelService {
	t.Helper()
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = openaiAPIURL

	return NewModelService(db, cfg)
}

func TestBuildLiteLLMParams_OpenAIURLConstructsBase(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	yaml := &yamlparser.ModelYAML{
		Slug:         "test-model",
		Port:         8000,
		Capabilities: []string{},
	}

	params, err := svc.buildLiteLLMParams(yaml, "http://localhost:8000", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apiBase, ok := params["api_base"].(string)
	if !ok {
		t.Fatal("api_base not found or not a string")
	}
	if apiBase != "http://localhost:8000/v1" {
		t.Errorf("api_base = %q, want %q", apiBase, "http://localhost:8000/v1")
	}

	modelVal, ok := params["model"].(string)
	if !ok || modelVal != "test-model" {
		t.Errorf("model = %v, want %q", modelVal, "test-model")
	}
}

func TestBuildLiteLLMParams_RequiresOpenAI_URL(t *testing.T) {
	svc := newTestModelService(t, "")

	yaml := &yamlparser.ModelYAML{
		Slug:         "test-model",
		Port:         8000,
		Capabilities: []string{},
	}

	_, err := svc.buildLiteLLMParams(yaml, "", "test-model")
	if err == nil {
		t.Fatal("expected error when OPENAI_API_URL is empty and no api_base in YAML")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_URL must be set") {
		t.Errorf("error message = %q, expected to contain 'OPENAI_API_URL must be set'", err.Error())
	}
}

func TestBuildLiteLLMParams_YAMLApiBaseOverridesOpenAI(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://should-not-be-used.com"
	svc := NewModelService(nil, cfg)

	yaml := &yamlparser.ModelYAML{
		LiteLLMParams: map[string]interface{}{
			"api_base": "http://custom-api:9000/v1",
			"variant":  "some-variant",
		},
		Capabilities: []string{},
	}

	params, err := svc.buildLiteLLMParams(yaml, "http://should-not-be-used.com", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apiBase := params["api_base"].(string)
	if apiBase != "http://custom-api:9000/v1" {
		t.Errorf("api_base = %q, want %q (from YAML, not from OpenAIAPIURL)", apiBase, "http://custom-api:9000/v1")
	}

	// Variant should also be present
	if params["variant"] != "some-variant" {
		t.Errorf("variant = %v, want %q", params["variant"], "some-variant")
	}
}

func TestBuildLiteLLMParams_StripsCostFromLiteLLMParams(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	yaml := &yamlparser.ModelYAML{
		LiteLLMParams: map[string]interface{}{
			"input_cost_per_token":   0.01,
			"output_cost_per_token":  0.02,
			"model_name":             "gpt-4-custom",
		},
		Capabilities: []string{},
	}

	params, err := svc.buildLiteLLMParams(yaml, "http://localhost:8000", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, exists := params["input_cost_per_token"]; exists {
		t.Error("input_cost_per_token should have been removed from litellm_params")
	}
	if _, exists := params["output_cost_per_token"]; exists {
		t.Error("output_cost_per_token should have been removed from litellm_params")
	}

	// But model_name should remain
	if params["model_name"] != "gpt-4-custom" {
		t.Errorf("model_name = %v, want %q", params["model_name"], "gpt-4-custom")
	}

	modelVal := params["model"].(string)
	if modelVal != "test-model" {
		t.Errorf("model = %q, want %q", modelVal, "test-model")
	}
}

func TestBuildLiteLLMParams_YAMLModelNamePreserved(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	yaml := &yamlparser.ModelYAML{
		LiteLLMParams: map[string]interface{}{
			"model": "gpt-custom-4",
		},
		Capabilities: []string{},
	}

	params, err := svc.buildLiteLLMParams(yaml, "http://localhost:8000", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	modelVal := params["model"].(string)
	if modelVal != "gpt-custom-4" {
		t.Errorf("model = %q, want %q (YAML-provided name preserved)", modelVal, "gpt-custom-4")
	}
}

func TestBuildLiteLLMParams_TrailingSlashStripped(t *testing.T) {
	svc := newTestModelService(t, "http://example.com/")

	yaml := &yamlparser.ModelYAML{
		Slug:         "test-model",
		Port:         8000,
		Capabilities: []string{},
	}

	params, err := svc.buildLiteLLMParams(yaml, "http://example.com/", "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apiBase := params["api_base"].(string)
	if apiBase != "http://example.com/v1" {
		t.Errorf("api_base = %q, want %q (trailing slash stripped + /v1 appended)", apiBase, "http://example.com/v1")
	}
}

func TestImportModel_FailsWhenNoOpenAIAPIURL(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "" // explicitly empty
	svc := NewModelService(db, cfg)

	tmpDir := t.TempDir()
	yamlContent := `slug: test-import-fail
name: "Test Import Fail"
engine: vllm
hf_repo: "test/import-fail"
container: test-container
port: 8080

capabilities:
  - reasoning
`
	yamlPath := tmpDir + "/import.yaml"
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	_, importErr := svc.ImportModel(yamlPath, ImportOverrides{})
	if importErr == nil {
		t.Fatal("expected error when OPENAI_API_URL is not set")
	}
	if !strings.Contains(importErr.Error(), "OPENAI_API_URL must be set") {
		t.Errorf("error message = %q, expected to contain 'OPENAI_API_URL must be set'", importErr.Error())
	}
}
