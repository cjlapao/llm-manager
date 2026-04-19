// Package cmd provides the command structure for the CLI application.
package cmd

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/version"
)

// RootCommand represents the root command for the application.
type RootCommand struct {
	cfg *config.Config
	db  database.DatabaseManager
}

// NewRootCommand creates a new RootCommand.
func NewRootCommand() *RootCommand {
	return &RootCommand{}
}

// Run executes the root command with the given arguments.
func (c *RootCommand) Run(args []string) int {
	c.cfg = mustLoadConfig()

	// Open database connection for all commands that need it
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

	if len(args) < 1 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.PrintHelp()
		return 0
	case "-v", "--version", "version":
		fmt.Print(version.Info())
		return 0
	case "config":
		return c.runConfig()
	case "migrate":
		return c.runMigrate()
	case "model":
		return NewModelCommand(c).Run(args[1:])
	case "container":
		return NewContainerCommand(c).Run(args[1:])
	case "service":
		return NewServiceCommand(c).Run(args[1:])
	case "hotspot":
		return NewHotspotCommand(c).Run(args[1:])
	case "logs":
		return NewLogsCommand(c).Run(args[1:])
	case "update":
		return NewUpdateCommand(c.cfg, c.db).Run(args[1:])
	case "mem":
		return NewMemCommand(c.cfg).Run(args[1:])
	case "comfyui":
		return NewComfyUICommand(c).Run(args[1:])
	case "embed":
		return NewEmbedCommand(c).Run(args[1:])
	case "rerank":
		return NewRerankCommand(c).Run(args[1:])
	case "rag":
		return NewRagCommand(c).Run(args[1:])
	case "speech":
		return NewSpeechCommand(c).Run(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runConfig prints the current configuration.
func (c *RootCommand) runConfig() int {
	fmt.Print(c.cfg.String())
	return 0
}

// runMigrate imports models from models.json into the database.
func (c *RootCommand) runMigrate() int {
	if err := c.db.AutoMigrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running migrations: %v\n", err)
		return 1
	}

	modelsPath := "models.json"
	count, err := c.db.MigrateFromJSON(modelsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error migrating models: %v\n", err)
		return 1
	}
	if count == 0 {
		fmt.Println("Database already populated, skipping migration")
	} else {
		fmt.Printf("Migrated %d models from models.json\n", count)
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
  config      Show current configuration
  migrate     Import models from models.json
  model       Manage LLM models (list, get, create, update, delete, import, export, compose)
  container   Manage Docker containers (list, start, stop, restart, swap, logs)
  service     Manage LLM services (high-level orchestration)
  hotspot     Manage the most recently used model
  logs        View container logs for a model
  update      Check for and install updates
  mem         Show system memory and disk usage
  comfyui     Manage ComfyUI (start, stop)
  embed       Manage embed container (start, stop)
  rerank      Manage rerank container (start, stop)
  rag         Manage RAG services - embed + rerank (start, stop)
  speech      Manage speech services - whisper + kokoro (start, stop)

OPTIONS:
  -h, --help      Show this help message
  -v, --version   Show version information

ENVIRONMENT VARIABLES:
  LLM_MANAGER_VERBOSE       Set to "true" or "1" to enable verbose output
  LLM_MANAGER_CONFIG        Path to configuration file
  LLM_MANAGER_DATA_DIR      Path to data directory
  LLM_MANAGER_LOG_DIR       Path to log directory
  LLM_MANAGER_DATABASE_URL  Path to SQLite database file

EXAMPLES:
  llm-manager version
  llm-manager config
  llm-manager migrate
  llm-manager model list
  llm-manager model compose qwen3_6
  llm-manager service start qwen3_6
  llm-manager container swap qwen3_6
  llm-manager hotspot restart
  llm-manager comfyui start
  llm-manager rag start
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
	"embed":      "llm-embed",
	"rerank":     "llm-rerank",
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
