package yamlparser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// TypedValue represents a YAML value with its type preserved.
type TypedValue struct {
	Type  string      // "string", "int", "float", "bool", "array"
	Value interface{} // The actual typed value
}

// String returns the string representation of the value.
func (tv TypedValue) String() string {
	switch tv.Type {
	case "bool":
		return strconv.FormatBool(tv.Value.(bool))
	case "int":
		return fmt.Sprintf("%d", tv.Value.(int))
	case "float":
		return fmt.Sprintf("%v", tv.Value.(float64))
	case "array":
		vals := tv.Value.([]string)
		return strings.Join(vals, ", ")
	default:
		return fmt.Sprintf("%v", tv.Value)
	}
}

// ToJSON marshals the value to JSON.
func (tv TypedValue) ToJSON() (string, error) {
	data, err := json.Marshal(tv.Value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// TypedCommandArgs is a map of string -> TypedValue.
type TypedCommandArgs map[string]TypedValue

// ParseTypedCommandArgs converts a string map to typed values.
// It attempts to infer types: "true"/"false" → bool, digits → int, digits with . → float, else string.
func ParseTypedCommandArgs(args map[string]string) TypedCommandArgs {
	typed := make(TypedCommandArgs)
	for key, val := range args {
		typed[key] = InferType(val)
	}
	return typed
}

// InferType attempts to infer the type of a string value.
func InferType(s string) TypedValue {
	// Try bool
	if s == "true" {
		return TypedValue{Type: "bool", Value: true}
	}
	if s == "false" {
		return TypedValue{Type: "bool", Value: false}
	}

	// Try int
	if i, err := strconv.Atoi(s); err == nil {
		return TypedValue{Type: "int", Value: i}
	}

	// Try float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return TypedValue{Type: "float", Value: f}
	}

	// Default: string
	return TypedValue{Type: "string", Value: s}
}

// CommandArgsToJSON converts TypedCommandArgs to a JSON string for DB storage.
func CommandArgsToJSON(typed TypedCommandArgs) (string, error) {
	// Build a map[string]interface{} preserving types
	m := make(map[string]interface{})
	for k, v := range typed {
		m[k] = v.Value
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("failed to marshal command args: %w", err)
	}
	return string(data), nil
}
