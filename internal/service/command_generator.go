package service

import (
	"fmt"
	"strconv"
	"strings"
)

// GeneratedFlags holds the vLLM command-line flags derived from model profile data.
type GeneratedFlags struct {
	MaxModelLen         string // e.g., "262144"
	MaxNumBatchedTokens string // e.g., "16384"
	MaxNumSeqs          string // e.g., "2"
	GPUMemoryUtil       string // e.g., "0.42"
}

// GenerateFlags creates vLLM command flags from model profile and memory data.
func GenerateFlags(profile ModelProfile, memResult *MemoryResult, contextLen int, numSequences int) *GeneratedFlags {
	flags := &GeneratedFlags{}

	// --max-model-len: use provided context, default 65536
	// For encoder models (0 attention layers), cap at a reasonable value
	// since encoder models don't benefit from extremely large context windows
	// and vLLM will pre-allocate memory proportional to this value.
	if contextLen > 0 {
		if profile.AttentionLayers == 0 && contextLen > 16384 {
			flags.MaxModelLen = "16384" // cap encoder context at 16K
		} else {
			flags.MaxModelLen = strconv.Itoa(contextLen)
		}
	} else if profile.DefaultContext > 0 {
		if profile.AttentionLayers == 0 && profile.DefaultContext > 16384 {
			flags.MaxModelLen = "16384"
		} else {
			flags.MaxModelLen = strconv.Itoa(profile.DefaultContext)
		}
	} else {
		flags.MaxModelLen = "65536"
	}

	// --max-num-batched-tokens: derive from off_budget_mb, default 8192
	flags.MaxNumBatchedTokens = deriveBatchedTokens(memResult)

	// --max-num-seqs: use provided, default 1
	// For encoder models, default to a higher number since they handle
	// concurrent requests efficiently (no KV cache contention).
	if numSequences > 0 {
		flags.MaxNumSeqs = strconv.Itoa(numSequences)
	} else if profile.AttentionLayers == 0 {
		flags.MaxNumSeqs = "128" // encoder: batch many concurrent requests
	} else {
		flags.MaxNumSeqs = "1"
	}

	// --gpu-memory-utilization: from calculated ratio, formatted to 2 decimal places
	// CalculateMemory handles encoder models with the full breakdown
	// (weights + CUDA context + off-budget + prefix cache) expressed as
	// a fraction of total GPU memory. For non-encoder models, it scales
	// against available GPU memory when other models are running.
	// Trust that value — no additional floor needed.
	util := memResult.GPUMemoryUtilization

	flags.GPUMemoryUtil = fmt.Sprintf("%.2f", util)

	return flags
}

// deriveBatchedTokens derives max-num-batched-tokens from total realistic memory.
// Higher memory models get larger batch sizes to amortize prefill latency.
func deriveBatchedTokens(mem *MemoryResult) string {
	switch {
	case mem.TotalRealisticMB >= 40000:
		return "32768"
	case mem.TotalRealisticMB >= 25000:
		return "16384"
	default:
		return "8192"
	}
}

// ParseExistingFlags extracts existing flag values from a command array.
// Flags may be stored as ["--flag-name", "value", ...] pairs (separate elements)
// or as ["--flag-name value", ...] (combined string). Both formats are supported
// for backward compatibility with existing DB data.
// Returns a map of flag name -> value.
func ParseExistingFlags(cmds []string) map[string]string {
	existing := make(map[string]string)
	for i := 0; i < len(cmds); i++ {
		arg := cmds[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		// Check if this is a combined format (e.g., "--flag value" as one string)
		if strings.Contains(arg, " ") {
			parts := strings.SplitN(arg, " ", 2)
			name := strings.TrimPrefix(parts[0], "--")
			existing[name] = parts[1]
			continue
		}
		// Separate format: "--flag-name" followed by "value" in the next element
		if next := i + 1; next < len(cmds) {
			name := strings.TrimPrefix(arg, "--")
			existing[name] = cmds[next]
			i++ // skip the value element
		}
	}
	return existing
}

// MergeFlags takes existing command args and merges auto-generated flags into them.
// Existing flags are replaced by value; missing flags are appended.
// All other YAML-provided flags remain untouched.
//
// Flags are output as combined strings ("--flag value") so the compose template
// renders them on a single line instead of splitting flag and value onto
// separate lines.
func MergeFlags(existingCmds []string, flags *GeneratedFlags) []string {
	replacements := map[string]string{
		"max-model-len":          flags.MaxModelLen,
		"max-num-batched-tokens": flags.MaxNumBatchedTokens,
		"max-num-seqs":           flags.MaxNumSeqs,
		"gpu-memory-utilization": flags.GPUMemoryUtil,
	}

	// Track which flags we've processed
	done := make(map[string]bool)
	result := make([]string, 0, len(existingCmds)+len(replacements))

	for i := 0; i < len(existingCmds); i++ {
		arg := existingCmds[i]
		if !strings.HasPrefix(arg, "--") {
			result = append(result, arg)
			continue
		}
		// Check if this is a combined format (e.g., "--max-model-len 4096" as one string)
		if strings.Contains(arg, " ") {
			parts := strings.SplitN(arg, " ", 2)
			flagName := strings.TrimPrefix(parts[0], "--")
			if newVal, ok := replacements[flagName]; ok {
				// Replace the value of this flag as a combined string
				result = append(result, fmt.Sprintf("--%s %s", flagName, newVal))
				done[flagName] = true
				continue
			}
			// Unknown flag in combined format — pass through unchanged
			result = append(result, arg)
			continue
		}
		// Separate format: "--flag-name" followed by "value" in the next element
		flagName := strings.TrimPrefix(arg, "--")
		if newVal, ok := replacements[flagName]; ok {
			if i+1 < len(existingCmds) {
				// Replace the value as a combined string
				result = append(result, fmt.Sprintf("--%s %s", flagName, newVal))
				done[flagName] = true
				i++ // skip the old value element
				continue
			}
		}
		result = append(result, arg)
	}

	// Append missing flags as combined strings
	for name, val := range replacements {
		if !done[name] {
			result = append(result, fmt.Sprintf("--%s %s", name, val))
		}
	}

	return result
}

// removeSpeculativeConfigFlag removes any --speculative-config flag and its value
// from a command args slice, handling both combined and separate formats.
func removeSpeculativeConfigFlag(cmds []string) []string {
	result := make([]string, 0, len(cmds))
	skipNext := false
	for i, arg := range cmds {
		if skipNext {
			skipNext = false
			continue
		}
		if !strings.HasPrefix(arg, "--speculative-config") {
			result = append(result, arg)
			continue
		}
		// This is the flag — skip it and the next element if it's a separate value
		if strings.Contains(arg, " ") {
			// Combined format: "--speculative-config {...}" — skip it
			continue
		}
		// Separate format: "--speculative-config" followed by value
		if i+1 < len(cmds) {
			skipNext = true
		}
		// Skip the flag itself
	}
	return result
}

// combineFlagPairs walks a command args slice and merges any standalone "--flag"
// elements that are immediately followed by their value (a non-flag string) into
// combined "--flag value" strings. This ensures the compose template renders each
// flag+value pair on a single line.
func combineFlagPairs(cmds []string) []string {
	result := make([]string, 0, len(cmds))
	for i := 0; i < len(cmds); i++ {
		arg := cmds[i]
		if strings.HasPrefix(arg, "--") && i+1 < len(cmds) && !strings.HasPrefix(cmds[i+1], "--") {
			// Combine flag with its value
			result = append(result, arg+" "+cmds[i+1])
			i++ // skip the value element
		} else {
			result = append(result, arg)
		}
	}
	return result
}
