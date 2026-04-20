// Package config provides configuration management for the application.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	// Verbose enables verbose output.
	Verbose bool
	// ConfigFile is the path to the configuration file.
	ConfigFile string
	// HomeDir is the user's home directory.
	HomeDir string
	// DataDir is the directory for application data.
	DataDir string
	// LogDir is the directory for log files.
	LogDir string
	// DatabaseURL is the path to the SQLite database file.
	DatabaseURL string
	// LLMDir is the directory containing docker-compose YAML files for LLM models.
	LLMDir string
	// InstallDir is the base installation directory for AI server.
	InstallDir string
	// HFCacheDir is the HuggingFace model cache directory.
	HFCacheDir string
	// LiteLLMURL is the base URL of the LiteLLM proxy for constructing api_base.
	// Format: http://host:port or http://host
	LiteLLMURL string
}

// validConfigKeys defines the set of supported config keys and their defaults.
var validConfigKeys = map[string]string{
	"LLM_MANAGER_VERBOSE":      "",
	"LLM_MANAGER_DATA_DIR":     "",
	"LLM_MANAGER_LOG_DIR":      "",
	"LLM_MANAGER_DATABASE_URL": "",
	"LLM_MANAGER_LLM_DIR":      "",
	"LLM_MANAGER_INSTALL_DIR":  "",
	"LLM_MANAGER_HF_CACHE_DIR": "",
	"LLM_MANAGER_LITELLM_URL":  "",
	"LLM_MANAGER_CONFIG":       "",
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return &Config{
		Verbose:     false,
		HomeDir:     homeDir,
		DataDir:     filepath.Join(homeDir, ".local", "share", "llm-manager"),
		LogDir:      filepath.Join(homeDir, ".local", "log", "llm-manager"),
		DatabaseURL: filepath.Join(homeDir, ".local", "share", "llm-manager", "llm-manager.db"),
		LLMDir:      "/opt/ai-server/llm-compose",
		InstallDir:  "/opt/ai-server",
		HFCacheDir:  "/opt/ai-server/models",
		LiteLLMURL:  "",
	}
}

// DefaultValues returns a map of config key to default string value.
func DefaultValues() map[string]string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return map[string]string{
		"LLM_MANAGER_VERBOSE":      "",
		"LLM_MANAGER_DATA_DIR":     filepath.Join(homeDir, ".local", "share", "llm-manager"),
		"LLM_MANAGER_LOG_DIR":      filepath.Join(homeDir, ".local", "log", "llm-manager"),
		"LLM_MANAGER_DATABASE_URL": filepath.Join(homeDir, ".local", "share", "llm-manager", "llm-manager.db"),
		"LLM_MANAGER_LLM_DIR":      "/opt/ai-server/llm-compose",
		"LLM_MANAGER_INSTALL_DIR":  "/opt/ai-server",
		"LLM_MANAGER_HF_CACHE_DIR": "/opt/ai-server/models",
		"LLM_MANAGER_LITELLM_URL":  "",
		"LLM_MANAGER_CONFIG":       "",
	}
}

// ConfigFilePath returns the path to the config file.
// If LLM_MANAGER_CONFIG is set, that path is returned directly.
// Otherwise, returns ~/.config/llm-manager/config.yaml.
func ConfigFilePath() string {
	if val := os.Getenv("LLM_MANAGER_CONFIG"); val != "" {
		return val
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".config/llm-manager/config.yaml"
	}
	return filepath.Join(homeDir, ".config", "llm-manager", "config.yaml")
}

// ValidConfigKeys returns the list of valid config key names, sorted.
func ValidConfigKeys() []string {
	keys := make([]string, 0, len(validConfigKeys))
	for k := range validConfigKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// NormalizeKey validates a key name and returns it if it's a known config key.
// Returns an error if the key is not recognized.
func NormalizeKey(key string) (string, error) {
	if _, ok := validConfigKeys[key]; ok {
		return key, nil
	}
	available := strings.Join(ValidConfigKeys(), ", ")
	return "", fmt.Errorf("unknown config key %q: valid keys are %s", key, available)
}

// ReadConfigFile reads the config file and returns a map of key->value.
// Returns an empty map if the file doesn't exist.
func ReadConfigFile() (map[string]string, error) {
	path := ConfigFilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if result == nil {
		result = make(map[string]string)
	}

	// Validate all keys in the file are known
	for k := range result {
		if _, ok := validConfigKeys[k]; !ok {
			return nil, fmt.Errorf("unknown config key %q in config file", k)
		}
	}

	return result, nil
}

// WriteConfigFile writes the config file from a map of key->value.
// Empty values are filtered out before writing.
func WriteConfigFile(values map[string]string) error {
	path := ConfigFilePath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Filter out empty values
	filtered := make(map[string]string)
	for k, v := range values {
		if v != "" {
			filtered[k] = v
		}
	}

	data, err := yaml.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// LoadConfig reads configuration from the environment and config file.
// Loading priority (highest to lowest): env vars > config file > defaults.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// Layer 1: Start with defaults
	// Layer 2: Override with config file values
	configValues, err := ReadConfigFile()
	if err != nil {
		// Log warning but don't fail — config file is optional
		fmt.Fprintf(os.Stderr, "Warning: could not read config file: %v\n", err)
	}

	if val, ok := configValues["LLM_MANAGER_VERBOSE"]; ok && (val == "true" || val == "1") {
		cfg.Verbose = true
	}

	if val, ok := configValues["LLM_MANAGER_DATA_DIR"]; ok {
		cfg.DataDir = val
	}

	if val, ok := configValues["LLM_MANAGER_LOG_DIR"]; ok {
		cfg.LogDir = val
	}

	if val, ok := configValues["LLM_MANAGER_DATABASE_URL"]; ok {
		cfg.DatabaseURL = val
	}

	if val, ok := configValues["LLM_MANAGER_LLM_DIR"]; ok {
		cfg.LLMDir = val
	}

	if val, ok := configValues["LLM_MANAGER_INSTALL_DIR"]; ok {
		cfg.InstallDir = val
	}

	if val, ok := configValues["LLM_MANAGER_HF_CACHE_DIR"]; ok {
		cfg.HFCacheDir = val
	}

	if val, ok := configValues["LLM_MANAGER_LITELLM_URL"]; ok {
		cfg.LiteLLMURL = val
	}

	// Layer 3: Override with environment variables (always wins)
	if val := os.Getenv("LLM_MANAGER_VERBOSE"); val == "true" || val == "1" {
		cfg.Verbose = true
	}

	if val := os.Getenv("LLM_MANAGER_CONFIG"); val != "" {
		cfg.ConfigFile = val
	}

	if val := os.Getenv("LLM_MANAGER_DATA_DIR"); val != "" {
		cfg.DataDir = val
	}

	if val := os.Getenv("LLM_MANAGER_LOG_DIR"); val != "" {
		cfg.LogDir = val
	}

	if val := os.Getenv("LLM_MANAGER_DATABASE_URL"); val != "" {
		cfg.DatabaseURL = val
	}

	if val := os.Getenv("LLM_MANAGER_LLM_DIR"); val != "" {
		cfg.LLMDir = val
	}

	if val := os.Getenv("LLM_MANAGER_INSTALL_DIR"); val != "" {
		cfg.InstallDir = val
	}

	if val := os.Getenv("LLM_MANAGER_HF_CACHE_DIR"); val != "" {
		cfg.HFCacheDir = val
	}

	if val := os.Getenv("LLM_MANAGER_LITELLM_URL"); val != "" {
		cfg.LiteLLMURL = val
	}

	// Ensure directories exist
	if err := ensureDir(cfg.DataDir); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	if err := ensureDir(cfg.LogDir); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	return cfg, nil
}

// ConfigDir returns the path to the application's configuration directory.
func ConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".config/llm-manager"
	}
	return filepath.Join(homeDir, ".config", "llm-manager")
}

// DataDir returns the path to the application's data directory.
func DataDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".local/share/llm-manager"
	}
	return filepath.Join(homeDir, ".local", "share", "llm-manager")
}

// ensureDir creates a directory if it does not exist.
func ensureDir(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	if path == "." {
		return nil
	}

	// Check if directory already exists
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	// Create the directory
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}

	return nil
}

// String returns a string representation of the configuration.
func (c *Config) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "llm-manager config:\n")
	fmt.Fprintf(&b, "  verbose:     %v\n", c.Verbose)
	fmt.Fprintf(&b, "  config file: %s\n", c.ConfigFile)
	fmt.Fprintf(&b, "  home dir:    %s\n", c.HomeDir)
	fmt.Fprintf(&b, "  data dir:    %s\n", c.DataDir)
	fmt.Fprintf(&b, "  log dir:     %s\n", c.LogDir)
	fmt.Fprintf(&b, "  database:    %s\n", c.DatabaseURL)
	fmt.Fprintf(&b, "  llm dir:     %s\n", c.LLMDir)
	fmt.Fprintf(&b, "  install dir: %s\n", c.InstallDir)
	fmt.Fprintf(&b, "  hf cache:    %s\n", c.HFCacheDir)
	fmt.Fprintf(&b, "  litellm url: %s\n", c.LiteLLMURL)
	return b.String()
}
