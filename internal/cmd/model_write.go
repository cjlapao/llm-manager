package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/database/models"
)

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

// parseKeyValue parses a key=value argument.
func parseKeyValue(arg string) (key, value string, ok bool) {
	for i, ch := range arg {
		if ch == '=' {
			return arg[:i], arg[i+1:], true
		}
	}
	return "", "", false
}

// runUpdate updates a model's fields.
func (c *ModelCommand) runUpdate(args []string) int {
	if len(args) < 1 {
		printUpdateUsage()
		return 0
	}

	slug := args[0]
	updates := map[string]interface{}{}

	for _, arg := range args[1:] {
		if key, val, ok := parseKeyValue(arg); ok {
			updates[key] = val
		} else {
			fmt.Fprintf(os.Stderr, "Warning: ignoring invalid argument %q (expected key=value)\n", arg)
		}
	}

	if len(updates) == 0 {
		printUpdateUsage()
		return 0
	}

	if err := c.svc.UpdateModel(slug, updates); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating model: %v\n", err)
		return 1
	}

	fmt.Printf("Updated model: %s\n", slug)
	return 0
}

func printUpdateUsage() {
	fmt.Println("Usage: llm-manager model update <slug> [key=value ...]")
	fmt.Println("Available fields: name, type, hf_repo, yml, container, port, engine_type, env_vars, command_args, input_token_cost, output_token_cost, cache_creation_input_token_cost, cache_read_input_token_cost, capabilities")
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
		allModels, err := c.svc.ListModels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing models for deletion: %v\n", err)
			return 1
		}
		if len(allModels) == 0 {
			fmt.Println("No models to delete.")
			return 0
		}
		successCount := 0
		failCount := 0
		for i, m := range allModels {
			fmt.Printf("[%d/%d] Deleting model: %s\n", i+1, len(allModels), m.Slug)
			if err := c.svc.DeleteModel(m.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "  Error deleting model %s: %v\n", m.Slug, err)
				failCount++
				continue
			}
			fmt.Printf("  Deleted model: %s\n", m.Slug)
			successCount++
		}
		fmt.Printf("\nTotal: %d/%d models deleted (%d failed)\n", successCount, len(allModels), failCount)
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
