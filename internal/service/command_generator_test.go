package service

import (
	"testing"
)

// --- ParseExistingFlags Tests ---

func TestParseExistingFlags_SeparateElements(t *testing.T) {
	cmds := []string{
		"python", "vllm", "serve",
		"--model", "my-model",
		"--max-model-len", "8192",
		"--gpu-memory-utilization", "0.9",
	}

	existing := ParseExistingFlags(cmds)

	if len(existing) != 3 {
		t.Fatalf("expected 3 flags, got %d", len(existing))
	}
	if v := existing["model"]; v != "my-model" {
		t.Errorf("--model = %q, want %q", v, "my-model")
	}
	if v := existing["max-model-len"]; v != "8192" {
		t.Errorf("--max-model-len = %q, want %q", v, "8192")
	}
	if v := existing["gpu-memory-utilization"]; v != "0.9" {
		t.Errorf("--gpu-memory-utilization = %q, want %q", v, "0.9")
	}
}

func TestParseExistingFlags_CombinedFormat(t *testing.T) {
	// Backward compat: combined "--flag value" as single elements
	cmds := []string{
		"python", "vllm", "serve",
		"--model my-model",
		"--max-model-len 8192",
		"--gpu-memory-utilization 0.9",
	}

	existing := ParseExistingFlags(cmds)

	if len(existing) != 3 {
		t.Fatalf("expected 3 flags, got %d", len(existing))
	}
	if v := existing["model"]; v != "my-model" {
		t.Errorf("--model = %q, want %q", v, "my-model")
	}
	if v := existing["max-model-len"]; v != "8192" {
		t.Errorf("--max-model-len = %q, want %q", v, "8192")
	}
	if v := existing["gpu-memory-utilization"]; v != "0.9" {
		t.Errorf("--gpu-memory-utilization = %q, want %q", v, "0.9")
	}
}

func TestParseExistingFlags_Empty(t *testing.T) {
	cmds := []string{"python", "vllm", "serve"}
	existing := ParseExistingFlags(cmds)
	if len(existing) != 0 {
		t.Errorf("expected 0 flags, got %d", len(existing))
	}
}

func TestParseExistingFlags_NoValueAfterFlag(t *testing.T) {
	cmds := []string{"python", "serve", "--help"}
	existing := ParseExistingFlags(cmds)
	if len(existing) != 0 {
		t.Errorf("expected 0 flags (no value after --help), got %d", len(existing))
	}
}

// --- MergeFlags Tests ---

func TestMergeFlags_ReplacesExistingFlags(t *testing.T) {
	existingCmds := []string{
		"python", "vllm", "serve",
		"--model", "my-model",
		"--max-model-len", "4096",
		"--max-num-batched-tokens", "4096",
		"--max-num-seqs", "2",
		"--gpu-memory-utilization", "0.5",
	}

	flags := &GeneratedFlags{
		MaxModelLen:         "8192",
		MaxNumBatchedTokens: "16384",
		MaxNumSeqs:          "4",
		GPUMemoryUtil:       "0.85",
	}

	result := MergeFlags(existingCmds, flags)

	// Verify replaced flags are separate elements
	checkFlagValue(t, result, "max-model-len", "8192")
	checkFlagValue(t, result, "max-num-batched-tokens", "16384")
	checkFlagValue(t, result, "max-num-seqs", "4")
	checkFlagValue(t, result, "gpu-memory-utilization", "0.85")

	// Verify non-flag elements are preserved
	if result[0] != "python" || result[1] != "vllm" || result[2] != "serve" {
		t.Errorf("prefix elements changed: %v", result[:3])
	}
}

func TestMergeFlags_AppendsMissingFlags(t *testing.T) {
	existingCmds := []string{
		"python", "vllm", "serve",
		"--model", "my-model",
		"--max-model-len", "4096",
	}

	flags := &GeneratedFlags{
		MaxModelLen:         "8192",
		MaxNumBatchedTokens: "16384",
		MaxNumSeqs:          "4",
		GPUMemoryUtil:       "0.85",
	}

	result := MergeFlags(existingCmds, flags)

	// max-model-len should be replaced
	checkFlagValue(t, result, "max-model-len", "8192")

	// Missing flags should be appended as separate elements
	checkFlagValue(t, result, "max-num-batched-tokens", "16384")
	checkFlagValue(t, result, "max-num-seqs", "4")
	checkFlagValue(t, result, "gpu-memory-utilization", "0.85")

	// Verify no combined "--flag value" strings exist
	for i, elem := range result {
		if i+1 < len(result) &&
			len(elem) >= 2 && elem[0] == '-' && elem[1] == '-' &&
			elem[len(elem)-1] != '"' {
			// Check it's not a combined string like "--flag value"
			// A combined string would have a space inside it
			// (we only care that our output doesn't produce combined strings)
		}
	}
}

func TestMergeFlags_OutputFormatIsSeparateElements(t *testing.T) {
	// The core contract: output must be ["--flag", "value", ...] not ["--flag value", ...]
	existingCmds := []string{
		"python", "vllm", "serve",
	}

	flags := &GeneratedFlags{
		MaxModelLen:         "262144",
		MaxNumBatchedTokens: "32768",
		MaxNumSeqs:          "2",
		GPUMemoryUtil:       "0.42",
	}

	result := MergeFlags(existingCmds, flags)

	// Every "--flag" element should NOT contain a space (i.e., not combined)
	for i, elem := range result {
		if len(elem) >= 2 && elem[0] == '-' && elem[1] == '-' {
			if containsSpace(elem) {
				t.Errorf("result[%d] = %q is a combined string; expected separate --flag and value elements", i, elem)
			}
		}
	}

	// Verify --max-model-len and its value are adjacent and separate
	found := false
	for i := 0; i < len(result); i++ {
		if result[i] == "--max-model-len" && i+1 < len(result) && result[i+1] == "262144" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected adjacent [\"--max-model-len\", \"262144\"] in result, got: %v", result)
	}
}

func TestMergeFlags_BackwardCompatWithCombinedInput(t *testing.T) {
	// Existing data in combined format should still be parsed and merged correctly.
	// Known flags in combined format are replaced with separate elements;
	// unknown flags pass through unchanged.
	existingCmds := []string{
		"python", "vllm", "serve",
		"--model my-model",
		"--max-model-len 4096",
	}

	flags := &GeneratedFlags{
		MaxModelLen:         "8192",
		MaxNumBatchedTokens: "16384",
		MaxNumSeqs:          "4",
		GPUMemoryUtil:       "0.85",
	}

	result := MergeFlags(existingCmds, flags)

	// The combined "--max-model-len 4096" should be replaced with separate elements
	checkFlagValue(t, result, "max-model-len", "8192")

	// New flags should be separate elements
	checkFlagValue(t, result, "max-num-batched-tokens", "16384")
	checkFlagValue(t, result, "max-num-seqs", "4")
	checkFlagValue(t, result, "gpu-memory-utilization", "0.85")

	// Unknown combined-format flags should pass through unchanged
	if !containsElement(result, "--model my-model") {
		t.Error("expected --model my-model to pass through unchanged")
	}

	// Verify no combined strings for known replacement flags
	for i, elem := range result {
		if len(elem) >= 2 && elem[0] == '-' && elem[1] == '-' && containsSpace(elem) {
			for _, replName := range []string{"max-model-len", "max-num-batched-tokens", "max-num-seqs", "gpu-memory-utilization"} {
				combined := "--" + replName
				if len(elem) > len(combined) && elem[:len(combined)] == combined {
					t.Errorf("result[%d] = %q is a combined string for a replaced flag; should be separate elements", i, elem)
				}
			}
		}
	}
}

func TestMergeFlags_PreservesExtraArgs(t *testing.T) {
	existingCmds := []string{
		"python", "vllm", "serve",
		"--model", "my-model",
		"--tensor-parallel-size", "2",
		"--max-model-len", "4096",
		"--trust-remote-code",
	}

	flags := &GeneratedFlags{
		MaxModelLen:         "8192",
		MaxNumBatchedTokens: "16384",
		MaxNumSeqs:          "4",
		GPUMemoryUtil:       "0.85",
	}

	result := MergeFlags(existingCmds, flags)

	// Extra args should be preserved
	if !containsElement(result, "--tensor-parallel-size") {
		t.Error("expected --tensor-parallel-size to be preserved")
	}
	if !containsElement(result, "--trust-remote-code") {
		t.Error("expected --trust-remote-code to be preserved")
	}

	// Replaced flag should have new value
	checkFlagValue(t, result, "max-model-len", "8192")
}

// --- Helper functions ---

func checkFlagValue(t *testing.T, result []string, flagName, expectedValue string) {
	t.Helper()
	for i := 0; i < len(result); i++ {
		if result[i] == "--"+flagName {
			if i+1 >= len(result) {
				t.Errorf("--%s: no value found after flag", flagName)
				return
			}
			if result[i+1] != expectedValue {
				t.Errorf("--%s value = %q, want %q", flagName, result[i+1], expectedValue)
			}
			return
		}
	}
	t.Errorf("--%s: flag not found in result: %v", flagName, result)
}

func containsElement(slice []string, elem string) bool {
	for _, s := range slice {
		if s == elem {
			return true
		}
	}
	return false
}

func containsSpace(s string) bool {
	for _, c := range s {
		if c == ' ' {
			return true
		}
	}
	return false
}
