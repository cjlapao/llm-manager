// Package cmd provides the command structure for the CLI application.
package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/user/llm-manager/internal/api"
	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
	"github.com/user/llm-manager/internal/version"
)

// RootCommand represents the root command for the application.
type RootCommand struct {
	cfg     *config.Config
	db      database.DatabaseManager
	apiPort int
	apiHost string
}

// NewRootCommand creates a new RootCommand.
func NewRootCommand() *RootCommand {
	return &RootCommand{}
}

// ParseGlobalFlags parses global flags that appear before any subcommand.
// Supported flags: --api-port, --api-host
// Environment variables (used when flags are not provided):
//   LLM_MANAGER_API_PORT  (default: 8780)
//   LLM_MANAGER_API_HOST  (default: 0.0.0.0)
func (c *RootCommand) ParseGlobalFlags(args []string) []string {
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--api-port":
			if i+1 < len(args) {
				port, err := strconv.Atoi(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: --api-port requires a numeric value, got %q\n", args[i+1])
					os.Exit(1)
				}
				c.apiPort = port
				i++ // skip the value
			} else {
				fmt.Fprintln(os.Stderr, "Error: --api-port requires a value")
				os.Exit(1)
			}
		case "--api-host":
			if i+1 < len(args) {
				c.apiHost = args[i+1]
				i++ // skip the value
			} else {
				fmt.Fprintln(os.Stderr, "Error: --api-host requires a value")
				os.Exit(1)
			}
		default:
			remaining = append(remaining, arg)
		}
	}

	// Apply defaults if flags were not set, falling back to environment variables.
	if c.apiPort <= 0 {
		if val := os.Getenv("LLM_MANAGER_API_PORT"); val != "" {
			if port, err := strconv.Atoi(val); err == nil {
				c.apiPort = port
			}
		}
		if c.apiPort <= 0 {
			c.apiPort = 8780
		}
	}
	if c.apiHost == "" {
		c.apiHost = os.Getenv("LLM_MANAGER_API_HOST")
	}
	if c.apiHost == "" {
		c.apiHost = "0.0.0.0"
	}

	return remaining
}

// Run executes the root command with the given arguments.
func (c *RootCommand) Run(args []string) int {
	c.cfg = mustLoadConfig()

	// Configure GPU memory source before any service initializes.
	service.SetGPUMemorySource(c.cfg.GPUMemorySource)

	// Parse global flags (--api-port, --api-host) before anything else
	apiArgs := c.ParseGlobalFlags(args)

	// Check if API mode is requested
	if c.apiPort > 0 {

		// Open database connection for the API server
		db, err := database.NewDatabaseManager(c.cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
			return 1
		}
		if err := db.Open(); err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			return 1
		}
		defer db.Close()

		// Auto-apply pending migrations
		if err := db.ApplyPendingMigrations(); err != nil {
			fmt.Fprintf(os.Stderr, "Error applying pending migrations: %v\n", err)
			return 1
		}

		// Merge database config into the config struct
		c.cfg.MergeFromDB(db)
		c.db = db

		// Create API context
		apiCtx := api.NewAPIContext(db, c.cfg)

		// Start the API server (this blocks until shutdown)
		if err := api.StartAPIServer(apiCtx, c.apiHost, c.apiPort, 0); err != nil {
			fmt.Fprintf(os.Stderr, "API server error: %v\n", err)
			return 1
		}
		return 0
	}

	// CLI mode: open database, apply migrations, dispatch commands
	db, err := database.NewDatabaseManager(c.cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		return 1
	}
	if err := db.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		return 1
	}
	c.db = db
	defer db.Close()

	// Auto-apply pending migrations on every command (except version/help which don't need DB data)
	if len(apiArgs) > 0 && apiArgs[0] != "-h" && apiArgs[0] != "--help" && apiArgs[0] != "help" && apiArgs[0] != "-v" && apiArgs[0] != "--version" && apiArgs[0] != "version" && apiArgs[0] != "migrate" {
		if err := c.db.ApplyPendingMigrations(); err != nil {
			fmt.Fprintf(os.Stderr, "Error applying pending migrations: %v\n", err)
			return 1
		}
	}

	// Merge database config into the config struct (env/file take priority)
	c.cfg.MergeFromDB(c.db)

	if len(apiArgs) < 1 {
		c.PrintHelp()
		return 0
	}

	// Handle built-in commands (no dispatch needed)
	switch apiArgs[0] {
	case "-h", "--help", "help":
		c.PrintHelp()
		return 0
	case "-v", "--version", "version":
		fmt.Print(version.Info())
		return 0
	case "config":
		return NewConfigCommand(c.cfg, c.db).Run(apiArgs[1:])
	case "migrate":
		return c.runMigrate()
	}

	// Dispatch to registered commands
	dispatcher := NewCommandDispatcher(c.cfg, c.db)
	exitCode := dispatcher.Dispatch(apiArgs[0], apiArgs[1:])
	if exitCode == 127 {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", apiArgs[0])
		c.PrintHelp()
		return 1
	}
	return exitCode
}

// runConfig prints the current configuration.
func (c *RootCommand) runConfig() int {
	fmt.Print(c.cfg.String())
	return 0
}

// runMigrate updates the database schema to match the current code.
func (c *RootCommand) runMigrate() int {
	if err := c.db.ApplyPendingMigrations(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running migrations: %v\n", err)
		return 1
	}
	version, _ := c.db.SchemaVersion()
	latest, _ := c.db.LatestVersion()
	if version >= latest {
		fmt.Println("Database schema is up to date.")
	} else {
		fmt.Printf("Applied migrations. Current schema version: %d/%d\n", version, latest)
	}
	return 0
}

// PrintHelp prints the help message.
func (c *RootCommand) PrintHelp() {
	fmt.Println(`llm-manager - A CLI tool for managing LLM resources.

USAGE:
  llm-manager [COMMAND] [ARGS]

COMMANDS:
  help        Show this help message
  version     Show version information
  config      Show or manage persistent configuration (list, get, set, unset, edit)
  migrate     Update database schema to match latest code
  model       Manage LLM models (list, get, create, update, delete, import, export, compose)
  import      Import a model or engine from a YAML file (auto-detects type)
  llm         Manage LLM model containers (start, stop, restart, swap, status, logs)
  container   Low-level Docker container operations (list, logs, status refresh)
  service     Manage LLM services (high-level orchestration)
  logs        View container logs for a model
  update      Check for and install updates
  mem         Show system memory and disk usage
  uninstall   Uninstall a model (stop container, delete YAML, clear cache, remove from LiteLLM and DB)
  comfyui     Manage ComfyUI and image generation models (start, stop, flux, 3d, status)
  speech      Manage speech services - whisper + kokoro (start, stop)

GLOBAL OPTIONS:
  --api-port <port>     Start API server on the given port (default: 8780)
  --api-host <host>     Host to bind the API server to (default: 0.0.0.0)

ENVIRONMENT VARIABLES:
  LLM_MANAGER_VERBOSE       Set to "true" or "1" to enable verbose output
  LLM_MANAGER_CONFIG        Path to configuration file
  LLM_MANAGER_DATA_DIR      Path to data directory
  LLM_MANAGER_LOG_DIR       Path to log directory
  LLM_MANAGER_DATABASE_URL  Path to SQLite database file
  LLM_MANAGER_API_PORT      Port for the API server (overrides --api-port default)
  LLM_MANAGER_API_HOST      Host for the API server (overrides --api-host default)

EXAMPLES:
  llm-manager version
  llm-manager config
  llm-manager migrate
  llm-manager model list
  llm-manager model compose qwen3_6
  llm-manager llm start qwen3_6
  llm-manager llm start latest
  llm-manager llm swap qwen3_6
  llm-manager comfyui start
  llm-manager comfyui flux start flux-schnell
  llm-manager speech stop
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

// ServiceAliases maps service name aliases to Docker container names.
var ServiceAliases = map[string]string{
	"comfyui":    "comfyui-flux",
	"flux":       "comfyui-flux",
	"whisper":    "whisper-stt",
	"kokoro":     "kokoro-tts",
	"litellm":    "litellm",
	"swap-api":   "swap-api",
	"swapapi":    "swap-api",
	"open-webui": "open-webui",
	"webui":      "open-webui",
	"mcp":        "mcpo",
}

// ResolveServiceAlias resolves a service alias to a Docker container name.
// Returns empty string if not found.
func ResolveServiceAlias(alias string) string {
	if name, ok := ServiceAliases[strings.ToLower(alias)]; ok {
		return name
	}
	return ""
}

// KnownServiceAliases returns a sorted list of known service aliases.
func KnownServiceAliases() []string {
	aliases := make([]string, 0, len(ServiceAliases))
	for alias := range ServiceAliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
