// Package cmd provides the command structure for the CLI application.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
)

// ConfigCommand implements the config subcommands: list, get, set, unset, edit.
type ConfigCommand struct {
	cfg *config.Config
	db  database.DatabaseManager
	svc *service.ConfigService
}

// NewConfigCommand creates a new ConfigCommand.
func NewConfigCommand(cfg *config.Config, db database.DatabaseManager) *ConfigCommand {
	return &ConfigCommand{
		cfg: cfg,
		db:  db,
		svc: service.NewConfigService(db),
	}
}

// Run executes the config subcommand with the given arguments.
func (c *ConfigCommand) Run(args []string) int {
	if len(args) == 0 {
		// No subcommand: show full config (backward compatible)
		fmt.Print(c.cfg.String())
		return 0
	}

	switch args[0] {
	case "list":
		return c.runList()
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: config get requires a key argument")
			fmt.Fprintln(os.Stderr)
			c.PrintHelp()
			return 1
		}
		return c.runGet(args[1])
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: config set requires a key and value")
			fmt.Fprintln(os.Stderr)
			c.PrintHelp()
			return 1
		}
		return c.runSet(args[1], args[2])
	case "unset":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: config unset requires a key argument")
			fmt.Fprintln(os.Stderr)
			c.PrintHelp()
			return 1
		}
		return c.runUnset(args[1])
	case "edit":
		return c.runEdit()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown config subcommand %q\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runList shows all config values with their source.
func (c *ConfigCommand) runList() int {
	defaults := config.DefaultValues()
	fileValues, err := config.ReadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read config file: %v\n", err)
		fileValues = make(map[string]string)
	}

	dbConfigs, err := c.svc.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read database config: %v\n", err)
	}

	// Build a map of DB values for quick lookup
	dbValues := make(map[string]string)
	for _, cfg := range dbConfigs {
		dbValues[cfg.Key] = cfg.Value
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tVALUE\tSOURCE")

	for _, key := range config.ValidConfigKeys() {
		source := "default"
		value := defaults[key]

		// Check in priority order: env > file > DB > default
		if envVal := os.Getenv(key); envVal != "" {
			source = "environment"
			value = envVal
		} else if fileVal, ok := fileValues[key]; ok {
			source = "config file"
			value = fileVal
		} else if dbVal, ok := dbValues[key]; ok {
			source = "DB"
			value = dbVal
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", key, value, source)
	}
	w.Flush()

	return 0
}

// runGet shows the effective value for a single key.
func (c *ConfigCommand) runGet(key string) int {
	if _, err := config.NormalizeKey(key); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Check env var first (highest priority)
	if envVal := os.Getenv(key); envVal != "" {
		fmt.Println(envVal)
		return 0
	}

	// Check config file
	fileValues, err := config.ReadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read config file: %v\n", err)
	} else if fileVal, ok := fileValues[key]; ok {
		fmt.Println(fileVal)
		return 0
	}

	// Check database
	dbConfig, err := c.svc.Get(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read database config: %v\n", err)
	} else if dbConfig != nil {
		fmt.Println(dbConfig.Value)
		return 0
	}

	// Fall back to default
	defaults := config.DefaultValues()
	fmt.Println(defaults[key])
	return 0
}

// runSet persists a key-value pair to the database.
func (c *ConfigCommand) runSet(key, value string) int {
	normalized, err := config.NormalizeKey(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	defaults := config.DefaultValues()

	// If value matches default, suggest unset
	if defaults[normalized] == value {
		fmt.Printf("Note: %s=%s matches the default value.\n", normalized, value)
		fmt.Printf("Consider running: llm-manager config unset %s\n", normalized)
		return 0
	}

	// Check if env var overrides — warn user
	if envVal := os.Getenv(normalized); envVal != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s is set in environment variable — DB value will not take effect until env var is removed\n", normalized)
	}

	// Check if config file overrides — warn user
	fileValues, err := config.ReadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read config file: %v\n", err)
	} else if fileVal, ok := fileValues[normalized]; ok && fileVal != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s is set in config file — DB value will not take effect until config file is updated\n", normalized)
	}

	// Store in database
	if err := c.svc.Set(normalized, value); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Set %s=%s in database\n", normalized, value)
	return 0
}

// runUnset removes a config key from the database.
func (c *ConfigCommand) runUnset(key string) int {
	normalized, err := config.NormalizeKey(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Check if env var is also set — warn user
	if envVal := os.Getenv(normalized); envVal != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s is set in environment variable — only DB value will be removed\n", normalized)
	}

	// Remove from database
	if err := c.svc.Unset(normalized); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Removed %s from database\n", normalized)
	return 0
}

// runEdit opens the config file in $EDITOR or vi if it exists.
// Otherwise, shows current DB values with a hint.
func (c *ConfigCommand) runEdit() int {
	path := config.ConfigFilePath()

	if _, err := os.Stat(path); err == nil {
		// Config file exists — edit it
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}

		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				// Editor exited with non-zero status (e.g., :q in vi)
				return 0
			}
			fmt.Fprintf(os.Stderr, "Error: could not run editor: %v\n", err)
			return 1
		}

		return 0
	}

	// No config file — show DB values
	dbConfigs, err := c.svc.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read database config: %v\n", err)
		return 1
	}

	fmt.Println("No config file found. Current database config values:")
	if len(dbConfigs) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, cfg := range dbConfigs {
			fmt.Printf("  %s = %s\n", cfg.Key, cfg.Value)
		}
	}
	fmt.Println("\nUse 'llm-manager config set KEY VALUE' to modify database config.")
	return 0
}

// PrintHelp prints the help message for the config subcommand.
func (c *ConfigCommand) PrintHelp() {
	fmt.Println(`config - Manage persistent configuration

USAGE:
  llm-manager config [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list      Show all config values with their source (default, DB, config file, or environment)
  get KEY   Show the effective value for a single key
  set KEY VALUE  Persist a key-value pair to the database
  unset KEY Remove a key from the database
  edit      Open the config file in $EDITOR (or vi), or show DB values

CONFIG FILE:
  Path: ~/.config/llm-manager/config.yaml
  Format: Flat YAML with full env var names as keys

LOADING PRIORITY (highest to lowest):
  1. Environment variables (always wins)
  2. Config file values
  3. Database values
  4. Default values

VALID KEYS:
  LLM_MANAGER_VERBOSE       bool ("true"/"false"/"1"/"0")
  LLM_MANAGER_DATA_DIR      string (path)
  LLM_MANAGER_LOG_DIR       string (path)
  LLM_MANAGER_DATABASE_URL  string (path)
  LLM_MANAGER_LLM_DIR       string (path)
  LLM_MANAGER_INSTALL_DIR   string (path)
  LLM_MANAGER_HF_CACHE_DIR  string (path)
  LLM_MANAGER_LITELLM_URL   string (URL)
  LLM_MANAGER_CONFIG        string (path)

EXAMPLES:
  llm-manager config                     # show full configuration
  llm-manager config list                # show all values with source
  llm-manager config get LLM_MANAGER_DATA_DIR
  llm-manager config set LLM_MANAGER_DATA_DIR /custom/data
  llm-manager config unset LLM_MANAGER_DATA_DIR
  llm-manager config edit
  EDITOR=nano llm-manager config edit`)
}
