package config

import (
	"fmt"
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

// --- Tests for ReadConfigFile / WriteConfigFile ---

func TestReadConfigFile_NonExistent(t *testing.T) {
	// Use a temp directory so no config file exists
	t.Setenv("LLM_MANAGER_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))

	result, err := ReadConfigFile()
	if err != nil {
		t.Fatalf("ReadConfigFile() returned error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("ReadConfigFile() = %d entries, want 0", len(result))
	}
}

func TestReadConfigFile_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a test config file
	err := os.WriteFile(configPath, []byte(`LITELLM_URL: "http://example.com"
LLM_MANAGER_DATA_DIR: "/custom/data"
`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	result, err := ReadConfigFile()
	if err != nil {
		t.Fatalf("ReadConfigFile() returned error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("ReadConfigFile() = %d entries, want 2", len(result))
	}

	if result["LITELLM_URL"] != "http://example.com" {
		t.Errorf("LITELLM_URL = %q, want %q", result["LITELLM_URL"], "http://example.com")
	}

	if result["LLM_MANAGER_DATA_DIR"] != "/custom/data" {
		t.Errorf("LLM_MANAGER_DATA_DIR = %q, want %q", result["LLM_MANAGER_DATA_DIR"], "/custom/data")
	}
}

func TestReadConfigFile_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a config file with an unknown key
	err := os.WriteFile(configPath, []byte(`LITELLM_URL: "http://example.com"
UNKNOWN_KEY: "value"
`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	_, err = ReadConfigFile()
	if err == nil {
		t.Fatal("ReadConfigFile() expected error for unknown key, got nil")
	}

	if !contains(err.Error(), "unknown config key") {
		t.Errorf("Error should mention unknown key: %v", err)
	}
}

func TestWriteConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	// Write values
	err := WriteConfigFile(map[string]string{
		"LITELLM_URL": "http://example.com",
		"LLM_MANAGER_DATA_DIR":    "/custom/data",
	})
	if err != nil {
		t.Fatalf("WriteConfigFile() returned error: %v", err)
	}

	// Read back and verify
	result, err := ReadConfigFile()
	if err != nil {
		t.Fatalf("ReadConfigFile() returned error: %v", err)
	}

	if result["LITELLM_URL"] != "http://example.com" {
		t.Errorf("LITELLM_URL = %q, want %q", result["LITELLM_URL"], "http://example.com")
	}

	if result["LLM_MANAGER_DATA_DIR"] != "/custom/data" {
		t.Errorf("LLM_MANAGER_DATA_DIR = %q, want %q", result["LLM_MANAGER_DATA_DIR"], "/custom/data")
	}
}

func TestWriteConfigFile_FiltersEmptyValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	// Write values including an empty one
	err := WriteConfigFile(map[string]string{
		"LITELLM_URL": "http://example.com",
		"LLM_MANAGER_DATA_DIR":    "", // should be filtered
	})
	if err != nil {
		t.Fatalf("WriteConfigFile() returned error: %v", err)
	}

	// Read back and verify empty value was filtered
	result, err := ReadConfigFile()
	if err != nil {
		t.Fatalf("ReadConfigFile() returned error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("WriteConfigFile() wrote %d entries, want 1 (empty values filtered)", len(result))
	}

	if _, ok := result["LLM_MANAGER_DATA_DIR"]; ok {
		t.Error("Empty value for LLM_MANAGER_DATA_DIR should have been filtered")
	}
}

func TestWriteConfigFile_UpdateExisting(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	// Write initial value
	err := WriteConfigFile(map[string]string{
		"LITELLM_URL": "http://old.com",
	})
	if err != nil {
		t.Fatalf("WriteConfigFile() returned error: %v", err)
	}

	// Update value
	err = WriteConfigFile(map[string]string{
		"LITELLM_URL": "http://new.com",
	})
	if err != nil {
		t.Fatalf("WriteConfigFile() returned error: %v", err)
	}

	// Read back and verify update
	result, err := ReadConfigFile()
	if err != nil {
		t.Fatalf("ReadConfigFile() returned error: %v", err)
	}

	if result["LITELLM_URL"] != "http://new.com" {
		t.Errorf("LITELLM_URL = %q, want %q", result["LITELLM_URL"], "http://new.com")
	}
}

// --- Tests for NormalizeKey / ValidConfigKeys ---

func TestNormalizeKey_Valid(t *testing.T) {
	validKeys := []string{
		"LLM_MANAGER_VERBOSE",
		"LLM_MANAGER_DATA_DIR",
		"LLM_MANAGER_LOG_DIR",
		"LLM_MANAGER_DATABASE_URL",
		"LLM_MANAGER_LLM_DIR",
		"LLM_MANAGER_INSTALL_DIR",
		"LLM_MANAGER_HF_CACHE_DIR",
		"LITELLM_URL",
		"LLM_MANAGER_CONFIG",
		"HF_TOKEN",
	}

	for _, key := range validKeys {
		normalized, err := NormalizeKey(key)
		if err != nil {
			t.Errorf("NormalizeKey(%q) returned error: %v", key, err)
		}
		if normalized != key {
			t.Errorf("NormalizeKey(%q) = %q, want %q", key, normalized, key)
		}
	}
}

func TestNormalizeKey_Invalid(t *testing.T) {
	_, err := NormalizeKey("INVALID_KEY")
	if err == nil {
		t.Fatal("NormalizeKey(INVALID_KEY) expected error, got nil")
	}

	if !contains(err.Error(), "unknown config key") {
		t.Errorf("Error should mention unknown key: %v", err)
	}

	if !contains(err.Error(), "LITELLM_URL") {
		t.Errorf("Error should list valid keys: %v", err)
	}
}

func TestValidConfigKeys(t *testing.T) {
	keys := ValidConfigKeys()

	if len(keys) == 0 {
		t.Fatal("ValidConfigKeys() returned empty slice")
	}

	// Check it's sorted
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			t.Errorf("ValidConfigKeys() not sorted: %q > %q", keys[i-1], keys[i])
		}
	}

	// Check known keys are present
	hasLitellm := false
	for _, k := range keys {
		if k == "LITELLM_URL" {
			hasLitellm = true
			break
		}
	}
	if !hasLitellm {
		t.Error("ValidConfigKeys() missing LITELLM_URL")
	}
}

// --- Tests for 3-layer loading priority ---

func TestLoadConfig_ConfigFileOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	customDataDir := filepath.Join(tmpDir, "custom-data")

	// Write a config file that overrides default data dir
	err := os.WriteFile(configPath, []byte(fmt.Sprintf("LLM_MANAGER_DATA_DIR: %q\n", customDataDir)), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	// Clear any env var override
	t.Setenv("LLM_MANAGER_DATA_DIR", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.DataDir != customDataDir {
		t.Errorf("LoadConfig().DataDir = %q, want %q (from config file)", cfg.DataDir, customDataDir)
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configDataDir := filepath.Join(tmpDir, "config-data")
	envDataDir := filepath.Join(tmpDir, "env-data")

	// Write a config file
	err := os.WriteFile(configPath, []byte(fmt.Sprintf("LLM_MANAGER_DATA_DIR: %q\n", configDataDir)), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	// Set env var that should override config file
	t.Setenv("LLM_MANAGER_DATA_DIR", envDataDir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.DataDir != envDataDir {
		t.Errorf("LoadConfig().DataDir = %q, want %q (env var wins)", cfg.DataDir, envDataDir)
	}
}

func TestLoadConfig_DefaultsWhenNoFileOrEnv(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("LLM_MANAGER_DATA_DIR", "")
	t.Setenv("LITELLM_URL", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	// Should use defaults
	expectedDataDir := filepath.Join(cfg.HomeDir, ".local", "share", "llm-manager")
	if cfg.DataDir != expectedDataDir {
		t.Errorf("LoadConfig().DataDir = %q, want %q (default)", cfg.DataDir, expectedDataDir)
	}
}

func TestLoadConfig_LiteLLMURLFromConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a config file with LiteLLM URL
	err := os.WriteFile(configPath, []byte(`LITELLM_URL: "http://litellm:4000"
`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("LITELLM_URL", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.LiteLLMURL != "http://litellm:4000" {
		t.Errorf("LoadConfig().LiteLLMURL = %q, want %q (from config file)", cfg.LiteLLMURL, "http://litellm:4000")
	}
}

func TestLoadConfig_LiteLLMURLEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a config file
	err := os.WriteFile(configPath, []byte(`LITELLM_URL: "http://litellm:4000"
`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("LITELLM_URL", "http://env-litellm:4000")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.LiteLLMURL != "http://env-litellm:4000" {
		t.Errorf("LoadConfig().LiteLLMURL = %q, want %q (env var wins)", cfg.LiteLLMURL, "http://env-litellm:4000")
	}
}

func TestDefaultValues(t *testing.T) {
	defaults := DefaultValues()

	if len(defaults) == 0 {
		t.Fatal("DefaultValues() returned empty map")
	}

	// Check a few known defaults
	if defaults["LLM_MANAGER_LLM_DIR"] != "/opt/ai-server/llm-compose" {
		t.Errorf("DefaultValues()[LLM_MANAGER_LLM_DIR] = %q, want %q", defaults["LLM_MANAGER_LLM_DIR"], "/opt/ai-server/llm-compose")
	}

	if defaults["LLM_MANAGER_INSTALL_DIR"] != "/opt/ai-server" {
		t.Errorf("DefaultValues()[LLM_MANAGER_INSTALL_DIR] = %q, want %q", defaults["LLM_MANAGER_INSTALL_DIR"], "/opt/ai-server")
	}
}

func TestConfigFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "custom-config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", customPath)

	path := ConfigFilePath()
	if path != customPath {
		t.Errorf("ConfigFilePath() = %q, want %q", path, customPath)
	}
}

// --- Tests for HF_TOKEN loading priority ---

func TestLoadConfig_HFTOKENFromConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a config file with HF_TOKEN
	err := os.WriteFile(configPath, []byte(`HF_TOKEN: "hf_config_file_token_12345"
`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.HfToken != "hf_config_file_token_12345" {
		t.Errorf("LoadConfig().HfToken = %q, want %q (from config file)", cfg.HfToken, "hf_config_file_token_12345")
	}
}

func TestLoadConfig_HF_TOKEEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a config file
	err := os.WriteFile(configPath, []byte(`HF_TOKEN: "hf_config_file_token_12345"
`), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("HF_TOKEN", "hf_env_token_override")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.HfToken != "hf_env_token_override" {
		t.Errorf("LoadConfig().HfToken = %q, want %q (HF_TOKEN env var wins over config file)", cfg.HfToken, "hf_env_token_override")
	}
}

func TestLoadConfig_HUGGING_FACE_HUB_TOKENFallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "hf_fallback_hub_token")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg.HfToken != "hf_fallback_hub_token" {
		t.Errorf("LoadConfig().HfToken = %q, want %q (HUGGING_FACE_HUB_TOKEN fallback)", cfg.HfToken, "hf_fallback_hub_token")
	}
}

func TestMaskAPIKey_HF_TOKEN(t *testing.T) {
	hfToken := DefaultValues()["HF_TOKEN"]
	masked := maskAPIKey(hfToken)

	if hfToken == "" && masked != "****" {
		t.Errorf("maskAPIKey(empty) = %q, want %q", masked, "****")
	}

	longToken := "hf_test_token_that_is_long_enough_for_masking"
	maskedLong := maskAPIKey(longToken)
	expectedPrefix := longToken[:8]
	expectedSuffix := longToken[len(longToken)-4:]
	expected := expectedPrefix + "..." + expectedSuffix
	if maskedLong != expected {
		t.Errorf("maskAPIKey(%q) = %q, want %q", longToken, maskedLong, expected)
	}
}


