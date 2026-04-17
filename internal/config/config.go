// Package config provides configuration management for the application.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return &Config{
		Verbose:  false,
		HomeDir:  homeDir,
		DataDir:  filepath.Join(homeDir, ".local", "share", "llm-manager"),
		LogDir:   filepath.Join(homeDir, ".local", "log", "llm-manager"),
	}
}

// LoadConfig reads configuration from the environment and config file.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// Override with environment variables
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
	fmt.Fprintf(&b, "  verbose:    %v\n", c.Verbose)
	fmt.Fprintf(&b, "  config file: %s\n", c.ConfigFile)
	fmt.Fprintf(&b, "  home dir:   %s\n", c.HomeDir)
	fmt.Fprintf(&b, "  data dir:   %s\n", c.DataDir)
	fmt.Fprintf(&b, "  log dir:    %s\n", c.LogDir)
	return b.String()
}
