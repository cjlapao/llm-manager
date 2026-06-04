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
	MaxNumSeqs         int     // optional override for number of concurrent sequences
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
	KVCacheMaxMB         int     // KV at full context
	KVCacheRealisticMB   int     // KV at realistic batch size
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

	// Use profile override for numSequences if set, otherwise use the passed value.
	seqs := numSequences
	if profile.MaxNumSeqs > 0 {
		seqs = profile.MaxNumSeqs
	}

	// 1. Model Weights
	bd.WeightsMB = int(profile.TotalParamsB * profile.QuantBytesPerParam * 1024)

	// 2. KV Cache (0 for encoder models with no attention layers)
	if profile.AttentionLayers > 0 {
		kvPerToken := float64(2 * profile.NumKvHeads * profile.HeadDim * profile.AttentionLayers * int(kvDtypeBytes))
		bd.KVCacheMB = int(kvPerToken*float64(contextLen*seqs)) / (1024 * 1024)
		bd.KVCacheRealisticMB = bd.KVCacheMB
	}

	// 3. GDN Recurrent State (hybrid models only)
	if profile.GdnLayers > 0 {
		bd.GDNStateMB = 50 * seqs
	}

	// 4. Prefix Cache Overhead
	if seqs >= 2 {
		bd.PrefixCacheMB = 2048
	} else {
		bd.PrefixCacheMB = 1024
	}

	// 5. MTP Speculative Decoding Overhead
	if mtpTokens > 0 && profile.SupportsMtp && profile.QuantBytesPerParam != 0.5 {
		if profile.IsMoe {
			bd.MTPMB = 670 * mtpTokens
		} else {
			bd.MTPMB = 2750 * mtpTokens
		}
	}

	// 8. Vision Encoder + Projector (multimodal models only)
	if profile.SupportsVision {
		const (
			visionFP8MB  = 500
			visionBF16MB = 1000
		)
		bd.VisionEncoderMB = int(float64(visionFP8MB) * profile.QuantBytesPerParam)
	}

	// 6. CUDA Context and Graph Capture
	if profile.AttentionLayers == 0 {
		bd.CUDAContextMB = 1500
	} else {
		bd.CUDAContextMB = 3000
	}

	// 8. Off-Budget Allocations
	// These cover intermediate activation tensors during prefill, FlashInfer/
	// Triton JIT kernel allocations, PyTorch CUDA allocator fragmentation, and
	// temporary buffers.
	if profile.AttentionLayers == 0 {
		bd.OffBudgetMB = 2000
	} else if profile.SupportsVision {
		if contextLen > 65536 {
			bd.OffBudgetMB = 5000
		} else {
			bd.OffBudgetMB = 4000
		}
	} else if contextLen > 65536 {
		bd.OffBudgetMB = 4000
	} else {
		bd.OffBudgetMB = 3000
	}

	// Total at MAX context (worst-case, for validation)
	totalMax := bd.WeightsMB + bd.KVCacheMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.OffBudgetMB + bd.VisionEncoderMB

	// Total at realistic batch size (for gpu_memory_utilization)
	totalRealistic := bd.WeightsMB + bd.KVCacheRealisticMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.OffBudgetMB + bd.VisionEncoderMB

	// Whether the model can physically fit at the requested context length
	// on this GPU.
	fitsAtMaxContext := totalMax <= gpuAvailable

	// ── Encoder/RAG models (0 attention layers) ──
	// These have no KV cache, so their memory footprint is dominated by weights
	// plus vLLM's internal overhead: CUDA context, JIT kernel caches, PagedAttention
	// metadata, tensor parallelism structures, activation buffers, PyTorch allocator
	// fragmentation, etc.
	//
	// We compute the total from the breakdown (weights + CUDA context + off-budget
	// + prefix cache) and express it as a fraction of total GPU memory.
	// This gives a stable reservation that works regardless of GPU sharing.
	if profile.AttentionLayers == 0 {
		utilization := float64(totalRealistic) / float64(TotalGPUMB)
		utilization = roundUpTo001(utilization)
		// Absolute minimum: vLLM needs some headroom to initialize even
		// for tiny models. Floor at 0.01 (~1.2 GB of the 120 GB GPU).
		if utilization < 0.01 {
			utilization = 0.01
		}
		return &MemoryResult{
			TotalMB:              totalMax,
			TotalRealisticMB:     totalRealistic,
			KVCacheMaxMB:         bd.KVCacheMB,
			KVCacheRealisticMB:   bd.KVCacheRealisticMB,
			GPUMemoryUtilization: utilization,
			DockerLimitGB:        int(math.Ceil(float64(totalMax*115) / 102400)),
			Breakdown:            bd,
			FitsAtMaxContext:     fitsAtMaxContext,
		}, nil
	}

	// ── Standard path for non-encoder models (LLM, chat, auto-complete) ──
	utilization := float64(totalRealistic) / float64(TotalGPUMB)
	utilization = utilization * 1.02 // +2% safety margin

	// If other models are running, scale utilization against available GPU memory
	if gpuAvailable > 0 && gpuAvailable < TotalGPUMB {
		availUtil := float64(totalRealistic) / float64(gpuAvailable)
		availUtil = availUtil * 1.02
		if availUtil > utilization {
			utilization = availUtil
		}
	}

	return &MemoryResult{
		TotalMB:              totalMax,
		TotalRealisticMB:     totalRealistic,
		KVCacheMaxMB:         bd.KVCacheMB,
		KVCacheRealisticMB:   bd.KVCacheRealisticMB,
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

// SetGPUMemorySource sets the source for GPU memory queries.
// Currently a no-op — the service always uses nvidia-smi.
func SetGPUMemorySource(source string) {
	_ = source
}

// EstimateMemory returns a MemoryResult for a model based on its profile data.
// Returns nil and no error if the model has no profile data.
func EstimateMemory(model *models.Model) (*MemoryResult, error) {
	if model.TotalParamsB == nil || model.QuantBytesPerParam == nil {
		return nil, nil
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
		SupportsVision:     false,
		DefaultContext:     defaultContext,
		MaxContext:         maxContext,
		QuantBytesPerParam: *model.QuantBytesPerParam,
		MaxNumSeqs:         derefOrZero(model.MaxNumSeqs),
	}

	return CalculateMemory(profile, *model.QuantBytesPerParam, defaultContext, 1, 0, 0)
}
