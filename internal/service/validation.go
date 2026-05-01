package service

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
)

// ValidationResult holds the outcome of a multi-model coexistence check.
type ValidationResult struct {
	Fits           bool
	TotalNeededMB  int
	SafeUsableMB   int
	HeadroomMB     int
	HeadroomGB     float64
	Risk           string // "safe" (>8GB), "ok" (>4GB), "tight" (>0), "does_not_fit" (<=0)
	Suggestions    []string
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
		Fits:           headroom >= 0,
		TotalNeededMB:  total,
		SafeUsableMB:   SafeUsableMB,
		HeadroomMB:     headroom,
		HeadroomGB:     headroomGB,
		Risk:           risk,
		Suggestions:    nil,
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

// CanFitDynamic checks whether a model can fit given current free memory.
// Uses /proc/meminfo MemAvailable as the source of truth.
func CanFitDynamic(profile ModelProfile, kvDtypeBytes float64, contextLen int, numSequences int, mtpTokens int) (*DynamicFitResult, error) {
	mem, err := CalculateMemory(profile, kvDtypeBytes, contextLen, numSequences, mtpTokens)
	if err != nil {
		return nil, err
	}

	freeMB, err := readMemAvailableMB()
	if err != nil {
		return nil, err
	}

	safetyMargin := 5120 // 5 GB
	available := freeMB - safetyMargin
	fits := mem.TotalMB <= available
	headroom := 0
	if fits {
		headroom = available - mem.TotalMB
	}

	return &DynamicFitResult{
		Fits:                 fits,
		NeededMB:             mem.TotalMB,
		AvailableMB:          available,
		FreeMB:               freeMB,
		HeadroomMB:           headroom,
		GPUMemoryUtilization: mem.GPUMemoryUtilization,
		DockerLimitGB:        mem.DockerLimitGB,
	}, nil
}
