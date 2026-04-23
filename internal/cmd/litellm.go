// Package cmd provides the litellm subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("litellm", func(root *RootCommand) Command { return NewLiteLLMCommand(root) })
}

// LiteLLMCommand handles LiteLLM proxy management operations.
type LiteLLMCommand struct {
	cfg       *config.Config
	db        database.DatabaseManager
	configSvc *service.ConfigService
	svc       *service.LiteLLMService
}

// NewLiteLLMCommand creates a new LiteLLMCommand.
func NewLiteLLMCommand(root *RootCommand) *LiteLLMCommand {
	configSvc := service.NewConfigService(root.db)
	return &LiteLLMCommand{
		cfg:       root.cfg,
		db:        root.db,
		configSvc: configSvc,
		svc:       service.NewLiteLLMService(root.db, root.cfg, configSvc),
	}
}

// Run executes the litellm command with the given subcommand and arguments.
func (c *LiteLLMCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "list", "ls":
		return c.runList()
	case "get":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'get' requires a model slug\n")
			return 1
		}
		return c.runGet(args[1])
	case "add":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'add' requires a model slug\n")
			return 1
		}
		return c.runAdd(args[1])
	case "update":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'update' requires a model slug\n")
			return 1
		}
		return c.runUpdate(args[1])
	case "delete", "del":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'delete' requires a model slug\n")
			return 1
		}
		return c.runDelete(args[1])
	case "sync":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'sync' requires a model slug\n")
			return 1
		}
		return c.runSync(args[1])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown litellm subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runList lists all models from LiteLLM.
func (c *LiteLLMCommand) runList() int {
	if c.cfg.LiteLLMURL == "" {
		fmt.Fprintf(os.Stderr, "Error: LiteLLM URL not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	models, err := c.svc.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No models found in LiteLLM.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tOBJECT\tOWNED_BY")
	fmt.Fprintln(w, "--\t------\n--------")

	for _, m := range models {
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.ID, m.Object, m.OwnedBy)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d models\n", len(models))
	return 0
}

// runGet retrieves detailed information for a model from LiteLLM.
func (c *LiteLLMCommand) runGet(slug string) int {
	if c.cfg.LiteLLMURL == "" {
		fmt.Fprintf(os.Stderr, "Error: LiteLLM URL not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	info, err := c.svc.GetModelInfo(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model info: %v\n", err)
		return 1
	}

	fmt.Printf("Model: %s\n", info.ModelName)
	fmt.Println(strings.Repeat("-", 60))

	// Display litellm_params
	if len(info.LiteLLMParams) > 0 {
		fmt.Println("\nlitellm_params:")
		printNestedMap(info.LiteLLMParams, "  ")
	}

	// Display model_info
	if len(info.ModelInfo) > 0 {
		fmt.Println("\nmodel_info:")
		printNestedMap(info.ModelInfo, "  ")
	}

	fmt.Println()
	return 0
}

// runAdd adds a model from the database to LiteLLM.
func (c *LiteLLMCommand) runAdd(slug string) int {
	if c.cfg.LiteLLMURL == "" {
		fmt.Fprintf(os.Stderr, "Error: LiteLLM URL not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	if err := c.svc.AddModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding model to LiteLLM: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Model %s added to LiteLLM\n", slug)
	return 0
}

// runUpdate updates a model in LiteLLM with fields from the database.
func (c *LiteLLMCommand) runUpdate(slug string) int {
	if c.cfg.LiteLLMURL == "" {
		fmt.Fprintf(os.Stderr, "Error: LiteLLM URL not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	if err := c.svc.UpdateModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating model in LiteLLM: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Model %s updated in LiteLLM\n", slug)
	return 0
}

// runDelete removes a model from LiteLLM.
func (c *LiteLLMCommand) runDelete(slug string) int {
	if c.cfg.LiteLLMURL == "" {
		fmt.Fprintf(os.Stderr, "Error: LiteLLM URL not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	if err := c.svc.DeleteModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting model from LiteLLM: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Model %s deleted from LiteLLM\n", slug)
	return 0
}

// runSync adds or updates a model in LiteLLM based on the database record.
func (c *LiteLLMCommand) runSync(slug string) int {
	if c.cfg.LiteLLMURL == "" {
		fmt.Fprintf(os.Stderr, "Error: LiteLLM URL not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	if err := c.svc.SyncModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing model to LiteLLM: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Model %s synced to LiteLLM\n", slug)
	return 0
}

// PrintHelp prints the litellm command help.
func (c *LiteLLMCommand) PrintHelp() {
	fmt.Println(`litellm - Manage models in the LiteLLM proxy.

USAGE:
  llm-manager litellm [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list, ls         List all models in LiteLLM
  get <slug>       Get detailed info for a model in LiteLLM
  add <slug>       Add a model from the database to LiteLLM
  update <slug>    Update a model in LiteLLM with database values
  delete, del <slug>  Delete a model from LiteLLM
  sync <slug>      Add or update a model in LiteLLM (auto-detect)

CONFIGURATION:
  Requires LITELLM_URL and LITELLM_API_KEY to be set.

EXAMPLES:
  llm-manager litellm list
  llm-manager litellm get qwen3_6
  llm-manager litellm add qwen3_6
  llm-manager litellm update qwen3_6
  llm-manager litellm delete qwen3_6
  llm-manager litellm sync qwen3_6`)
}
