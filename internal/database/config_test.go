package database

import (
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
)

// --- Config table AutoMigrate tests ---

func TestAutoMigrate_ConfigTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Verify config table was created
	sqlDB, err := mgr.DB().DB()
	if err != nil {
		t.Fatalf("DB().DB() returned error: %v", err)
	}

	var exists int
	err = sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='config'`).Scan(&exists)
	if err != nil {
		t.Fatalf("Query for config table returned error: %v", err)
	}
	if exists != 1 {
		t.Error("Table config does not exist")
	}
}

func TestAutoMigrate_ConfigTableIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	// Run AutoMigrate twice — should not error
	for i := 0; i < 2; i++ {
		err = mgr.AutoMigrate()
		if err != nil {
			t.Fatalf("AutoMigrate() iteration %d returned error: %v", i, err)
		}
	}
}

// --- Config CRUD tests ---

func TestSetConfig_GetConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Set a config value
	err = mgr.SetConfig("LLM_MANAGER_DATA_DIR", "/custom/data")
	if err != nil {
		t.Fatalf("SetConfig() returned error: %v", err)
	}

	// Get it back
	cfg, err := mgr.GetConfig("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("GetConfig() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("GetConfig() returned nil")
	}
	if cfg.Key != "LLM_MANAGER_DATA_DIR" {
		t.Errorf("Key = %q, want %q", cfg.Key, "LLM_MANAGER_DATA_DIR")
	}
	if cfg.Value != "/custom/data" {
		t.Errorf("Value = %q, want %q", cfg.Value, "/custom/data")
	}
	if cfg.ID != 1 {
		t.Errorf("ID = %d, want 1", cfg.ID)
	}
}

func TestSetConfig_MultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Set multiple config values
	keys := []string{
		"LLM_MANAGER_DATA_DIR",
		"LLM_MANAGER_LOG_DIR",
		"LLM_MANAGER_LITELLM_URL",
	}
	values := []string{
		"/custom/data",
		"/custom/log",
		"http://example.com:4000",
	}

	for i, key := range keys {
		err = mgr.SetConfig(key, values[i])
		if err != nil {
			t.Fatalf("SetConfig(%q) returned error: %v", key, err)
		}
	}

	// Verify all keys exist
	for i, key := range keys {
		cfg, err := mgr.GetConfig(key)
		if err != nil {
			t.Fatalf("GetConfig(%q) returned error: %v", key, err)
		}
		if cfg == nil {
			t.Fatalf("GetConfig(%q) returned nil", key)
		}
		if cfg.Value != values[i] {
			t.Errorf("GetConfig(%q) value = %q, want %q", key, cfg.Value, values[i])
		}
	}
}

func TestSetConfig_UpdateExisting(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Set initial value
	err = mgr.SetConfig("LLM_MANAGER_DATA_DIR", "/initial/data")
	if err != nil {
		t.Fatalf("SetConfig() initial returned error: %v", err)
	}

	// Update the value
	err = mgr.SetConfig("LLM_MANAGER_DATA_DIR", "/updated/data")
	if err != nil {
		t.Fatalf("SetConfig() update returned error: %v", err)
	}

	// Verify updated value
	cfg, err := mgr.GetConfig("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("GetConfig() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("GetConfig() returned nil")
	}
	if cfg.Value != "/updated/data" {
		t.Errorf("Value = %q, want %q", cfg.Value, "/updated/data")
	}
}

func TestGetConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Get a key that doesn't exist
	cfg, err := mgr.GetConfig("LLM_MANAGER_NONEXISTENT")
	if err != nil {
		t.Fatalf("GetConfig() for nonexistent key returned error: %v", err)
	}
	if cfg != nil {
		t.Errorf("GetConfig() for nonexistent key = %+v, want nil", cfg)
	}
}

func TestUnsetConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Set a value
	err = mgr.SetConfig("LLM_MANAGER_DATA_DIR", "/custom/data")
	if err != nil {
		t.Fatalf("SetConfig() returned error: %v", err)
	}

	// Unset it
	err = mgr.UnsetConfig("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("UnsetConfig() returned error: %v", err)
	}

	// Verify it's gone
	cfg, err := mgr.GetConfig("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("GetConfig() after unset returned error: %v", err)
	}
	if cfg != nil {
		t.Errorf("GetConfig() after unset = %+v, want nil", cfg)
	}
}

func TestUnsetConfig_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Unset a key that doesn't exist — should not error
	err = mgr.UnsetConfig("LLM_MANAGER_NONEXISTENT")
	if err != nil {
		t.Fatalf("UnsetConfig() for nonexistent key returned error: %v", err)
	}
}

func TestListConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// Set some values
	mgr.SetConfig("LLM_MANAGER_LITELLM_URL", "http://example.com")
	mgr.SetConfig("LLM_MANAGER_DATA_DIR", "/custom/data")
	mgr.SetConfig("LLM_MANAGER_LOG_DIR", "/custom/log")

	// List all
	configs, err := mgr.ListConfig()
	if err != nil {
		t.Fatalf("ListConfig() returned error: %v", err)
	}

	if len(configs) != 3 {
		t.Errorf("ListConfig() returned %d entries, want 3", len(configs))
	}

	// Verify sorted by key
	if len(configs) >= 2 {
		if configs[0].Key > configs[1].Key {
			t.Errorf("ListConfig() not sorted: %q > %q", configs[0].Key, configs[1].Key)
		}
	}

	// Verify all expected keys present
	keyMap := make(map[string]string)
	for _, cfg := range configs {
		keyMap[cfg.Key] = cfg.Value
	}

	expected := map[string]string{
		"LLM_MANAGER_DATA_DIR":    "/custom/data",
		"LLM_MANAGER_LOG_DIR":     "/custom/log",
		"LLM_MANAGER_LITELLM_URL": "http://example.com",
	}

	for k, v := range expected {
		if keyMap[k] != v {
			t.Errorf("ListConfig()[%s] = %q, want %q", k, keyMap[k], v)
		}
	}
}

func TestListConfig_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	// List with no values
	configs, err := mgr.ListConfig()
	if err != nil {
		t.Fatalf("ListConfig() returned error: %v", err)
	}

	if len(configs) != 0 {
		t.Errorf("ListConfig() returned %d entries, want 0", len(configs))
	}
}

func TestConfig_NotOpen(t *testing.T) {
	mgr, err := NewDatabaseManager("test.db")
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	// All config methods should error without Open()
	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetConfig", func() error { _, err := mgr.GetConfig("key"); return err }},
		{"SetConfig", func() error { return mgr.SetConfig("key", "value") }},
		{"UnsetConfig", func() error { return mgr.UnsetConfig("key") }},
		{"ListConfig", func() error { _, err := mgr.ListConfig(); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Errorf("%s() without Open() should return error, got nil", tt.name)
			}
		})
	}
}

func TestConfig_TableName(t *testing.T) {
	cfg := models.Config{}
	if cfg.TableName() != "config" {
		t.Errorf("Config.TableName() = %q, want %q", cfg.TableName(), "config")
	}
}
