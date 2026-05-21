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

func TestBuildDeploymentSpecs_ThinkingModel_DualAlias(t *testing.T) {
	// A model with thinking capability should produce:
	//   1. base deployment (slug)
	//   2. active alias (thinking disabled)
	//   3. active-thinking alias (thinking enabled, preserve enabled)
	params := map[string]interface{}{
		"model":       "qwen3.6-35b-a3b-fp8",
		"temperature": 0.7,
	}
	minfo := map[string]interface{}{
		"id":   "model-123",
		"name": "Qwen 3.6",
	}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3.6-35b-a3b-fp8", true, nil, 0.0001, 0.0002)
	if err != nil {
		t.Fatalf("buildDeploymentSpecs returned error: %v", err)
	}

	// Should have exactly 3 specs: base + active + active-thinking
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}

	// 1. Base deployment
	base := specs[0]
	if base.Name != "qwen3.6-35b-a3b-fp8" {
		t.Errorf("base name: got %q, want %q", base.Name, "qwen3.6-35b-a3b-fp8")
	}
	if base.Type != "base" {
		t.Errorf("base type: got %q, want %q", base.Type, "base")
	}

	// 2. Active alias — thinking disabled
	active := specs[1]
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

	// 3. Active-thinking alias — thinking enabled
	thinkAlias := specs[2]
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

func TestBuildDeploymentSpecs_NonThinkingModel_SingleAlias(t *testing.T) {
	// A model without thinking capability should produce:
	//   1. base deployment (slug)
	//   2. active alias (thinking disabled)
	// No active-thinking alias.
	params := map[string]interface{}{
		"model":       "qwen3-coder-next-fp8",
		"temperature": 0.8,
	}
	minfo := map[string]interface{}{
		"id":   "model-456",
		"name": "Qwen Coder",
	}

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3-coder-next-fp8", false, nil, 0.00005, 0.0001)
	if err != nil {
		t.Fatalf("buildDeploymentSpecs returned error: %v", err)
	}

	// Should have exactly 2 specs: base + active (no active-thinking)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}

	// 1. Base deployment
	base := specs[0]
	if base.Name != "qwen3-coder-next-fp8" {
		t.Errorf("base name: got %q, want %q", base.Name, "qwen3-coder-next-fp8")
	}
	if base.Type != "base" {
		t.Errorf("base type: got %q, want %q", base.Type, "base")
	}

	// 2. Active alias — thinking disabled
	active := specs[1]
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

func TestBuildDeploymentSpecs_ExistingChatTemplateKwargs_Merge(t *testing.T) {
	// When a model already has chat_template_kwargs with non-thinking keys,
	// buildDeploymentSpecs must preserve those keys and only set/override
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

	specs, err := buildDeploymentSpecs(params, minfo, "qwen3.6-35b-a3b-fp8", true, nil, 0.0001, 0.0002)
	if err != nil {
		t.Fatalf("buildDeploymentSpecs returned error: %v", err)
	}

	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}

	// Helper to extract chat_template_kwargs from a spec's params
	extractKT := func(sp DeploymentSpec) map[string]interface{} {
		ebr := sp.Params["extra_body"].(map[string]interface{})
		return ebr["chat_template_kwargs"].(map[string]interface{})
	}

	// Active alias: thinking disabled, existing keys preserved
	active := specs[1]
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
	thinkAlias := specs[2]
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
