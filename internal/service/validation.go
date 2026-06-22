package service

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// ValidationResult holds the outcome of a multi-model coexistence check.
type ValidationResult struct {
	Fits          bool
	TotalNeededMB int
	SafeUsableMB  int
	HeadroomMB    int
	HeadroomGB    float64
	Risk          string // "safe" (>8GB), "ok" (>4GB), "tight" (>0), "does_not_fit" (<=0)
	Suggestions   []string
}

// DynamicFitResult holds the outcome of a dynamic fit check against current free memory.
type DynamicFitResult struct {
	Fits                 bool
	NeededMB             int
	AvailableMB          int
	FreeMB               int
	HeadroomMB           int
	GPUMemoryUtilization float64
	DockerLimitGB        int
	SafetyMarginMB       int // computed safety margin in MB for this check
}

// ValidateMultiModel checks whether a set of model memory results can coexist
// on the same GPU within the safe usable memory budget.
func ValidateMultiModel(results []MemoryResult) *ValidationResult {
	total := 0
	for _, r := range results {
		total += r.TotalMB
	}
	headroom := SafeUsableMB - total
	headroomGB := math.Round(float64(headroom)/1024*10) / 10

	risk := "does_not_fit"
	if headroom > 0 {
		risk = "tight"
	}
	if headroom > 4096 {
		risk = "ok"
	}
	if headroom > 8192 {
		risk = "safe"
	}

	return &ValidationResult{
		Fits:          headroom >= 0,
		TotalNeededMB: total,
		SafeUsableMB:  SafeUsableMB,
		HeadroomMB:    headroom,
		HeadroomGB:    headroomGB,
		Risk:          risk,
		Suggestions:   nil,
	}
}

// readMemAvailableMB reads /proc/meminfo and returns MemAvailable in MB.
// This is the preferred method on unified-memory systems (DGX Spark GB10).
func readMemAvailableMB() (int, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				return 0, fmt.Errorf("unexpected MemAvailable line format: %s", line)
			}
			var kb int
			fmt.Sscanf(parts[1], "%d", &kb)
			return kb / 1024, nil
		}
	}
	return 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
}

// isSpeechType returns true if the given subtype indicates a speech model.
func isSpeechType(subtype string) bool {
	switch strings.ToLower(subtype) {
	case "stt", "tts", "omni":
		return true
	default:
		return false
	}
}

// CanFitDynamic checks whether a model can fit given current free memory.
// Uses /proc/meminfo MemAvailable as the source of truth.
// safetyMarginPct is a percentage string like "5" (5%) of the model's total
// memory footprint. Defaults to 5% if empty.
func CanFitDynamic(profile ModelProfile, kvDtypeBytes float64, contextLen int, numSequences int, mtpTokens int, safetyMarginPct string) (*DynamicFitResult, error) {
	freeMB, err := readMemAvailableMB()
	if err != nil {
		return nil, err
	}

	mem, err := CalculateMemory(profile, kvDtypeBytes, contextLen, numSequences, mtpTokens, freeMB)
	if err != nil {
		return nil, err
	}

	// Debug: print detailed memory breakdown
	fmt.Fprintf(os.Stderr, "\n=== GPU Memory Calculation ===\n")
	fmt.Fprintf(os.Stderr, "  Profile: %.1fB params, %.1f bytes/param, attention=%d, gdn=%d\n",
		profile.TotalParamsB, profile.QuantBytesPerParam, profile.AttentionLayers, profile.GdnLayers)
	fmt.Fprintf(os.Stderr, "  Context: %d tokens, %d sequences, MTP=%d\n", contextLen, numSequences, mtpTokens)
	fmt.Fprintf(os.Stderr, "  Free GPU: %d\n", ReadFreeGPUMemory())
	fmt.Fprintf(os.Stderr, "  Free RAM (/proc/meminfo): %d MB\n", freeMB)
	fmt.Fprintf(os.Stderr, "  Weights:           %6d MB (%.1fB × %.1f × 1024)\n",
		mem.Breakdown.WeightsMB, profile.TotalParamsB, profile.QuantBytesPerParam)
	fmt.Fprintf(os.Stderr, "  KV Cache (full ctx): %5d MB (kv/token=%.0fKB × %d ctx × %d seq)\n",
		mem.KVCacheMaxMB, kvDtypeBytes*2*float64(profile.NumKvHeads)*float64(profile.HeadDim)*float64(profile.AttentionLayers)/1024, contextLen, numSequences)
	fmt.Fprintf(os.Stderr, "  KV Cache (realistic): %4d MB\n", mem.KVCacheRealisticMB)
	fmt.Fprintf(os.Stderr, "  GDN state:         %6d MB\n", mem.Breakdown.GDNStateMB)
	fmt.Fprintf(os.Stderr, "  Prefix cache:      %6d MB\n", mem.Breakdown.PrefixCacheMB)
	fmt.Fprintf(os.Stderr, "  MTP overhead:      %6d MB\n", mem.Breakdown.MTPMB)
	fmt.Fprintf(os.Stderr, "  Vision encoder:    %6d MB\n", mem.Breakdown.VisionEncoderMB)
	fmt.Fprintf(os.Stderr, "  CUDA ctx+graphs:   %6d MB\n", mem.Breakdown.CUDAContextMB)
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  Total (max ctx):   %6d MB\n", mem.TotalMB)
	fmt.Fprintf(os.Stderr, "  Total (realistic): %6d MB\n", mem.TotalRealisticMB)

	safetyMargin := ComputeSafetyMargin(mem.TotalMB, safetyMarginPct)
	available := freeMB - safetyMargin
	pct := 5.0 // default for display
	if parsed, err := strconv.ParseFloat(safetyMarginPct, 64); err == nil && parsed > 0 {
		pct = parsed
	}
	fmt.Fprintf(os.Stderr, "  Available (free - %.1f%% margin = %d MB): %6d MB\n",
		pct, safetyMargin, available)
	fmt.Fprintf(os.Stderr, "  gpu_memory_utilization: %.2f (total_realistic / %.0f total)\n",
		mem.GPUMemoryUtilization, float64(TotalGPUMB))
	fmt.Fprintf(os.Stderr, "  Docker limit: %dg\n", mem.DockerLimitGB)

	fits := mem.TotalMB <= available
	headroom := 0
	if fits {
		headroom = available - mem.TotalMB
	}

	fmt.Fprintf(os.Stderr, "  Fits: %v (need %d MB <= available %d MB)\n", fits, mem.TotalMB, available)

	return &DynamicFitResult{
		Fits:                 fits,
		NeededMB:             mem.TotalMB,
		AvailableMB:          available,
		FreeMB:               freeMB,
		HeadroomMB:           headroom,
		GPUMemoryUtilization: mem.GPUMemoryUtilization,
		DockerLimitGB:        mem.DockerLimitGB,
		SafetyMarginMB:       safetyMargin,
	}, nil
}
