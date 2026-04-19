package yamlparser

import (
	"encoding/json"
	"testing"
)

func TestInferType_Bool(t *testing.T) {
	tv := InferType("true")
	if tv.Type != "bool" || tv.Value.(bool) != true {
		t.Errorf("InferType(true) = %+v, want Type=bool Value=true", tv)
	}

	tv = InferType("false")
	if tv.Type != "bool" || tv.Value.(bool) != false {
		t.Errorf("InferType(false) = %+v, want Type=bool Value=false", tv)
	}
}

func TestInferType_Int(t *testing.T) {
	tv := InferType("8080")
	if tv.Type != "int" || tv.Value.(int) != 8080 {
		t.Errorf("InferType(8080) = %+v, want Type=int Value=8080", tv)
	}
}

func TestInferType_Float(t *testing.T) {
	tv := InferType("0.72")
	if tv.Type != "float" {
		t.Errorf("InferType(0.72) type = %q, want float", tv.Type)
	}
	if v, ok := tv.Value.(float64); !ok || v != 0.72 {
		t.Errorf("InferType(0.72) value = %v (%T), want 0.72 (float64)", tv.Value, tv.Value)
	}
}

func TestInferType_String(t *testing.T) {
	tv := InferType("Qwen/Qwen3-Next")
	if tv.Type != "string" || tv.Value.(string) != "Qwen/Qwen3-Next" {
		t.Errorf("InferType(Qwen/Qwen3-Next) = %+v", tv)
	}
}

func TestParseTypedCommandArgs(t *testing.T) {
	args := map[string]string{
		"model":           "Qwen/Qwen3-Next",
		"max-model-len":   "131072",
		"gpu-memory-util": "0.72",
		"enable-chunked":  "true",
		"enable-prefix":   "false",
	}
	typed := ParseTypedCommandArgs(args)

	if typed["model"].Type != "string" {
		t.Errorf("model type = %q, want string", typed["model"].Type)
	}
	if typed["max-model-len"].Type != "int" {
		t.Errorf("max-model-len type = %q, want int", typed["max-model-len"].Type)
	}
	if typed["gpu-memory-util"].Type != "float" {
		t.Errorf("gpu-memory-util type = %q, want float", typed["gpu-memory-util"].Type)
	}
	if typed["enable-chunked"].Type != "bool" || typed["enable-chunked"].Value.(bool) != true {
		t.Errorf("enable-chunked = %+v", typed["enable-chunked"])
	}
	if typed["enable-prefix"].Type != "bool" || typed["enable-prefix"].Value.(bool) != false {
		t.Errorf("enable-prefix = %+v", typed["enable-prefix"])
	}
}

func TestCommandArgsToJSON(t *testing.T) {
	args := ParseTypedCommandArgs(map[string]string{
		"model":          "Qwen/Qwen3-Next",
		"max-model-len":  "131072",
		"enable-chunked": "true",
	})

	jsonStr, err := CommandArgsToJSON(args)
	if err != nil {
		t.Fatalf("CommandArgsToJSON error: %v", err)
	}

	// Verify JSON preserves types
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if _, ok := result["max-model-len"].(float64); !ok {
		t.Error("int value was not preserved as number in JSON")
	}
	if _, ok := result["enable-chunked"].(bool); !ok {
		t.Error("bool value was not preserved as bool in JSON")
	}
	if _, ok := result["model"].(string); !ok {
		t.Error("string value was not preserved as string in JSON")
	}
}

func TestTypedValue_String(t *testing.T) {
	tests := []struct {
		name   string
		tv     TypedValue
		expect string
	}{
		{"bool true", TypedValue{Type: "bool", Value: true}, "true"},
		{"bool false", TypedValue{Type: "bool", Value: false}, "false"},
		{"int", TypedValue{Type: "int", Value: 8080}, "8080"},
		{"float", TypedValue{Type: "float", Value: 0.72}, "0.72"},
		{"string", TypedValue{Type: "string", Value: "hello"}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tv.String()
			if got != tt.expect {
				t.Errorf("TypedValue.String() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestTypedValue_ToJSON(t *testing.T) {
	tests := []struct {
		name   string
		tv     TypedValue
		expect string
	}{
		{"bool", TypedValue{Type: "bool", Value: true}, "true"},
		{"int", TypedValue{Type: "int", Value: 42}, "42"},
		{"float", TypedValue{Type: "float", Value: 3.14}, "3.14"},
		{"string", TypedValue{Type: "string", Value: "hello"}, `"hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tv.ToJSON()
			if err != nil {
				t.Fatalf("ToJSON error: %v", err)
			}
			if got != tt.expect {
				t.Errorf("ToJSON() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestInferType_NegativeInt(t *testing.T) {
	// Negative integers should still be parsed as int
	tv := InferType("-1")
	if tv.Type != "int" {
		t.Errorf("InferType(-1) type = %q, want int", tv.Type)
	}
}

func TestInferType_Zero(t *testing.T) {
	tv := InferType("0")
	if tv.Type != "int" {
		t.Errorf("InferType(0) type = %q, want int", tv.Type)
	}
}

func TestParseTypedCommandArgs_Empty(t *testing.T) {
	args := ParseTypedCommandArgs(map[string]string{})
	if len(args) != 0 {
		t.Errorf("expected empty map, got %d entries", len(args))
	}
}

func TestCommandArgsToJSON_Empty(t *testing.T) {
	args := ParseTypedCommandArgs(map[string]string{})
	jsonStr, err := CommandArgsToJSON(args)
	if err != nil {
		t.Fatalf("CommandArgsToJSON error: %v", err)
	}
	if jsonStr != "{}" {
		t.Errorf("expected '{}', got %q", jsonStr)
	}
}
