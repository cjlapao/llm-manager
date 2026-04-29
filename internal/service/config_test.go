package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/crypto"
	"github.com/user/llm-manager/internal/database"
)

func newTestConfigService(t *testing.T) (*ConfigService, string) {
	t.Helper()

	// Set encryption key for tests (32 bytes base64-encoded)
	t.Setenv("LLM_MANAGER_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")

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
	err = svc.Set("LITELLM_URL", "http://example.com")
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

	err := svc.Set("LITELLM_URL", "")
	if err != nil {
		t.Fatalf("Set() with empty value returned error: %v", err)
	}

	cfg, err := svc.Get("LITELLM_URL")
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

func TestConfigService_HFTOKENSecretEncrypted(t *testing.T) {
	svc, _ := newTestConfigService(t)

	hfToken := "hf_test_secret_token_abc123"

	err := svc.Set("HF_TOKEN", hfToken)
	if err != nil {
		t.Fatalf("Set(HF_TOKEN) returned error: %v", err)
	}

	cfg, err := svc.Get("HF_TOKEN")
	if err != nil {
		t.Fatalf("Get(HF_TOKEN) returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Get(HF_TOKEN) returned nil")
	}

	// The stored value should be encrypted (bcrypt prefixed)
	if !crypto.IsEncrypted(cfg.Value) {
		t.Errorf("HF_TOKEN was not encrypted in DB: got raw value %q", cfg.Value)
	}

	// It should NOT match the plaintext
	if cfg.Value == hfToken {
		t.Error("HF_TOKEN stored as plaintext in DB instead of encrypted")
	}
}

func TestConfigService_HFTOKENVerifyCorrectly(t *testing.T) {
	svc, _ := newTestConfigService(t)

	hfToken := "hf_verify_correct_token_xyz789"

	err := svc.Set("HF_TOKEN", hfToken)
	if err != nil {
		t.Fatalf("Set(HF_TOKEN) returned error: %v", err)
	}

	matched, err := svc.VerifySecret("HF_TOKEN", hfToken)
	if err != nil {
		t.Fatalf("VerifySecret() returned error: %v", err)
	}
	if !matched {
		t.Error("VerifySecret() expected true for correct token, got false")
	}

	wrongToken := "wrong_token_here"
	matched, err = svc.VerifySecret("HF_TOKEN", wrongToken)
	if err != nil {
		t.Fatalf("VerifySecret() with wrong token returned error: %v", err)
	}
	if matched {
		t.Error("VerifySecret() expected false for wrong token, got true")
	}
}
