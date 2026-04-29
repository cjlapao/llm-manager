package cmd

import (
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

// TestMemCommand_Help verifies mem command help prints.
func TestMemCommand_Help(t *testing.T) {
	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg}
	cmd := NewMemCommand(root)
	exitCode := cmd.Run([]string{"help"})
	if exitCode != 0 {
		t.Errorf("mem help returned non-zero exit code: %d", exitCode)
	}
}

// TestMemCommand_NoArgs verifies mem with no args shows help.
func TestMemCommand_NoArgs(t *testing.T) {
	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg}
	cmd := NewMemCommand(root)
	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("mem with no args should return 0")
	}
}

// TestMemCommand_EstimateNoModelsJSON verifies mem fails gracefully when models.json is missing.
func TestMemCommand_EstimateNoModelsJSON(t *testing.T) {
	cfg := config.DefaultConfig()
	// Use a non-existent install dir to ensure models.json is not found
	cfg.InstallDir = "/tmp/llm-manager-test-no-models-json"
	root := &RootCommand{cfg: cfg}
	cmd := NewMemCommand(root)
	exitCode := cmd.Run([]string{"qwen3_6"})
	if exitCode == 0 {
		t.Error("mem with missing models.json should return non-zero")
	}
}

// TestMemCommand_EstimateServiceError verifies MemService handles nil DB gracefully.
func TestMemCommand_EstimateServiceError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.InstallDir = "/tmp/llm-manager-test-no-models-json"
	svc := service.NewMemService(nil, cfg)

	// Should return error, not panic
	results, err := svc.EstimateVRAM("qwen3_6")
	if err == nil {
		t.Error("EstimateVRAM with missing models.json should return error")
	}
	if len(results) != 0 {
		t.Errorf("EstimateVRAM returned %d results, want 0", len(results))
	}
}

// TestFormatVRAM tests the VRAM formatting function.
func TestFormatVRAM(t *testing.T) {
	tests := []struct {
		input  uint64
		expect string
	}{
		{0, "0MB"},
		{1000000, "1MB"},
		{1000000000, "1.0GB"},
		{5000000000, "5.0GB"},
		{69300000000, "69.3GB"},
		{100000000000, "100GB"},
		{1000000000000, "1000GB"},
	}

	for _, tt := range tests {
		result := service.FormatVRAM(tt.input)
		if result != tt.expect {
			t.Errorf("FormatVRAM(%d) = %q, want %q", tt.input, result, tt.expect)
		}
	}
}

// TestFormatKV tests the KV cache formatting function.
func TestFormatKV(t *testing.T) {
	if service.FormatKV(0) != "—" {
		t.Errorf("FormatKV(0) = %q, want —", service.FormatKV(0))
	}
	if service.FormatKV(1000000000) != "1.0GB" {
		t.Errorf("FormatKV(1000000000) = %q, want 1.0GB", service.FormatKV(1000000000))
	}
}

// TestMemService_QuantBytes tests that QuantBytes has expected entries.
func TestMemService_QuantBytes(t *testing.T) {
	if service.QuantBytes["bf16"] != 2.0 {
		t.Errorf("QuantBytes[bf16] = %f, want 2.0", service.QuantBytes["bf16"])
	}
	if service.QuantBytes["fp8"] != 1.0 {
		t.Errorf("QuantBytes[fp8] = %f, want 1.0", service.QuantBytes["fp8"])
	}
	if service.QuantBytes["int4"] != 0.5 {
		t.Errorf("QuantBytes[int4] = %f, want 0.5", service.QuantBytes["int4"])
	}
}

// TestMemService_KVBytes tests that KVBytes has expected entries.
func TestMemService_KVBytes(t *testing.T) {
	if service.KVBytes["bf16"] != 2.0 {
		t.Errorf("KVBytes[bf16] = %f, want 2.0", service.KVBytes["bf16"])
	}
	if service.KVBytes["fp8"] != 1.0 {
		t.Errorf("KVBytes[fp8] = %f, want 1.0", service.KVBytes["fp8"])
	}
}

// TestContainerCommand_FluxStartStop verifies flux start/stop detection works.
func TestContainerCommand_FluxStartStop(t *testing.T) {
	// Verify flux model detection
	if !isFluxModel("flux-schnell") {
		t.Error("flux-schnell should be detected as flux model")
	}
	if !isFluxModel("flux-dev") {
		t.Error("flux-dev should be detected as flux model")
	}
	if isFluxModel("qwen3_6") {
		t.Error("qwen3_6 should not be detected as flux model")
	}

	// Verify checkpoint filenames
	if fluxCheckpoint("flux-schnell") != "flux1-schnell.safetensors" {
		t.Error("flux-schnell checkpoint mismatch")
	}
	if fluxCheckpoint("flux-dev") != "flux1-dev.safetensors" {
		t.Error("flux-dev checkpoint mismatch")
	}
}

// TestContainerCommand_3DStartStop verifies 3D model detection works.
func TestContainerCommand_3DStartStop(t *testing.T) {
	// Verify 3D model detection
	if !is3DModel("hunyuan3d") {
		t.Error("hunyuan3d should be detected as 3D model")
	}
	if !is3DModel("trellis") {
		t.Error("trellis should be detected as 3D model")
	}
	if is3DModel("qwen3_6") {
		t.Error("qwen3_6 should not be detected as 3D model")
	}

	// Verify directory names
	if dirFor3DModel("hunyuan3d") != "hunyuan3d" {
		t.Error("hunyuan3d dir mismatch")
	}
	if dirFor3DModel("trellis") != "trellis" {
		t.Error("trellis dir mismatch")
	}
}

// TestContainerCommand_StatusAll verifies status all works.
func TestContainerCommand_StatusAll(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewContainerCommand(root)

	exitCode := cmd.Run([]string{"status"})
	if exitCode != 0 {
		t.Errorf("container status returned non-zero: %d", exitCode)
	}
}

// TestContainerCommand_ResolveServiceAlias verifies service alias resolution in container cmd.
func TestContainerCommand_ResolveServiceAlias(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewContainerCommand(root)

	// Test that known aliases resolve to container names
	aliases := map[string]string{
		"comfyui":    "comfyui-flux",
		"embed":      "llm-embed",
		"embedding":  "llm-embed",
		"rerank":     "llm-rerank",
		"reranker":   "llm-rerank",
		"whisper":    "whisper-stt",
		"kokoro":     "kokoro-tts",
		"litellm":    "litellm",
		"swap-api":   "swap-api",
		"open-webui": "open-webui",
		"mcp":        "mcpo",
	}

	for alias, expected := range aliases {
		result, err := cmd.resolveContainer(alias)
		if err != nil {
			t.Errorf("resolveContainer(%q) error: %v", alias, err)
		}
		if result != expected {
			t.Errorf("resolveContainer(%q) = %q, want %q", alias, result, expected)
		}
	}
}

// TestModelCommand_ListWithNewDB verifies model list works with a real DB.
func TestModelCommand_ListWithNewDB(t *testing.T) {
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
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewModelCommand(root)

	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("model list returned non-zero: %d", exitCode)
	}
}

// TestModelCommand_GetWithNewDB verifies model get works with a real DB.
func TestModelCommand_GetWithNewDB(t *testing.T) {
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

	// Create a test model
	model := &models.Model{
		Slug:      "test-model",
		Type:      "llm",
		Name:      "Test Model",
		HFRepo:    "test/test-model",
		YML:       "test.yml",
		Container: "test-container",
		Port:      8080,
		Default:   false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewModelCommand(root)

	exitCode := cmd.Run([]string{"get", "test-model"})
	if exitCode != 0 {
		t.Errorf("model get returned non-zero: %d", exitCode)
	}

	// Test getting non-existent model
	exitCode = cmd.Run([]string{"get", "nonexistent"})
	if exitCode == 0 {
		t.Error("model get nonexistent should return non-zero")
	}
}

// TestLogsCommand_ResolveUnknown verifies unknown service/model errors.
func TestLogsCommand_ResolveUnknown(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewLogsCommand(root)

	// Unknown slug should fail with helpful error
	_, err = cmd.resolveContainer("totally-unknown")
	if err == nil {
		t.Error("resolveContainer(totally-unknown) should return error")
	}
}
