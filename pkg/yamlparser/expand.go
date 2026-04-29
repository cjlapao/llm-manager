package yamlparser

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var templateRegex = regexp.MustCompile(`\$\{\{[^\}]*\}\}`)

// ApplyTemplateVars expands all template (${{ ... }}) variables in a ModelYAML
// struct in-place. cfgValues is a flat map of uppercase ENV key => value used
// for .config.XXX resolution. Returns an error when a referenced variable
// cannot be resolved; the original text is left untouched so validation can
// still catch the problem at import-time without crashing.
func ApplyTemplateVars(y *ModelYAML, cfgValues map[string]string) error {
	var errs []error

	resolveFunc := func(ref string) (string, error) {
		ref = strings.TrimSpace(ref)
		if len(ref) == 0 {
			return "", fmt.Errorf("empty reference")
		}

		// Config references -- resolved from environment / configuration map.
		if strings.HasPrefix(ref, ".config.") {
			key := strings.ToUpper(ref[8:])
			if v, ok := cfgValues[key]; ok && v != "" {
				return v, nil
			}
			if v := os.Getenv(key); v != "" {
				return v, nil
			}
			return "", fmt.Errorf("no value for config key %q", ref)
		}

		// Self-references -- resolve against current ModelYAML fields.
		resolved, errField := lookupPath(strings.TrimLeft(ref, "."), y)
		if errField != nil {
			return "", fmt.Errorf("field %q not found: %w", ref, errField)
		}
		return formatValue(resolved), nil
	}

	y.Slug = doReplaceAll(y.Slug, resolveFunc)
	y.Name = doReplaceAll(y.Name, resolveFunc)
	y.Engine = doReplaceAll(y.Engine, resolveFunc)
	y.HFRepo = doReplaceAll(y.HFRepo, resolveFunc)
	y.Container = doReplaceAll(y.Container, resolveFunc)

	for i, c := range y.Capabilities {
		y.Capabilities[i] = doReplaceAll(c, resolveFunc)
	}

	for i, item := range y.CommandArgs {
		y.CommandArgs[i] = doReplaceAll(item, resolveFunc)
	}

	liteExpansions := expandMapNested(y.LiteLLMParams, resolveFunc)
	errs = append(errs, liteExpansions...)

	infoExpansions := expandMapNested(y.ModelInfo, resolveFunc)
	errs = append(errs, infoExpansions...)

	if len(errs) > 0 {
		return fmt.Errorf("template resolution failed (%d errors): %w", len(errs), errs[0])
	}

	return nil
}

// --- field access helpers ---

// splitPath splits a dot/path into segments, preserving bracketed array indices.
// e.g. "foo.bar.baz[2][qux]" -> ["foo","bar","baz[2]","qux"]
func splitPath(s string) []string {
	result := make([]string, 0)
	var buf strings.Builder
	inBracket := false

	for _, r := range s {
		switch r {
		case '[':
			inBracket = true
			buf.WriteRune(r)
		case ']':
			inBracket = false
			buf.WriteRune(r)
		case '.':
			if !inBracket {
				if buf.Len() > 0 {
					result = append(result, buf.String())
					buf.Reset()
				}
			} else {
				buf.WriteRune(r)
			}
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		result = append(result, buf.String())
	}
	return result
}

// segmentIndex tries to extract [n] from a segment like "arr[5]"
// Returns cleaned name, index, and hasIndex flag.
func segmentIndex(seg string) (cleaned string, idx int, hasIdx bool) {
	s := strings.TrimSpace(seg)
	left := strings.LastIndex(s, "[")
	right := strings.LastIndex(s, "]")
	if left == -1 || right == -1 || right <= left+1 {
		return s, -1, false
	}
	n, err := strconv.Atoi(s[left+1:right])
	if err != nil {
		return s, -1, false
	}
	return s[:left], n, true
}

// resolveRootField resolves the first path segment to a root ModelYAML field.
func resolveRootField(name string, y *ModelYAML) (interface{}, error) {
	switch strings.ToLower(name) {
	case "slug":
		return y.Slug, nil
	case "name":
		return y.Name, nil
	case "engine":
		return y.Engine, nil
	case "type":
		return y.Type, nil
	case "hfrepo", "hf_repo":
		return y.HFRepo, nil
	case "container":
		return y.Container, nil
	case "port":
		return y.Port, nil
	case "capabilities":
		return y.Capabilities, nil
	case "env_vars", "environment", "env":
		return y.EnvVars, nil
	case "command_args", "command":
		return y.CommandArgs, nil
	case "input_token_cost", "inputcost":
		if y.InputTokenCost != nil {
			return *y.InputTokenCost, nil
		}
		return nil, nil
	case "output_token_cost", "outputcost":
		if y.OutputTokenCost != nil {
			return *y.OutputTokenCost, nil
		}
		return nil, nil
	case "litellm_params", "litellmparams", "litellm.params":
		return y.LiteLLMParams, nil
	case "model_info", "modelinfo", "model.info":
		return y.ModelInfo, nil
	default:
		return nil, fmt.Errorf("unrecognized field %q", name)
	}
}

// lookupPath resolves a full dot-separated path starting from a ModelYAML root.
// Supports simple scalar paths (.slug), nested map lookups (e.g.
// litellm_params.variants.thinking.top_p),
// and array indexing (.command.items[2]).
func lookupPath(path string, y *ModelYAML) (interface{}, error) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	// Step 1: resolve top-level ModelYAML field.
	cur, err := resolveRootField(parts[0], y)
	if err != nil {
		return nil, fmt.Errorf("root field %q: %w", parts[0], err)
	}

	// Early return if no remaining path segments.
	if len(parts) == 1 {
		return cur, nil
	}

	// Walk remaining segments.
	for _, seg := range parts[1:] {
		cleaned, arrIdx, hasArr := segmentIndex(seg)
		cur = traverse(cur, cleaned)
		if cur == nil {
			return nil, fmt.Errorf("path broken at segment %q, value was nil", seg)
		}
		if hasArr {
			cur = indexAny(cur, arrIdx)
			if cur == nil {
				return nil, fmt.Errorf("index out of bounds [%d]", arrIdx)
			}
		}
	}

	return cur, nil
}

// traverse walks one segment deeper into `cur`. Handles map[string]interface{}
// as well as other common map types.
func traverse(cur interface{}, k string) interface{} {
	switch m := cur.(type) {
	case map[string]interface{}:
		if v, ok := m[k]; ok {
			return v
		}
		return nil
	case map[string]string:
		val, ok := m[k]
		if !ok {
			return ""
		}
		return val
	}
	return nil
}

// indexAny pulls element `idx` from a slice-like interface.
func indexAny(cur interface{}, idx int) interface{} {
	switch s := cur.(type) {
	case []interface{}:
		if idx >= 0 && idx < len(s) {
			return s[idx]
		}
	case []string:
		if idx >= 0 && idx < len(s) {
			return s[idx]
		}
	}
	return nil
}

// --- expansion engine ---

// doReplaceAll replaces every ${{ ... }} in `s` using resolveFunc.
// Unresolvable templates are left unharmed as-is.
func doReplaceAll(s string, resolveFunc func(string) (string, error)) string {
	if s == "" {
		return s
	}
	if !strings.Contains(s, "{{") {
		return s
	}

	// Build a local wrapper that discards the error.
	wrapper := func(ref string) string {
		r, _ := resolveFunc(ref)
		return r
	}

	return templateRegex.ReplaceAllStringFunc(s, func(match string) string {
		openIdx := strings.Index(match, "{{")
		closeIdx := strings.LastIndex(match, "}}")
		if openIdx == -1 || closeIdx <= openIdx {
			return match
		}
		inner := strings.TrimSpace(match[openIdx+2 : closeIdx])
		if inner == "" {
			return match
		}
		result := wrapper(inner)
		if result == "" {
			return match
		}
		return result
	})
}

// formatValue renders non-string values to a string for inline embedding.
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		fStr := strconv.FormatFloat(v, 'f', -1, 64)
		if strings.Contains(fStr, ".") {
			fStr = strings.TrimRight(fStr, "0")
			fStr = strings.TrimRight(fStr, ".")
		}
		return fStr
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return "<nil>"
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

// expandMapStrings iterates map[string]string values and expands them in-place.
func expandMapStrings(m map[string]string, resolveFunc func(string) (string, error)) []error {
	if m == nil {
		return nil
	}
	var errs []error
	for k, oldVal := range m {
		m[k] = doReplaceAll(oldVal, resolveFunc)
	}
	return errs
}

// expandSliceInterface walks through []interface{} and expands all nested string leafs.
func expandSliceInterface(slice []interface{}, resolveFunc func(string) (string, error)) []error {
	if slice == nil {
		return nil
	}
	var errs []error
	for i, val := range slice {
		slice[i] = expandItem(val, resolveFunc, &errs)
	}
	return errs
}

// expandMapNested walks through map[string]interface{} and expands all nested string leafs.
func expandMapNested(m map[string]interface{}, resolveFunc func(string) (string, error)) []error {
	if m == nil {
		return nil
	}
	var errs []error
	for k, v := range m {
		m[k] = expandItem(v, resolveFunc, &errs)
	}
	return errs
}

// expandItem handles the recursive expansion of generic items encountered during
// expansion of maps / slices.
func expandItem(val interface{}, resolveFunc func(string) (string, error), errs *[]error) interface{} {
	switch v := val.(type) {
	case string:
		return doReplaceAll(v, resolveFunc)
	case map[string]interface{}:
		for mk, mv := range v {
			v[mk] = expandItem(mv, resolveFunc, errs)
		}
		return v
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = expandItem(item, resolveFunc, errs)
		}
		return out
	default:
		return val
	}
}
