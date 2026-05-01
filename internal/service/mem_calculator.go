package service

import (
	"fmt"
	"math"
)

// System constants from vLLM memory calculator spec.
const (
	TotalGPUMB        = 121856 // 119 GB as reported by nvidia-smi
	OSReserveMB       = 7168   // 7 GB — kernel, system services, Docker daemon
	FileBufferMB      = 5120   // 5 GB — Linux page cache, disk buffers
	SystemReserveMB   = 12288  // OS_RESERVE + FILE_BUFFER = 12 GB
	UsableMB          = 109568 // TOTAL_GPU_MB - SYSTEM_RESERVE_MB
	SafeUsableMB      = 105912 // USABLE_MB - EARLYOOM_RESERVE_MB (3% of total)
	EarlyoomThreshold = 3      // percent — earlyoom kills at 3% free
	EarlyoomReserveMB = 3656   // 3% of TOTAL_GPU_MB
)

// ModelProfile holds architecture-specific constants for GPU memory calculation.
type ModelProfile struct {
	TotalParamsB       float64 // total parameter count in billions
	ActiveParamsB      float64 // active parameters in billions (MoE)
	IsMoe              bool
	AttentionLayers    int     // only attention layers contribute to KV cache
	GdnLayers          int     // GatedDeltaNet layers (hybrid models)
	NumKvHeads         int
	HeadDim            int
	SupportsMtp        bool
	DefaultContext     int
	MaxContext         int
	QuantBytesPerParam float64 // bytes per param (2.0 BF16, 1.0 FP8, 0.5 NVFP4)
}

// MemoryBreakdown holds per-component VRAM usage in MB.
type MemoryBreakdown struct {
	WeightsMB     int
	KVCacheMB     int
	GDNStateMB    int
	PrefixCacheMB int
	MTPMB         int
	CUDAContextMB int
	OffBudgetMB   int
}

// MemoryResult holds the computed GPU memory requirements.
type MemoryResult struct {
	TotalMB              int
	GPUMemoryUtilization float64 // rounded up to nearest 0.01
	DockerLimitGB        int
	Breakdown            MemoryBreakdown
}

// roundUpTo001 rounds a float up to the nearest 0.01.
func roundUpTo001(v float64) float64 {
	return math.Ceil(v*100) / 100
}

// CalculateMemory computes the GPU memory required for a model instance.
// Follows the formula in docs/vllm_memory_calc.md.
func CalculateMemory(profile ModelProfile, kvDtypeBytes float64, contextLen int, numSequences int, mtpTokens int) (*MemoryResult, error) {
	// Validate inputs
	if contextLen > 0 && profile.MaxContext > 0 && contextLen > profile.MaxContext {
		return nil, fmt.Errorf("context length %d exceeds max %d", contextLen, profile.MaxContext)
	}
	if profile.TotalParamsB <= 0 {
		return nil, fmt.Errorf("total_params_b must be > 0, got %f", profile.TotalParamsB)
	}
	if profile.QuantBytesPerParam <= 0 {
		return nil, fmt.Errorf("quant_bytes_per_param must be > 0, got %f", profile.QuantBytesPerParam)
	}

	bd := MemoryBreakdown{}

	// 1. Model Weights
	bd.WeightsMB = int(profile.TotalParamsB * profile.QuantBytesPerParam * 1024)

	// 2. KV Cache (0 for encoder models with no attention layers)
	if profile.AttentionLayers > 0 {
		kvPerToken := float64(2 * profile.NumKvHeads * profile.HeadDim * profile.AttentionLayers * int(kvDtypeBytes))
		bd.KVCacheMB = int(kvPerToken*float64(contextLen*numSequences)) / (1024 * 1024)
	}

	// 3. GDN Recurrent State (hybrid models only)
	if profile.GdnLayers > 0 {
		bd.GDNStateMB = 50 * numSequences
	}

	// 4. Prefix Cache Overhead
	if numSequences >= 2 {
		bd.PrefixCacheMB = 2048 // heavy multi-agent
	} else {
		bd.PrefixCacheMB = 1024 // standard
	}

	// 5. MTP Speculative Decoding Overhead
	// NVFP4 (0.5) does NOT support MTP. Also skip if model doesn't support MTP.
	if mtpTokens > 0 && profile.SupportsMtp && profile.QuantBytesPerParam != 0.5 {
		active := profile.ActiveParamsB
		mtpHead := active * 500
		mtpDraft := active * 333 * float64(mtpTokens)
		mtpVerify := active * 167 * float64(mtpTokens+1)
		bd.MTPMB = int(mtpHead + mtpDraft + mtpVerify)
	}

	// 6. CUDA Context and Graph Capture
	if profile.AttentionLayers == 0 {
		bd.CUDAContextMB = 1500 // encoder: context only, no graph capture
	} else {
		bd.CUDAContextMB = 3000 // vLLM: context + graphs
	}

	// 7. Off-Budget Allocations
	if profile.AttentionLayers == 0 {
		bd.OffBudgetMB = 500 // encoder
	} else {
		bd.OffBudgetMB = 3000 // standard
	}

	// Total
	total := bd.WeightsMB + bd.KVCacheMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.OffBudgetMB

	return &MemoryResult{
		TotalMB:              total,
		GPUMemoryUtilization: roundUpTo001(float64(total) / TotalGPUMB),
		DockerLimitGB:        int(math.Ceil(float64(total*115) / 102400)),
		Breakdown:            bd,
	}, nil
}
