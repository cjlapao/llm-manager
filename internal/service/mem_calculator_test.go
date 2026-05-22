package service

import (
	"strings"
	"testing"
)

// --- Vision Encoder Memory Tests ---

func TestCalculateMemory_VisionEncoder_FP8(t *testing.T) {
	profile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     true,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 1.0, // FP8
	}

	result, err := CalculateMemory(profile, 1.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	if result.Breakdown.VisionEncoderMB != 500 {
		t.Errorf("VisionEncoderMB = %d, want 500 for FP8 vision model", result.Breakdown.VisionEncoderMB)
	}
}

func TestCalculateMemory_VisionEncoder_BF16(t *testing.T) {
	profile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     true,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 2.0, // BF16
	}

	result, err := CalculateMemory(profile, 2.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	if result.Breakdown.VisionEncoderMB != 1000 {
		t.Errorf("VisionEncoderMB = %d, want 1000 for BF16 vision model", result.Breakdown.VisionEncoderMB)
	}
}

func TestCalculateMemory_VisionEncoder_NVFP4(t *testing.T) {
	profile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     true,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 0.5, // NVFP4
	}

	result, err := CalculateMemory(profile, 1.0, 65536, 1, 0, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	if result.Breakdown.VisionEncoderMB != 250 {
		t.Errorf("VisionEncoderMB = %d, want 250 for NVFP4 vision model", result.Breakdown.VisionEncoderMB)
	}
}

func TestCalculateMemory_VisionEncoder_NotSupported(t *testing.T) {
	profile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     false, // Not a vision model
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 1.0,
	}

	result, err := CalculateMemory(profile, 1.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	if result.Breakdown.VisionEncoderMB != 0 {
		t.Errorf("VisionEncoderMB = %d, want 0 for non-vision model", result.Breakdown.VisionEncoderMB)
	}
}

func TestCalculateMemory_VisionEncoder_IncludedInTotals(t *testing.T) {
	// Verify that VisionEncoderMB is included in both totalMax and totalRealistic
	visionProfile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     true,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 1.0,
	}

	result, err := CalculateMemory(visionProfile, 1.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	// Compute expected totals manually (all components except vision)
	weights := 27 * 1024
	kv := int(2*4*128*16*1*65536*1) / (1024 * 1024)
	gdn := 50
	prefix := 1024
	mtp := 2750 * 3
	cuda := 3000
	offBudget := 4000 // vision model, context=65536 (not > 65536)
	vision := 500

	expectedTotalMax := weights + kv + gdn + prefix + mtp + cuda + offBudget + vision
	expectedTotalRealistic := weights + kv + gdn + prefix + mtp + cuda + offBudget + vision

	if result.TotalMB != expectedTotalMax {
		t.Errorf("TotalMB = %d, want %d (with vision encoder)", result.TotalMB, expectedTotalMax)
	}
	if result.TotalRealisticMB != expectedTotalRealistic {
		t.Errorf("TotalRealisticMB = %d, want %d (with vision encoder)", result.TotalRealisticMB, expectedTotalRealistic)
	}
}

func TestCalculateMemory_VisionEncoder_AffectsUtilization(t *testing.T) {
	// Vision encoder should increase gpu_memory_utilization.
	// Use BF16 (1000 MB vision) to ensure the delta is visible after rounding.
	nonVisionProfile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     false,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 2.0, // BF16
	}

	visionProfile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     true,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 2.0, // BF16
	}

	nonVisionResult, err := CalculateMemory(nonVisionProfile, 2.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	visionResult, err := CalculateMemory(visionProfile, 2.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	// Vision model should require higher utilization
	if visionResult.GPUMemoryUtilization <= nonVisionResult.GPUMemoryUtilization {
		t.Errorf("Vision utilization (%.4f) should be > non-vision (%.4f)",
			visionResult.GPUMemoryUtilization, nonVisionResult.GPUMemoryUtilization)
	}
}

func TestCalculateMemory_VisionEncoder_DockerLimit(t *testing.T) {
	// Docker limit should increase with vision encoder.
	// Use BF16 (1000 MB vision) to ensure the delta is visible after rounding.
	nonVisionProfile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     false,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 2.0, // BF16
	}

	visionProfile := ModelProfile{
		TotalParamsB:       27,
		ActiveParamsB:      27,
		IsMoe:              false,
		AttentionLayers:    16,
		GdnLayers:          32,
		NumKvHeads:         4,
		HeadDim:            128,
		SupportsMtp:        true,
		SupportsVision:     true,
		DefaultContext:     262144,
		MaxContext:         262144,
		QuantBytesPerParam: 2.0, // BF16
	}

	nonVisionResult, err := CalculateMemory(nonVisionProfile, 2.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	visionResult, err := CalculateMemory(visionProfile, 2.0, 65536, 1, 3, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	if visionResult.DockerLimitGB <= nonVisionResult.DockerLimitGB {
		t.Errorf("Vision DockerLimitGB (%d) should be > non-vision (%d)",
			visionResult.DockerLimitGB, nonVisionResult.DockerLimitGB)
	}
}

// --- Table-driven tests for vision encoder scaling ---

func TestCalculateMemory_VisionEncoder_Scaling(t *testing.T) {
	baseProfile := func(vision bool, quant float64) ModelProfile {
		return ModelProfile{
			TotalParamsB:       27,
			ActiveParamsB:      27,
			IsMoe:              false,
			AttentionLayers:    16,
			GdnLayers:          32,
			NumKvHeads:         4,
			HeadDim:            128,
			SupportsMtp:        true,
			SupportsVision:     vision,
			DefaultContext:     262144,
			MaxContext:         262144,
			QuantBytesPerParam: quant,
		}
	}

	tests := []struct {
		name         string
		vision       bool
		quant        float64
		wantVisionMB int
	}{
		{"FP8 vision", true, 1.0, 500},
		{"BF16 vision", true, 2.0, 1000},
		{"NVFP4 vision", true, 0.5, 250},
		{"Int8 vision", true, 1.0, 500},
		{"Non-vision FP8", false, 1.0, 0},
		{"Non-vision BF16", false, 2.0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := baseProfile(tt.vision, tt.quant)
			kvBytes := tt.quant // use quant as kv dtype bytes for simplicity
			result, err := CalculateMemory(profile, kvBytes, 65536, 1, 0, 0)
			if err != nil {
				t.Fatalf("CalculateMemory() error: %v", err)
			}
			if result.Breakdown.VisionEncoderMB != tt.wantVisionMB {
				t.Errorf("VisionEncoderMB = %d, want %d", result.Breakdown.VisionEncoderMB, tt.wantVisionMB)
			}
		})
	}
}

// --- RAG Encoder Memory Tests ---

func TestCalculateMemory_RAGEncoder_BF16(t *testing.T) {
	// Encoder model (0 attention layers) with BF16 quantization.
	// This is the path used for embedding and reranker RAG models.
	profile := ModelProfile{
		TotalParamsB:       0.6,
		ActiveParamsB:      0.6,
		IsMoe:              false,
		AttentionLayers:    0,
		GdnLayers:          0,
		NumKvHeads:         0,
		HeadDim:            0,
		SupportsMtp:        false,
		SupportsVision:     false,
		DefaultContext:     8192,
		MaxContext:         8192,
		QuantBytesPerParam: 2.0, // BF16
	}

	result, err := CalculateMemory(profile, 2.0, 8192, 1, 0, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	// Weights: 0.6 * 2.0 * 1024 = 1228.8 → truncated to 1228
	if result.Breakdown.WeightsMB != 1228 {
		t.Errorf("WeightsMB = %d, want 1228", result.Breakdown.WeightsMB)
	}

	// Encoder has no attention layers → KV cache is 0
	if result.Breakdown.KVCacheMB != 0 {
		t.Errorf("KVCacheMB = %d, want 0 for encoder", result.Breakdown.KVCacheMB)
	}

	// No GDN layers
	if result.Breakdown.GDNStateMB != 0 {
		t.Errorf("GDNStateMB = %d, want 0", result.Breakdown.GDNStateMB)
	}

	// numSequences=1 → standard prefix cache
	if result.Breakdown.PrefixCacheMB != 1024 {
		t.Errorf("PrefixCacheMB = %d, want 1024", result.Breakdown.PrefixCacheMB)
	}

	// No MTP support
	if result.Breakdown.MTPMB != 0 {
		t.Errorf("MTPMB = %d, want 0", result.Breakdown.MTPMB)
	}

	// Encoder path: CUDA context = 1500
	if result.Breakdown.CUDAContextMB != 1500 {
		t.Errorf("CUDAContextMB = %d, want 1500 for encoder", result.Breakdown.CUDAContextMB)
	}

	// Encoder path: off-budget = 2000 (updated for activation tensors and JIT kernels)
	if result.Breakdown.OffBudgetMB != 2000 {
		t.Errorf("OffBudgetMB = %d, want 2000 for encoder", result.Breakdown.OffBudgetMB)
	}

	// No vision
	if result.Breakdown.VisionEncoderMB != 0 {
		t.Errorf("VisionEncoderMB = %d, want 0", result.Breakdown.VisionEncoderMB)
	}

	// Total: 1228 + 0 + 0 + 1024 + 0 + 1500 + 2000 + 0 = 5752
	if result.TotalMB != 5752 {
		t.Errorf("TotalMB = %d, want 5752", result.TotalMB)
	}

	// Fits on GPU? 4252 <= 121856 → true
	if !result.FitsAtMaxContext {
		t.Errorf("FitsAtMaxContext = false, want true")
	}

	// GPU utilization: (5752 / 121856) * 1.02 = 0.0481 → ceil to 0.05
	if result.GPUMemoryUtilization != 0.05 {
		t.Errorf("GPUMemoryUtilization = %.4f, want 0.05", result.GPUMemoryUtilization)
	}
}

func TestCalculateMemory_RAGEncoder_Reranker_BF16(t *testing.T) {
	// Reranker model has identical profile structure to embedding model.
	// Verify it produces the same memory result.
	profile := ModelProfile{
		TotalParamsB:       0.6,
		ActiveParamsB:      0.6,
		IsMoe:              false,
		AttentionLayers:    0,
		GdnLayers:          0,
		NumKvHeads:         0,
		HeadDim:            0,
		SupportsMtp:        false,
		SupportsVision:     false,
		DefaultContext:     8192,
		MaxContext:         8192,
		QuantBytesPerParam: 2.0, // BF16
	}

	result, err := CalculateMemory(profile, 2.0, 8192, 1, 0, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	// Reranker should produce the same total as embedding model
	if result.TotalMB != 5752 {
		t.Errorf("TotalMB = %d, want 5752 (same as embed)", result.TotalMB)
	}
	if result.GPUMemoryUtilization != 0.05 {
		t.Errorf("GPUMemoryUtilization = %.4f, want 0.05", result.GPUMemoryUtilization)
	}
	if !result.FitsAtMaxContext {
		t.Errorf("FitsAtMaxContext = false, want true")
	}
}

func TestCalculateMemory_RAGEncoder_FP8(t *testing.T) {
	// Same encoder profile but with FP8 quantization (1.0 bytes/param).
	profile := ModelProfile{
		TotalParamsB:       0.6,
		ActiveParamsB:      0.6,
		IsMoe:              false,
		AttentionLayers:    0,
		GdnLayers:          0,
		NumKvHeads:         0,
		HeadDim:            0,
		SupportsMtp:        false,
		SupportsVision:     false,
		DefaultContext:     8192,
		MaxContext:         8192,
		QuantBytesPerParam: 1.0, // FP8
	}

	result, err := CalculateMemory(profile, 1.0, 8192, 1, 0, 0)
	if err != nil {
		t.Fatalf("CalculateMemory() error: %v", err)
	}

	// Weights: 0.6 * 1.0 * 1024 = 614.4 → truncated to 614
	if result.Breakdown.WeightsMB != 614 {
		t.Errorf("WeightsMB = %d, want 614", result.Breakdown.WeightsMB)
	}

	// Total: 614 + 0 + 0 + 1024 + 0 + 1500 + 2000 + 0 = 5138
	if result.TotalMB != 5138 {
		t.Errorf("TotalMB = %d, want 5138", result.TotalMB)
	}

	if !result.FitsAtMaxContext {
		t.Errorf("FitsAtMaxContext = false, want true")
	}
}

func TestCalculateMemory_NoProfileData(t *testing.T) {
	// Model with zero/invalid profile should return an error, not panic.
	profile := ModelProfile{
		TotalParamsB: 0,
	}

	_, err := CalculateMemory(profile, 2.0, 8192, 1, 0, 0)
	if err == nil {
		t.Fatalf("CalculateMemory() expected error for zero profile, got nil")
	}

	wantMsg := "total_params_b must be > 0"
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q, want to contain %q", err.Error(), wantMsg)
	}
}
