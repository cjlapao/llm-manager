// Package cmd provides the command structure for the CLI application.
package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/version"
)

// RootCommand represents the root command for the application.
type RootCommand struct {
	cfg *config.Config
}

// NewRootCommand creates a new RootCommand.
func NewRootCommand() *RootCommand {
	return &RootCommand{}
}

// Run executes the root command with the given arguments.
func (c *RootCommand) Run(args []string) int {
	c.cfg = mustLoadConfig()

	if len(args) < 1 {
		c.printHelp()
		return 0
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printHelp()
		return 0
	case "-v", "--version", "version":
		fmt.Print(version.Info())
		return 0
	case "config":
		return c.runConfig()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		c.printHelp()
		return 1
	}
}

// runConfig prints the current configuration.
func (c *RootCommand) runConfig() int {
	fmt.Print(c.cfg.String())
	return 0
}

// printHelp prints the help message.
func (c *RootCommand) printHelp() {
	fmt.Println(`llm-manager - A CLI tool for managing LLM resources.

USAGE:
  llm-manager [COMMAND] [ARGS]

COMMANDS:
  help        Show this help message
  version     Show version information
  config      Show current configuration

OPTIONS:
  -h, --help      Show this help message
  -v, --version   Show version information

ENVIRONMENT VARIABLES:
  LLM_MANAGER_VERBOSE   Set to "true" or "1" to enable verbose output
  LLM_MANAGER_CONFIG    Path to configuration file
  LLM_MANAGER_DATA_DIR  Path to data directory
  LLM_MANAGER_LOG_DIR   Path to log directory

EXAMPLES:
  llm-manager version
  llm-manager config
  LLM_MANAGER_VERBOSE=true llm-manager

For more information, visit: https://github.com/user/llm-manager`)
}

// mustLoadConfig loads the configuration or exits with an error.
func mustLoadConfig() *config.Config {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

// PrintVersion prints the version information.
func PrintVersion() {
	fmt.Print(version.Info())
}

// PrintShortVersion prints a short version string.
func PrintShortVersion() {
	fmt.Println(version.ShortVersion())
}

// PlatformInfo returns platform information as a string.
func PlatformInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
