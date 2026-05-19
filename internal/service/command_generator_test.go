package service

import (
	"strings"
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

	// Verify replaced flags are combined strings ("--flag value")
	checkCombinedFlagValue(t, result, "max-model-len", "8192")
	checkCombinedFlagValue(t, result, "max-num-batched-tokens", "16384")
	checkCombinedFlagValue(t, result, "max-num-seqs", "4")
	checkCombinedFlagValue(t, result, "gpu-memory-utilization", "0.85")

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
	checkCombinedFlagValue(t, result, "max-model-len", "8192")

	// Missing flags should be appended as combined strings
	checkCombinedFlagValue(t, result, "max-num-batched-tokens", "16384")
	checkCombinedFlagValue(t, result, "max-num-seqs", "4")
	checkCombinedFlagValue(t, result, "gpu-memory-utilization", "0.85")
}

func TestMergeFlags_OutputFormatIsCombinedStrings(t *testing.T) {
	// The core contract: output must be ["--flag value", ...] combined strings
	// so the compose template renders each on a single line.
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

	// Every replacement flag should be a combined string "--flag value"
	for _, elem := range result {
		if strings.HasPrefix(elem, "--max-model-len ") {
			if elem != "--max-model-len 262144" {
				t.Errorf("--max-model-len = %q, want %q", elem, "--max-model-len 262144")
			}
		}
		if strings.HasPrefix(elem, "--max-num-batched-tokens ") {
			if elem != "--max-num-batched-tokens 32768" {
				t.Errorf("--max-num-batched-tokens = %q, want %q", elem, "--max-num-batched-tokens 32768")
			}
		}
		if strings.HasPrefix(elem, "--max-num-seqs ") {
			if elem != "--max-num-seqs 2" {
				t.Errorf("--max-num-seqs = %q, want %q", elem, "--max-num-seqs 2")
			}
		}
		if strings.HasPrefix(elem, "--gpu-memory-utilization ") {
			if elem != "--gpu-memory-utilization 0.42" {
				t.Errorf("--gpu-memory-utilization = %q, want %q", elem, "--gpu-memory-utilization 0.42")
			}
		}
	}
}

func TestMergeFlags_BackwardCompatWithCombinedInput(t *testing.T) {
	// Existing data in combined format should still be parsed and merged correctly.
	// Known flags in combined format are replaced with combined strings;
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

	// The combined "--max-model-len 4096" should be replaced with combined string
	checkCombinedFlagValue(t, result, "max-model-len", "8192")

	// New flags should be combined strings
	checkCombinedFlagValue(t, result, "max-num-batched-tokens", "16384")
	checkCombinedFlagValue(t, result, "max-num-seqs", "4")
	checkCombinedFlagValue(t, result, "gpu-memory-utilization", "0.85")

	// Unknown combined-format flags should pass through unchanged
	if !containsElement(result, "--model my-model") {
		t.Error("expected --model my-model to pass through unchanged")
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
	checkCombinedFlagValue(t, result, "max-model-len", "8192")
}

// --- combineFlagPairs Tests ---

func TestCombineFlagPairs_CombinesAdjacentPairs(t *testing.T) {
	cmds := []string{
		"python", "vllm", "serve",
		"--model", "my-model",
		"--max-model-len", "8192",
		"--max-num-seqs", "4",
	}

	result := combineFlagPairs(cmds)

	expected := []string{
		"python", "vllm", "serve",
		"--model my-model",
		"--max-model-len 8192",
		"--max-num-seqs 4",
	}

	if len(result) != len(expected) {
		t.Fatalf("expected %d elements, got %d: %v", len(expected), len(result), result)
	}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("result[%d] = %q, want %q", i, result[i], exp)
		}
	}
}

func TestCombineFlagPairs_AlreadyCombinedPassesThrough(t *testing.T) {
	cmds := []string{
		"python", "vllm", "serve",
		"--model my-model",
		"--max-model-len 8192",
	}

	result := combineFlagPairs(cmds)

	if len(result) != len(cmds) {
		t.Fatalf("expected %d elements, got %d: %v", len(cmds), len(result), result)
	}
	for i, elem := range cmds {
		if result[i] != elem {
			t.Errorf("result[%d] = %q, want %q", i, result[i], elem)
		}
	}
}

func TestCombineFlagPairs_NoFlags(t *testing.T) {
	cmds := []string{"python", "vllm", "serve"}
	result := combineFlagPairs(cmds)

	if len(result) != len(cmds) {
		t.Fatalf("expected %d elements, got %d: %v", len(cmds), len(result), result)
	}
}

func TestCombineFlagPairs_FlagWithoutValue(t *testing.T) {
	cmds := []string{
		"python", "serve",
		"--trust-remote-code",
		"--model", "my-model",
	}

	result := combineFlagPairs(cmds)

	// --trust-remote-code has no value following it, should stay alone
	if result[2] != "--trust-remote-code" {
		t.Errorf("--trust-remote-code = %q, want %q", result[2], "--trust-remote-code")
	}
	// --model should be combined with its value
	if result[3] != "--model my-model" {
		t.Errorf("result[3] = %q, want %q", result[3], "--model my-model")
	}
}

// --- removeSpeculativeConfigFlag Tests ---

func TestRemoveSpeculativeConfigFlag_CombinedFormat(t *testing.T) {
	cmds := []string{
		"python", "serve",
		"--model", "my-model",
		"--speculative-config '{\"method\":\"mtp\",\"num_speculative_tokens\":3}'",
		"--max-model-len", "8192",
	}

	result := removeSpeculativeConfigFlag(cmds)

	if len(result) != 6 {
		t.Fatalf("expected 6 elements, got %d: %v", len(result), result)
	}
	if containsElement(result, "--speculative-config") {
		t.Error("--speculative-config should be removed")
	}
}

func TestRemoveSpeculativeConfigFlag_SeparateElements(t *testing.T) {
	cmds := []string{
		"python", "serve",
		"--model", "my-model",
		"--speculative-config",
		"{\"method\":\"mtp\"}",
		"--max-model-len", "8192",
	}

	result := removeSpeculativeConfigFlag(cmds)

	if len(result) != 6 {
		t.Fatalf("expected 6 elements, got %d: %v", len(result), result)
	}
	if containsElement(result, "--speculative-config") {
		t.Error("--speculative-config should be removed")
	}
}

// --- Helper functions ---

func checkCombinedFlagValue(t *testing.T, result []string, flagName, expectedValue string) {
	t.Helper()
	combined := "--" + flagName + " " + expectedValue
	if !containsElement(result, combined) {
		t.Errorf("expected combined flag %q in result: %v", combined, result)
	}
}

func containsElement(slice []string, elem string) bool {
	for _, s := range slice {
		if s == elem {
			return true
		}
	}
	return false
}
