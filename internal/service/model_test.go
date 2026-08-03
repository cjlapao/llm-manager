package service

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
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
			"input_cost_per_token":  0.01,
			"output_cost_per_token": 0.02,
			"model_name":            "gpt-4-custom",
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
	// Register known engine types so validateEngineAndVersion passes.
	for _, slug := range []string{"vllm", "sglang", "llama.cpp"} {
		if _, err := db.GetEngineTypeBySlug(slug); err != nil {
			nameMap := map[string]string{
				"vllm":      "vLLM",
				"sglang":    "Sglang",
				"llama.cpp": "llama.cpp",
			}
			et := models.EngineType{Slug: slug, Name: nameMap[slug]}
			db.CreateEngineType(&et)
		}
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

// kiloCodeTestModel returns a standard test model with coder + coder-thinking variants.
func kiloCodeTestModel(slug string) *models.Model {
	return &models.Model{
		Slug:            slug,
		Type:            "llm",
		Name:            "Test Model",
		Port:            8000,
		EngineType:      "vllm",
		Capabilities:    `["tool-use","reasoning","image"]`,
		LiteLLMParams:   `{"variants":{"coder":{"temperature":0.1},"coder-thinking":{"temperature":0.1,"extra_body":{"chat_template_kwargs":{"enable_thinking":true}}}},"api_base":"http://localhost:8000/v1","api_key":"test"}`,
		ModelInfo:       `{"input_tokens_limits":[262144],"output_token_limits":[32768]}`,
		InputTokenCost:  0.000003,
		OutputTokenCost: 0.000015,
	}
}

func TestGenerateKiloCodeModel_SingleModel(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	m := kiloCodeTestModel("kiloc-single")
	if err := svc.db.CreateModel(m); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	data, err := svc.GenerateKiloCodeModel("kiloc-single")
	if err != nil {
		t.Fatalf("GenerateKiloCodeModel() error: %v", err)
	}

	var wrapper struct {
		Models map[string]*KiloCodeModelEntry `json:"models"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if len(wrapper.Models) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(wrapper.Models))
	}

	// Check the base coder variant entry
	coder, ok := wrapper.Models["kiloc-single-coder"]
	if !ok {
		t.Fatal("missing entry for kiloc-single-coder")
	}
	if coder.Name != "Test Model Coder" {
		t.Errorf("coder.Name = %q, want %q", coder.Name, "Test Model Coder")
	}
	if coder.Limit == nil {
		t.Fatal("coder.Limit is nil")
	}
	if coder.Limit.Context != 262144 {
		t.Errorf("coder.Limit.Context = %d, want 262144", coder.Limit.Context)
	}
	if coder.Limit.Output != 32768 {
		t.Errorf("coder.Limit.Output = %d, want 32768", coder.Limit.Output)
	}
	if coder.Cost == nil {
		t.Fatal("coder.Cost is nil")
	}
	if coder.Cost.Input == nil || math.Abs(*coder.Cost.Input-3.0) > 0.001 {
		t.Errorf("coder.Cost.Input = %v, want ~3.0", coder.Cost.Input)
	}
	if coder.Cost.Output == nil || math.Abs(*coder.Cost.Output-15.0) > 0.001 {
		t.Errorf("coder.Cost.Output = %v, want ~15.0", coder.Cost.Output)
	}
	if !coder.ToolCall {
		t.Error("coder.ToolCall = false, want true")
	}
	if !coder.Temperature {
		t.Error("coder.Temperature = false, want true")
	}
	if coder.Modalities == nil {
		t.Fatal("coder.Modalities is nil")
	}
	if len(coder.Modalities["input"]) != 2 || coder.Modalities["input"][0] != "text" || coder.Modalities["input"][1] != "image" {
		t.Errorf("coder.Modalities[\"input\"] = %v, want [text image]", coder.Modalities["input"])
	}
	if len(coder.Modalities["output"]) != 1 || coder.Modalities["output"][0] != "text" {
		t.Errorf("coder.Modalities[\"output\"] = %v, want [text]", coder.Modalities["output"])
	}
	if coder.Reasoning {
		t.Error("coder.Reasoning = true, want false (model has no thinking capability)")
	}

	// Check the coder-thinking variant entry
	thinking, ok := wrapper.Models["kiloc-single-coder-thinking"]
	if !ok {
		t.Fatal("missing entry for kiloc-single-coder-thinking")
	}
	if thinking.Name != "Test Model Coder Thinking" {
		t.Errorf("thinking.Name = %q, want %q", thinking.Name, "Test Model Coder Thinking")
	}
	if thinking.ToolCall {
		// Should be true (same as coder)
	} else {
		t.Error("thinking.ToolCall = false, want true")
	}
}

func TestGenerateKiloCodeModels_AllModels(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	m1 := kiloCodeTestModel("kiloc-all-a")
	if err := svc.db.CreateModel(m1); err != nil {
		t.Fatalf("CreateModel(m1) error: %v", err)
	}
	m2 := kiloCodeTestModel("kiloc-all-b")
	m2.Name = "Second Model"
	if err := svc.db.CreateModel(m2); err != nil {
		t.Fatalf("CreateModel(m2) error: %v", err)
	}

	data, err := svc.GenerateKiloCodeModels()
	if err != nil {
		t.Fatalf("GenerateKiloCodeModels() error: %v", err)
	}

	var wrapper struct {
		Models map[string]*KiloCodeModelEntry `json:"models"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	expectedKeys := []string{
		"kiloc-all-a-coder",
		"kiloc-all-a-coder-thinking",
		"kiloc-all-b-coder",
		"kiloc-all-b-coder-thinking",
	}
	for _, key := range expectedKeys {
		if _, ok := wrapper.Models[key]; !ok {
			t.Errorf("missing entry for %q", key)
		}
	}
}

func TestGenerateKiloCode_ExcludesRAGEmbedRerankSpeech(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	excluded := []*models.Model{
		{Slug: "kiloc-rag", Type: "rag", Name: "RAG Model", Port: 8000, EngineType: "vllm",
			Capabilities: `["tool-use"]`, LiteLLMParams: `{"variants":{"coder":{}}}`, InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
		{Slug: "kiloc-embed", Type: "embed", Name: "Embed Model", Port: 8000, EngineType: "vllm",
			Capabilities: `["embedding"]`, LiteLLMParams: `{"variants":{"coder":{}}}`, InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
		{Slug: "kiloc-rerank", Type: "rerank", Name: "Rerank Model", Port: 8000, EngineType: "vllm",
			Capabilities: `["reranker"]`, LiteLLMParams: `{"variants":{"coder":{}}}`, InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
		{Slug: "kiloc-stt", SubType: "stt", Name: "STT Model", Port: 8000, EngineType: "vllm",
			Capabilities: `["stt"]`, LiteLLMParams: `{"variants":{"coder":{}}}`, InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
		{Slug: "kiloc-tts", SubType: "tts", Name: "TTS Model", Port: 8000, EngineType: "vllm",
			Capabilities: `["tts"]`, LiteLLMParams: `{"variants":{"coder":{}}}`, InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
	}
	for _, m := range excluded {
		if err := svc.db.CreateModel(m); err != nil {
			t.Fatalf("CreateModel(%q) error: %v", m.Slug, err)
		}
	}

	// Also add a valid model that should appear
	valid := kiloCodeTestModel("kiloc-valid")
	if err := svc.db.CreateModel(valid); err != nil {
		t.Fatalf("CreateModel(kiloc-valid) error: %v", err)
	}

	data, err := svc.GenerateKiloCodeModels()
	if err != nil {
		t.Fatalf("GenerateKiloCodeModels() error: %v", err)
	}

	var wrapper struct {
		Models map[string]*KiloCodeModelEntry `json:"models"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	excludedSlugs := []string{"kiloc-rag", "kiloc-embed", "kiloc-rerank", "kiloc-stt", "kiloc-tts"}
	for _, slug := range excludedSlugs {
		key := slug + "-coder"
		if _, ok := wrapper.Models[key]; ok {
			t.Errorf("excluded model %q appeared in output", key)
		}
	}

	if _, ok := wrapper.Models["kiloc-valid-coder"]; !ok {
		t.Error("valid model kiloc-valid-coder did not appear in output")
	}
}

func TestGenerateKiloCode_ReasoningFlag(t *testing.T) {
	svc := newTestModelService(t, "http://localhost:8000")

	// Model with thinking capability (name contains "thinking") + coder-thinking variant
	thinkModel := &models.Model{
		Slug:            "kiloc-reason-think",
		Type:            "llm",
		Name:            "Test Thinking Model",
		Port:            8000,
		EngineType:      "vllm",
		Capabilities:    `["tool-use","reasoning","image"]`,
		LiteLLMParams:   `{"variants":{"coder":{"temperature":0.1},"coder-thinking":{"temperature":0.1}}}`,
		ModelInfo:       `{"input_tokens_limits":[262144],"output_token_limits":[32768]}`,
		InputTokenCost:  0.000003,
		OutputTokenCost: 0.000015,
	}
	if err := svc.db.CreateModel(thinkModel); err != nil {
		t.Fatalf("CreateModel(thinkModel) error: %v", err)
	}

	data, err := svc.GenerateKiloCodeModel("kiloc-reason-think")
	if err != nil {
		t.Fatalf("GenerateKiloCodeModel() error: %v", err)
	}

	var wrapper struct {
		Models map[string]*KiloCodeModelEntry `json:"models"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	thinkingEntry, ok := wrapper.Models["kiloc-reason-think-coder-thinking"]
	if !ok {
		t.Fatal("missing entry for kiloc-reason-think-coder-thinking")
	}
	if !thinkingEntry.Reasoning {
		t.Error("thinkingEntry.Reasoning = false, want true (model has thinking capability + variant contains 'think')")
	}

	// Same model but with coder-fast variant — reasoning should be absent
	fastModel := &models.Model{
		Slug:            "kiloc-reason-fast",
		Type:            "llm",
		Name:            "Test Thinking Model",
		Port:            8000,
		EngineType:      "vllm",
		Capabilities:    `["tool-use","reasoning","image"]`,
		LiteLLMParams:   `{"variants":{"coder":{"temperature":0.1},"coder-fast":{"temperature":0.5}}}`,
		ModelInfo:       `{"input_tokens_limits":[262144],"output_token_limits":[32768]}`,
		InputTokenCost:  0.000003,
		OutputTokenCost: 0.000015,
	}
	if err := svc.db.CreateModel(fastModel); err != nil {
		t.Fatalf("CreateModel(fastModel) error: %v", err)
	}

	data2, err := svc.GenerateKiloCodeModel("kiloc-reason-fast")
	if err != nil {
		t.Fatalf("GenerateKiloCodeModel(fast) error: %v", err)
	}

	var wrapper2 struct {
		Models map[string]*KiloCodeModelEntry `json:"models"`
	}
	if err := json.Unmarshal(data2, &wrapper2); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	fastEntry, ok := wrapper2.Models["kiloc-reason-fast-coder-fast"]
	if !ok {
		t.Fatal("missing entry for kiloc-reason-fast-coder-fast")
	}
	if fastEntry.Reasoning {
		t.Error("fastEntry.Reasoning = true, want false (variant name does not contain 'think')")
	}

	// Verify "reasoning" key is truly absent from the JSON (not just false)
	var rawEntry map[string]interface{}
	json.Unmarshal([]byte(data2), &struct {
		Models map[string]json.RawMessage `json:"models"`
	}{})
	// Re-parse to get raw JSON for the fast entry
	var rawWrapper struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(data2, &rawWrapper); err != nil {
		t.Fatalf("failed to unmarshal raw: %v", err)
	}
	if rawFast, ok := rawWrapper.Models["kiloc-reason-fast-coder-fast"]; ok {
		if err := json.Unmarshal(rawFast, &rawEntry); err != nil {
			t.Fatalf("failed to unmarshal raw entry: %v", err)
		}
		if _, hasKey := rawEntry["reasoning"]; hasKey {
			t.Error("reasoning key should be absent for coder-fast variant")
		}
	}
}

func TestVariantToDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"coder-thinking", "Coder Thinking"},
		{"coder-fast", "Coder Fast"},
		{"coder", "Coder"},
	}
	for _, tt := range tests {
		got := variantToDisplayName(tt.input)
		if got != tt.want {
			t.Errorf("variantToDisplayName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
