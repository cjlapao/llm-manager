package service

import (
	"os"
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

	params := svc.buildLiteLLMParams(yaml, "http://localhost:8000", "test-model", 8000)

	apiBase, ok := params["api_base"].(string)
	if !ok {
		t.Fatal("api_base not found or not a string")
	}
	if apiBase != "http://localhost:8000:8000/v1" {
		t.Errorf("api_base = %q, want %q", apiBase, "http://localhost:8000:8000/v1")
	}

	modelVal, ok := params["model"].(string)
	if !ok || modelVal != "test-model" {
		t.Errorf("model = %v, want %q", modelVal, "test-model")
	}
}

func TestBuildLiteLLMParams_NoOpenAIUrl(t *testing.T) {
	// When OPENAI_API_URL is empty and no api_base in YAML, buildLiteLLMParams
	// returns a map with only the auto-set model name (no api_base). This is
	// intentional — non-LLM models clear params afterwards, and LLM imports
	// can still proceed (they just won't have api_base set).
	svc := newTestModelService(t, "")

	yaml := &yamlparser.ModelYAML{
		Slug:         "test-model",
		Port:         8000,
		Capabilities: []string{},
	}

	params := svc.buildLiteLLMParams(yaml, "", "test-model", 8000)
	if params == nil {
		t.Fatal("expected non-nil params map")
	}
	// Should have auto-set model name
	if params["model"] != "test-model" {
		t.Errorf("model = %v, want %q", params["model"], "test-model")
	}
	// Should NOT have api_base when no OPENAI_API_URL
	if _, hasAPIBase := params["api_base"]; hasAPIBase {
		t.Error("unexpected api_base when OPENAI_API_URL is empty")
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

	params := svc.buildLiteLLMParams(yaml, "http://should-not-be-used.com", "test-model", 0)

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

	params := svc.buildLiteLLMParams(yaml, "http://localhost:8000", "test-model", 8000)

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

	params := svc.buildLiteLLMParams(yaml, "http://localhost:8000", "test-model", 8000)

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

	params := svc.buildLiteLLMParams(yaml, "http://example.com/", "test-model", 8000)

	apiBase := params["api_base"].(string)
	if apiBase != "http://example.com:8000/v1" {
		t.Errorf("api_base = %q, want %q (trailing slash stripped, port appended + /v1)", apiBase, "http://example.com:8000/v1")
	}
}

func TestImportModel_SucceedsWithoutOpenAIAPIURL(t *testing.T) {
	// ImportModel does not require OPENAI_API_URL — for non-LLM types the
	// LiteLLM params are cleared anyway, and for LLM types the import can
	// still proceed without api_base set.
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
	yamlContent := `slug: test-import-no-url
name: "Test Import No URL"
engine: vllm
hf_repo: "test/import-no-url"
container: test-container
port: 8080

capabilities:
  - reasoning
`
	yamlPath := tmpDir + "/import.yaml"
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	model, importErr := svc.ImportModel(yamlPath, ImportOverrides{})
	if importErr != nil {
		t.Fatalf("unexpected error: %v", importErr)
	}
	if model == nil {
		t.Fatal("expected non-nil model")
	}
	if model.Slug != "test-import-no-url" {
		t.Errorf("model.Slug = %q, want %q", model.Slug, "test-import-no-url")
	}
}
