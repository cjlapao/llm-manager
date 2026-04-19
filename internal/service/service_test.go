package service

import (
	"os"
	"path/filepath"
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
		Type:      "embed",
		Name:      "Embed Model",
		HFRepo:    "test/embed",
		YML:       "embed.yml",
		Container: "embed-container",
		Port:      8020,
	}
	if err := db.CreateModel(nonLLM); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	// Seed an LLM model without a YML file
	noYML := &models.Model{
		Slug:      "llm-no-yml",
		Type:      "llm",
		Name:      "No YML Model",
		HFRepo:    "test/no-yml",
		YML:       "",
		Container: "no-yml-container",
		Port:      8099,
	}
	if err := db.CreateModel(noYML); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	svc := NewContainerService(db, config.DefaultConfig())
	err := svc.StopAllLLMs()
	// Without Docker, the command will fail — that's expected in tests
	if err == nil {
		t.Error("StopAllLLMs() without Docker should return error")
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

// --- HotspotService.StopHotspot tests ---

func TestHotspotService_StopHotspot_NoHotspot(t *testing.T) {
	db := newTestDB(t)
	_ = NewHotspotService(db) // verified constructor works without config

	// Without config, StopHotspot will fail on GetModel, but we test with config
	svcWithCfg := NewHotspotServiceWithConfig(db, config.DefaultConfig())
	err := svcWithCfg.StopHotspot()
	if err == nil {
		t.Error("StopHotspot() with no hotspot should return error")
	}
}

func TestHotspotService_StopHotspot(t *testing.T) {
	db := newTestDB(t)
	seedTestModel(t, db, "hotspot-to-stop")

	svc := NewHotspotService(db)
	if err := svc.SetHotspot("hotspot-to-stop"); err != nil {
		t.Fatalf("SetHotspot() error: %v", err)
	}

	// Verify hotspot is set
	hotspot, err := svc.GetCurrentHotspot()
	if err != nil {
		t.Fatalf("GetCurrentHotspot() error: %v", err)
	}
	if hotspot == nil || hotspot.ModelSlug != "hotspot-to-stop" {
		t.Error("Hotspot should be set before stopping")
	}

	svcWithCfg := NewHotspotServiceWithConfig(db, config.DefaultConfig())
	err = svcWithCfg.StopHotspot()
	// The docker compose down will fail (no actual docker), but the function
	// should handle it gracefully
	if err != nil {
		t.Logf("StopHotspot() returned error (expected without Docker): %v", err)
	}

	// Verify hotspot is cleared
	hotspot, _ = svc.GetCurrentHotspot()
	if hotspot != nil {
		t.Error("Hotspot should be cleared after StopHotspot()")
	}
}

// --- HotspotService.RestartHotspot tests ---

func TestHotspotService_RestartHotspot_NoHotspot(t *testing.T) {
	db := newTestDB(t)
	svc := NewHotspotServiceWithConfig(db, config.DefaultConfig())

	err := svc.RestartHotspot()
	if err == nil {
		t.Error("RestartHotspot() with no hotspot should return error")
	}
}

func TestHotspotService_RestartHotspot(t *testing.T) {
	db := newTestDB(t)
	seedTestModel(t, db, "hotspot-to-restart")

	svc := NewHotspotService(db)
	if err := svc.SetHotspot("hotspot-to-restart"); err != nil {
		t.Fatalf("SetHotspot() error: %v", err)
	}

	svcWithCfg := NewHotspotServiceWithConfig(db, config.DefaultConfig())
	err := svcWithCfg.RestartHotspot()
	// The docker compose operations will fail (no actual Docker), so we expect an error
	if err == nil {
		t.Error("RestartHotspot() without Docker should return error")
	}

	// After a failed restart, hotspot should NOT be cleared (data preservation fix)
	hotspot, _ := svc.GetCurrentHotspot()
	if hotspot == nil {
		t.Error("Hotspot should be preserved after failed RestartHotspot()")
	}
	if hotspot.ModelSlug != "hotspot-to-restart" {
		t.Errorf("Hotspot model should still be hotspot-to-restart, got %s", hotspot.ModelSlug)
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

// --- ContainerService.StartEmbed / StopEmbed tests ---

func TestContainerService_StartEmbed_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	err := svc.StartEmbed()
	if err == nil {
		t.Error("StartEmbed() without Docker should return error")
	}
}

func TestContainerService_StopEmbed_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	err := svc.StopEmbed()
	if err != nil {
		t.Errorf("StopEmbed() without Docker should not error, got: %v", err)
	}
}

// --- ContainerService.StartRerank / StopRerank tests ---

func TestContainerService_StartRerank_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	err := svc.StartRerank()
	if err == nil {
		t.Error("StartRerank() without Docker should return error")
	}
}

func TestContainerService_StopRerank_NoDocker(t *testing.T) {
	db := newTestDB(t)
	svc := NewContainerService(db, config.DefaultConfig())

	err := svc.StopRerank()
	if err != nil {
		t.Errorf("StopRerank() without Docker should not error, got: %v", err)
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
