package cmd

import (
	"os"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

// TestUpdateCommand_Help verifies the update command help prints.
func TestUpdateCommand_Help(t *testing.T) {
	cfg := config.DefaultConfig()
	cmd := NewUpdateCommand(cfg, nil)
	exitCode := cmd.Run([]string{"help"})
	if exitCode != 0 {
		t.Errorf("update help returned non-zero exit code: %d", exitCode)
	}
}

// TestUpdateCommand_NoToken verifies update fails without HF_TOKEN.
func TestUpdateCommand_NoToken(t *testing.T) {
	// Ensure no HF_TOKEN is set
	os.Unsetenv("HF_TOKEN")
	os.Unsetenv("HUGGING_FACE_HUB_TOKEN")

	cfg := config.DefaultConfig()
	cmd := NewUpdateCommand(cfg, nil)
	exitCode := cmd.Run([]string{"qwen3_6"})
	if exitCode == 0 {
		t.Error("update without HF_TOKEN should return non-zero exit code")
	}
}

// TestUpdateCommand_AllWithNoToken verifies update all fails without HF_TOKEN.
func TestUpdateCommand_AllWithNoToken(t *testing.T) {
	os.Unsetenv("HF_TOKEN")
	os.Unsetenv("HUGGING_FACE_HUB_TOKEN")

	cfg := config.DefaultConfig()
	cmd := NewUpdateCommand(cfg, nil)
	exitCode := cmd.Run([]string{"all"})
	if exitCode == 0 {
		t.Error("update all without HF_TOKEN should return non-zero exit code")
	}
}

// TestUpdateCommand_WithToken verifies update requires a valid model when token is set.
func TestUpdateCommand_WithToken(t *testing.T) {
	os.Setenv("HF_TOKEN", "test-token")
	defer os.Unsetenv("HF_TOKEN")

	cfg := config.DefaultConfig()
	// Use a non-existent model to test error path
	cmd := NewUpdateCommand(cfg, nil)
	exitCode := cmd.Run([]string{"nonexistent-model"})
	// Should fail because model not found (db is nil)
	if exitCode == 0 {
		t.Error("update nonexistent model should return non-zero exit code")
	}
}

// TestUpdateCommand_DBRequired verifies update works with a DB.
func TestUpdateCommand_DBRequired(t *testing.T) {
	os.Setenv("HF_TOKEN", "test-token")
	defer os.Unsetenv("HF_TOKEN")

	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cmd := NewUpdateCommand(cfg, db)

	// Non-existent model
	exitCode := cmd.Run([]string{"nonexistent"})
	if exitCode == 0 {
		t.Error("update nonexistent model should return non-zero")
	}
}

// TestResolveServiceAlias tests service name resolution.
func TestResolveServiceAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"comfyui", "comfyui-flux"},
		{"flux", "comfyui-flux"},
		{"embed", "llm-embed"},
		{"rerank", "llm-rerank"},
		{"whisper", "whisper-stt"},
		{"kokoro", "kokoro-tts"},
		{"litellm", "litellm"},
		{"swap-api", "swap-api"},
		{"swapapi", "swap-api"},
		{"open-webui", "open-webui"},
		{"webui", "open-webui"},
		{"mcp", "mcpo"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := resolveServiceAlias(tt.input)
			if result != tt.expected {
				t.Errorf("resolveServiceAlias(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestResolveServiceAliasCaseInsensitive tests case-insensitive resolution.
func TestResolveServiceAliasCaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ComfyUI", "comfyui-flux"},
		{"COMFYUI", "comfyui-flux"},
		{"Embed", "llm-embed"},
		{"EMBED", "llm-embed"},
		{"MCP", "mcpo"},
		{"Mcp", "mcpo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := resolveServiceAlias(tt.input)
			if result != tt.expected {
				t.Errorf("resolveServiceAlias(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsFluxModel tests flux model detection.
func TestIsFluxModel(t *testing.T) {
	if !isFluxModel("flux-schnell") {
		t.Error("flux-schnell should be a flux model")
	}
	if !isFluxModel("flux-dev") {
		t.Error("flux-dev should be a flux model")
	}
	if isFluxModel("qwen3_6") {
		t.Error("qwen3_6 should not be a flux model")
	}
	if isFluxModel("unknown") {
		t.Error("unknown should not be a flux model")
	}
}

// TestIs3DModel tests 3D model detection.
func TestIs3DModel(t *testing.T) {
	if !is3DModel("hunyuan3d") {
		t.Error("hunyuan3d should be a 3D model")
	}
	if !is3DModel("trellis") {
		t.Error("trellis should be a 3D model")
	}
	if is3DModel("qwen3_6") {
		t.Error("qwen3_6 should not be a 3D model")
	}
}

// TestFluxCheckpoint tests flux checkpoint filename resolution.
func TestFluxCheckpoint(t *testing.T) {
	if fluxCheckpoint("flux-schnell") != "flux1-schnell.safetensors" {
		t.Errorf("flux-schnell checkpoint = %q, want %q", fluxCheckpoint("flux-schnell"), "flux1-schnell.safetensors")
	}
	if fluxCheckpoint("flux-dev") != "flux1-dev.safetensors" {
		t.Errorf("flux-dev checkpoint = %q, want %q", fluxCheckpoint("flux-dev"), "flux1-dev.safetensors")
	}
	if fluxCheckpoint("unknown") != "" {
		t.Errorf("unknown checkpoint = %q, want %q", fluxCheckpoint("unknown"), "")
	}
}

// TestDirFor3DModel tests 3D model directory resolution.
func TestDirFor3DModel(t *testing.T) {
	if dirFor3DModel("hunyuan3d") != "hunyuan3d" {
		t.Errorf("hunyuan3d dir = %q, want %q", dirFor3DModel("hunyuan3d"), "hunyuan3d")
	}
	if dirFor3DModel("trellis") != "trellis" {
		t.Errorf("trellis dir = %q, want %q", dirFor3DModel("trellis"), "trellis")
	}
	if dirFor3DModel("unknown") != "" {
		t.Errorf("unknown dir = %q, want %q", dirFor3DModel("unknown"), "")
	}
}

// TestReadActiveFileNonExistent tests reading a non-existent active file.
func TestReadActiveFileNonExistent(t *testing.T) {
	content := readActiveFile("/tmp/llm-manager-test-nonexistent-12345")
	if content != "" {
		t.Errorf("readActiveFile(nonexistent) = %q, want empty string", content)
	}
}

// TestReadActiveFileExisting tests reading an existing active file.
func TestReadActiveFileExisting(t *testing.T) {
	tmpFile := t.TempDir() + "/.active-model"
	if err := os.WriteFile(tmpFile, []byte("flux-schnell"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	content := readActiveFile(tmpFile)
	if content != "flux-schnell" {
		t.Errorf("readActiveFile = %q, want %q", content, "flux-schnell")
	}
}

// TestWriteAndReadActiveFile tests writing and reading an active file.
func TestWriteAndReadActiveFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/.active-model"

	if err := writeActiveFile(path, "flux-dev"); err != nil {
		t.Fatalf("writeActiveFile() error: %v", err)
	}

	content := readActiveFile(path)
	if content != "flux-dev" {
		t.Errorf("readActiveFile after write = %q, want %q", content, "flux-dev")
	}
}

// TestParseKeyValue tests key=value parsing.
func TestParseKeyValue(t *testing.T) {
	tests := []struct {
		input string
		key   string
		value string
		ok    bool
	}{
		{"name=test", "name", "test", true},
		{"port=8080", "port", "8080", true},
		{"hf_repo=Qwen/Qwen3.6-35B-A3B", "hf_repo", "Qwen/Qwen3.6-35B-A3B", true},
		{"noequals", "", "", false},
		{"", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			key, value, ok := parseKeyValue(tt.input)
			if ok != tt.ok {
				t.Errorf("parseKeyValue(%q).ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if key != tt.key {
				t.Errorf("parseKeyValue(%q).key = %q, want %q", tt.input, key, tt.key)
			}
			if value != tt.value {
				t.Errorf("parseKeyValue(%q).value = %q, want %q", tt.input, value, tt.value)
			}
		})
	}
}

// TestModelCommand_FluxDetection tests flux model detection helpers.
func TestModelCommand_FluxDetection(t *testing.T) {
	fluxModels := knownFluxModels()
	if len(fluxModels) != 2 {
		t.Errorf("knownFluxModels() length = %d, want 2", len(fluxModels))
	}
	if fluxModels[0] != "flux-schnell" || fluxModels[1] != "flux-dev" {
		t.Errorf("knownFluxModels() = %v, want [flux-schnell, flux-dev]", fluxModels)
	}
}

// TestModelCommand_3DDetection tests 3D model detection helpers.
func TestModelCommand_3DDetection(t *testing.T) {
	threeDModels := known3DModels()
	if len(threeDModels) != 2 {
		t.Errorf("known3DModels() length = %d, want 2", len(threeDModels))
	}
	if threeDModels[0] != "hunyuan3d" || threeDModels[1] != "trellis" {
		t.Errorf("known3DModels() = %v, want [hunyuan3d, trellis]", threeDModels)
	}
}
