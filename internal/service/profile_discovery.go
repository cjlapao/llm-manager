package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/user/llm-manager/pkg/yamlparser"
)

// ProfileSource tracks where a discovered profile field came from.
type ProfileSource string

const (
	SourceHFConfig ProfileSource = "hf_config"
	SourceHFAPI    ProfileSource = "hf_api"
	SourceSlug     ProfileSource = "slug_detection"
	SourceDefault  ProfileSource = "default"
)

// Default values for non-discoverable fields.
const (
	DefaultIsMoe            = false
	DefaultSupportsMtp      = false
	DefaultQuantBytes       = 2.0 // BF16 baseline
	DefaultActiveParamsB    = -1.0 // Invalid marker when is_moe=false
)

var discoveryHTTPClient = &http.Client{Timeout: 30 * time.Second}

// DiscoveredProfile holds auto-discovered profile data with source tracking.
type DiscoveredProfile struct {
	Profile yamlparser.ModelProfile
	Sources map[string]ProfileSource
}

// DiscoverProfile fetches HuggingFace config.json and API metadata to auto-fill
// the model profile. Returns a DiscoveredProfile with source tracking.
// Network failure is non-fatal — returns partial profile with defaults.
func DiscoverProfile(hfRepo string) (*DiscoveredProfile, error) {
	dp := &DiscoveredProfile{
		Sources: make(map[string]ProfileSource),
	}

	// Start with defaults
	dp.Profile.IsMoe = boolPtr(DefaultIsMoe)
	dp.Profile.SupportsMtp = boolPtr(DefaultSupportsMtp)
	dp.Profile.QuantBytesPerParam = floatPtr(DefaultQuantBytes)
	dp.Profile.ActiveParamsB = floatPtr(DefaultActiveParamsB)
	dp.Sources["is_moe"] = SourceDefault
	dp.Sources["supports_mtp"] = SourceDefault
	dp.Sources["quant_bytes_per_param"] = SourceDefault
	dp.Sources["active_params_b"] = SourceDefault

	if hfRepo == "" {
		return dp, nil
	}

	// 1. Fetch HF API metadata for total params
	if params := getParamsFromHFAPI(hfRepo); params > 0 {
		b := float64(params) / 1e9
		dp.Profile.TotalParamsB = &b
		dp.Sources["total_params_b"] = SourceHFAPI
	}

	// 2. Fetch HF config.json for architecture details
	if cfg, err := fetchHFConfig(hfRepo); err == nil {
		fillProfileFromConfig(cfg, dp)
	} else {
		// Non-fatal — continue with what we have
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch HF config for %s: %v\n", hfRepo, err)
	}

	// 3. Slug-based quantization detection (overrides discovered if slug indicates quant)
	if quant := detectQuantFromSlug(hfRepo); quant > 0 {
		dp.Profile.QuantBytesPerParam = &quant
		dp.Sources["quant_bytes_per_param"] = SourceSlug
	}

	return dp, nil
}

// fillProfileFromConfig populates DiscoveredProfile from HF config.json data.
func fillProfileFromConfig(cfg *HFConfig, dp *DiscoveredProfile) {
	if cfg.NumHiddenLayers > 0 {
		// For hybrid models, estimate attention layers
		// Heuristic: if architecture suggests hybrid (GDN/Mamba), ~25% attention
		attLayers := cfg.NumHiddenLayers
		if isHybridModel(cfg.Architectures) {
			attLayers = cfg.NumHiddenLayers / 4
			if attLayers < 1 {
				attLayers = 1
			}
		}
		dp.Profile.AttentionLayers = &attLayers
		dp.Sources["attention_layers"] = SourceHFConfig
	}

	if cfg.NumKeyValHeads > 0 {
		dp.Profile.NumKvHeads = &cfg.NumKeyValHeads
		dp.Sources["num_kv_heads"] = SourceHFConfig
	}

	if cfg.HiddenSize > 0 && cfg.NumAttentionHeads > 0 {
		hd := cfg.HiddenSize / cfg.NumAttentionHeads
		dp.Profile.HeadDim = &hd
		dp.Sources["head_dim"] = SourceHFConfig
	}

	ctx := cfg.MaxPositionEmbeddings
	if ctx == 0 {
		ctx = cfg.ModelMaxLength
	}
	if ctx > 0 {
		dp.Profile.DefaultContext = &ctx
		dp.Profile.MaxContext = &ctx
		dp.Sources["default_context"] = SourceHFConfig
		dp.Sources["max_context"] = SourceHFConfig
	}
}

// isHybridModel checks if the architecture list suggests a hybrid model
// (contains GDN, Mamba, SSM, or DeltaNet patterns).
func isHybridModel(architectures []string) bool {
	patterns := []string{"GatedDeltaNet", "GDN", "Mamba", "SSM", "DeltaNet"}
	for _, arch := range architectures {
		lower := strings.ToLower(arch)
		for _, pat := range patterns {
			if strings.Contains(lower, pat) {
				return true
			}
		}
	}
	return false
}

// detectQuantFromSlug detects quantization from the HF repo name.
func detectQuantFromSlug(hfRepo string) float64 {
	lower := strings.ToLower(hfRepo)
	switch {
	case strings.Contains(lower, "nvfp4") || strings.Contains(lower, "nf4"):
		return 0.5
	case strings.Contains(lower, "fp8"):
		return 1.0
	case strings.Contains(lower, "fp4"):
		return 0.5
	case strings.Contains(lower, "int4") || strings.Contains(lower, "awq") || strings.Contains(lower, "gptq"):
		return 0.5
	case strings.Contains(lower, "int8"):
		return 1.0
	}
	return 0 // not detected
}

// HFConfig is an alias for ModelConfig used in profile discovery.
type HFConfig = ModelConfig

// fetchHFConfig fetches and parses config.json from HuggingFace.
func fetchHFConfig(hfRepo string) (*HFConfig, error) {
	url := "https://huggingface.co/" + hfRepo + "/resolve/main/config.json"
	resp, err := discoveryHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from HF", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, err
	}

	var cfg HFConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// getParamsFromHFAPI fetches total parameter count from HF API.
func getParamsFromHFAPI(hfRepo string) int64 {
	url := "https://huggingface.co/api/models/" + hfRepo
	resp, err := discoveryHTTPClient.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return 0
	}

	var hfInfo struct {
		Safetensors *struct {
			Total  int64 `json:"total"`
			BF16   int64 `json:"BF16"`
			FP16   int64 `json:"FP16"`
			FP32   int64 `json:"FP32"`
		} `json:"safetensors"`
	}
	if err := json.Unmarshal(data, &hfInfo); err != nil {
		return 0
	}

	if hfInfo.Safetensors != nil {
		if hfInfo.Safetensors.BF16 > 0 {
			return hfInfo.Safetensors.BF16
		}
		if hfInfo.Safetensors.Total > 0 {
			return hfInfo.Safetensors.Total
		}
	}
	return 0
}

// MergeProfile merges a discovered profile into a YAML profile.
// YAML values take precedence over discovered values.
// Default values are applied for any fields still nil.
func MergeProfile(yamlProfile *yamlparser.ModelProfile, discovered *DiscoveredProfile) *yamlparser.ModelProfile {
	if yamlProfile == nil {
		yamlProfile = &yamlparser.ModelProfile{}
	}

	// Start with discovered values
	if discovered != nil {
		if yamlProfile.TotalParamsB == nil && discovered.Profile.TotalParamsB != nil {
			yamlProfile.TotalParamsB = discovered.Profile.TotalParamsB
		}
		if yamlProfile.AttentionLayers == nil && discovered.Profile.AttentionLayers != nil {
			yamlProfile.AttentionLayers = discovered.Profile.AttentionLayers
		}
		if yamlProfile.NumKvHeads == nil && discovered.Profile.NumKvHeads != nil {
			yamlProfile.NumKvHeads = discovered.Profile.NumKvHeads
		}
		if yamlProfile.HeadDim == nil && discovered.Profile.HeadDim != nil {
			yamlProfile.HeadDim = discovered.Profile.HeadDim
		}
		if yamlProfile.DefaultContext == nil && discovered.Profile.DefaultContext != nil {
			yamlProfile.DefaultContext = discovered.Profile.DefaultContext
		}
		if yamlProfile.MaxContext == nil && discovered.Profile.MaxContext != nil {
			yamlProfile.MaxContext = discovered.Profile.MaxContext
		}
		if yamlProfile.QuantBytesPerParam == nil && discovered.Profile.QuantBytesPerParam != nil {
			yamlProfile.QuantBytesPerParam = discovered.Profile.QuantBytesPerParam
		}
	}

	// Apply defaults for fields still nil
	if yamlProfile.IsMoe == nil {
		v := DefaultIsMoe
		yamlProfile.IsMoe = &v
	}
	if yamlProfile.SupportsMtp == nil {
		v := DefaultSupportsMtp
		yamlProfile.SupportsMtp = &v
	}
	if yamlProfile.QuantBytesPerParam == nil {
		v := DefaultQuantBytes
		yamlProfile.QuantBytesPerParam = &v
	}
	if yamlProfile.ActiveParamsB == nil {
		v := DefaultActiveParamsB
		yamlProfile.ActiveParamsB = &v
	}

	return yamlProfile
}

// helper functions for creating pointer values
func boolPtr(v bool) *bool { return &v }
func floatPtr(v float64) *float64 { return &v }
