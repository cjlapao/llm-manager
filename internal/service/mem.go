// Package service provides GPU memory estimation logic for LLM models.
package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

// QuantBytes maps quantization types to bytes per parameter.
var QuantBytes = map[string]float64{
	"nvfp4":          0.55,
	"fp4":            0.5,
	"fp8":            1.0,
	"int8":           1.0,
	"bf16":           2.0,
	"bfloat16":       2.0,
	"fp16":           2.0,
	"float16":        2.0,
	"int4":           0.5,
	"awq":            0.5,
	"gptq":           0.5,
	"modelopt_mixed": 0.6,
}

// KVBytes maps KV cache dtypes to bytes per element.
var KVBytes = map[string]float64{
	"fp8":      1.0,
	"fp16":     2.0,
	"bf16":     2.0,
	"bfloat16": 2.0,
	"auto":     2.0,
}

// jsonModel represents an entry in models.json.
type jsonModel struct {
	Slug string `json:"slug"`
	Type string `json:"type"`
	Name string `json:"name"`
	YML  string `json:"yml"`
}

// ModelConfig represents the config.json from a HuggingFace model.
type ModelConfig struct {
	Architectures         []string `json:"architectures"`
	HiddenSize            int      `json:"hidden_size"`
	NumHiddenLayers       int      `json:"num_hidden_layers"`
	NumAttentionHeads     int      `json:"num_attention_heads"`
	NumKeyValHeads        int      `json:"num_key_value_heads"`
	IntermediateSize      int      `json:"intermediate_size"`
	VocabSize             int      `json:"vocab_size"`
	MaxPositionEmbeddings int      `json:"max_position_embeddings"`
	ModelMaxLength        int      `json:"model_max_length"`
	NumExperts            int      `json:"num_experts"`
	NumCodebooks          int      `json:"num_codebooks"`
	ImageSize             int      `json:"image_size"`
	PaddingSide           string   `json:"padding_side"`
	TieWordEmbeddings     bool     `json:"tie_word_embeddings"`
	QuantizationConfig    *struct {
		LoadIn4Bit   bool   `json:"load_in_4bit"`
		LoadIn8Bit   bool   `json:"load_in_8bit"`
		BnbQuantType string `json:"bnb_4bit_quant_type"`
	} `json:"quantization_config"`
}

// MemEstimationResult holds the VRAM estimation for a single model.
type MemEstimationResult struct {
	Slug    string
	Name    string
	Quant   string
	Weights uint64
	KV4K    uint64
	KV32K   uint64
	KV128K  uint64
	KV262K  uint64
	Source  string // "cache", "api", or "estimate"
	MaxCtx  int
}

// MemService handles GPU VRAM estimation.
type MemService struct {
	db     database.DatabaseManager
	cfg    *config.Config
	client *http.Client
	mu     sync.Mutex
}

// NewMemService creates a new MemService.
func NewMemService(db database.DatabaseManager, cfg *config.Config) *MemService {
	return &MemService{
		db:     db,
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// EstimateVRAM estimates VRAM usage for models.
// If slug is empty, estimates for all LLM-type models.
func (s *MemService) EstimateVRAM(slug string) ([]MemEstimationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find models.json
	modelsJSONPath := s.findModelsJSON()
	if modelsJSONPath == "" {
		return nil, fmt.Errorf("models.json not found")
	}

	// Read models.json
	modelsData, err := os.ReadFile(modelsJSONPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read models.json: %w", err)
	}

	// Parse models.json
	var jsonModels []jsonModel
	if err := json.Unmarshal(modelsData, &jsonModels); err != nil {
		return nil, fmt.Errorf("failed to parse models.json: %w", err)
	}

	// Filter to LLM-type models
	var llmModels []jsonModel
	for _, m := range jsonModels {
		if m.Type != "llm" {
			continue
		}
		if slug != "" && m.Slug != slug {
			continue
		}
		llmModels = append(llmModels, m)
	}

	if len(llmModels) == 0 {
		if slug != "" {
			return nil, fmt.Errorf("model %s not found or not an LLM type", slug)
		}
		return nil, fmt.Errorf("no LLM models found in models.json")
	}

	// Estimate VRAM for each
	var results []MemEstimationResult
	for _, m := range llmModels {
		result, err := s.estimateSingleModel(m)
		if err != nil {
			// Continue with partial results
			results = append(results, MemEstimationResult{
				Slug:   m.Slug,
				Name:   m.Name,
				Quant:  "unknown",
				Source: "error",
			})
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// findModelsJSON locates models.json from known paths.
func (s *MemService) findModelsJSON() string {
	// Try InstallDir first
	paths := []string{
		filepath.Join(s.cfg.InstallDir, "models.json"),
		filepath.Join(s.cfg.InstallDir, "..", "models.json"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// estimateSingleModel estimates VRAM for a single model.
func (s *MemService) estimateSingleModel(m jsonModel) (MemEstimationResult, error) {
	result := MemEstimationResult{
		Slug: m.Slug,
		Name: m.Name,
	}

	// Read yml file for flags
	ymlContent, _, err := s.readYML(m.YML)
	if err != nil {
		// YML not found, continue with defaults
		ymlContent = ""
	}

	// Detect quantization
	quant := s.detectQuant(m.Slug, ymlContent)
	result.Quant = quant

	// Get bytes per param
	qb, ok := QuantBytes[quant]
	if !ok {
		qb = 2.0 // default to bf16
	}

	// Load model config
	config, source, err := s.loadModelConfig(m.Slug)
	if err != nil {
		result.Source = source
		return result, nil
	}

	// Compute parameters
	params := s.computeParams(config)
	if params == 0 {
		result.Source = source
		return result, fmt.Errorf("could not compute params for %s", m.Slug)
	}

	// Compute weights in bytes
	result.Weights = uint64(float64(params) * qb)

	// Compute KV cache sizes
	kvBytes := s.getKVBytes(ymlContent)
	kvBPT := 2.0 * float64(config.NumHiddenLayers) * float64(config.NumKeyValHeads) * float64(s.headDim(config)) * kvBytes

	contexts := []int{4096, 32768, 131072, 262144}
	for i, ctx := range contexts {
		kvSize := uint64(kvBPT * float64(ctx))
		switch i {
		case 0:
			result.KV4K = kvSize
		case 1:
			result.KV32K = kvSize
		case 2:
			result.KV128K = kvSize
		case 3:
			result.KV262K = kvSize
		}
	}

	// Max context
	result.MaxCtx = s.maxContext(config, ymlContent)
	result.Source = source

	return result, nil
}

// readYML reads the yml file and returns its content and path.
func (s *MemService) readYML(yml string) (string, string, error) {
	if yml == "" {
		return "", "", fmt.Errorf("no yml specified")
	}

	ymlPath := yml
	if !strings.HasPrefix(ymlPath, "/") {
		ymlPath = filepath.Join(s.cfg.LLMDir, yml)
	}

	content, err := os.ReadFile(ymlPath)
	if err != nil {
		return "", "", err
	}
	return string(content), ymlPath, nil
}

// detectQuant detects quantization from model slug and yml content.
func (s *MemService) detectQuant(slug, ymlContent string) string {
	// Check repo name (slug) for quant types
	slugLower := strings.ToLower(slug)
	quantPatterns := []string{"nvfp4", "fp8", "fp4", "int4", "gptq", "awq", "int8"}
	for _, q := range quantPatterns {
		if strings.Contains(slugLower, q) {
			return q
		}
	}

	// Parse yml for --quantization=X
	if m := quantRe.FindStringSubmatch(ymlContent); m != nil {
		return strings.ToLower(m[1])
	}

	// Parse yml for --dtype=X
	if m := dtypeRe.FindStringSubmatch(ymlContent); m != nil {
		return strings.ToLower(m[1])
	}

	return "bf16"
}

// loadModelConfig loads model config from local cache or HF API.
func (s *MemService) loadModelConfig(slug string) (*ModelConfig, string, error) {
	// Try local cache first
	if config, err := s.loadConfigFromCache(slug); err == nil {
		return config, "cache", nil
	}

	// Try HF API
	if config, err := s.loadConfigFromAPI(slug); err == nil {
		return config, "api", nil
	}

	return nil, "not_found", fmt.Errorf("config not found for %s", slug)
}

// loadConfigFromCache loads config from HF cache directory.
func (s *MemService) loadConfigFromCache(slug string) (*ModelConfig, error) {
	// Get HF repo from DB
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil, err
	}
	if model.HFRepo == "" {
		return nil, fmt.Errorf("no HF repo for %s", slug)
	}

	// Convert repo to cache dir: Qwen/Qwen3.6-35B-A3B -> models--Qwen--Qwen3.6-35B-A3B
	cacheDir := "models--" + strings.ReplaceAll(model.HFRepo, "/", "--")
	snapshotsDir := filepath.Join(s.cfg.HFCacheDir, cacheDir, "snapshots")

	// Find snapshot directory
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		configPath := filepath.Join(snapshotsDir, entry.Name(), "config.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var config ModelConfig
		if err := json.Unmarshal(data, &config); err != nil {
			continue
		}
		return &config, nil
	}

	return nil, fmt.Errorf("no snapshots found in %s", snapshotsDir)
}

// loadConfigFromAPI loads config from HuggingFace API.
func (s *MemService) loadConfigFromAPI(slug string) (*ModelConfig, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil, err
	}
	if model.HFRepo == "" {
		return nil, fmt.Errorf("no HF repo for %s", slug)
	}

	url := "https://huggingface.co/" + model.HFRepo + "/resolve/main/config.json"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	hfToken := os.Getenv("HF_TOKEN")
	if hfToken == "" {
		hfToken = os.Getenv("HUGGING_FACE_HUB_TOKEN")
	}
	if hfToken != "" {
		req.Header.Set("Authorization", "Bearer "+hfToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from HF API", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, err
	}

	var config ModelConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// computeParams computes total parameter count from model config.
// Formula: L*H*(nh*hd + 2*kh*hd + nh*hd) + L*ff*H*3*ne + V*H*2 + L*H*4
// where: L=num_hidden_layers, H=hidden_size, V=vocab_size,
//
//	nh=num_attention_heads, kh=num_key_value_heads,
//	ff=intermediate_size, hd=head_dim, ne=num_experts
func (s *MemService) computeParams(config *ModelConfig) int64 {
	L := int64(config.NumHiddenLayers)
	H := int64(config.HiddenSize)
	V := int64(config.VocabSize)
	nh := int64(config.NumAttentionHeads)
	kh := int64(config.NumKeyValHeads)
	ff := int64(config.IntermediateSize)
	ne := int64(config.NumExperts)

	// head_dim
	hd := int64(1)
	if nh > 0 {
		hd = H / nh
	}
	if hd == 0 {
		hd = 1
	}

	// Attention params: L * H * (nh * hd + 2 * kh * hd + nh * hd)
	// = L * H * hd * (nh + 2*kh + nh)
	// = L * H * hd * (2*nh + 2*kh)
	attentionParams := L * H * hd * (2*nh + 2*kh)

	// Feed-forward params: L * ff * H * 3 * ne
	// (3 for the 3 matrices in MoE, ne for number of experts)
	ffParams := L * ff * H * 3 * ne

	// Embedding params: V * H * 2 (input + output embeddings)
	embedParams := V * H * 2

	// Norm params: L * H * 4 (rmsnorm/glayernorm: 2 weights + 2 biases per layer)
	normParams := L * H * 4

	return attentionParams + ffParams + embedParams + normParams
}

// headDim returns the head dimension.
func (s *MemService) headDim(config *ModelConfig) int64 {
	if config.NumAttentionHeads == 0 {
		return 1
	}
	return int64(config.HiddenSize) / int64(config.NumAttentionHeads)
}

// getKVBytes returns the bytes per element for KV cache.
func (s *MemService) getKVBytes(ymlContent string) float64 {
	// Parse yml for --kv-cache-dtype=X
	kvDtypeRe := regexp.MustCompile(`--kv-cache-dtype=(\S+)`)
	if m := kvDtypeRe.FindStringSubmatch(ymlContent); m != nil {
		dtype := strings.ToLower(m[1])
		if b, ok := KVBytes[dtype]; ok {
			return b
		}
	}

	// Default to bf16
	return KVBytes["bf16"]
}

// maxContext returns the maximum context length.
func (s *MemService) maxContext(config *ModelConfig, ymlContent string) int {
	// From yml: --max-model-len=X
	maxLenRe := regexp.MustCompile(`--max-model-len=(\S+)`)
	if m := maxLenRe.FindStringSubmatch(ymlContent); m != nil {
		var maxLen int
		fmt.Sscanf(m[1], "%d", &maxLen)
		if maxLen > 0 {
			return maxLen
		}
	}

	// From config.json
	if config.MaxPositionEmbeddings > 0 {
		return config.MaxPositionEmbeddings
	}
	if config.ModelMaxLength > 0 {
		return config.ModelMaxLength
	}

	return 0
}

// ymlFlagRe is a regex to match --quantization=X patterns in yml files.
var quantRe = regexp.MustCompile(`--quantization=(\S+)`)

// dtypeRe is a regex to match --dtype=X patterns in yml files.
var dtypeRe = regexp.MustCompile(`--dtype=(\S+)`)

// kvDtypeRe is a regex to match --kv-cache-dtype=X patterns in yml files.
var kvDtypeRe = regexp.MustCompile(`--kv-cache-dtype=(\S+)`)

// maxLenRe is a regex to match --max-model-len=X patterns in yml files.
var maxLenRe = regexp.MustCompile(`--max-model-len=(\S+)`)

// ymlFlagRe is a regex to match --flag=value patterns in yml files.
var ymlFlagRe = regexp.MustCompile(`--(\w[\w-]*)=(\S+)`)

// FormatVRAM formats a byte count as human-readable GB/MB.
func FormatVRAM(n uint64) string {
	gb := float64(n) / 1e9
	if gb < 1 {
		return fmt.Sprintf("%.0fMB", gb*1024)
	}
	if gb < 100 {
		return fmt.Sprintf("%.1fGB", gb)
	}
	return fmt.Sprintf("%.0fGB", gb)
}

// FormatKV formats a KV cache size, returning "—" if zero.
func FormatKV(n uint64) string {
	if n == 0 {
		return "—"
	}
	return FormatVRAM(n)
}

// IsHFCached checks if a model's weights are cached locally.
func (s *ContainerService) IsHFCached(hfRepo string) bool {
	if hfRepo == "" {
		return false
	}

	// Convert repo to cache dir: Qwen/Qwen3.6-35B-A3B -> models--Qwen--Qwen3.6-35B-A3B
	cacheDir := "models--" + strings.ReplaceAll(hfRepo, "/", "--")
	snapshotsDir := filepath.Join(s.cfg.HFCacheDir, cacheDir, "snapshots")

	// Check if snapshots directory exists and has content
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return false
	}

	// Check for at least one snapshot subdirectory
	for _, entry := range entries {
		if entry.IsDir() {
			// Check for config.json in snapshot
			configPath := filepath.Join(snapshotsDir, entry.Name(), "config.json")
			if _, err := os.Stat(configPath); err == nil {
				return true
			}
		}
	}

	return false
}
