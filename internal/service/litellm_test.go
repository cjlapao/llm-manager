package service

import (
	"reflect"
	"testing"
)

func TestDeepMerge_ScalarReplacement(t *testing.T) {
	dst := map[string]interface{}{"a": "old"}
	src := map[string]interface{}{"a": "new"}
	result := DeepMerge(dst, src).(map[string]interface{})

	if result["a"] != "new" {
		t.Errorf("scalar replacement: got %v, want %v", result["a"], "new")
	}
	if len(result) != 1 {
		t.Errorf("scalar replacement: expected 1 key, got %d", len(result))
	}
}

func TestDeepMerge_AddsNewKeysFromSrc(t *testing.T) {
	dst := map[string]interface{}{"a": 1}
	src := map[string]interface{}{"b": 2}
	result := DeepMerge(dst, src).(map[string]interface{})

	if result["a"] != 1 || result["b"] != 2 {
		t.Errorf("added keys: got %+v, want {a: 1, b: 2}", result)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 keys, got %d", len(result))
	}
}

func TestDeepMerge_NestedMapsMergedRecursively(t *testing.T) {
	dst := map[string]interface{}{
		"x": map[string]interface{}{"y": 1},
	}
	src := map[string]interface{}{
		"x": map[string]interface{}{"z": 2},
	}
	result := DeepMerge(dst, src).(map[string]interface{})

	xMap, ok := result["x"].(map[string]interface{})
	if !ok {
		t.Fatalf("x should be map[string]interface{}, got %T", result["x"])
	}
	if xMap["y"] != 1 {
		t.Errorf("nested merge y: got %v, want 1", xMap["y"])
	}
	if xMap["z"] != 2 {
		t.Errorf("nested merge z: got %v, want 2", xMap["z"])
	}
	if len(xMap) != 2 {
		t.Errorf("nested merge: expected 2 keys under x, got %d", len(xMap))
	}
}

func TestDeepMerge_DeepNestingThreeLevels(t *testing.T) {
	dst := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "old",
			},
		},
	}
	src := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"d": "new",
			},
		},
	}
	result := DeepMerge(dst, src).(map[string]interface{})

	aMap := result["a"].(map[string]interface{})
	bMap := aMap["b"].(map[string]interface{})

	if bMap["c"] != "old" {
		t.Errorf("deep nesting c: got %v, want old", bMap["c"])
	}
	if bMap["d"] != "new" {
		t.Errorf("deep nesting d: got %v, want new", bMap["d"])
	}
	if len(bMap) != 2 {
		t.Errorf("deep nesting: expected 2 keys under a.b, got %d", len(bMap))
	}
}

func TestDeepMerge_LeafOverwriteWins(t *testing.T) {
	dst := map[string]interface{}{
		"a": map[string]interface{}{
			"b": "keep",
			"c": "old",
		},
	}
	src := map[string]interface{}{
		"a": map[string]interface{}{
			"c": "new",
		},
	}
	result := DeepMerge(dst, src).(map[string]interface{})

	aMap := result["a"].(map[string]interface{})
	if aMap["b"] != "keep" {
		t.Errorf("leaf overwrite sibling b: got %v, want keep", aMap["b"])
	}
	if aMap["c"] != "new" {
		t.Errorf("leaf overwrite c: got %v, want new", aMap["c"])
	}
}

func TestDeepMerge_NilSrcReturnsNil(t *testing.T) {
	dst := map[string]interface{}{
		"placeholder": true,
	}
	result := DeepMerge(dst, nil)

	if result != nil {
		t.Errorf("nil src: got %v, want nil", result)
	}
}

func TestDeepMerge_NonMapSrcReplacesDst(t *testing.T) {
	dst := map[string]interface{}{"x": 1}
	src := "string"
	result := DeepMerge(dst, src)

	if result != "string" {
		t.Errorf("non-map src: got %v, want string", result)
	}
}

func TestDeepMerge_RealWorldExtraBodyChatTemplateKwargs(t *testing.T) {
	dst := map[string]interface{}{
		"extra_body": map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"enable_thinking":   true,
				"max_prefix_tokens": 100,
				"other_value":       "preserved",
			},
		},
	}
	src := map[string]interface{}{
		"extra_body": map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"max_prefix_tokens": 200,
			},
		},
	}
	result := DeepMerge(dst, src).(map[string]interface{})

	ebr, ok := result["extra_body"].(map[string]interface{})
	if !ok {
		t.Fatal("extra_body should be map[string]interface{}")
	}

	ctk, ok := ebr["chat_template_kwargs"].(map[string]interface{})
	if !ok {
		t.Fatal("chat_template_kwargs should be map[string]interface{}")
	}

	if ctk["enable_thinking"] != true {
		t.Errorf("enable_thinking preserved: got %v, want true", ctk["enable_thinking"])
	}
	if ctk["max_prefix_tokens"] != 200 {
		t.Errorf("max_prefix_tokens overwritten: got %v, want 200", ctk["max_prefix_tokens"])
	}
	if ctk["other_value"] != "preserved" {
		t.Errorf("other_value preserved: got %v, want preserved", ctk["other_value"])
	}
	if len(ctk) != 3 {
		t.Errorf("chat_template_kwargs: expected 3 keys, got %d", len(ctk))
	}
}

// Table-driven tests for additional edge cases.

func TestDeepMerge_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		dst    interface{}
		src    interface{}
		expect interface{}
	}{
		{
			name:   "empty dst empty src",
			dst:    map[string]interface{}{},
			src:    map[string]interface{}{},
			expect: map[string]interface{}{},
		},
		{
			name:   "empty dst with src adds all keys",
			dst:    map[string]interface{}{},
			src:    map[string]interface{}{"a": 1, "b": 2},
			expect: map[string]interface{}{"a": 1, "b": 2},
		},
		{
			name:   "boolean passthrough true",
			dst:    map[string]interface{}{"flag": false},
			src:    map[string]interface{}{"flag": true},
			expect: map[string]interface{}{"flag": true},
		},
		{
			name:   "boolean passthrough false",
			dst:    map[string]interface{}{"flag": true},
			src:    map[string]interface{}{"flag": false},
			expect: map[string]interface{}{"flag": false},
		},
		{
			name:   "numeric scalar replacement",
			dst:    map[string]interface{}{"count": 10},
			src:    map[string]interface{}{"count": 42},
			expect: map[string]interface{}{"count": 42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeepMerge(tt.dst, tt.src)
			if !reflect.DeepEqual(result, tt.expect) {
				t.Errorf("got %#v, want %#v", result, tt.expect)
			}
		})
	}
}

func TestDeepMerge_EdgeCases_EmptyMapMerge(t *testing.T) {
	dst := map[string]interface{}{
		"a": map[string]interface{}{"x": 1},
	}

	// Merging with empty inner maps still works
	src := map[string]interface{}{
		"a": map[string]interface{}{},
	}
	result := DeepMerge(dst, src).(map[string]interface{})
	aMap := result["a"].(map[string]interface{})
	if len(aMap) != 1 || aMap["x"] != 1 {
		t.Errorf("empty overlay should preserve dst leaves: got %#v", aMap)
	}
}

func TestDeepMerge_NumberComparedCorrectly(t *testing.T) {
	dst := map[string]interface{}{
		"temperature": 1.0,
	}
	src := map[string]interface{}{
		"temperature": 0.7,
	}
	result := DeepMerge(dst, src).(map[string]interface{})

	if val, ok := result["temperature"].(float64); !ok || val != 0.7 {
		t.Errorf("number comparison: got %v, want 0.7", result["temperature"])
	}
}

// ─── buildDeploymentSpecs tests ───

func TestBuildDeploymentSpecs_ThinkingModel_BaseOnly(t *testing.T) {
	// buildDeploymentSpecs now returns only base + variants (no aliases).
	// Aliases are created separately by buildActiveSpecs.
	params := map[string]interface{}{
		"model":       "qwen3.6-35b-a3b-fp8",
		"temperature": 0.7,
	}
	minfo := map[string]interface{}{
		"id":   "model-123",
		"name": "Qwen 3.6",
	}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3.6-35b-a3b-fp8", true, nil, 0.0001, 0.0002, 0, 0, "", "", 0)
	if err != nil {
		t.Fatalf("buildDeploymentSpecs returned error: %v", err)
	}

	// Should have exactly 1 spec: base only (no aliases)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	base := specs[0]
	if base.Name != "qwen3.6-35b-a3b-fp8" {
		t.Errorf("base name: got %q, want %q", base.Name, "qwen3.6-35b-a3b-fp8")
	}
	if base.Type != "base" {
		t.Errorf("base type: got %q, want %q", base.Type, "base")
	}
}

func TestBuildActiveSpecs_ThinkingModel_DualAlias(t *testing.T) {
	// buildActiveSpecs should produce:
	//   1. active alias (thinking disabled)
	//   2. active-thinking alias (thinking enabled, preserve enabled)
	params := map[string]interface{}{
		"model":       "qwen3.6-35b-a3b-fp8",
		"temperature": 0.7,
	}
	minfo := map[string]interface{}{
		"id":   "model-123",
		"name": "Qwen 3.6",
	}

	specs := buildActiveSpecs("qwen3.6-35b-a3b-fp8", params, minfo, true)

	// Should have exactly 2 specs: active + active-thinking
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// 1. Active alias — thinking disabled
	active := specs[0]
	if active.Name != "active" {
		t.Errorf("active alias name: got %q, want %q", active.Name, "active")
	}
	if active.Type != "alias" {
		t.Errorf("active alias type: got %q, want %q", active.Type, "alias")
	}

	activeParams := active.Params["extra_body"].(map[string]interface{})
	kt := activeParams["chat_template_kwargs"].(map[string]interface{})
	if kt["enable_thinking"] != false {
		t.Errorf("active enable_thinking: got %v, want false", kt["enable_thinking"])
	}
	if kt["preserve_thinking"] != false {
		t.Errorf("active preserve_thinking: got %v, want false", kt["preserve_thinking"])
	}

	// 2. Active-thinking alias — thinking enabled
	thinkAlias := specs[1]
	if thinkAlias.Name != "active-thinking" {
		t.Errorf("active-thinking alias name: got %q, want %q", thinkAlias.Name, "active-thinking")
	}
	if thinkAlias.Type != "alias" {
		t.Errorf("active-thinking alias type: got %q, want %q", thinkAlias.Type, "alias")
	}

	thinkParams := thinkAlias.Params["extra_body"].(map[string]interface{})
	thinkKt := thinkParams["chat_template_kwargs"].(map[string]interface{})
	if thinkKt["enable_thinking"] != true {
		t.Errorf("active-thinking enable_thinking: got %v, want true", thinkKt["enable_thinking"])
	}
	if thinkKt["preserve_thinking"] != true {
		t.Errorf("active-thinking preserve_thinking: got %v, want true", thinkKt["preserve_thinking"])
	}
}

func TestBuildActiveSpecs_NonThinkingModel_SingleAlias(t *testing.T) {
	// A model without thinking capability should produce only the "active" alias.
	params := map[string]interface{}{
		"model":       "qwen3-coder-next-fp8",
		"temperature": 0.8,
	}
	minfo := map[string]interface{}{
		"id":   "model-456",
		"name": "Qwen Coder",
	}

	specs := buildActiveSpecs("qwen3-coder-next-fp8", params, minfo, false)

	// Should have exactly 1 spec: active only (no active-thinking)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	active := specs[0]
	if active.Name != "active" {
		t.Errorf("active alias name: got %q, want %q", active.Name, "active")
	}
	if active.Type != "alias" {
		t.Errorf("active alias type: got %q, want %q", active.Type, "alias")
	}

	activeParams := active.Params["extra_body"].(map[string]interface{})
	kt := activeParams["chat_template_kwargs"].(map[string]interface{})
	if kt["enable_thinking"] != false {
		t.Errorf("active enable_thinking: got %v, want false", kt["enable_thinking"])
	}
	if kt["preserve_thinking"] != false {
		t.Errorf("active preserve_thinking: got %v, want false", kt["preserve_thinking"])
	}

	// Verify no active-thinking alias exists
	for _, sp := range specs {
		if sp.Name == "active-thinking" {
			t.Errorf("unexpected active-thinking alias found in non-thinking model specs")
		}
	}
}

func TestBuildDeploymentSpecs_NonThinkingModel_BaseOnly(t *testing.T) {
	// buildDeploymentSpecs now returns only base (no aliases).
	params := map[string]interface{}{
		"model":       "qwen3-coder-next-fp8",
		"temperature": 0.8,
	}
	minfo := map[string]interface{}{
		"id":   "model-456",
		"name": "Qwen Coder",
	}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3-coder-next-fp8", false, nil, 0.00005, 0.0001, 0, 0, "", "", 0)
	if err != nil {
		t.Fatalf("buildDeploymentSpecs returned error: %v", err)
	}

	// Should have exactly 1 spec: base only
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	base := specs[0]
	if base.Name != "qwen3-coder-next-fp8" {
		t.Errorf("base name: got %q, want %q", base.Name, "qwen3-coder-next-fp8")
	}
	if base.Type != "base" {
		t.Errorf("base type: got %q, want %q", base.Type, "base")
	}
}

func TestBuildDeploymentSpecs_ExistingChatTemplateKwargs_Preserved(t *testing.T) {
	// buildDeploymentSpecs should preserve chat_template_kwargs in the base spec.
	params := map[string]interface{}{
		"model":       "qwen3.6-35b-a3b-fp8",
		"temperature": 0.7,
		"extra_body": map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"max_prefix_tokens": 100,
				"other_value":       "preserved",
				"enable_thinking":   false,
			},
		},
	}
	minfo := map[string]interface{}{
		"id":   "model-789",
		"name": "Qwen with kwargs",
	}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3.6-35b-a3b-fp8", true, nil, 0.0001, 0.0002, 0, 0, "", "", 0)
	if err != nil {
		t.Fatalf("buildDeploymentSpecs returned error: %v", err)
	}

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	base := specs[0]
	if base.Type != "base" {
		t.Errorf("expected base type, got %q", base.Type)
	}

	// Verify chat_template_kwargs are preserved in base
	extraBody := base.Params["extra_body"].(map[string]interface{})
	kt := extraBody["chat_template_kwargs"].(map[string]interface{})
	if kt["max_prefix_tokens"] != 100 {
		t.Errorf("max_prefix_tokens preserved: got %v, want 100", kt["max_prefix_tokens"])
	}
	if kt["other_value"] != "preserved" {
		t.Errorf("other_value preserved: got %v, want preserved", kt["other_value"])
	}
}

func TestBuildActiveSpecs_ExistingChatTemplateKwargs_Merge(t *testing.T) {
	// buildActiveSpecs should preserve chat_template_kwargs and set/override
	// enable_thinking and preserve_thinking.
	params := map[string]interface{}{
		"model":       "qwen3.6-35b-a3b-fp8",
		"temperature": 0.7,
		"extra_body": map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"max_prefix_tokens": 100,
				"other_value":       "preserved",
				"enable_thinking":   false, // should be overridden
			},
		},
	}
	minfo := map[string]interface{}{
		"id":   "model-789",
		"name": "Qwen with kwargs",
	}

	specs := buildActiveSpecs("qwen3.6-35b-a3b-fp8", params, minfo, true)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// Helper to extract chat_template_kwargs from a spec's params
	extractKT := func(sp DeploymentSpec) map[string]interface{} {
		ebr := sp.Params["extra_body"].(map[string]interface{})
		return ebr["chat_template_kwargs"].(map[string]interface{})
	}

	// Active alias: thinking disabled, existing keys preserved
	active := specs[0]
	activeKT := extractKT(active)
	if activeKT["enable_thinking"] != false {
		t.Errorf("active enable_thinking: got %v, want false", activeKT["enable_thinking"])
	}
	if activeKT["preserve_thinking"] != false {
		t.Errorf("active preserve_thinking: got %v, want false", activeKT["preserve_thinking"])
	}
	if activeKT["max_prefix_tokens"] != 100 {
		t.Errorf("active max_prefix_tokens preserved: got %v, want 100", activeKT["max_prefix_tokens"])
	}
	if activeKT["other_value"] != "preserved" {
		t.Errorf("active other_value preserved: got %v, want preserved", activeKT["other_value"])
	}

	// Active-thinking alias: thinking enabled, existing keys preserved
	thinkAlias := specs[1]
	thinkKT := extractKT(thinkAlias)
	if thinkKT["enable_thinking"] != true {
		t.Errorf("active-thinking enable_thinking: got %v, want true", thinkKT["enable_thinking"])
	}
	if thinkKT["preserve_thinking"] != true {
		t.Errorf("active-thinking preserve_thinking: got %v, want true", thinkKT["preserve_thinking"])
	}
	if thinkKT["max_prefix_tokens"] != 100 {
		t.Errorf("active-thinking max_prefix_tokens preserved: got %v, want 100", thinkKT["max_prefix_tokens"])
	}
	if thinkKT["other_value"] != "preserved" {
		t.Errorf("active-thinking other_value preserved: got %v, want preserved", thinkKT["other_value"])
	}
}


// ──────────────────────────────────────────────────────────────────────
// Tests for single-active-alias changes
// ──────────────────────────────────────────────────────────────────────

func TestFilterOutActiveAliases_RemovesActiveAndActiveThinking(t *testing.T) {
	specs := []DeploymentSpec{
		{Name: "my-model", Type: "base"},
		{Name: "active", Type: "alias"},
		{Name: "active-thinking", Type: "alias"},
		{Name: "my-model-think", Type: "variant"},
	}

	filtered := filterOutActiveAliases(specs)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(filtered))
	}

	if filtered[0].Name != "my-model" {
		t.Errorf("expected first spec to be 'my-model', got %q", filtered[0].Name)
	}
	if filtered[1].Name != "my-model-think" {
		t.Errorf("expected second spec to be 'my-model-think', got %q", filtered[1].Name)
	}
}

func TestFilterOutActiveAliases_NoAliasesToFilter(t *testing.T) {
	specs := []DeploymentSpec{
		{Name: "my-model", Type: "base"},
		{Name: "my-model-v2", Type: "variant"},
	}

	filtered := filterOutActiveAliases(specs)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(filtered))
	}
}

func TestFilterOutActiveAliases_AllAliases(t *testing.T) {
	specs := []DeploymentSpec{
		{Name: "active", Type: "alias"},
		{Name: "active-thinking", Type: "alias"},
	}

	filtered := filterOutActiveAliases(specs)

	if len(filtered) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(filtered))
	}
}

func TestFilterOutActiveAliases_EmptyInput(t *testing.T) {
	filtered := filterOutActiveAliases(nil)
	if len(filtered) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(filtered))
	}
}

func TestFilterOutActiveAliases_RAGAliasesFiltered(t *testing.T) {
	// RAG aliases (active-reranker, active-embeddings) ARE filtered during sync.
	// They are created on 'llm start' via ActivateSpeechRAGModel.
	specs := []DeploymentSpec{
		{Name: "my-reranker", Type: "alias"},
		{Name: "active-reranker", Type: "alias"},
		{Name: "active-embeddings", Type: "alias"},
		{Name: "active", Type: "alias"},
	}

	filtered := filterOutActiveAliases(specs)

	// Only my-reranker should remain (it's not a reserved alias name)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(filtered))
	}
	if filtered[0].Name != "my-reranker" {
		t.Errorf("expected my-reranker, got %q", filtered[0].Name)
	}
}

func TestFilterOutActiveAliases_SpeechAliasesFiltered(t *testing.T) {
	// Speech aliases (active-stt, active-tts, active-omni) ARE filtered during sync.
	// They are created on 'llm start' via ActivateSpeechRAGModel.
	specs := []DeploymentSpec{
		{Name: "active-stt", Type: "alias"},
		{Name: "active-tts", Type: "alias"},
		{Name: "active-omni", Type: "alias"},
		{Name: "active", Type: "alias"},
	}

	filtered := filterOutActiveAliases(specs)

	// All should be filtered out
	if len(filtered) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(filtered))
	}
}

func TestBuildActiveSpecs_ParamsModelSetCorrectly(t *testing.T) {
	params := map[string]interface{}{
		"model":       "original-model-name",
		"temperature": 0.7,
	}
	minfo := map[string]interface{}{
		"id":   "model-123",
		"name": "Test Model",
	}

	specs := buildActiveSpecs("my-new-slug", params, minfo, false)

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	// The alias params should have model set to the slug passed in
	modelVal := specs[0].Params["model"]
	if modelVal != "my-new-slug" {
		t.Errorf("expected model to be 'my-new-slug', got %q", modelVal)
	}
}

func TestBuildActiveSpecs_PreservesNonThinkingKwargs(t *testing.T) {
	params := map[string]interface{}{
		"model":       "test-model",
		"temperature": 0.7,
		"extra_body": map[string]interface{}{
			"chat_template_kwargs": map[string]interface{}{
				"max_prefix_tokens": 200,
				"custom_flag":       true,
			},
		},
	}
	minfo := map[string]interface{}{
		"id":   "model-999",
		"name": "Test",
	}

	specs := buildActiveSpecs("test-model", params, minfo, false)

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	kt := specs[0].Params["extra_body"].(map[string]interface{})["chat_template_kwargs"].(map[string]interface{})
	if kt["max_prefix_tokens"] != 200 {
		t.Errorf("max_prefix_tokens: got %v, want 200", kt["max_prefix_tokens"])
	}
	if kt["custom_flag"] != true {
		t.Errorf("custom_flag: got %v, want true", kt["custom_flag"])
	}
	// enable_thinking and preserve_thinking should be set to false
	if kt["enable_thinking"] != false {
		t.Errorf("enable_thinking: got %v, want false", kt["enable_thinking"])
	}
	if kt["preserve_thinking"] != false {
		t.Errorf("preserve_thinking: got %v, want false", kt["preserve_thinking"])
	}
}

func TestBuildActiveSpecs_VariantParamsNotIncluded(t *testing.T) {
	// Variants key should be excluded from alias params
	params := map[string]interface{}{
		"model": "test",
		"variants": map[string]interface{}{
			"thinking": map[string]interface{}{
				"suffix": "-think",
			},
		},
	}
	minfo := map[string]interface{}{}

	specs := buildActiveSpecs("test", params, minfo, false)

	for _, spec := range specs {
		if _, hasVariants := spec.Params["variants"]; hasVariants {
			t.Errorf("alias spec %q should not contain 'variants' key", spec.Name)
		}
	}
}

// Note: DeactivateAll and ActivateModel make real HTTP calls to the LiteLLM API.
// They are tested via integration tests when a LiteLLM instance is available.
// Unit tests cover the pure logic parts (filtering, spec building) above.

// ──────────────────────────────────────────────────────────────────────
// Tests for speech/RAG alias creation
// ──────────────────────────────────────────────────────────────────────

func TestBuildSpeechRAGSpecs_STT(t *testing.T) {
	params := map[string]interface{}{
		"model": "whisper-large-v3",
	}
	minfo := map[string]interface{}{"id": "whisper-123"}

	specs := buildSpeechRAGSpecs("whisper-large-v3", params, minfo, "stt", "http://localhost:8000", 8001)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs (base + alias), got %d", len(specs))
	}

	// 1. Base deployment
	base := specs[0]
	if base.Name != "whisper-large-v3" {
		t.Errorf("base name: got %q, want %q", base.Name, "whisper-large-v3")
	}
	if base.Type != "base" {
		t.Errorf("base type: got %q, want %q", base.Type, "base")
	}
	if base.Params["model"] != "whisper-large-v3" {
		t.Errorf("base model: got %q, want %q", base.Params["model"], "whisper-large-v3")
	}
	if base.Params["api_base"] != "http://localhost:8000:8001/v1" {
		t.Errorf("base api_base: got %q, want %q", base.Params["api_base"], "http://localhost:8000:8001/v1")
	}

	// 2. Alias deployment
	alias := specs[1]
	if alias.Name != "active-stt" {
		t.Errorf("alias name: got %q, want %q", alias.Name, "active-stt")
	}
	if alias.Type != "alias" {
		t.Errorf("alias type: got %q, want %q", alias.Type, "alias")
	}
	if alias.Params["model"] != "whisper-large-v3" {
		t.Errorf("alias model: got %q, want %q", alias.Params["model"], "whisper-large-v3")
	}
}

func TestBuildSpeechRAGSpecs_TTS(t *testing.T) {
	params := map[string]interface{}{"model": "kokoro-82m"}
	minfo := map[string]interface{}{"id": "kokoro-123"}

	specs := buildSpeechRAGSpecs("kokoro-82m", params, minfo, "tts", "http://localhost:8000", 8002)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs (base + alias), got %d", len(specs))
	}

	if specs[0].Name != "kokoro-82m" || specs[0].Type != "base" {
		t.Errorf("expected base kokoro-82m, got %q/%q", specs[0].Name, specs[0].Type)
	}
	if specs[1].Name != "active-tts" || specs[1].Type != "alias" {
		t.Errorf("expected alias active-tts, got %q/%q", specs[1].Name, specs[1].Type)
	}
}

func TestBuildSpeechRAGSpecs_Omni(t *testing.T) {
	params := map[string]interface{}{"model": "gpt-omni"}
	minfo := map[string]interface{}{"id": "omni-123"}

	specs := buildSpeechRAGSpecs("gpt-omni", params, minfo, "omni", "http://localhost:8000", 8003)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs (base + alias), got %d", len(specs))
	}

	if specs[0].Name != "gpt-omni" || specs[0].Type != "base" {
		t.Errorf("expected base gpt-omni, got %q/%q", specs[0].Name, specs[0].Type)
	}
	if specs[1].Name != "active-omni" || specs[1].Type != "alias" {
		t.Errorf("expected alias active-omni, got %q/%q", specs[1].Name, specs[1].Type)
	}
}

func TestBuildSpeechRAGSpecs_Reranker(t *testing.T) {
	params := map[string]interface{}{"model": "bge-reranker-v2"}
	minfo := map[string]interface{}{"id": "reranker-123"}

	specs := buildSpeechRAGSpecs("bge-reranker-v2", params, minfo, "reranker", "http://localhost:8000", 8004)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs (base + alias), got %d", len(specs))
	}

	if specs[0].Name != "bge-reranker-v2" || specs[0].Type != "base" {
		t.Errorf("expected base bge-reranker-v2, got %q/%q", specs[0].Name, specs[0].Type)
	}
	if specs[1].Name != "active-reranker" || specs[1].Type != "alias" {
		t.Errorf("expected alias active-reranker, got %q/%q", specs[1].Name, specs[1].Type)
	}
}

func TestBuildSpeechRAGSpecs_Embedding(t *testing.T) {
	params := map[string]interface{}{"model": "bge-large-en"}
	minfo := map[string]interface{}{"id": "embed-123"}

	specs := buildSpeechRAGSpecs("bge-large-en", params, minfo, "embedding", "http://localhost:8000", 8005)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs (base + alias), got %d", len(specs))
	}

	if specs[0].Name != "bge-large-en" || specs[0].Type != "base" {
		t.Errorf("expected base bge-large-en, got %q/%q", specs[0].Name, specs[0].Type)
	}
	if specs[1].Name != "active-embeddings" || specs[1].Type != "alias" {
		t.Errorf("expected alias active-embeddings, got %q/%q", specs[1].Name, specs[1].Type)
	}
}

func TestBuildSpeechRAGSpecs_NonSpeechRAG_ReturnsNil(t *testing.T) {
	params := map[string]interface{}{"model": "qwen3"}
	minfo := map[string]interface{}{"id": "qwen-123"}

	specs := buildSpeechRAGSpecs("qwen3", params, minfo, "chat", "http://localhost:8000", 8000)

	if len(specs) != 0 {
		t.Fatalf("expected 0 specs for non-speech/RAG model, got %d", len(specs))
	}
}

func TestBuildSpeechRAGSpecs_VariantsExcluded(t *testing.T) {
	params := map[string]interface{}{
		"model": "whisper",
		"variants": map[string]interface{}{
			"large": map[string]interface{}{},
		},
	}
	minfo := map[string]interface{}{}

	specs := buildSpeechRAGSpecs("whisper", params, minfo, "stt", "http://localhost:8000", 8001)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// Neither base nor alias should have variants
	for i, spec := range specs {
		if _, hasVariants := spec.Params["variants"]; hasVariants {
			t.Errorf("spec %d (%s) should not have variants key", i, spec.Name)
		}
	}
}

func TestBuildSpeechRAGSpecs_CustomLLMProvider(t *testing.T) {
	params := map[string]interface{}{"model": "test"}
	minfo := map[string]interface{}{}

	specs := buildSpeechRAGSpecs("test", params, minfo, "stt", "http://localhost:8000", 8001)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// Both base and alias should have custom_llm_provider
	for i, spec := range specs {
		if spec.Params["custom_llm_provider"] != "hosted_vllm" {
			t.Errorf("spec %d (%s) custom_llm_provider: got %v, want hosted_vllm", i, spec.Name, spec.Params["custom_llm_provider"])
		}
	}
}

// ──────────────────────────────────────────────────────────────────────
// Tests for custom_llm_provider default
// ──────────────────────────────────────────────────────────────────────

func TestEnsureCustomLLMProvider_SetsDefault(t *testing.T) {
	params := map[string]interface{}{
		"model": "test",
	}
	ensureCustomLLMProvider(params)
	if params["custom_llm_provider"] != "hosted_vllm" {
		t.Errorf("expected hosted_vllm, got %v", params["custom_llm_provider"])
	}
}

func TestEnsureCustomLLMProvider_PreservesExisting(t *testing.T) {
	params := map[string]interface{}{
		"model":               "test",
		"custom_llm_provider": "openai",
	}
	ensureCustomLLMProvider(params)
	if params["custom_llm_provider"] != "openai" {
		t.Errorf("expected openai to be preserved, got %v", params["custom_llm_provider"])
	}
}

func TestBuildDeploymentSpecs_CustomLLMProviderDefault(t *testing.T) {
	params := map[string]interface{}{
		"model": "qwen3",
	}
	minfo := map[string]interface{}{}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3", false, nil, 0, 0, 0, 0, "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	if specs[0].Params["custom_llm_provider"] != "hosted_vllm" {
		t.Errorf("expected custom_llm_provider hosted_vllm, got %v", specs[0].Params["custom_llm_provider"])
	}
}

func TestBuildDeploymentSpecs_CustomLLMProviderPreserved(t *testing.T) {
	params := map[string]interface{}{
		"model":               "qwen3",
		"custom_llm_provider": "anthropic",
	}
	minfo := map[string]interface{}{}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3", false, nil, 0, 0, 0, 0, "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if specs[0].Params["custom_llm_provider"] != "anthropic" {
		t.Errorf("expected custom_llm_provider anthropic, got %v", specs[0].Params["custom_llm_provider"])
	}
}

func TestBuildActiveSpecs_CustomLLMProviderDefault(t *testing.T) {
	params := map[string]interface{}{"model": "qwen3"}
	minfo := map[string]interface{}{}

	specs := buildActiveSpecs("qwen3", params, minfo, false)

	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	if specs[0].Params["custom_llm_provider"] != "hosted_vllm" {
		t.Errorf("expected custom_llm_provider hosted_vllm, got %v", specs[0].Params["custom_llm_provider"])
	}
}

func TestBuildSpeechRAGSpecs_CustomLLMProviderDefault(t *testing.T) {
	params := map[string]interface{}{"model": "whisper"}
	minfo := map[string]interface{}{}

	specs := buildSpeechRAGSpecs("whisper", params, minfo, "stt", "http://localhost:8000", 8001)

	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// Both base and alias should have custom_llm_provider
	for i, spec := range specs {
		if spec.Params["custom_llm_provider"] != "hosted_vllm" {
			t.Errorf("spec %d (%s) custom_llm_provider: got %v, want hosted_vllm", i, spec.Name, spec.Params["custom_llm_provider"])
		}
	}
}
