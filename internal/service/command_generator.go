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
	if contextLen > 0 {
		flags.MaxModelLen = strconv.Itoa(contextLen)
	} else if profile.DefaultContext > 0 {
		flags.MaxModelLen = strconv.Itoa(profile.DefaultContext)
	} else {
		flags.MaxModelLen = "65536"
	}

	// --max-num-batched-tokens: derive from off_budget_mb, default 8192
	flags.MaxNumBatchedTokens = deriveBatchedTokens(memResult)

	// --max-num-seqs: use provided, default 1
	if numSequences > 0 {
		flags.MaxNumSeqs = strconv.Itoa(numSequences)
	} else {
		flags.MaxNumSeqs = "1"
	}

	// --gpu-memory-utilization: from calculated ratio, formatted to 2 decimal places
	flags.GPUMemoryUtil = fmt.Sprintf("%.2f", memResult.GPUMemoryUtilization)

	return flags
}

// deriveBatchedTokens derives max-num-batched-tokens from the memory breakdown.
func deriveBatchedTokens(mem *MemoryResult) string {
	switch {
	case mem.Breakdown.OffBudgetMB >= 4000:
		return "32768"
	case mem.Breakdown.OffBudgetMB >= 3000:
		return "16384"
	default:
		return "8192"
	}
}

// ParseExistingFlags extracts existing flag values from a command array.
// Flags are stored as ["--flag-name", "value", ...] pairs.
// Returns a map of flag name -> value.
func ParseExistingFlags(cmds []string) map[string]string {
	existing := make(map[string]string)
	for i := 0; i < len(cmds); i++ {
		if strings.HasPrefix(cmds[i], "--") && i+1 < len(cmds) {
			name := strings.TrimPrefix(cmds[i], "--")
			existing[name] = cmds[i+1]
		}
	}
	return existing
}

// MergeFlags takes existing command args and merges auto-generated flags into them.
// Existing flags are replaced by value; missing flags are appended.
// All other YAML-provided flags remain untouched.
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
		if strings.HasPrefix(arg, "--") && i+1 < len(existingCmds) {
			name := strings.TrimPrefix(arg, "--")
			if newVal, ok := replacements[name]; ok {
				// Replace the value of this flag
				result = append(result, fmt.Sprintf("--%s %s", name, newVal))
				done[name] = true
				i++ // skip the old value element
				continue
			}
		}
		result = append(result, arg)
	}

	// Append missing flags
	for name, val := range replacements {
		if !done[name] {
			result = append(result, fmt.Sprintf("--%s %s", name, val))
		}
	}

	return result
}
