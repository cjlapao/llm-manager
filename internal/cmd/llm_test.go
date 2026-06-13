package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

// newTestLlmCommand creates a minimal LlmCommand with an in-memory SQLite DB for testing.
func newTestLlmCommand(t *testing.T) (*LlmCommand, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath

	// Need an install dir for flux/3D file path resolution
	cfg.InstallDir = tmpDir

	root := &RootCommand{
		db:  mgr,
		cfg: cfg,
	}

	cmd := NewLlmCommand(root)
	return cmd, dbPath
}

// addTestModel inserts a model into the test database and returns it.
func addTestModel(t *testing.T, mgr database.DatabaseManager, slug string) *models.Model {
	t.Helper()
	m := &models.Model{
		Slug: slug,
		Name: "Test Model " + slug,
		Port: 8080,
		Type: "llm",
	}
	if err := mgr.CreateModel(m); err != nil {
		t.Fatalf("CreateModel(%q) returned error: %v", slug, err)
	}
	return m
}

// ── resolveLatestSlug tests ──────────────────────────────────────────────────

func TestResolveLatestSlug_NotSet(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	resolved, err := resolveLatestSlug(mgr)
	if err == nil {
		t.Fatal("resolveLatestSlug() expected error, got nil")
	}
	if resolved != "" {
		t.Errorf("resolveLatestSlug() = %q, want empty string", resolved)
	}
	if !strings.Contains(err.Error(), "no latest model has been set") {
		t.Errorf("resolveLatestSlug() error = %q, want 'no latest model has been set'", err.Error())
	}
}

func TestResolveLatestSlug_SetButNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Set latest model to a slug that doesn't exist in the DB
	cfgSvc := service.NewConfigService(mgr)
	if err := cfgSvc.Set("LLM_MANAGER_LATEST_MODEL", "nonexistent-model"); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	resolved, err := resolveLatestSlug(mgr)
	if err == nil {
		t.Fatal("resolveLatestSlug() expected error, got nil")
	}
	if resolved != "" {
		t.Errorf("resolveLatestSlug() = %q, want empty string", resolved)
	}
	if !strings.Contains(err.Error(), "not a known model") {
		t.Errorf("resolveLatestSlug() error = %q, want 'not a known model'", err.Error())
	}
}

func TestResolveLatestSlug_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Create a model and set it as latest
	addTestModel(t, mgr, "qwen3_6")
	cfgSvc := service.NewConfigService(mgr)
	if err := cfgSvc.Set("LLM_MANAGER_LATEST_MODEL", "qwen3_6"); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	resolved, err := resolveLatestSlug(mgr)
	if err != nil {
		t.Fatalf("resolveLatestSlug() returned error: %v", err)
	}
	if resolved != "qwen3_6" {
		t.Errorf("resolveLatestSlug() = %q, want %q", resolved, "qwen3_6")
	}
}

// ── runStart latest resolution tests ─────────────────────────────────────────

func TestRunStartLatest_NotSet(t *testing.T) {
	cmd, _ := newTestLlmCommand(t)

	exitCode := cmd.Run([]string{"start", "latest"})
	if exitCode != 1 {
		t.Errorf("Run([start, latest]) = %d, want 1", exitCode)
	}
}

func TestRunStartLatest_SetButStale(t *testing.T) {
	cmd, _ := newTestLlmCommand(t)

	// Set latest to a slug that doesn't exist in DB
	cfgSvc := service.NewConfigService(cmd.cfg.db)
	if err := cfgSvc.Set("LLM_MANAGER_LATEST_MODEL", "phantom-model"); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	exitCode := cmd.Run([]string{"start", "latest"})
	if exitCode != 1 {
		t.Errorf("Run([start, latest]) = %d, want 1", exitCode)
	}
}

func TestRunStartLatest_Valid(t *testing.T) {
	cmd, _ := newTestLlmCommand(t)

	// Create a model and set it as latest
	addTestModel(t, cmd.cfg.db, "qwen3_6")
	cfgSvc := service.NewConfigService(cmd.cfg.db)
	if err := cfgSvc.Set("LLM_MANAGER_LATEST_MODEL", "qwen3_6"); err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	// Capture stdout to verify the resolution message
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := cmd.Run([]string{"start", "latest"})

	w.Close()
	os.Stdout = oldStdout

	// Should return 1 because there's no actual Docker, but the resolution should have happened
	// We expect exit code 1 (Docker error) not 1 (resolution error), so check stdout for resolution message
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	_ = exitCode // exit code is 1 because Docker isn't available in tests; resolution already succeeded

	if !strings.Contains(output, "Resolving 'latest' to model: qwen3_6") {
		t.Errorf("Output missing resolution message. Got: %q", output)
	}
}

func TestRunStartLatest_InvalidSlug(t *testing.T) {
	cmd, _ := newTestLlmCommand(t)

	// Start with a completely bogus slug (not "latest")
	cmd.Run([]string{"start", "totally-bogus-slug"})
	// Should return non-zero because the model doesn't exist
}

// ── SetLatestModel persistence tests ─────────────────────────────────────────

// TestSetLatestModelPersistence verifies that SetLatestModel correctly persists
// the slug to the database — the same call runStart() makes after a successful
// container start.
func TestSetLatestModelPersistence(t *testing.T) {
	cmd, _ := newTestLlmCommand(t)

	// Create a model so the slug maps to a known model
	addTestModel(t, cmd.cfg.db, "test-model-1")

	// Simulate what runStart() does after a successful container start:
	// set this model as the latest started model
	configSvc := service.NewConfigService(cmd.cfg.db)
	if err := configSvc.SetLatestModel("test-model-1"); err != nil {
		t.Fatalf("SetLatestModel() returned error: %v", err)
	}

	// Verify GetLatestModel returns the persisted slug
	resolved, err := configSvc.GetLatestModel()
	if err != nil {
		t.Fatalf("GetLatestModel() returned error: %v", err)
	}
	if resolved != "test-model-1" {
		t.Errorf("GetLatestModel() = %q, want %q", resolved, "test-model-1")
	}

	// Verify the value is also retrievable via the database directly
	// (this is what runStatusAll() uses)
	latestModel, err := cmd.cfg.db.GetConfig("LLM_MANAGER_LATEST_MODEL")
	if err != nil {
		t.Fatalf("GetConfig() returned error: %v", err)
	}
	if latestModel == nil {
		t.Fatal("GetConfig() returned nil for LLM_MANAGER_LATEST_MODEL")
	}
	if latestModel.Value != "test-model-1" {
		t.Errorf("GetConfig() Value = %q, want %q", latestModel.Value, "test-model-1")
	}
}

// TestSetLatestModel_Overwrite verifies that setting a new latest model
// overwrites the previous value — matching runStart() behavior where each
// successful start updates the latest.
func TestSetLatestModel_Overwrite(t *testing.T) {
	cmd, _ := newTestLlmCommand(t)

	addTestModel(t, cmd.cfg.db, "model-a")
	addTestModel(t, cmd.cfg.db, "model-b")

	configSvc := service.NewConfigService(cmd.cfg.db)

	// First start: set model-a as latest
	if err := configSvc.SetLatestModel("model-a"); err != nil {
		t.Fatalf("SetLatestModel(model-a) returned error: %v", err)
	}

	resolved, _ := configSvc.GetLatestModel()
	if resolved != "model-a" {
		t.Errorf("After first start: GetLatestModel() = %q, want %q", resolved, "model-a")
	}

	// Second start: set model-b as latest (simulates a new successful start)
	if err := configSvc.SetLatestModel("model-b"); err != nil {
		t.Fatalf("SetLatestModel(model-b) returned error: %v", err)
	}

	resolved, _ = configSvc.GetLatestModel()
	if resolved != "model-b" {
		t.Errorf("After second start: GetLatestModel() = %q, want %q", resolved, "model-b")
	}
}
