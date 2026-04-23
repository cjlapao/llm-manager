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
				"enable_thinking":    true,
				"max_prefix_tokens":  100,
				"other_value":        "preserved",
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
			name: "boolean passthrough true",
			dst:  map[string]interface{}{"flag": false},
			src:  map[string]interface{}{"flag": true},
			expect: map[string]interface{}{"flag": true},
		},
		{
			name: "boolean passthrough false",
			dst:  map[string]interface{}{"flag": true},
			src:  map[string]interface{}{"flag": false},
			expect: map[string]interface{}{"flag": false},
		},
		{
			name: "numeric scalar replacement",
			dst:  map[string]interface{}{"count": 10},
			src:  map[string]interface{}{"count": 42},
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
