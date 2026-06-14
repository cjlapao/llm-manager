package service

import (
	"encoding/json"
	"fmt"
	"github.com/user/llm-manager/internal/database/models"
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
	TotalParamsB              float64 // total parameter count in billions
	ActiveParamsB             float64 // active parameters in billions (MoE)
	IsMoe                     bool
	AttentionLayers           int // only attention layers contribute to KV cache
	GdnLayers                 int // GatedDeltaNet layers (hybrid models)
	NumKvHeads                int
	HeadDim                   int
	SupportsMtp               bool
	SupportsVision            bool
	DefaultContext            int
	MaxContext                int
	QuantBytesPerParam        float64 // bytes per param (2.0 BF16, 1.0 FP8, 0.5 NVFP4)
	MaxNumSeqs                int     // optional override for number of concurrent sequences
	SubType                   string  // "chat", "embedding", "reranker", etc.
	KvCacheOverheadMultiplier float64 // applied to raw KV bytes (1.0 default, 1.44 for hybrid/GDN models)
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

	VisionEncoderMB int // vision encoder + projector for multimodal models
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
// availableGPUmb is the amount of free system memory currently available
// (from /proc/meminfo on unified-memory systems, or torch.cuda.mem_get_info
// on traditional GPU systems). If 0, TotalGPUMB is used (single-model
// scenario with no other memory consumers).
//
// The utilization formula is always totalRealistic / TotalGPUMB (with a
// 2% safety margin), matching vLLM's check:
//
//	free_after_weights >= utilization × TotalGPUMB
//
// availableGPUmb is used only for the fit check, not for utilization scaling.
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
		kvPerToken := 2.0 * float64(profile.NumKvHeads) * float64(profile.HeadDim) * float64(profile.AttentionLayers) * kvDtypeBytes
		effectiveKvPerToken := kvPerToken * profile.KvCacheOverheadMultiplier
		bd.KVCacheMB = int(effectiveKvPerToken*float64(contextLen*seqs)) / (1024 * 1024)
		// Realistic: assume ~50% average context utilization across sequences
		realisticContext := contextLen / 2
		bd.KVCacheRealisticMB = int(effectiveKvPerToken*float64(realisticContext*seqs)) / (1024 * 1024)
	}

	// 3. GDN Recurrent State (hybrid models only)
	if profile.GdnLayers > 0 {
		bd.GDNStateMB = 50 * profile.GdnLayers * seqs / 3
	}

	// 4. Prefix Cache Overhead
	if seqs >= 2 {
		bd.PrefixCacheMB = 2048
	} else {
		bd.PrefixCacheMB = 1024
	}

	// 5. MTP Speculative Decoding Overhead
	// vLLM's MTP does NOT load a separate draft model copy — speculative heads
	// share the loaded checkpoint through views/aliases. The actual overhead is
	// activation buffers + draft KV cache per speculative token.
	// MoE models use a smaller speculative tree and thus less overhead.
	if mtpTokens > 0 && profile.SupportsMtp {
		if profile.IsMoe {
			bd.MTPMB = 670 * mtpTokens
		} else {
			// Dense MTP: activation buffers + draft KV cache per speculative token
			bd.MTPMB = 800 * mtpTokens
		}
	}

	// 7. Vision Encoder + Projector (multimodal models only)
	if profile.SupportsVision {
		const (
			visionFP8MB  = 500
			visionBF16MB = 1000
		)
		bd.VisionEncoderMB = int(float64(visionFP8MB) * profile.QuantBytesPerParam)
	}

	// 6. CUDA Context and Graph Capture
	switch {
	case profile.QuantBytesPerParam == 0.5 && profile.SupportsVision:
		// Multi-modal NVFP4/MVP4 + async scheduling + instanttensor loader
		// maintains both loaded tensors and intermediate buffers simultaneously;
		// MARLIN gemm libraries hook into custom kernels — larger capture needed.
		bd.CUDAContextMB = 7000
	case profile.QuantBytesPerParam == 0.5:
		// Non-standard quantization (INT4/NF4/NVFP4) = larger graph capture
		bd.CUDAContextMB = 5000
	default:
		bd.CUDAContextMB = 3000
	}
	if profile.AttentionLayers == 0 {
		bd.CUDAContextMB /= 2
	}

	// Total at MAX context (worst-case, for validation)
	totalMax := bd.WeightsMB + bd.KVCacheMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.VisionEncoderMB

	// Total at realistic batch size (for gpu_memory_utilization)
	totalRealistic := bd.WeightsMB + bd.KVCacheRealisticMB + bd.GDNStateMB + bd.PrefixCacheMB + bd.MTPMB + bd.CUDAContextMB + bd.VisionEncoderMB

	// Encoder/RAG models (0 attention layers) need special handling.
	// Embedding models have NO KV cache (no attention mechanism at all).
	// Reranker models ARE treated as generative by vLLM and DO allocate
	// KV cache even though our profile has attention_layers=0.
	//
	// Empirical: Qwen3-Reranker-0.6B at max_model_len=8192 needs ~900 MB KV cache.
	// Heuristic: ~112 KB per token for cross-encoders (key+value projections).
	// Embeddings need 0 KV cache — no attention layers means no KV storage.
	//
	// vLLM overhead model: for encoder models, vLLM's internal overhead
	// (activation buffers, etc.) is approximately 3× weights_size. This is
	// significantly higher than the CUDA context + prefix cache we model
	// for attention-based models, because vLLM treats encoder models as
	// generative and allocates full activation buffers during prefill.
	if profile.AttentionLayers == 0 {
		// vLLM overhead for encoder models: ~3× weights (activation buffers)
		vllmOverheadMB := int(bd.WeightsMB * 3.0)
		if profile.SubType == "reranker" {
			var kvPerTokenKB int
			kvPerTokenKB = 112
			kvEstimateMB := contextLen * kvPerTokenKB / 1024 * seqs
			if kvEstimateMB < 256 {
				kvEstimateMB = 256
			}
			bd.KVCacheRealisticMB = kvEstimateMB
			bd.KVCacheMB = kvEstimateMB
			// totalRealistic = weights + KV + vLLM overhead (no CUDA/prefix for encoders)
			totalRealistic = bd.WeightsMB + bd.KVCacheRealisticMB + vllmOverheadMB
			totalMax = bd.WeightsMB + bd.KVCacheMB + vllmOverheadMB
		} else {
			// Embeddings: no KV cache, but still need vLLM overhead
			totalRealistic = bd.WeightsMB + vllmOverheadMB
			totalMax = bd.WeightsMB + vllmOverheadMB
		}
		// Update breakdown to include vLLM overhead for transparency
		bd.PrefixCacheMB = vllmOverheadMB
	}

	// Whether the model can physically fit at the requested context length
	// on this GPU.
	fitsAtMaxContext := totalMax <= gpuAvailable

	// ── Encoder/RAG models (0 attention layers) ──
	// These have no KV cache in our model, but vLLM treats rerankers as
	// generative models and DOES allocate KV cache.
	//
	// Utilization is always totalRealistic / TotalGPUMB, matching vLLM's
	// check: free_after_weights >= utilization × TotalGPUMB.
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
	// Utilization is always totalRealistic / TotalGPUMB, matching vLLM's
	// check: free_after_weights >= utilization × TotalGPUMB.
	utilization := float64(totalRealistic) / float64(TotalGPUMB)
	utilization = utilization * 1.02 // +2% safety margin

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

// ComputeSafetyMargin returns the safety margin in MB for the given total
// model footprint. pctStr is a percentage like "5" (meaning 5%).
// Defaults to 5% if pctStr is empty or invalid.
func ComputeSafetyMargin(totalMB int, pctStr string) int {
	pct := 5.0 // default
	if parsed, err := strconv.ParseFloat(pctStr, 64); err == nil && parsed > 0 {
		pct = parsed
	}
	return int(float64(totalMB) * pct / 100)
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

// getKVDtypeBytes parses the --kv-cache-dtype flag from the model's
// command args and returns the corresponding bytes per KV cache element.
//
// Supported KV cache dtypes:
//
//	fp8_e4m3      -> 1.0  (8-bit floating point, E4M3 exponent)
//	fp8_e5m2      -> 1.0  (8-bit floating point, E5M2 exponent)
//	fp8           -> 1.0  (alias for fp8_e4m3)
//	float         -> 2.0  (16-bit floating point / FP16)
//	half          -> 2.0  (alias for float/FP16)
//	bf16          -> 2.0  (Brain floating point 16)
//	float16       -> 2.0  (alias for bf16/FP16)
//	double        -> 4.0  (32-bit floating point)
//	int8          -> 1.0  (8-bit integer)
//	auto          -> 2.0  (default: vLLM infers from model dtype, usually FP16/BF16)
//	<none>        -> 2.0  (no --kv-cache-dtype flag = defaults to float16/BF16)
func getKVDtypeBytes(commandArgs string) float64 {
	// Parse the command args (which are JSON-encoded in the DB) to find --kv-cache-dtype.
	// The command args stored in the DB are a JSON array like:
	//   ["${{ .hf_repo }}", "--model ...", "--kv-cache-dtype fp8", ...]
	// We need to unmarshal and search for the flag.
	var args []string
	if err := json.Unmarshal([]byte(commandArgs), &args); err != nil {
		// If we can't parse the JSON, fall back to searching the raw string.
		args = nil
	}

	for _, arg := range args {
		// Handle combined flags like "--kv-cache-dtype fp8"
		if strings.HasPrefix(arg, "--kv-cache-dtype") {
			var dtype string
			// Check if value is attached: --kv-cache-dtype=fp8
			if idx := strings.Index(arg, "="); idx >= 0 {
				dtype = strings.TrimPrefix(arg[idx+1:], " ")
			} else {
				// Split on space: --kv-cache-dtype fp8
				parts := strings.Fields(arg)
				if len(parts) > 1 {
					dtype = parts[1]
				}
			}
			return kvDtypeBytesFor(dtype)
		}
	}

	// No --kv-cache-dtype found — default to float16/BF16 (2.0 bytes/element).
	return 2.0
}

// kvDtypeBytesFor returns the bytes per element for a given KV cache dtype string.
func kvDtypeBytesFor(dtype string) float64 {
	switch strings.ToLower(dtype) {
	case "fp8_e4m3", "fp8_e5m2", "fp8":
		return 1.0
	case "float", "half", "bf16", "float16", "float32":
		return 2.0
	case "double", "float64":
		return 4.0
	case "int8":
		return 1.0
	case "auto":
		return 2.0 // vLLM infers from model dtype, typically FP16/BF16
	default:
		return 2.0 // fallback to float16
	}
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
		SubType:            model.SubType,
	}

	seqs := profile.MaxNumSeqs
	if seqs < 1 {
		seqs = 1
	}

	// Detect hybrid (GDN) models and apply KV cache overhead multiplier.
	// Dense hybrid models with MTP need a higher multiplier because the draft
	// model's speculative tokens also consume KV cache entries, and GDN
	// attention kernels allocate additional page buffers.
	if profile.GdnLayers > 0 {
		if profile.SupportsMtp && !profile.IsMoe {
			profile.KvCacheOverheadMultiplier = 2.00
		} else {
			profile.KvCacheOverheadMultiplier = 1.44
		}
	}

	return CalculateMemory(profile, getKVDtypeBytes(model.CommandArgs), defaultContext, seqs, 0, 0)
}
