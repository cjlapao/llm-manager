package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

func newTestConfigService(t *testing.T) (*ConfigService, string) {
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

	svc := NewConfigService(mgr)
	return svc, dbPath
}

func TestConfigService_SetAndGet(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Set("LLM_MANAGER_DATA_DIR", "/custom/data")
	if err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	cfg, err := svc.Get("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}
	if cfg.Value != "/custom/data" {
		t.Errorf("Value = %q, want %q", cfg.Value, "/custom/data")
	}
}

func TestConfigService_SetInvalidKey(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Set("INVALID_KEY", "value")
	if err == nil {
		t.Fatal("Set() with invalid key should return error, got nil")
	}
}

func TestConfigService_UpdateExisting(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Set("LLM_MANAGER_DATA_DIR", "/initial/data")
	if err != nil {
		t.Fatalf("Set() initial returned error: %v", err)
	}

	err = svc.Set("LLM_MANAGER_DATA_DIR", "/updated/data")
	if err != nil {
		t.Fatalf("Set() update returned error: %v", err)
	}

	cfg, err := svc.Get("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if cfg.Value != "/updated/data" {
		t.Errorf("Value = %q, want %q", cfg.Value, "/updated/data")
	}
}

func TestConfigService_Unset(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Set("LLM_MANAGER_DATA_DIR", "/custom/data")
	if err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	err = svc.Unset("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("Unset() returned error: %v", err)
	}

	cfg, err := svc.Get("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("Get() after unset returned error: %v", err)
	}
	if cfg != nil {
		t.Errorf("Get() after unset = %+v, want nil", cfg)
	}
}

func TestConfigService_UnsetInvalidKey(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Unset("INVALID_KEY")
	if err == nil {
		t.Fatal("Unset() with invalid key should return error, got nil")
	}
}

func TestConfigService_List(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Set("LLM_MANAGER_DATA_DIR", "/custom/data")
	if err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}
	err = svc.Set("LLM_MANAGER_LITELLM_URL", "http://example.com")
	if err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	entries, err := svc.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("List() returned %d entries, want 2", len(entries))
	}
}

func TestConfigService_SetEmptyValue(t *testing.T) {
	svc, _ := newTestConfigService(t)

	err := svc.Set("LLM_MANAGER_LITELLM_URL", "")
	if err != nil {
		t.Fatalf("Set() with empty value returned error: %v", err)
	}

	cfg, err := svc.Get("LLM_MANAGER_LITELLM_URL")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}
	if cfg.Value != "" {
		t.Errorf("Value = %q, want empty string", cfg.Value)
	}
}

func TestConfigService_AllValidKeys(t *testing.T) {
	svc, _ := newTestConfigService(t)

	validKeys := config.ValidConfigKeys()
	for _, key := range validKeys {
		err := svc.Set(key, "test-value")
		if err != nil {
			t.Errorf("Set(%q) returned error: %v", key, err)
		}

		cfg, err := svc.Get(key)
		if err != nil {
			t.Errorf("Get(%q) returned error: %v", key, err)
		}
		if cfg == nil {
			t.Errorf("Get(%q) returned nil", key)
		}

		err = svc.Unset(key)
		if err != nil {
			t.Errorf("Unset(%q) returned error: %v", key, err)
		}
	}
}

func TestConfigService_ConfigFileOverride(t *testing.T) {
	// This test verifies that the service layer correctly stores values
	// even when config file or env vars override them (warnings are CLI-layer concern)
	svc, _ := newTestConfigService(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("LLM_MANAGER_DATA_DIR: /file/data\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set env var to use custom config path
	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	// Service should still be able to store the value in DB regardless of file/env
	err = svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")
	if err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	cfg, err := svc.Get("LLM_MANAGER_DATA_DIR")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if cfg.Value != "/db/data" {
		t.Errorf("Value = %q, want %q (service should store DB value regardless of overrides)", cfg.Value, "/db/data")
	}
}
