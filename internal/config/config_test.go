package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Verbose != false {
		t.Errorf("DefaultConfig().Verbose = %v, want false", cfg.Verbose)
	}

	if cfg.HomeDir == "" {
		t.Error("DefaultConfig().HomeDir is empty")
	}

	if cfg.DataDir == "" {
		t.Error("DefaultConfig().DataDir is empty")
	}

	if cfg.LogDir == "" {
		t.Error("DefaultConfig().LogDir is empty")
	}
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil config")
	}

	if cfg.DataDir == "" {
		t.Error("LoadConfig().DataDir is empty")
	}

	if cfg.LogDir == "" {
		t.Error("LoadConfig().LogDir is empty")
	}
}

func TestLoadConfigVerbose(t *testing.T) {
	t.Setenv("LLM_MANAGER_VERBOSE", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if !cfg.Verbose {
		t.Error("LoadConfig().Verbose = false, want true when LLM_MANAGER_VERBOSE=true")
	}
}

func TestLoadConfigConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.ConfigFile != configPath {
		t.Errorf("LoadConfig().ConfigFile = %q, want %q", cfg.ConfigFile, configPath)
	}
}

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	if dir == "" {
		t.Error("ConfigDir() returned empty string")
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("ConfigDir() = %q, want absolute path", dir)
	}
}

func TestDataDir(t *testing.T) {
	dir := DataDir()
	if dir == "" {
		t.Error("DataDir() returned empty string")
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("DataDir() = %q, want absolute path", dir)
	}
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name  string
		path  string
		empty bool
	}{
		{"existing dir", tmpDir, false},
		{"current dir", ".", false},
		{"empty path", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureDir(tt.path)
			if tt.empty {
				if err == nil {
					t.Error("ensureDir() with empty path should return error")
				}
				return
			}

			if err != nil {
				t.Errorf("ensureDir(%q) returned error: %v", tt.path, err)
			}
		})
	}
}

func TestConfigString(t *testing.T) {
	cfg := DefaultConfig()
	s := cfg.String()

	if s == "" {
		t.Error("Config.String() returned empty string")
	}

	expectedFields := []string{"verbose", "data dir", "log dir"}
	for _, field := range expectedFields {
		if !contains(s, field) {
			t.Errorf("Config.String() does not contain %q", field)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
