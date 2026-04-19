// Package cmd provides the model subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

// ModelCommand handles model CRUD operations.
type ModelCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewModelCommand creates a new ModelCommand.
func NewModelCommand(root *RootCommand) *ModelCommand {
	return &ModelCommand{
		cfg: root,
		svc: service.NewModelService(root.db),
	}
}

// Run executes the model command with the given subcommand and arguments.
func (c *ModelCommand) Run(args []string) int {
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
		return c.runUpdate(args[1])
	case "delete", "del":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'delete' requires a model slug\n")
			return 1
		}
		return c.runDelete(args[1])
	case "import":
		return NewImportCommand(c.cfg).Run(args[1:])
	case "export":
		return NewExportCommand(c.cfg).Run(args[1:])
	case "compose":
		return NewComposeCommand(c.cfg).Run(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown model subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

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
	fmt.Fprintln(w, "SLUG\tTYPE\tNAME\tPORT\tSTATUS\tCACHED\tENGINE")
	fmt.Fprintln(w, "----\t----\t----\t----\t------\t------\t------")

	containerSvc := service.NewContainerService(c.cfg.db, c.cfg.cfg)

	for _, m := range models {
		container := m.Container
		status := "unknown"
		cached := "no"
		engine := m.EngineType
		if engine == "" {
			engine = "vllm"
		}

		if container != "" {
			// Query live Docker status
			cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", container)
			if output, err := cmd.Output(); err == nil {
				status = strings.TrimSpace(string(output))
			}
			container = m.Container
		}

		// Check HF cache
		if m.HFRepo != "" {
			cached = "yes"
			if containerSvc.IsHFCached(m.HFRepo) {
				cached = "yes"
			} else {
				cached = "no"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			m.Slug, m.Type, m.Name, m.Port, status, cached, engine)
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

	tmpl := `Slug:          {{.Slug}}
Type:          {{.Type}}
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

	func() {
		t, err := template.New("model").Parse(tmpl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing template: %v\n", err)
			return
		}
		if err := t.Execute(os.Stdout, model); err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering template: %v\n", err)
		}
	}()

	fmt.Println()
	return 0
}

// runCreate creates a new model from command line arguments.
func (c *ModelCommand) runCreate(args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: llm-manager model create <slug> [type] [name] [port]\n")
		return 1
	}

	slug := args[0]
	model := &models.Model{
		Slug: slug,
		Type: "llm",
		Port: 0,
	}

	if len(args) > 1 {
		model.Type = args[1]
	}
	if len(args) > 2 {
		model.Name = args[2]
	}
	if len(args) > 3 {
		fmt.Sscanf(args[3], "%d", &model.Port)
	}

	if err := c.svc.CreateModel(model); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating model: %v\n", err)
		return 1
	}

	fmt.Printf("Created model: %s\n", slug)
	return 0
}

// runUpdate updates a model's fields.
func (c *ModelCommand) runUpdate(slug string) int {
	updates := map[string]interface{}{}

	// Simple key=value updates from remaining args
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == slug {
			continue
		}
		for ; i < len(os.Args); i++ {
			arg := os.Args[i]
			if key, val, ok := parseKeyValue(arg); ok {
				updates[key] = val
			}
		}
		break
	}

	if len(updates) == 0 {
		fmt.Println("Usage: llm-manager model update <slug> [key=value ...]")
		fmt.Println("Available fields: name, type, hf_repo, yml, container, port, engine_type, env_vars, command_args, input_token_cost, output_token_cost, capabilities")
		return 0
	}

	if err := c.svc.UpdateModel(slug, updates); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating model: %v\n", err)
		return 1
	}

	fmt.Printf("Updated model: %s\n", slug)
	return 0
}

// runDelete removes a model from the database.
func (c *ModelCommand) runDelete(slug string) int {
	if err := c.svc.DeleteModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting model: %v\n", err)
		return 1
	}

	fmt.Printf("Deleted model: %s\n", slug)
	return 0
}

// parseKeyValue parses a key=value argument.
func parseKeyValue(arg string) (key, value string, ok bool) {
	for i, ch := range arg {
		if ch == '=' {
			return arg[:i], arg[i+1:], true
		}
	}
	return "", "", false
}

// PrintHelp prints the model command help.
func (c *ModelCommand) PrintHelp() {
	fmt.Println(`model - Manage LLM models in the registry.

USAGE:
  llm-manager model [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list, ls      List all models (with live STATUS, CACHED, and ENGINE columns)
  get <slug>    Show details for a model
  create <slug> [type] [name] [port]  Create a new model
  update <slug> [key=value ...]       Update model fields
  delete, del <slug>                  Delete a model
  import <file.yaml> [options]        Import a model from a YAML file
  export <slug> [options]             Export a model to a YAML file
  compose <slug> [options]            Generate a docker-compose.yml file

OPTIONS:
  --input-cost <float>              Override input token cost (import)
  --output-cost <float>             Override output token cost (import)
  --capabilities <comma,list>       Override capabilities list (import)
  --output <file.yaml>              Output file path (export)

EXAMPLES:
  llm-manager model list
  llm-manager model get qwen3_6
  llm-manager model create my-model llm "My Model" 8080
  llm-manager model update qwen3_6 name="Updated Name"
  llm-manager model delete old-model
  llm-manager model import model.yaml
  llm-manager model import model.yaml --input-cost 0.000001
  llm-manager model export qwen3_6
  llm-manager model export qwen3_6 --output backup.yaml`)
}

// knownFluxModels returns the list of known flux model slugs.
func knownFluxModels() []string {
	return []string{"flux-schnell", "flux-dev"}
}

// isFluxModel checks if a slug is a known flux model.
func isFluxModel(slug string) bool {
	for _, m := range knownFluxModels() {
		if slug == m {
			return true
		}
	}
	return false
}

// known3DModels returns the list of known 3D model slugs.
func known3DModels() []string {
	return []string{"hunyuan3d", "trellis"}
}

// is3DModel checks if a slug is a known 3D model.
func is3DModel(slug string) bool {
	for _, m := range known3DModels() {
		if slug == m {
			return true
		}
	}
	return false
}

// fluxCheckpoint returns the checkpoint filename for a flux model.
func fluxCheckpoint(slug string) string {
	switch slug {
	case "flux-schnell":
		return "flux1-schnell.safetensors"
	case "flux-dev":
		return "flux1-dev.safetensors"
	}
	return ""
}

// dirFor3DModel returns the directory name for a 3D model.
func dirFor3DModel(slug string) string {
	switch slug {
	case "hunyuan3d":
		return "hunyuan3d"
	case "trellis":
		return "trellis"
	}
	return ""
}

// readActiveFile reads the content of an active model file.
func readActiveFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeActiveFile writes content to an active model file.
func writeActiveFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
