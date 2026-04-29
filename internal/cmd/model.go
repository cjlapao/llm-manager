// Package cmd provides the model subcommand for llm-manager.
package cmd

import (
	"encoding/json"
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

func init() {
	RegisterCommand("model", func(root *RootCommand) Command { return NewModelCommand(root) })
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
		return c.runUpdate(args[1:])
	case "delete", "del":
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
		container := m.Container
		status := "unknown"
		cached := "—"
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
			cacheInfo := containerSvc.HFCacheSize(m.HFRepo)
			if cacheInfo.Cached {
				cached = service.FormatVRAM(uint64(cacheInfo.Size))
			} else {
				cached = "no"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
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

	tmpl := `Slug:          {{.Slug}}
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
		Slug:    slug,
		Type:    "llm",
		Port:    0,
		Default: false,
	}

	if len(args) > 1 {
		model.Type = args[1]
	}
	if len(args) > 2 {
		model.Name = args[2]
	}
	if len(args) > 3 {
		if _, err := fmt.Sscanf(args[3], "%d", &model.Port); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid port %q, defaulting to 0\n", args[3])
		}
	}

	if err := c.svc.CreateModel(model); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating model: %v\n", err)
		return 1
	}

	fmt.Printf("Created model: %s\n", slug)
	return 0
}

// runUpdate updates a model's fields.
func (c *ModelCommand) runUpdate(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: llm-manager model update <slug> [key=value ...]")
		fmt.Println("Available fields: name, type, hf_repo, yml, container, port, engine_type, env_vars, command_args, input_token_cost, output_token_cost, capabilities")
		return 0
	}

	slug := args[0]
	updates := map[string]interface{}{}

	// Parse key=value pairs from remaining args
	for _, arg := range args[1:] {
		if key, val, ok := parseKeyValue(arg); ok {
			updates[key] = val
		} else {
			fmt.Fprintf(os.Stderr, "Warning: ignoring invalid argument %q (expected key=value)\n", arg)
		}
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

// runDelete removes a model from the database or all models with --all.
func (c *ModelCommand) runDelete(args []string) int {
	all := false
	var slugArgs []string
	for _, arg := range args {
		if arg == "--all" {
			all = true
		} else {
			slugArgs = append(slugArgs, arg)
		}
	}

	if all && len(slugArgs) > 0 {
		fmt.Fprintln(os.Stderr, "Error: cannot specify --all with a slug")
		return 1
	}

	if all {
		models, err := c.svc.ListModels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing models for deletion: %v\n", err)
			return 1
		}
		if len(models) == 0 {
			fmt.Println("No models to delete.")
			return 0
		}
		successCount := 0
		failCount := 0
		for i, m := range models {
			fmt.Printf("[%d/%d] Deleting model: %s\n", i+1, len(models), m.Slug)
			if err := c.svc.DeleteModel(m.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "  Error deleting model %s: %v\n", m.Slug, err)
				failCount++
				continue
			}
			fmt.Printf("  Deleted model: %s\n", m.Slug)
			successCount++
		}
		fmt.Printf("\nTotal: %d/%d models deleted (%d failed)\n", successCount, len(models), failCount)
		if failCount > 0 {
			return 1
		}
		return 0
	}

	if len(slugArgs) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: llm-manager model delete <slug>")
		fmt.Fprintln(os.Stderr, "       llm-manager model delete --all")
		return 1
	}

	if err := c.svc.DeleteModel(slugArgs[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting model: %v\n", err)
		return 1
	}

	fmt.Printf("Deleted model: %s\n", slugArgs[0])
	return 0
}

// runInfo displays model information organized into grouped sections.
func (c *ModelCommand) runInfo(slug string) int {
	model, err := c.svc.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model: %v\n", err)
		return 1
	}

	pad := strings.Repeat("-", 60)
	slab := func(indent int) string { return strings.Repeat("  ", indent) }

	fmt.Printf("Model: %s (%s)\n", model.Slug, model.Name)
	fmt.Println(pad)

	// ---- base properties ----
	hasBase := model.Slug != "" || model.HFRepo != "" || model.Port > 0
	if hasBase {
		fmt.Println("\nbase properties:")
		fmt.Printf("%-24s%s\n", "slug:", model.Slug)
		fmt.Printf("%-24s%s\n", "name:", model.Name)
		fmt.Printf("%-24s%s\n", "type:", model.Type)
		if model.SubType != "" {
			fmt.Printf("%-24s%s\n", "subtype:", model.SubType)
		}
		if model.EngineType != "" {
			fmt.Printf("%-24s%s\n", "engine:", model.EngineType)
		}
		if model.HFRepo != "" {
			fmt.Printf("%-24s%s\n", "hf_repo:", model.HFRepo)
		}
		fmt.Printf("%-24s%d\n", "port:", model.Port)
		if model.InputTokenCost > 0 && model.OutputTokenCost > 0 {
			fmt.Printf("%-24s%.8f / %.8f\n", "cost:", model.InputTokenCost, model.OutputTokenCost)
		} else if model.InputTokenCost > 0 {
			fmt.Printf("%-24sinput:   %.8f\n", "", model.InputTokenCost)
		} else if model.OutputTokenCost > 0 {
			fmt.Printf("%-24soutput:  %.8f\n", "", model.OutputTokenCost)
		}

		caps := []string{}
		json.Unmarshal([]byte(model.Capabilities), &caps)
		if len(caps) > 0 {
			fmt.Printf("%-24s%+v\n", "capabilities:", strings.Join(caps, ", "))
		}
	}

	// ---- docker ----
	hasDocker := model.Container != "" || len(map[string]string{}) > 0 || model.EnvVars != "" || model.CommandArgs != ""
	{
		if model.Container == "" && len(map[string]string{}) == 0 {
			var ev map[string]string
			json.Unmarshal([]byte(model.EnvVars), &ev)
			var ca []string
			json.Unmarshal([]byte(model.CommandArgs), &ca)
			hasDocker = model.Container != "" || len(ev) > 0 || len(ca) > 0
		}
	}
	if model.Container != "" || model.EnvVars != "" || model.CommandArgs != "" {
		fmt.Println("\ndocker:")
		if model.Container != "" {
			fmt.Printf("%s%-20s%s\n", "  ", "container:", model.Container)
		}

		if model.EnvVars != "" {
			var envVars map[string]string
			if json.Unmarshal([]byte(model.EnvVars), &envVars) == nil && len(envVars) > 0 {
				fmt.Printf("%senvironment:\n", slab(1))
				for k, v := range envVars {
					fmt.Printf("  %s%s=%s\n", slab(2), k, v)
				}
			}
		}

		if model.CommandArgs != "" {
			var cmdArgs []string
			if json.Unmarshal([]byte(model.CommandArgs), &cmdArgs) == nil && len(cmdArgs) > 0 {
				fmt.Printf("%scommand:\n", slab(1))
				for _, arg := range cmdArgs {
					fmt.Printf("    - %s\n", arg)
				}
			}
		}
	}

	// ---- litellm ----
	hasLiteLLM := model.LiteLLMParams != "" || model.ModelInfo != ""
	if hasLiteLLM {
		fmt.Println("\nlitellm:")
		if model.LiteLLMParams != "" {
			var litellmParams map[string]interface{}
			if err := json.Unmarshal([]byte(model.LiteLLMParams), &litellmParams); err == nil {
				fmt.Println("  litellm_params:")
				printNestedMap(litellmParams, "    ")
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse litellm_params: %v\n", err)
			}
		}
		if model.ModelInfo != "" {
			var modelInfo map[string]interface{}
			if err := json.Unmarshal([]byte(model.ModelInfo), &modelInfo); err == nil {
				fmt.Println("  model_info:")
				printNestedMap(modelInfo, "    ")
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse model_info: %v\n", err)
			}
		}
	}

	// No data at all
	if !hasBase && !hasDocker && !hasLiteLLM {
		fmt.Println("\nno model information available.")
	}

	fmt.Println()
	return 0
}

// runClearCache removes the entire HF cache directory for a model (blobs, refs, snapshots).
func (c *ModelCommand) runClearCache(slug string) int {
	model, err := c.svc.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: model %s not found: %v\n", slug, err)
		return 1
	}

	if model.HFRepo == "" {
		fmt.Fprintf(os.Stderr, "Error: model %s has no HF repo configured\n", slug)
		return 1
	}

	// Convert HF repo to cache directory name: Qwen/Qwen3.6-35B-A3B -> models--Qwen--Qwen3.6-35B-A3B
	cacheDir := "models--" + strings.ReplaceAll(model.HFRepo, "/", "--")

	// Check both standard and legacy cache layouts
	cachePaths := []string{
		filepath.Join(c.cfg.cfg.HFCacheDir, "hub", cacheDir),
		filepath.Join(c.cfg.cfg.HFCacheDir, cacheDir),
	}

	var deletedPaths []string
	for _, dir := range cachePaths {
		if _, err := os.Stat(dir); err == nil {
			// Count files before deletion
			fileCount, dirSize := countDirFiles(dir)

			fmt.Printf("Removing cache for %s (%s):\n", slug, model.HFRepo)
			fmt.Printf("  Path: %s\n", dir)
			fmt.Printf("  Files: %d (%s)\n", fileCount, formatSize(dirSize))

			if err := os.RemoveAll(dir); err != nil {
				fmt.Fprintf(os.Stderr, "  Error: failed to remove %s: %v\n", dir, err)
				continue
			}

			deletedPaths = append(deletedPaths, dir)
			fmt.Printf("  ✓ Removed\n")
		}
	}

	if len(deletedPaths) == 0 {
		fmt.Printf("No cache found for %s (%s)\n", slug, model.HFRepo)
		return 0
	}

	fmt.Printf("\nCache cleared for %d path(s)\n", len(deletedPaths))
	return 0
}

// countDirFiles recursively counts files and sums total size under a directory.
func countDirFiles(root string) (int64, int64) {
	var files int64
	var size int64
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			files++
			size += info.Size()
		}
		return nil
	})
	return files, size
}

// formatSize formats a byte count as human-readable.
func formatSize(n int64) string {
	const (
		_  = iota
		KB = 1 << (10 * iota)
		MB
		GB
		TB
	)
	switch {
	case n >= TB:
		return fmt.Sprintf("%.1fTB", float64(n)/TB)
	case n >= GB:
		return fmt.Sprintf("%.1fGB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1fMB", float64(n)/MB)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// printNestedMap recursively prints a map with indentation.
func printNestedMap(m map[string]interface{}, indent string) {
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Printf("%s%s:\n", indent, k)
			printNestedMap(val, indent+"  ")
		case []interface{}:
			fmt.Printf("%s%s:\n", indent, k)
			for _, item := range val {
				fmt.Printf("%s  - %v\n", indent, item)
			}
		default:
			fmt.Printf("%s%s: %v\n", indent, k, v)
		}
	}
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
  info <slug>   Show LiteLLM model information
  create <slug> [type] [name] [port]  Create a new model
  update <slug> [key=value ...]       Update model fields
  delete, del [--all] <slug>                  Delete a model
  import <file.yaml> [options]        Import a model from a YAML file
  export <slug> [options]             Export a model to a YAML file
  compose <slug> [options]            Generate a docker-compose.yml file
  clear-cache <slug>                  Remove cached model weights

OPTIONS:
  --input-cost <float>              Override input token cost (import)
  --output-cost <float>             Override output token cost (import)
  --capabilities <comma,list>       Override capabilities list (import)
  --output <file.yaml>              Output file path (export)

EXAMPLES:
  llm-manager model list
  llm-manager model get qwen3_6
  llm-manager model info qwen3_6
  llm-manager model create my-model llm "My Model" 8080
  llm-manager model update qwen3_6 name="Updated Name"
  llm-manager model delete old-model
  llm-manager model delete --all
  llm-manager model import model.yaml
  llm-manager model import model.yaml --input-cost 0.000001
  llm-manager model export qwen3_6
  llm-manager model export qwen3_6 --output backup.yaml
  llm-manager model clear-cache qwen3_6`)
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
