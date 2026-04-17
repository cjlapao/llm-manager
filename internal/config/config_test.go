package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig_DatabaseURL(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DatabaseURL == "" {
		t.Error("DefaultConfig().DatabaseURL is empty")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot determine home dir: %v", err)
	}

	expected := filepath.Join(homeDir, ".local", "share", "llm-manager", "llm-manager.db")
	if cfg.DatabaseURL != expected {
		t.Errorf("DefaultConfig().DatabaseURL = %q, want %q", cfg.DatabaseURL, expected)
	}
}

func TestLoadConfig_DatabaseURL(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.DatabaseURL == "" {
		t.Error("LoadConfig().DatabaseURL is empty")
	}
}

func TestLoadConfig_DatabaseURLOverride(t *testing.T) {
	tmpDir := t.TempDir()
	customDB := filepath.Join(tmpDir, "custom.db")

	t.Setenv("LLM_MANAGER_DATABASE_URL", customDB)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.DatabaseURL != customDB {
		t.Errorf("LoadConfig().DatabaseURL = %q, want %q", cfg.DatabaseURL, customDB)
	}
}

func TestLoadConfig_DatabaseURLEmptyOverride(t *testing.T) {
	t.Setenv("LLM_MANAGER_DATABASE_URL", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	// Should use default, not empty string
	defaultDB := filepath.Join(cfg.HomeDir, ".local", "share", "llm-manager", "llm-manager.db")
	if cfg.DatabaseURL != defaultDB {
		t.Errorf("LoadConfig().DatabaseURL = %q, want %q", cfg.DatabaseURL, defaultDB)
	}
}

func TestConfigString_DatabaseURL(t *testing.T) {
	cfg := DefaultConfig()
	s := cfg.String()

	if !contains(s, "database") {
		t.Error("Config.String() does not contain database field")
	}

	if !contains(s, cfg.DatabaseURL) {
		t.Errorf("Config.String() does not contain the actual database URL: %q", cfg.DatabaseURL)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
