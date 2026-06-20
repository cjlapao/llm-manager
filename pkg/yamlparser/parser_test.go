package yamlparser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseYAML_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `slug: test-model
name: "Test Model"
engine: vllm
hf_repo: "test/repo"
container: test-container
port: 8080
`
	path := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	y, err := ParseYAML(path)
	if err != nil {
		t.Fatalf("ParseYAML() error: %v", err)
	}

	if y.Slug != "test-model" {
		t.Errorf("Slug = %q, want %q", y.Slug, "test-model")
	}
	if y.Name != "Test Model" {
		t.Errorf("Name = %q, want %q", y.Name, "Test Model")
	}
	if y.Engine != "vllm" {
		t.Errorf("Engine = %q, want %q", y.Engine, "vllm")
	}
	if y.HFRepo != "test/repo" {
		t.Errorf("HFRepo = %q, want %q", y.HFRepo, "test/repo")
	}
	if y.Container != "test-container" {
		t.Errorf("Container = %q, want %q", y.Container, "test-container")
	}
	if y.Port != 8080 {
		t.Errorf("Port = %d, want %d", y.Port, 8080)
	}
}

func TestParseYAML_MissingFile(t *testing.T) {
	_, err := ParseYAML("/nonexistent/path/to/file.yaml")
	if err == nil {
		t.Error("ParseYAML() with missing file should return error")
	}
}

func TestParseYAML_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	invalidContent := `slug: test
name: [unclosed bracket`
	path := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(path, []byte(invalidContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := ParseYAML(path)
	if err == nil {
		t.Error("ParseYAML() with invalid YAML should return error")
	}
}

func TestValidate_MissingSlug(t *testing.T) {
	y := &ModelYAML{
		Name:   "Test",
		Engine: "vllm",
		Port:   8080,
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate() with missing slug should return error")
	}
}

func TestValidate_InvalidSlug(t *testing.T) {
	tests := []string{
		"-invalid",  // starts with hyphen
		"_invalid",  // starts with underscore
		"Bad",       // starts with uppercase
		"has space", // contains space
		"has/dash",  // contains slash
		"",          // empty
	}

	for _, slug := range tests {
		y := &ModelYAML{
			Slug:   slug,
			Name:   "Test",
			Engine: "vllm",
			Port:   8080,
		}
		errs := Validate(y)
		found := false
		for _, e := range errs {
			if e != nil && e.Error() != "" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Validate(slug=%q) should return error, got no errors", slug)
		}
	}
}

func TestValidate_MissingName(t *testing.T) {
	y := &ModelYAML{
		Slug:   "valid-slug",
		Engine: "vllm",
		Port:   8080,
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate() with missing name should return error")
	}
}


func TestValidate_MissingEngine(t *testing.T) {
	y := &ModelYAML{
		Slug: "valid-slug",
		Name: "Test",
		Port: 8080,
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate() with missing engine should return error")
	}
}

func TestValidate_PortOutOfRange(t *testing.T) {
	tests := []int{0, -1, 65536, 100000}
	for _, port := range tests {
		y := &ModelYAML{
			Slug:   "valid-slug",
			Name:   "Test",
			Engine: "vllm",
			Port:   port,
		}
		errs := Validate(y)
		found := false
		for _, e := range errs {
			if e != nil && e.Error() != "" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Validate(port=%d) should return error, got no errors", port)
		}
	}
}

func TestValidate_NegativeCost(t *testing.T) {
	neg := -0.001
	y := &ModelYAML{
		Slug:            "valid-slug",
		Name:            "Test",
		Engine:          "vllm",
		Port:            8080,
		InputTokenCost:  &neg,
		OutputTokenCost: &neg,
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate() with negative costs should return error")
	}
}

func TestValidate_ValidAllFields(t *testing.T) {
	inputCost := 0.0000003
	outputCost := 0.0000004
	y := &ModelYAML{
		Slug:            "qwen3-next",
		Name:            "Qwen3-Next 80B-A3B",
		Engine:          "vllm",
		HFRepo:          "Qwen/Qwen3-Next-80B-A3B-Instruct",
		Container:       "llm-qwen3-next",
		Port:            8017,
		EnvVars:         map[string]string{"HUGGING_FACE_HUB_TOKEN": "${HF_TOKEN}", "VLLM_HOST": "0.0.0.0"},
		CommandArgs:     []string{"--model", "Qwen/Qwen3-Next-80B-A3B-Instruct", "--max-model-len", "131072"},
		InputTokenCost:  &inputCost,
		OutputTokenCost: &outputCost,
		Capabilities:    []string{"reasoning", "tool-use", "multi-turn"},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate() with valid YAML returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_EnvVarsAndCommandArgs(t *testing.T) {
	y := &ModelYAML{
		Slug:   "test-model",
		Name:   "Test",
		Engine: "sglang",
		Port:   8000,
		EnvVars: map[string]string{
			"KEY1": "value1",
			"KEY2": "value2",
		},
		CommandArgs: []string{"--arg1", "val1", "--arg2", "val2"},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate() with env/command maps returned %d errors: %v", len(errs), errs)
	}
	if len(y.EnvVars) != 2 {
		t.Errorf("EnvVars has %d entries, want 2", len(y.EnvVars))
	}
	if len(y.CommandArgs) != 4 {
		t.Errorf("CommandArgs has %d entries, want 4", len(y.CommandArgs))
	}
}

func TestValidate_Capabilities(t *testing.T) {
	y := &ModelYAML{
		Slug:         "test-model",
		Name:         "Test",
		Engine:       "vllm",
		Port:         8080,
		Capabilities: []string{"reasoning", "tool-use", "multi-turn"},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate() with capabilities returned %d errors: %v", len(errs), errs)
	}
	if len(y.Capabilities) != 3 {
		t.Errorf("Capabilities has %d entries, want 3", len(y.Capabilities))
	}
}

func TestParseYAML_FullExample(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `slug: qwen3-next
name: "Qwen3-Next 80B-A3B"
engine: vllm
hf_repo: "Qwen/Qwen3-Next-80B-A3B-Instruct"
container: llm-qwen3-next
port: 8017

environment:
  HUGGING_FACE_HUB_TOKEN: "${HF_TOKEN}"
  VLLM_HOST: "0.0.0.0"

command:
  - "--model Qwen/Qwen3-Next-80B-A3B-Instruct"
  - "-served-model-name qwen3-next"
  - "-max-model-len 131072"
  - "-kv-cache-dtype fp8"
  - "-gpu-memory-utilization 0.78"

input_token_cost: 0.0000003
output_token_cost: 0.0000004

capabilities:
  - reasoning
  - tool-use
  - multi-turn
`
	path := filepath.Join(tmpDir, "model-import.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	y, err := ParseYAML(path)
	if err != nil {
		t.Fatalf("ParseYAML() error: %v", err)
	}

	if y.Slug != "qwen3-next" {
		t.Errorf("Slug = %q, want %q", y.Slug, "qwen3-next")
	}
	if y.Name != "Qwen3-Next 80B-A3B" {
		t.Errorf("Name = %q, want %q", y.Name, "Qwen3-Next 80B-A3B")
	}
	if y.Engine != "vllm" {
		t.Errorf("Engine = %q, want %q", y.Engine, "vllm")
	}
	if y.HFRepo != "Qwen/Qwen3-Next-80B-A3B-Instruct" {
		t.Errorf("HFRepo = %q, want %q", y.HFRepo, "Qwen/Qwen3-Next-80B-A3B-Instruct")
	}
	if y.Container != "llm-qwen3-next" {
		t.Errorf("Container = %q, want %q", y.Container, "llm-qwen3-next")
	}
	if y.Port != 8017 {
		t.Errorf("Port = %d, want %d", y.Port, 8017)
	}
	if len(y.EnvVars) != 2 {
		t.Errorf("EnvVars has %d entries, want 2", len(y.EnvVars))
	}
	if y.EnvVars["HUGGING_FACE_HUB_TOKEN"] != "${HF_TOKEN}" {
		t.Errorf("EnvVars[HUGGING_FACE_HUB_TOKEN] = %q, want %q", y.EnvVars["HUGGING_FACE_HUB_TOKEN"], "${HF_TOKEN}")
	}
	if y.EnvVars["VLLM_HOST"] != "0.0.0.0" {
		t.Errorf("EnvVars[VLLM_HOST] = %q, want %q", y.EnvVars["VLLM_HOST"], "0.0.0.0")
	}
	if len(y.CommandArgs) != 5 {
		t.Errorf("CommandArgs has %d entries, want 5", len(y.CommandArgs))
	}
	foundModel := false
	for _, arg := range y.CommandArgs {
		if strings.Contains(arg, "--model") && strings.Contains(arg, "Qwen/Qwen3-Next-80B-A3B-Instruct") {
			foundModel = true
		}
	}
	if !foundModel {
		t.Errorf("CommandArgs missing --model Qwen/Qwen3-Next-80B-A3B-Instruct, got %v", y.CommandArgs)
	}
	foundKVCache := false
	for _, arg := range y.CommandArgs {
		if strings.Contains(arg, "kv-cache-dtype") && strings.Contains(arg, "fp8") {
			foundKVCache = true
		}
	}
	if !foundKVCache {
		t.Errorf("CommandArgs missing kv-cache-dtype fp8, got %v", y.CommandArgs)
	}

	inputCost := 0.0000003
	outputCost := 0.0000004
	if y.InputTokenCost == nil || *y.InputTokenCost != inputCost {
		t.Errorf("InputTokenCost = %v, want %v", y.InputTokenCost, inputCost)
	}
	if y.OutputTokenCost == nil || *y.OutputTokenCost != outputCost {
		t.Errorf("OutputTokenCost = %v, want %v", y.OutputTokenCost, outputCost)
	}
	if len(y.Capabilities) != 3 {
		t.Errorf("Capabilities has %d entries, want 3", len(y.Capabilities))
	}
	if y.Capabilities[0] != "reasoning" {
		t.Errorf("Capabilities[0] = %q, want %q", y.Capabilities[0], "reasoning")
	}

	// Full validation should pass
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate() returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_SglangEngine(t *testing.T) {
	y := &ModelYAML{
		Slug:   "sglang-model",
		Name:   "SGLang Model",
		Engine: "sglang",
		Port:   8000,
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(sglang) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_ZeroCosts(t *testing.T) {
	y := &ModelYAML{
		Slug:            "zero-cost",
		Name:            "Zero Cost Model",
		Engine:          "vllm",
		Port:            8080,
		InputTokenCost:  floatPtr(0),
		OutputTokenCost: floatPtr(0),
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(0 costs) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_PortBoundaries(t *testing.T) {
	// Port 1 should be valid
	y1 := &ModelYAML{Slug: "p1", Name: "P1", Engine: "vllm", Port: 1}
	errs1 := Validate(y1)
	if len(errs1) != 0 {
		t.Errorf("Validate(port=1) returned %d errors: %v", len(errs1), errs1)
	}

	// Port 65535 should be valid
	y65535 := &ModelYAML{Slug: "p65535", Name: "P65535", Engine: "vllm", Port: 65535}
	errs65535 := Validate(y65535)
	if len(errs65535) != 0 {
		t.Errorf("Validate(port=65535) returned %d errors: %v", len(errs65535), errs65535)
	}
}

func TestSlugRegex(t *testing.T) {
	validSlugs := []string{
		"a",
		"qwen3_6",
		"my-model",
		"model_123",
		"a-b-c",
		"a_b_c",
		"123",
		"a1b2c3",
		"has.dot",       // dots are allowed
		"model.name.v2", // multiple dots
	}
	for _, slug := range validSlugs {
		if !slugRegex.MatchString(slug) {
			t.Errorf("slug %q should be valid", slug)
		}
	}

	invalidSlugs := []string{
		"-start-hyphen",
		"_start-underscore",
		"Start-Caps",
		"has space",
		"has/slash",
		"@start-symbol",
	}
	for _, slug := range invalidSlugs {
		if slugRegex.MatchString(slug) {
			t.Errorf("slug %q should be invalid", slug)
		}
	}
}

func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func strPtr(s string) *string {
	return &s
}

// --- Profile validation tests ---

func TestValidate_ProfileValid(t *testing.T) {
	totalParams := 35.0
	activeParams := 3.0
	quantBytes := 1.0
	y := &ModelYAML{
		Slug:   "valid-model",
		Name:   "Valid Model",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			TotalParamsB:         &totalParams,
			ActiveParamsB:        &activeParams,
			IsMoe:                boolPtr(true),
			AttentionLayers:      intPtr(10),
			GdnLayers:            intPtr(30),
			NumKvHeads:           intPtr(2),
			HeadDim:              intPtr(256),
			SupportsMtp:          boolPtr(true),
			DefaultContext:       intPtr(262144),
			MaxContext:           intPtr(262144),
			QuantBytesPerParam:   &quantBytes,
			MaxNumSeqs:           intPtr(64),
			MaxNumBatchedTokens:  intPtr(8192),
			SpeculativeDecoding:  strPtr("mtp"),
			NumSpeculativeTokens: intPtr(4),
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(valid profile) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_ProfileParseYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `slug: test-model
name: "Test Model"
engine: vllm
port: 8080

profile:
  total_params_b: 35
  active_params_b: 3
  is_moe: true
  attention_layers: 10
  gdn_layers: 30
  num_kv_heads: 2
  head_dim: 256
  supports_mtp: true
  default_context: 262144
  max_context: 262144
  quant_bytes_per_param: 1.0
  max_num_seqs: 64
  max_num_batched_tokens: 8192
  speculative_decoding: mtp
  num_speculative_tokens: 4
`
	path := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	y, err := ParseYAML(path)
	if err != nil {
		t.Fatalf("ParseYAML() error: %v", err)
	}

	if y.Profile == nil {
		t.Fatal("Profile is nil, expected non-nil")
	}
	if y.Profile.TotalParamsB == nil || *y.Profile.TotalParamsB != 35.0 {
		t.Errorf("TotalParamsB = %v, want 35.0", y.Profile.TotalParamsB)
	}
	if y.Profile.ActiveParamsB == nil || *y.Profile.ActiveParamsB != 3.0 {
		t.Errorf("ActiveParamsB = %v, want 3.0", y.Profile.ActiveParamsB)
	}
	if y.Profile.IsMoe == nil || *y.Profile.IsMoe != true {
		t.Errorf("IsMoe = %v, want true", y.Profile.IsMoe)
	}
	if y.Profile.AttentionLayers == nil || *y.Profile.AttentionLayers != 10 {
		t.Errorf("AttentionLayers = %v, want 10", y.Profile.AttentionLayers)
	}
	if y.Profile.GdnLayers == nil || *y.Profile.GdnLayers != 30 {
		t.Errorf("GdnLayers = %v, want 30", y.Profile.GdnLayers)
	}
	if y.Profile.NumKvHeads == nil || *y.Profile.NumKvHeads != 2 {
		t.Errorf("NumKvHeads = %v, want 2", y.Profile.NumKvHeads)
	}
	if y.Profile.HeadDim == nil || *y.Profile.HeadDim != 256 {
		t.Errorf("HeadDim = %v, want 256", y.Profile.HeadDim)
	}
	if y.Profile.SupportsMtp == nil || *y.Profile.SupportsMtp != true {
		t.Errorf("SupportsMtp = %v, want true", y.Profile.SupportsMtp)
	}
	if y.Profile.DefaultContext == nil || *y.Profile.DefaultContext != 262144 {
		t.Errorf("DefaultContext = %v, want 262144", y.Profile.DefaultContext)
	}
	if y.Profile.MaxContext == nil || *y.Profile.MaxContext != 262144 {
		t.Errorf("MaxContext = %v, want 262144", y.Profile.MaxContext)
	}
	if y.Profile.QuantBytesPerParam == nil || *y.Profile.QuantBytesPerParam != 1.0 {
		t.Errorf("QuantBytesPerParam = %v, want 1.0", y.Profile.QuantBytesPerParam)
	}
	if y.Profile.MaxNumSeqs == nil || *y.Profile.MaxNumSeqs != 64 {
		t.Errorf("MaxNumSeqs = %v, want 64", y.Profile.MaxNumSeqs)
	}
	if y.Profile.MaxNumBatchedTokens == nil || *y.Profile.MaxNumBatchedTokens != 8192 {
		t.Errorf("MaxNumBatchedTokens = %v, want 8192", y.Profile.MaxNumBatchedTokens)
	}
	if y.Profile.SpeculativeDecoding == nil || *y.Profile.SpeculativeDecoding != "mtp" {
		t.Errorf("SpeculativeDecoding = %v, want mtp", y.Profile.SpeculativeDecoding)
	}
	if y.Profile.NumSpeculativeTokens == nil || *y.Profile.NumSpeculativeTokens != 4 {
		t.Errorf("NumSpeculativeTokens = %v, want 4", y.Profile.NumSpeculativeTokens)
	}

	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate() returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_ProfileNegativeTotalParams(t *testing.T) {
	neg := -1.0
	y := &ModelYAML{
		Slug:   "neg-params",
		Name:   "Neg Params",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			TotalParamsB: &neg,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(negative total_params_b) should return error")
	}
	found := false
	for _, e := range errs {
		if e != nil && strings.Contains(e.Error(), "total_params_b") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected error about total_params_b, got: %v", errs)
	}
}

func TestValidate_ProfileNegativeActiveParams(t *testing.T) {
	// -1.0 is a sentinel (allowed for non-MoE models); test with -2.0 which is invalid
	neg := -2.0
	y := &ModelYAML{
		Slug:   "neg-active",
		Name:   "Neg Active",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			ActiveParamsB: &neg,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(negative active_params_b) should return error")
	}
	found := false
	for _, e := range errs {
		if e != nil && strings.Contains(e.Error(), "active_params_b") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected error about active_params_b, got: %v", errs)
	}
}

func TestValidate_ProfileActiveParamsBSentinel(t *testing.T) {
	// -1.0 is a sentinel value meaning "active params equals total params";
	// used for non-MoE models where all params are active. Should NOT error.
	sentinel := -1.0
	y := &ModelYAML{
		Slug:   "sentinel-active",
		Name:   "Sentinel Active",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			ActiveParamsB: &sentinel,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(active_params_b=-1.0 sentinel) should NOT return error, got: %v", errs)
	}
}

func TestValidate_ProfileNegativeAttentionLayers(t *testing.T) {
	neg := -1
	y := &ModelYAML{
		Slug:   "neg-attention",
		Name:   "Neg Attention",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			AttentionLayers: &neg,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(negative attention_layers) should return error")
	}
}

func TestValidate_ProfileNegativeGdnLayers(t *testing.T) {
	neg := -1
	y := &ModelYAML{
		Slug:   "neg-gdn",
		Name:   "Neg GDN",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			GdnLayers: &neg,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(negative gdn_layers) should return error")
	}
}

func TestValidate_ProfileZeroNumKvHeads(t *testing.T) {
	// Zero is valid for encoder models (embeddings, rerankers) that have no attention layers.
	// Only negative values should be rejected.
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-kv",
		Name:   "Zero KV",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			NumKvHeads: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(zero num_kv_heads) should not return error for encoder models, got: %v", errs)
	}

	// Negative should still be rejected
	neg := -1
	yNeg := &ModelYAML{
		Slug:   "neg-kv",
		Name:   "Neg KV",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			NumKvHeads: &neg,
		},
	}
	errNeg := Validate(yNeg)
	if len(errNeg) == 0 {
		t.Error("Validate(negative num_kv_heads) should return error")
	}
}

func TestValidate_ProfileZeroHeadDim(t *testing.T) {
	// Zero is valid for encoder models (embeddings, rerankers) that have no attention layers.
	// Only negative values should be rejected.
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-head",
		Name:   "Zero Head",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			HeadDim: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(zero head_dim) should not return error for encoder models, got: %v", errs)
	}

	// Negative should still be rejected
	neg := -1
	yNeg := &ModelYAML{
		Slug:   "neg-head",
		Name:   "Neg Head",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			HeadDim: &neg,
		},
	}
	errNeg := Validate(yNeg)
	if len(errNeg) == 0 {
		t.Error("Validate(negative head_dim) should return error")
	}
}

func TestValidate_ProfileZeroDefaultContext(t *testing.T) {
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-context",
		Name:   "Zero Context",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			DefaultContext: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(zero default_context) should return error")
	}
}

func TestValidate_ProfileZeroMaxContext(t *testing.T) {
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-maxctx",
		Name:   "Zero MaxCtx",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			MaxContext: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(zero max_context) should return error")
	}
}

func TestValidate_ProfileInvalidQuantBytes(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		field string
	}{
		{"invalid_quant_3", 3.0, "quant_bytes_per_param"},
		{"invalid_quant_0", 0.0, "quant_bytes_per_param"},
		{"invalid_quant_neg", -1.0, "quant_bytes_per_param"},
		{"invalid_quant_1_5", 1.5, "quant_bytes_per_param"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			y := &ModelYAML{
				Slug:   "bad-quant",
				Name:   "Bad Quant",
				Engine: "vllm",
				Port:   8080,
				Profile: &ModelProfile{
					QuantBytesPerParam: &tc.value,
				},
			}
			errs := Validate(y)
			if len(errs) == 0 {
				t.Errorf("Validate(quant=%f) should return error", tc.value)
			}
			found := false
			for _, e := range errs {
				if e != nil && strings.Contains(e.Error(), tc.field) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected error about %s, got: %v", tc.field, errs)
			}
		})
	}
}

func TestValidate_ProfileValidQuantBytes(t *testing.T) {
	validValues := []float64{0.5, 1.0, 2.0}
	for _, v := range validValues {
		t.Run(fmt.Sprintf("quant_%s", strconv.FormatFloat(v, 'f', -1, 64)), func(t *testing.T) {
			y := &ModelYAML{
				Slug:   "good-quant",
				Name:   "Good Quant",
				Engine: "vllm",
				Port:   8080,
				Profile: &ModelProfile{
					QuantBytesPerParam: &v,
				},
			}
			errs := Validate(y)
			if len(errs) != 0 {
				t.Errorf("Validate(quant=%f) returned %d errors: %v", v, len(errs), errs)
			}
		})
	}
}

func TestValidate_ProfileZeroValuesValid(t *testing.T) {
	// attention_layers and gdn_layers can be 0 (>= 0)
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-layers",
		Name:   "Zero Layers",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			AttentionLayers: &zero,
			GdnLayers:       &zero,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(zero layers) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_BackwardCompatibilityNoProfile(t *testing.T) {
	y := &ModelYAML{
		Slug:   "no-profile",
		Name:   "No Profile Model",
		Engine: "vllm",
		Port:   8080,
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(no profile) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_BackwardCompatibilityParseYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `slug: legacy-model
name: "Legacy Model"
engine: vllm
port: 8080
`
	path := filepath.Join(tmpDir, "legacy.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	y, err := ParseYAML(path)
	if err != nil {
		t.Fatalf("ParseYAML() error: %v", err)
	}

	if y.Profile != nil {
		t.Error("Profile should be nil for YAML without profile block")
	}

	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(legacy yaml) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidateNonCapabilities_Profile(t *testing.T) {
	totalParams := 35.0
	y := &ModelYAML{
		Slug:   "valid-model",
		Name:   "Valid Model",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			TotalParamsB: &totalParams,
			IsMoe:        boolPtr(true),
		},
	}
	errs := ValidateNonCapabilities(y)
	if len(errs) != 0 {
		t.Errorf("ValidateNonCapabilities(valid profile) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidateNonCapabilities_ProfileInvalid(t *testing.T) {
	neg := -1.0
	y := &ModelYAML{
		Slug:   "bad-model",
		Name:   "Bad Model",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			TotalParamsB: &neg,
		},
	}
	errs := ValidateNonCapabilities(y)
	if len(errs) == 0 {
		t.Error("ValidateNonCapabilities(negative total_params_b) should return error")
	}
}

func TestValidate_ProfileZeroMaxNumSeqs(t *testing.T) {
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-maxseqs",
		Name:   "Zero MaxSeqs",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			MaxNumSeqs: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(zero max_num_seqs) should return error")
	}
}

func TestValidate_ProfileNegativeMaxNumSeqs(t *testing.T) {
	neg := -1
	y := &ModelYAML{
		Slug:   "neg-maxseqs",
		Name:   "Neg MaxSeqs",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			MaxNumSeqs: &neg,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(negative max_num_seqs) should return error")
	}
}

func TestValidate_ProfileZeroMaxNumBatchedTokens(t *testing.T) {
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-batch",
		Name:   "Zero Batch",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			MaxNumBatchedTokens: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(zero max_num_batched_tokens) should return error")
	}
}

func TestValidate_ProfileInvalidSpeculativeDecoding(t *testing.T) {
	invalid := "invalid_method"
	y := &ModelYAML{
		Slug:   "bad-spec",
		Name:   "Bad Spec",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			SpeculativeDecoding: &invalid,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(invalid speculative_decoding) should return error")
	}
	found := false
	for _, e := range errs {
		if e != nil && strings.Contains(e.Error(), "speculative_decoding") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected error about speculative_decoding, got: %v", errs)
	}
}

func TestValidate_ProfileValidSpeculativeDecoding(t *testing.T) {
	mtp := "mtp"
	y := &ModelYAML{
		Slug:   "good-spec",
		Name:   "Good Spec",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			SpeculativeDecoding: &mtp,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(valid speculative_decoding) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_ProfileValidDflashSpeculativeDecoding(t *testing.T) {
	dflash := "dflash"
	y := &ModelYAML{
		Slug:   "dflash-model",
		Name:   "DFlash Model",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			SpeculativeDecoding: &dflash,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(valid dflash speculative_decoding) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_ProfileEmptySpeculativeDecoding(t *testing.T) {
	empty := ""
	y := &ModelYAML{
		Slug:   "empty-spec",
		Name:   "Empty Spec",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			SpeculativeDecoding: &empty,
		},
	}
	errs := Validate(y)
	if len(errs) != 0 {
		t.Errorf("Validate(empty speculative_decoding) returned %d errors: %v", len(errs), errs)
	}
}

func TestValidate_ProfileZeroNumSpeculativeTokens(t *testing.T) {
	zero := 0
	y := &ModelYAML{
		Slug:   "zero-spec-tokens",
		Name:   "Zero Spec Tokens",
		Engine: "vllm",
		Port:   8080,
		Profile: &ModelProfile{
			NumSpeculativeTokens: &zero,
		},
	}
	errs := Validate(y)
	if len(errs) == 0 {
		t.Error("Validate(zero num_speculative_tokens) should return error")
	}
}

// TestParseYAML_HealthCheckFlatJSON verifies that a "healthcheck:" YAML block
// is extracted as FLAT JSON (no wrapper key), so BuildHealthcheckSection can
// render individual fields correctly. Regression test for the double-nesting
// bug where the whole map was dumped as Go text.
func TestParseYAML_HealthCheckFlatJSON(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `slug: qwen3-tts-1.7b
name: "Qwen3 TTS 1.7B"
engine: vllm
port: 8004
type: speech
subtype: tts
healthcheck:
  test: ["CMD", "curl", "-fsS", "http://localhost:8000/v1/models"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 600s
`
	path := filepath.Join(tmpDir, "model-import.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	y, err := ParseYAML(path)
	if err != nil {
		t.Fatalf("ParseYAML() error: %v", err)
	}

	if y.HealthCheckJSON == "" {
		t.Fatal("HealthCheckJSON should not be empty when healthcheck block is present")
	}

	// The JSON must NOT contain the wrapper key "healthcheck" mapping to an inner map.
	// It should be flat: {"test":[...],"interval":"30s",...}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(y.HealthCheckJSON), &parsed); err != nil {
		t.Fatalf("HealthCheckJSON is not valid JSON: %v\n%s", err, y.HealthCheckJSON)
	}

	expectedKeys := map[string]bool{
		"test":         true,
		"interval":     true,
		"timeout":      true,
		"retries":      true,
		"start_period": true,
	}
	for k := range expectedKeys {
		if _, ok := parsed[k]; !ok {
			t.Errorf("HealthCheckJSON missing expected key %q.\nJSON: %s", k, y.HealthCheckJSON)
		}
	}

	// Verify types match what docker-compose expects (BEFORE deleting keys)
	if val, ok := parsed["test"].([]interface{}); !ok || len(val) != 4 {
		t.Errorf("HealthCheck JSON 'test' should be []interface{} with 4 elements, got %#v", parsed["test"])
	}
	if val, ok := parsed["interval"].(string); !ok || val != "30s" {
		t.Errorf("HealthCheck JSON 'interval' should be string \"30s\", got type=%T value=%#v", parsed["interval"], parsed["interval"])
	}
	if val, ok := parsed["retries"].(float64); !ok || int(val) != 3 {
		t.Errorf("HealthCheck JSON 'retries' should be float64(3), got %#v", parsed["retries"])
	}

	delete(parsed, "test") // already verified above

	// Check there are exactly 5 keys — no more (no wrapper key).
	if len(parsed) != 4 {
		t.Errorf("Expected exactly 4 extra keys (after removing 'test'), got %d.\nJSON: %s", len(parsed), y.HealthCheckJSON)
	}
}
