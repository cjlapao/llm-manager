package service

import (
	"testing"

	"github.com/user/llm-manager/pkg/yamlparser"
)

// TestCalculateMemory_Qwen36_35b_A3B_FP8 reads the model profile from the
// YAML file, converts it to a service.ModelProfile, and calls CalculateMemory
// with various parameters. It prints all intermediate values for inspection.
func TestCalculateMemory_Qwen36_35b_A3B_FP8(t *testing.T) {
	// 1. Read the YAML profile
	yamlPath := "../../models/qwen3.6-35b-a3b-fp8.yaml"
	yamlData, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	if yamlData.Profile == nil {
		t.Fatal("YAML profile is nil")
	}

	// 2. Convert yamlparser.ModelProfile → service.ModelProfile
	//    (dereference pointer fields to value fields)
	sp := yamlData.Profile

	profile := ModelProfile{
		TotalParamsB:       derefFloat64(sp.TotalParamsB),
		ActiveParamsB:      derefFloat64(sp.ActiveParamsB),
		IsMoe:              derefBool(sp.IsMoe),
		AttentionLayers:    derefInt(sp.AttentionLayers),
		GdnLayers:          derefInt(sp.GdnLayers),
		NumKvHeads:         derefInt(sp.NumKvHeads),
		HeadDim:            derefInt(sp.HeadDim),
		SupportsMtp:        derefBool(sp.SupportsMtp),
		DefaultContext:     derefInt(sp.DefaultContext),
		MaxContext:         derefInt(sp.MaxContext),
		QuantBytesPerParam: derefFloat64(sp.QuantBytesPerParam),
		MaxNumSeqs:         derefInt(sp.MaxNumSeqs),
		SubType:            yamlData.SubType,
	}

	t.Logf("=== Model Profile (converted) ===")
	t.Logf("  TotalParamsB:       %.1f", profile.TotalParamsB)
	t.Logf("  ActiveParamsB:      %.1f", profile.ActiveParamsB)
	t.Logf("  IsMoe:              %v", profile.IsMoe)
	t.Logf("  AttentionLayers:    %d", profile.AttentionLayers)
	t.Logf("  GdnLayers:          %d", profile.GdnLayers)
	t.Logf("  NumKvHeads:         %d", profile.NumKvHeads)
	t.Logf("  HeadDim:            %d", profile.HeadDim)
	t.Logf("  SupportsMtp:        %v", profile.SupportsMtp)
	t.Logf("  DefaultContext:     %d", profile.DefaultContext)
	t.Logf("  MaxContext:         %d", profile.MaxContext)
	t.Logf("  QuantBytesPerParam: %.1f", profile.QuantBytesPerParam)
	t.Logf("  MaxNumSeqs:         %d", profile.MaxNumSeqs)
	t.Logf("  SubType:            %s", profile.SubType)

	// 3. Test with availableGPUmb=0 (baseline)
	t.Run("baseline_no_free_ram", func(t *testing.T) {
		kvDtypeBytes := 1.0  // FP8
		contextLen := 262144 // 256k
		numSequences := 8
		mtpTokens := 0
		availableGPUmb := 0

		result, err := CalculateMemory(profile, kvDtypeBytes, contextLen, numSequences, mtpTokens, availableGPUmb)
		if err != nil {
			t.Fatalf("CalculateMemory() error: %v", err)
		}

		t.Logf("=== Memory Breakdown (availableGPUmb=%d) ===", availableGPUmb)
		t.Logf("  WeightsMB:          %d", result.Breakdown.WeightsMB)
		t.Logf("  KVCacheMB:          %d", result.Breakdown.KVCacheMB)
		t.Logf("  KVCacheRealisticMB: %d", result.Breakdown.KVCacheRealisticMB)
		t.Logf("  GDNStateMB:         %d", result.Breakdown.GDNStateMB)
		t.Logf("  PrefixCacheMB:      %d", result.Breakdown.PrefixCacheMB)
		t.Logf("  MTPMB:              %d", result.Breakdown.MTPMB)
		t.Logf("  CUDAContextMB:      %d", result.Breakdown.CUDAContextMB)
		t.Logf("  VisionEncoderMB:    %d", result.Breakdown.VisionEncoderMB)
		t.Logf("  TotalMB (max):      %d", result.TotalMB)
		t.Logf("  TotalRealisticMB:   %d", result.TotalRealisticMB)
		t.Logf("  GPUMemUtilization:  %.4f", result.GPUMemoryUtilization)
		t.Logf("  DockerLimitGB:      %d", result.DockerLimitGB)
		t.Logf("  FitsAtMaxContext:   %v", result.FitsAtMaxContext)
	})

	// 4. Test with availableGPUmb=41407 (free RAM scenario)
	t.Run("with_free_ram", func(t *testing.T) {
		kvDtypeBytes := 1.0  // FP8
		contextLen := 262144 // 256k
		numSequences := 8
		mtpTokens := 0
		availableGPUmb := 41407

		result, err := CalculateMemory(profile, kvDtypeBytes, contextLen, numSequences, mtpTokens, availableGPUmb)
		if err != nil {
			t.Fatalf("CalculateMemory() error: %v", err)
		}

		t.Logf("=== Memory Breakdown (availableGPUmb=%d) ===", availableGPUmb)
		t.Logf("  WeightsMB:          %d", result.Breakdown.WeightsMB)
		t.Logf("  KVCacheMB:          %d", result.Breakdown.KVCacheMB)
		t.Logf("  KVCacheRealisticMB: %d", result.Breakdown.KVCacheRealisticMB)
		t.Logf("  GDNStateMB:         %d", result.Breakdown.GDNStateMB)
		t.Logf("  PrefixCacheMB:      %d", result.Breakdown.PrefixCacheMB)
		t.Logf("  MTPMB:              %d", result.Breakdown.MTPMB)
		t.Logf("  CUDAContextMB:      %d", result.Breakdown.CUDAContextMB)
		t.Logf("  VisionEncoderMB:    %d", result.Breakdown.VisionEncoderMB)
		t.Logf("  TotalMB (max):      %d", result.TotalMB)
		t.Logf("  TotalRealisticMB:   %d", result.TotalRealisticMB)
		t.Logf("  GPUMemUtilization:  %.4f", result.GPUMemoryUtilization)
		t.Logf("  DockerLimitGB:      %d", result.DockerLimitGB)
		t.Logf("  FitsAtMaxContext:   %v", result.FitsAtMaxContext)
	})
}

// TestCalculateMemory_Qwen36_27b_NVFP4 validates the memory calculation for the
// Qwen3.6 27B NVFP4 dense hybrid model with MTP. The MTP overhead uses a flat
// per-token cost (800 MB/token for activation buffers + draft KV cache) because
// vLLM's MTP shares the loaded checkpoint through views/aliases rather than
// loading a separate draft model copy.
func TestCalculateMemory_Qwen36_27b_NVFP4(t *testing.T) {
	yamlPath := "../../models/qwen3.6-27b-nvfp4.yaml"
	yamlData, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}
	if yamlData.Profile == nil {
		t.Fatal("YAML profile is nil")
	}
	sp := yamlData.Profile

	profile := ModelProfile{
		TotalParamsB:       derefFloat64(sp.TotalParamsB),
		ActiveParamsB:      derefFloat64(sp.ActiveParamsB),
		IsMoe:              derefBool(sp.IsMoe),
		AttentionLayers:    derefInt(sp.AttentionLayers),
		GdnLayers:          derefInt(sp.GdnLayers),
		NumKvHeads:         derefInt(sp.NumKvHeads),
		HeadDim:            derefInt(sp.HeadDim),
		SupportsMtp:        derefBool(sp.SupportsMtp),
		SupportsVision:     true, // Qwen3.6-27B is multimodal
		DefaultContext:     derefInt(sp.DefaultContext),
		MaxContext:         derefInt(sp.MaxContext),
		QuantBytesPerParam: derefFloat64(sp.QuantBytesPerParam),
		MaxNumSeqs:         1,    // runtime uses --max-num-seqs 1
		SubType:            yamlData.SubType,
	}

	// Apply the GDN overhead multiplier that the real code path applies.
	if profile.GdnLayers > 0 {
		if profile.SupportsMtp && !profile.IsMoe {
			profile.KvCacheOverheadMultiplier = 2.00
		} else {
			profile.KvCacheOverheadMultiplier = 1.44
		}
	}

	t.Logf("=== Model Profile: Qwen3.6 27B NVFP4 ===")
	t.Logf("  TotalParamsB:       %.1f", profile.TotalParamsB)
	t.Logf("  IsMoe:              %v", profile.IsMoe)
	t.Logf("  AttentionLayers:    %d", profile.AttentionLayers)
	t.Logf("  GdnLayers:          %d", profile.GdnLayers)
	t.Logf("  NumKvHeads:         %d", profile.NumKvHeads)
	t.Logf("  HeadDim:            %d", profile.HeadDim)
	t.Logf("  SupportsMtp:        %v", profile.SupportsMtp)
	t.Logf("  SupportsVision:     %v", profile.SupportsVision)
	t.Logf("  QuantBytesPerParam: %.1f", profile.QuantBytesPerParam)
	t.Logf("  KvCacheOverhead:    %.2f", profile.KvCacheOverheadMultiplier)
	t.Logf("  MaxNumSeqs:         %d", profile.MaxNumSeqs)

	// Test with MTP enabled (num_speculative_tokens=3 from profile).
	// Note: the YAML has num_speculative_tokens=3 but the runtime showed
	// it was overridden to 1 via CLI. We test both.
	t.Run("mtp_3_tokens", func(t *testing.T) {
		kvDtypeBytes := 1.0  // fp8
		contextLen := 262144
		numSequences := 1 // profile default
		mtpTokens := 3

		result, err := CalculateMemory(profile, kvDtypeBytes, contextLen, numSequences, mtpTokens, 0)
		if err != nil {
			t.Fatalf("CalculateMemory() error: %v", err)
		}

		t.Logf("=== Memory Breakdown (MTP=%d tokens) ===", mtpTokens)
		t.Logf("  WeightsMB:          %d", result.Breakdown.WeightsMB)
		t.Logf("  KVCacheMB:          %d", result.Breakdown.KVCacheMB)
		t.Logf("  GDNStateMB:         %d", result.Breakdown.GDNStateMB)
		t.Logf("  PrefixCacheMB:      %d", result.Breakdown.PrefixCacheMB)
		t.Logf("  MTPMB:              %d", result.Breakdown.MTPMB)
		t.Logf("  VisionEncoderMB:    %d", result.Breakdown.VisionEncoderMB)
		t.Logf("  CUDAContextMB:      %d", result.Breakdown.CUDAContextMB)
		t.Logf("  TotalMB (max):      %d", result.TotalMB)
		t.Logf("  TotalRealisticMB:   %d", result.TotalRealisticMB)
		t.Logf("  GPUMemUtilization:  %.2f", result.GPUMemoryUtilization)
		t.Logf("  DockerLimitGB:      %d", result.DockerLimitGB)

		// The key assertion: the estimated total must be large enough that
		// the resulting gpu_memory_utilization would give vLLM enough KV
		// cache to serve at least one request at max context.
		//
		// vLLM error said: need 8.99 GiB KV, had 2.41 GiB.
		// With the fix, the pool should cover the KV need.
		poolMB := result.GPUMemoryUtilization * TotalGPUMB
		availableKV := poolMB - float64(result.TotalRealisticMB) + float64(result.KVCacheMaxMB)
		t.Logf("  Pool (util×Total):  %.0f MB", poolMB)
		t.Logf("  Est KV available:    %.0f MB", availableKV)

		// The KV cache at full context should fit within the pool.
		// We allow a small tolerance for rounding.
		if float64(result.KVCacheMaxMB) > poolMB-2000 {
			t.Errorf("KV cache (%d MB) may not fit in pool (%.0f MB) — util=%.2f may need increase",
				result.KVCacheMaxMB, poolMB, result.GPUMemoryUtilization)
		}
	})

	t.Run("mtp_1_token", func(t *testing.T) {
		kvDtypeBytes := 1.0
		contextLen := 262144
		numSequences := 1
		mtpTokens := 1 // as seen in runtime override

		result, err := CalculateMemory(profile, kvDtypeBytes, contextLen, numSequences, mtpTokens, 0)
		if err != nil {
			t.Fatalf("CalculateMemory() error: %v", err)
		}

		t.Logf("=== Memory Breakdown (MTP=%d token, runtime override) ===", mtpTokens)
		t.Logf("  WeightsMB:          %d", result.Breakdown.WeightsMB)
		t.Logf("  KVCacheMB:          %d", result.Breakdown.KVCacheMB)
		t.Logf("  MTPMB:              %d", result.Breakdown.MTPMB)
		t.Logf("  TotalMB (max):      %d", result.TotalMB)
		t.Logf("  TotalRealisticMB:   %d", result.TotalRealisticMB)
		t.Logf("  GPUMemUtilization:  %.2f", result.GPUMemoryUtilization)

		// Verify the calculation produces a utilization that would
		// allow the model to start (i.e., total <= TotalGPUMB).
		if result.TotalMB > TotalGPUMB {
			t.Errorf("Total %d MB exceeds GPU total %d MB", result.TotalMB, TotalGPUMB)
		}
	})
}

// derefFloat64 dereferences a *float64, returning 0 if nil.
func derefFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// derefInt dereferences a *int, returning 0 if nil.
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// derefBool dereferences a *bool, returning false if nil.
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}