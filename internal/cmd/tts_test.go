package cmd

import (
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

func newTestTtsCommand(t *testing.T) (*TtsCommand, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

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
	cfg.InstallDir = tmpDir

	root := &RootCommand{db: mgr, cfg: cfg}
	cmd := NewTtsCommand(root)
	return cmd, dbPath
}

func addTestTTSModel(t *testing.T, mgr database.DatabaseManager, slug string, isDefault bool) *models.Model {
	t.Helper()
	m := &models.Model{
		Slug:      slug,
		Name:      "TTS Model " + slug,
		Port:      8091,
		Type:      "speech",
		SubType:   "tts",
		Container: "tts-" + slug,
		Default:   isDefault,
	}
	if err := mgr.CreateModel(m); err != nil {
		t.Fatalf("CreateModel(%q) returned error: %v", slug, err)
	}
	return m
}

// --- resolveSlug tests for TTS ---

func TestTtsResolveSlug_NoModels(t *testing.T) {
	cmd, _ := newTestTtsCommand(t)
	_, err := cmd.resolveSlug("", false)
	if err == nil {
		t.Fatal("resolveSlug with no models expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTS models configured") {
		t.Errorf("resolveSlug error = %q, want substring 'no TTS models configured'", err.Error())
	}
}

func TestTtsResolveSlug_ExplicitValidTTS(t *testing.T) {
	cmd, _ := newTestTtsCommand(t)
	addTestTTSModel(t, cmd.cfg.db, "kokoro-tts", false)

	resolved, err := cmd.resolveSlug("kokoro-tts", false)
	if err != nil {
		t.Fatalf("resolveSlug(explicit TTS) returned error: %v", err)
	}
	if resolved != "kokoro-tts" {
		t.Errorf("resolveSlug = %q, want 'kokoro-tts'", resolved)
	}
}

func TestTtsResolveSlug_FirstAvailable(t *testing.T) {
	cmd, _ := newTestTtsCommand(t)
	addTestTTSModel(t, cmd.cfg.db, "model-a", false)
	addTestTTSModel(t, cmd.cfg.db, "model-b", false)

	resolved, err := cmd.resolveSlug("", false)
	if err != nil {
		t.Fatalf("resolveSlug(no args) returned error: %v", err)
	}
	if resolved != "model-a" && resolved != "model-b" {
		t.Errorf("resolveSlug = %q, want one of [model-a, model-b]", resolved)
	}
}
