// Package cmd provides the update subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("update", func(root *RootCommand) Command { return NewUpdateCommand(root) })
}

// UpdateCommand handles HF weight pull operations.
type UpdateCommand struct {
	cfg *config.Config
	db  database.DatabaseManager
	svc *service.ContainerService
}

// NewUpdateCommand creates a new UpdateCommand.
func NewUpdateCommand(root *RootCommand) *UpdateCommand {
	return &UpdateCommand{
		cfg: root.cfg,
		db:  root.db,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the update command with the given subcommand and arguments.
func (c *UpdateCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		// Treat as model slug or "all"
		return c.runUpdateModel(args[0])
	}
}

// runUpdateModel pulls weights for a single model or all models.
func (c *UpdateCommand) runUpdateModel(target string) int {
	// Check for HF_TOKEN via config first
	hfToken := c.cfg.HfToken
	if hfToken == "" {
		hfToken = os.Getenv("HUGGING_FACE_HUB_TOKEN")
	}
	if hfToken == "" {
		fmt.Fprintf(os.Stderr, "Error: HF_TOKEN is not configured. Set it via 'llm-manager config set HF_TOKEN <token>' or HUGGING_FACE_HUB_TOKEN environment variable.\n")
		return 1
	}

	var models []struct {
		Slug   string
		Name   string
		HFRepo string
	}

	if c.db == nil {
		fmt.Fprintf(os.Stderr, "Error: database not initialized\n")
		return 1
	}

	if target == "all" {
		all, err := c.db.ListModels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
			return 1
		}
		for _, m := range all {
			if m.HFRepo != "" {
				models = append(models, struct {
					Slug   string
					Name   string
					HFRepo string
				}{m.Slug, m.Name, m.HFRepo})
			}
		}
		if len(models) == 0 {
			fmt.Println("No models with HF repo configured found.")
			return 0
		}
	} else {
		model, err := c.db.GetModel(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: unknown model or no HF repo configured: %s\n", target)
			return 1
		}
		if model.HFRepo == "" {
			fmt.Fprintf(os.Stderr, "Error: unknown model or no HF repo configured: %s\n", target)
			return 1
		}
		models = append(models, struct {
			Slug   string
			Name   string
			HFRepo string
		}{model.Slug, model.Name, model.HFRepo})
	}

	// Pull weights for each model
	successes := 0
	failures := 0

	for _, m := range models {
		fmt.Printf("Pulling weights for %s (%s)...\n", m.Slug, m.HFRepo)

		// Run hf download with real-time output streaming
		cmd := exec.Command("hf", "download", m.HFRepo, "--token", hfToken)
		cmd.Env = append(os.Environ(),
			"HF_HOME="+c.cfg.HFCacheDir,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Failed to update %s\n", m.Slug)
			failures++
			continue
		}

		fmt.Printf("  ✓ %s updated\n", m.Slug)
		successes++
	}

	// Print summary for "all"
	if target == "all" {
		fmt.Printf("\nUpdate complete: %d succeeded, %d failed\n", successes, failures)
		if failures > 0 {
			return 1
		}
	}

	return 0
}

// PrintHelp prints the update command help.
func (c *UpdateCommand) PrintHelp() {
	fmt.Println(`update - Pull model weights from HuggingFace.

USAGE:
  llm-manager update <slug|all>

ARGUMENTS:
  slug    Pull weights for a specific model
  all     Pull weights for all models with HF repos configured

ENVIRONMENT VARIABLES:
  HF_TOKEN                HuggingFace API token (required)
  HUGGING_FACE_HUB_TOKEN  Alternative HuggingFace token name

EXAMPLES:
  llm-manager update qwen3_6
  llm-manager update all
  HF_TOKEN=hf_xxx llm-manager update qwen3_6

NOTES:
  Requires the 'hf' CLI tool from huggingface_hub:
    pip install -U huggingface_hub
  The HF_TOKEN environment variable must be set for authenticated repos.`)
}
