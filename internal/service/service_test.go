package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

// newTestDB creates an in-memory SQLite database, runs migrations, and returns a DatabaseManager.
func newTestDB(t *testing.T) database.DatabaseManager {
	t.Helper()
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db
}

// seedTestModel creates a model with a unique slug and returns it.
func seedTestModel(t *testing.T, db database.DatabaseManager, slug string) *models.Model {
	t.Helper()
	model := &models.Model{
		Slug:      slug,
		Type:      "llm",
		Name:      "Test Model " + slug,
		HFRepo:    "test/" + slug,
		YML:       slug + ".yml",
		Container: slug + "-container",
		Port:      8000,
		Default:   false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel(%s) error: %v", slug, err)
	}
	return model
}

// --- ContainerService.StopAllLLMs tests ---

func TestContainerService_StopAllLLMs(t *testing.T) {
	db := newTestDB(t)

	// Seed multiple LLM models
	seedTestModel(t, db, "llm-1")
	seedTestModel(t, db, "llm-2")

	// Seed a non-LLM model
	nonLLM := &models.Model{
		Slug:      "embed-1",
		Type:      "embedding",
		Name:      "Embed Model",
		HFRepo:    "test/embed",
		Container: "embed-container",
		Port:      8020,
		Default:   false,
	}
	if err := db.CreateModel(nonLLM); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	svc := NewContainerService(db, config.DefaultConfig())
	err := svc.StopAllLLMs()
	// Without running containers, this should succeed (best-effort, logs skipped)
	if err != nil {
		t.Errorf("StopAllLLMs() with no running containers should succeed, got error: %v", err)
	}
}

func TestContainerService_StopAllLLMs_NoModels(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())
	err := svc.StopAllLLMs()
	if err != nil {
		t.Errorf("StopAllLLMs() with no models error: %v", err)
	}
}

// --- ContainerService.DropPageCache tests ---

func TestContainerService_DropPageCache(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	// sync should always work
	err := svc.DropPageCache()
	// The function handles errors gracefully (non-fatal for /proc/sys/vm/drop_caches)
	// so we just verify it doesn't panic
	_ = err
}

func TestContainerService_DropPageCache_SyncFails(t *testing.T) {
	// This test verifies that DropPageCache doesn't panic even if sync fails.
	// We can't easily mock sync, but we can verify the function structure handles errors.
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	// Call it — if sync fails, the function should still return without panicking
	// because error is handled (non-fatal for the overall operation)
	_ = svc.DropPageCache()
}

// --- ContainerService.ActivateFlux / DeactivateFlux tests ---

func TestContainerService_ActivateFlux(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		InstallDir: tmpDir,
	}
	db := newTestDB(t)
	svc := NewContainerService(db, cfg)

	err := svc.ActivateFlux("test-flux-model")
	if err != nil {
		t.Fatalf("ActivateFlux() error: %v", err)
	}

	// Verify file exists and has correct content
	path := filepath.Join(tmpDir, "comfyui", ".active-model")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read active model file: %v", err)
	}
	if string(content) != "test-flux-model" {
		t.Errorf("Active flux file content = %q, want %q", string(content), "test-flux-model")
	}
}

func TestContainerService_DeactivateFlux(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		InstallDir: tmpDir,
	}
	db := newTestDB(t)
	svc := NewContainerService(db, cfg)

	// Create the file first
	if err := svc.ActivateFlux("test-model"); err != nil {
		t.Fatalf("ActivateFlux() error: %v", err)
	}

	// Now deactivate
	err := svc.DeactivateFlux()
	if err != nil {
		t.Fatalf("DeactivateFlux() error: %v", err)
	}

	// Verify file is removed
	path := filepath.Join(tmpDir, "comfyui", ".active-model")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("DeactivateFlux() did not remove the active model file")
	}
}

func TestContainerService_DeactivateFlux_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		InstallDir: tmpDir,
	}
	db := newTestDB(t)
	svc := NewContainerService(db, cfg)

	// Should not error when file doesn't exist
	err := svc.DeactivateFlux()
	if err != nil {
		t.Errorf("DeactivateFlux() on non-existent file should not error, got: %v", err)
	}
}

// --- ContainerService.Activate3D / Deactivate3D tests ---

func TestContainerService_Activate3D(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		InstallDir: tmpDir,
	}
	db := newTestDB(t)
	svc := NewContainerService(db, cfg)

	err := svc.Activate3D("test-3d-model")
	if err != nil {
		t.Fatalf("Activate3D() error: %v", err)
	}

	// Verify file exists and has correct content
	path := filepath.Join(tmpDir, "comfyui", ".active-3d")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read active 3d file: %v", err)
	}
	if string(content) != "test-3d-model" {
		t.Errorf("Active 3d file content = %q, want %q", string(content), "test-3d-model")
	}
}

func TestContainerService_Deactivate3D(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		InstallDir: tmpDir,
	}
	db := newTestDB(t)
	svc := NewContainerService(db, cfg)

	// Create the file first
	if err := svc.Activate3D("test-3d"); err != nil {
		t.Fatalf("Activate3D() error: %v", err)
	}

	// Now deactivate
	err := svc.Deactivate3D()
	if err != nil {
		t.Fatalf("Deactivate3D() error: %v", err)
	}

	// Verify file is removed
	path := filepath.Join(tmpDir, "comfyui", ".active-3d")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Deactivate3D() did not remove the active 3d file")
	}
}

func TestContainerService_Deactivate3D_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		InstallDir: tmpDir,
	}
	db := newTestDB(t)
	svc := NewContainerService(db, cfg)

	// Should not error when file doesn't exist
	err := svc.Deactivate3D()
	if err != nil {
		t.Errorf("Deactivate3D() on non-existent file should not error, got: %v", err)
	}
}

// --- ContainerService.StartComfyUI / StopComfyUI tests ---

func TestContainerService_StartComfyUI_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	// Without Docker, this will fail — that's expected
	err := svc.StartComfyUI()
	if err == nil {
		t.Error("StartComfyUI() without Docker should return error")
	}
}

func TestContainerService_StopComfyUI_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	// Without Docker, this should handle gracefully
	err := svc.StopComfyUI()
	if err != nil {
		t.Errorf("StopComfyUI() without Docker should not error, got: %v", err)
	}
}

// --- ContainerService.StartSpeech / StopSpeech tests ---

func TestContainerService_StartSpeech_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	err := svc.StartSpeech()
	if err == nil {
		t.Error("StartSpeech() without Docker should return error")
	}
}

func TestContainerService_StopSpeech_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	err := svc.StopSpeech()
	if err != nil {
		t.Errorf("StopSpeech() without Docker should not error, got: %v", err)
	}
}

// --- ContainerService.ensureCompose tests ---

func TestEnsureCompose(t *testing.T) {
	db := newTestDB(t)

	// Create engine type and version
	db.CreateEngineType(&models.EngineType{Slug: "vllm", Name: "vLLM", Description: "x"})
	db.CreateEngineVersion(&models.EngineVersion{
		Slug:            "test-v1",
		EngineTypeSlug:  "vllm",
		Version:         "001",
		Image:           "cjlapao/pgx-vllm:latest",
		ContainerName:   "vllm-node",
		Entrypoint:      "python3 -m vllm.entrypoints.openai.api_server",
		IsDefault:       true,
		IsLatest:        true,
		EnvironmentJSON: `{"HF_HUB_OFFLINE":"0"}`,
		VolumesJSON:     `{}`,
		CommandArgs:     `[]`,
	})

	// Create model
	model := &models.Model{
		Slug:              "compose-test",
		Type:              "llm",
		Name:              "Compose Test",
		HFRepo:            "test/model",
		Container:         "llm-compose-test",
		Port:              8010,
		EngineType:        "vllm",
		EngineVersionSlug: "test-v1",
		EnvVars:           `{}`,
		CommandArgs:       `[]`,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	svc := NewContainerService(db, config.DefaultConfig())
	tmpDir := t.TempDir()
	svc.cfg.LLMDir = tmpDir

	// First call — should create the file
	err := svc.ensureCompose(model)
	if err != nil {
		t.Fatalf("ensureCompose() error: %v", err)
	}

	composePath := filepath.Join(tmpDir, "compose-test.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Fatal("compose file was not created")
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read compose file: %v", err)
	}
	content := string(data)

	// Verify service name uses the type-slug pattern
	if !strings.Contains(content, "llm-compose-test:") {
		t.Error("compose YAML missing service name 'llm-compose-test:'")
	}
	if !strings.Contains(content, "cjlapao/pgx-vllm:latest") {
		t.Error("compose YAML missing image")
	}
	if !strings.Contains(content, "llm-compose-test") {
		t.Error("compose YAML missing container name")
	}

	// Second call — should overwrite (idempotent)
	err = svc.ensureCompose(model)
	if err != nil {
		t.Fatalf("ensureCompose() second call error: %v", err)
	}

	data2, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read compose file after second call: %v", err)
	}
	if string(data) != string(data2) {
		t.Error("ensureCompose() should produce identical output on repeated calls")
	}
}

func TestEnsureCompose_ModelNotFound(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	// Model with no engine config — ensureCompose will fail during engine resolution
	model := &models.Model{Slug: "no-engine", Type: "llm", EngineType: ""}
	err := svc.ensureCompose(model)
	if err == nil {
		t.Error("ensureCompose() with missing engine config should error")
	}

	if !strings.Contains(err.Error(), "resolve engine config") {
		t.Errorf("expected 'resolve engine config' error, got: %v", err)
	}
}

// ──────────────────────────────────────────────
// pickFirstVolumePath tests (AC-2)
// ──────────────────────────────────────────────

func TestPickFirstVolumePath_NilMap(t *testing.T) {
	result := pickFirstVolumePath(nil)
	if result != "" {
		t.Errorf("pickFirstVolumePath(nil) = %q, want empty string", result)
	}
}

func TestPickFirstVolumePath_EmptyMap(t *testing.T) {
	result := pickFirstVolumePath(map[string]string{})
	if result != "" {
		t.Errorf("pickFirstVolumePath(empty) = %q, want empty string", result)
	}
}

func TestPickFirstVolumePath_SingleEntry(t *testing.T) {
	vols := map[string]string{"/home/runner/ComfyUI/models": "/opt/comfyui/models"}
	result := pickFirstVolumePath(vols)
	if result != "/opt/comfyui/models" {
		t.Errorf("pickFirstVolumePath(single) = %q, want %q", result, "/opt/comfyui/models")
	}
}

func TestPickFirstVolumePath_MultipleEntries(t *testing.T) {
	vols := map[string]string{
		"/home/runner/ComfyUI/models":             "/opt/comfyui/models",
		"/home/runner/ComfyUI/models/checkpoints": "/opt/comfyui/checkpoints",
	}
	result := pickFirstVolumePath(vols)
	// Should return the first host path found (deterministic for a single entry)
	if result == "" {
		t.Error("pickFirstVolumePath(multi) returned empty string")
	}
}

func TestPickFirstVolumePath_NestedPaths(t *testing.T) {
	vols := map[string]string{
		"/models":             "/data/models",
		"/models/checkpoints": "/data/checkpoints",
		"/models/loras":       "/data/loras",
	}
	result := pickFirstVolumePath(vols)
	if result == "" {
		t.Error("pickFirstVolumePath(nested) returned empty string")
	}
}
