package service

import (
	"github.com/user/llm-manager/internal/database/models"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
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
	AttentionLayers    int // only attention layers contribute to KV cache
	GdnLayers          int // GatedDeltaNet layers (hybrid models)
	NumKvHeads         int
	HeadDim            int
	SupportsMtp        bool
	SupportsVision     bool
	DefaultContext     int
	MaxContext         int
	QuantBytesPerParam float64 // bytes per param (2.0 BF16, 1.0 FP8, 0.5 NVFP4)
}

// MemoryBreakdown holds per-component VRAM usage in MB.
type MemoryBreakdown struct {
	WeightsMB          int
	KVCacheMB          int // KV at full context (worst-case)
	KVCacheRealisticMB int // KV at realistic batch size (for util calc)
	GDNStateMB         int
	PrefixCacheMB      int
	MTPMB              int
	CUDAContextMB      int
	OffBudgetMB        int
	VisionEncoderMB    int // vision encoder + projector for multimodal models
}

// MemoryResult holds the computed GPU memory requirements.
type MemoryResult struct {
	TotalMB              int     // worst-case total (for validation)
	TotalRealisticMB     int     // realistic estimate (for util calc)
	KVCacheMaxMB         int     // KV cache at full context (worst-case)
	KVCacheRealisticMB   int     // KV cache at realistic batch size
	GPUMemoryUtilization float64 // rounded up to nearest 0.01
	DockerLimitGB        int
	Breakdown            MemoryBreakdown
	FitsAtMaxContext     bool // true if total_max <= total_gpu_mb
}

// roundUpTo001 rounds a float up to the nearest 0.01.
func roundUpTo001(v float64) float64 {
	return math.Ceil(v*100) / 100
}

// CalculateMemory computes the GPU memory required for a model instance.
// Follows the formula in docs/vllm_memory_calc.md.
//
// availableGPUmb is the amount of GPU memory currently available (from
// torch.cuda.mem_get_info or nvidia-smi). If 0, TotalGPUMB is used
// (single-model scenario with no other GPU consumers).
func CalculateMemory(profile ModelProfile, kvDtypeBytes float64, contextLen int, numSequences int, mtpTokens int, availableGPUmb int) (*MemoryResult, error) {
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

	// Use available GPU memory, or fall back to total if not provided.
	gpuAvailable := availableGPUmb
	if gpuAvailable <= 0 {
		gpuAvailable = TotalGPUMB
	}

	bd := MemoryBreakdown{}

	// 1. Model Weights
	bd.WeightsMB = int(profile.TotalParamsB * profile.QuantBytesPerParam * 1024)

	// 2. KV Cache (0 for encoder models with no attention layers)
	// vLLM's --gpu-memory-utilization reserves a pool of GPU memory.
	// After loading weights, the remainder is used for KV cache.
	// We must reserve enough for KV at the requested context length,
	// because vLLM pre-allocates KV cache for that length at startup.
	if profile.AttentionLayers > 0 {
		kvPerToken := float64(2 * profile.NumKvHeads * profile.HeadDim * profile.AttentionLayers * int(kvDtypeBytes))
		bd.KVCacheMB = int(kvPerToken*float64(contextLen*numSequences)) / (1024 * 1024)
		// Use full-context KV for utilization calculation — vLLM needs this
		// much KV cache memory reserved. The "realistic" estimate was causing
		// OOM because vLLM pre-allocates KV cache for max_model_len.
		bd.KVCacheRealisticMB = bd.KVCacheMB
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
		// MTP overhead is estimated from the worked examples in vllm_memory_calc.md.
		// The formula (active * 500 + active * 333 * tokens + active * 167 * (tokens+1))
		// produces wildly inflated values (e.g., 58 GB for 27B dense MTP=3) that
		// contradict the documented reference table. Use reference values instead.
		//
		// Reference values from docs/vllm_memory_calc.md:
		//   Qwen3.6-35B-A3B (MoE, 3B active), MTP=2  → ~2,000 MB
		//   Qwen3.6-35B-A3B (MoE, 3B active), MTP=3  → ~2,700 MB
		//   Qwen3.6-27B    (dense, 27B active), MTP=2 → ~5,500 MB
		//   Qwen3.6-27B    (dense, 27B active), MTP=3 → ~7,500 MB
		//   Qwen3-Coder-Next (MoE, 3B active), MTP=2 → ~2,000 MB
		//
		// Interpolate: MoE ~670 MB/token, Dense ~2,750 MB/token
		if profile.IsMoe {
			bd.MTPMB = 670 * mtpTokens
		} else {
			bd.MTPMB = 2750 * mtpTokens
		}
	}

	// 8. Vision Encoder + Projector (multimodal models only)
	// vLLM loads a vision encoder (e.g., SigLIP, CLIP) and a vision projector
	// (connector) for multimodal models. These sit in the same reserved pool
	// as the language model weights but were previously unaccounted for,
	// causing ~500 MB underestimation and CUDA OOM during inference.
	//
	// FP8 (1.0 bytes/param):  ~400 MB encoder + ~100 MB projector ≈ 500 MB
	// BF16 (2.0 bytes/param): ~800 MB encoder + ~200 MB projector ≈ 1,000 MB
	// Scaled proportionally for other quantizations.
	if profile.SupportsVision {
		const (
			visionFP8MB  = 500  // FP8 vision encoder + projector
			visionBF16MB = 1000 // BF16 vision encoder + projector
		)
		// Base is FP8 (1.0 bytes/param) = 500 MB. Scale linearly.
		bd.VisionEncoderMB = int(float64(visionFP8MB) * profile.QuantBytesPerParam)
	}

	// 6. CUDA Context and Graph Capture
	if profile.AttentionLayers == 0 {
		bd.CUDAContextMB = 1500 // encoder: context only, no graph capture
	} else {
		bd.CUDAContextMB = 3000 // vLLM: context + graphs
	}

	// 7. Off-Budget Allocations
	// These cover intermediate activation tensors during prefill, FlashInfer/
	// Triton JIT kernel allocations, PyTorch CUDA allocator fragmentation, and
	// temporary buffers. Vision models need extra headroom because the encoder
	// adds additional activation overhead during multimodal processing.
	if profile.AttentionLayers == 0 {
		bd.OffBudgetMB = 500 // encoder
	} else if profile.SupportsVision {
		if contextLen > 65536 {
			bd.OffBudgetMB = 5000 // vision + large context = bigger activations
		} else {
			bd.OffBudgetMB = 4000 // vision model overhead
		}
	} else if contextLen > 65536 {
		bd.OffBudgetMB = 4000 // large context = bigger activation tensors
	} else {
		bd.OffBudgetMB = 3000 // standard
	}

	// Total at MAX context (worst-case, for validation)
	totalMax := bd.WeightsMB + bd.KVCacheMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.OffBudgetMB + bd.VisionEncoderMB

	// Total at realistic batch size (for gpu_memory_utilization)
	// Uses realistic KV cache estimate instead of full-context KV cache
	totalRealistic := bd.WeightsMB + bd.KVCacheRealisticMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.OffBudgetMB + bd.VisionEncoderMB

	// Whether the model can physically fit at the requested context length
	// on this GPU. If false, the model will fail to start with OOM.
	fitsAtMaxContext := totalMax <= gpuAvailable

	// gpu_memory_utilization is a fraction of TOTAL GPU memory (121,856 MB).
	// vLLM always reserves against the total, not free memory. Multi-model
	// safety comes from the sum of all models' reservations fitting within
	// SAFE_USABLE_MB.
	//
	// Add a 2% safety margin to account for untracked overhead: PyTorch
	// CUDA allocator fragmentation, JIT kernel cache growth, activation
	// tensor spikes during prefill, and any vision encoder / multimodal
	// processor overhead that slips past detection.
	utilization := float64(totalRealistic) / float64(TotalGPUMB)
	utilization = utilization * 1.02 // +2% safety margin

	return &MemoryResult{
		TotalMB:            totalMax,       // worst-case total (for validation)
		TotalRealisticMB:   totalRealistic, // realistic estimate (for util calc)
		KVCacheMaxMB:       bd.KVCacheMB,   // KV at full context
		KVCacheRealisticMB: bd.KVCacheRealisticMB,
		GPUMemoryUtilization: roundUpTo001(utilization),
		DockerLimitGB:        int(math.Ceil(float64(totalMax*115) / 102400)),
		Breakdown:            bd,
		FitsAtMaxContext:     fitsAtMaxContext,
	}, nil
}

// ReadFreeGPUMemory queries NVIDIA-SMI for free GPU memory in MB.
// Returns 0 if nvidia-smi is unavailable or fails.
func ReadFreeGPUMemory() int {
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.free",
		"--format=csv,noheader,nounits", "-i", "0")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	line := strings.TrimSpace(string(output))
	val, err := strconv.Atoi(line)
	if err != nil {
		return 0
	}
	return val
}

// EstimateMemory returns a MemoryResult for a model based on its profile data.
// Returns nil and no error if the model has no profile data.
func EstimateMemory(model *models.Model) (*MemoryResult, error) {
	if model.TotalParamsB == nil || model.QuantBytesPerParam == nil {
		return nil, nil // no profile data
	}

	attentionLayers := 0
	if model.AttentionLayers != nil {
		attentionLayers = *model.AttentionLayers
	}
	gdnLayers := 0
	if model.GdnLayers != nil {
		gdnLayers = *model.GdnLayers
	}
	numKvHeads := 0
	if model.NumKvHeads != nil {
		numKvHeads = *model.NumKvHeads
	}
	headDim := 0
	if model.HeadDim != nil {
		headDim = *model.HeadDim
	}
	maxContext := 262144
	if model.MaxContext != nil && *model.MaxContext > 0 {
		maxContext = *model.MaxContext
	}
	defaultContext := 262144
	if model.DefaultContext != nil && *model.DefaultContext > 0 {
		defaultContext = *model.DefaultContext
	}

	profile := ModelProfile{
		TotalParamsB:       *model.TotalParamsB,
		ActiveParamsB:      derefOrZero(model.ActiveParamsB),
		IsMoe:              derefOrFalse(model.IsMoe),
		AttentionLayers:    attentionLayers,
		GdnLayers:          gdnLayers,
		NumKvHeads:         numKvHeads,
		HeadDim:            headDim,
		SupportsMtp:        derefOrFalse(model.SupportsMtp),
		SupportsVision:     false, // RAG models don't have vision capability
		DefaultContext:     defaultContext,
		MaxContext:         maxContext,
		QuantBytesPerParam: *model.QuantBytesPerParam,
	}

	return CalculateMemory(profile, *model.QuantBytesPerParam, defaultContext, 1, 0, 0)
}
