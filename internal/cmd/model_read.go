// Package cmd provides the model subcommand for llm-manager.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"text/template"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("models", func(root *RootCommand) Command { return NewModelCommand(root) })
}

// ModelCommand handles model CRUD operations.
type ModelCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewModelCommand creates a new ModelCommand.
func NewModelCommand(root *RootCommand) *ModelCommand {
	return &ModelCommand{
		cfg: root,
		svc: service.NewModelService(root.db, root.cfg),
	}
}

// Run executes the model command with the given subcommand and arguments.
func (c *ModelCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "ls", "list":
		return c.runList()
	case "get":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'get' requires a model slug\n")
			return 1
		}
		return c.runGet(args[1])
	case "create":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'create' requires a model slug\n")
			return 1
		}
		return c.runCreate(args[1:])
	case "update":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'update' requires a model slug\n")
			return 1
		}
		return c.runUpdate(args[1:])
	case "del", "delete":
		return c.runDelete(args[1:])
	case "info":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'info' requires a model slug\n")
			return 1
		}
		return c.runInfo(args[1])
	case "import":
		return NewImportCommand(c.cfg).Run(args[1:])
	case "export":
		return NewExportCommand(c.cfg).Run(args[1:])
	case "compose":
		return NewComposeCommand(c.cfg).Run(args[1:])
	case "clear-cache":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'clear-cache' requires a model slug\n")
			return 1
		}
		return c.runClearCache(args[1])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown model subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

const modelDetailTmpl = `Slug:          {{.Slug}}
Type:          {{.Type}}
Sub-Type:      {{.SubType}}
Name:          {{.Name}}
Engine:        {{.EngineType}}
HF Repo:       {{.HFRepo}}
YML:           {{.YML}}
Container:     {{.Container}}
Port:          {{.Port}}
Env Vars:      {{.EnvVars}}
Command Args:  {{.CommandArgs}}
Input Cost:    {{.InputTokenCost}}
Output Cost:   {{.OutputTokenCost}}
Capabilities:  {{.Capabilities}}
Created:       {{.CreatedAt}}
Updated:       {{.UpdatedAt}}`

// runList displays all models with STATUS, CACHED, and ENGINE columns.
func (c *ModelCommand) runList() int {
	models, err := c.svc.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No models found. Run 'llm-manager migrate' to import models.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tTYPE\tSUBTYPE\tNAME\tPORT\tSTATUS\tCACHED\tENGINE")
	fmt.Fprintln(w, "----\t----\t-------\t----\t----\t------\t------\t------")

	containerSvc := service.NewContainerService(c.cfg.db, c.cfg.cfg)

	for _, m := range models {
		status := inspectContainerState(m.GetContainerName())
		cached := "\u2014"
		engine := m.EngineType
		if engine == "" {
			engine = "vllm"
		}

		var caps []string
		json.Unmarshal([]byte(m.Capabilities), &caps)

		if m.HFRepo != "" {
			cacheInfo := containerSvc.HFCacheSize(m.HFRepo)
			if cacheInfo.Cached {
				cached = service.FormatVRAM(uint64(cacheInfo.Size))
			} else {
				cached = "no"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%v\n",
			m.Slug, m.Type, m.SubType, m.Name, m.Port, status, cached, engine)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d models\n", len(models))
	return 0
}

// runGet displays a single model.
func (c *ModelCommand) runGet(slug string) int {
	model, err := c.svc.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model: %v\n", err)
		return 1
	}

	t, err := template.New("model").Parse(modelDetailTmpl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing template: %v\n", err)
		return 1
	}
	if err := t.Execute(os.Stdout, model); err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering template: %v\n", err)
	}

	fmt.Println()
	return 0
}
