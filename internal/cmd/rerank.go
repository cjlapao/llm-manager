// Package cmd provides the rerank subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("rerank", func(root *RootCommand) Command { return NewRerankCommand(root) })
}

// RerankCommand handles the rerank container operations.
type RerankCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewRerankCommand creates a new RerankCommand.
func NewRerankCommand(root *RootCommand) *RerankCommand {
	return &RerankCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the rerank command with the given subcommand and arguments.
func (c *RerankCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "start":
		return c.runStart(args[1:])
	case "stop":
		return c.runStop(args[1:])
	case "info":
		return c.runInfo()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown rerank subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// resolveSlug selects a rerank model slug from the arguments.
// It handles three cases:
//   - "--default" flag: finds the model with Default=true
//   - positional slug: uses the first positional arg as the slug
//   - no args: selects the first model from the database
func (c *RerankCommand) resolveSlug(args []string) (string, error) {
	models, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
	if err != nil {
		return "", fmt.Errorf("failed to list rerank models: %w", err)
	}
	if len(models) == 0 {
		return "", fmt.Errorf("no rerank models found in database")
	}

	hasDefault := false
	var slugArgs []string

	for _, arg := range args {
		if arg == "--default" {
			hasDefault = true
		} else {
			slugArgs = append(slugArgs, arg)
		}
	}

	if hasDefault {
		for _, m := range models {
			if m.Default {
				return m.Slug, nil
			}
		}
		return "", fmt.Errorf("no default rerank model found in database")
	}

	if len(slugArgs) > 0 {
		return slugArgs[0], nil
	}

	// Default: first model, preferring the default one
	for _, m := range models {
		if m.Default {
			return m.Slug, nil
		}
	}
	return models[0].Slug, nil
}

// runStart starts a rerank model.
// Usage: rerank start [--default] [<slug>]
func (c *RerankCommand) runStart(args []string) int {
	slug, err := c.resolveSlug(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Starting rerank model %s...\n", slug)
	if err := c.svc.StartModelBySlug(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting rerank model: %v\n", err)
		return 1
	}
	return 0
}

// runStop stops a rerank model.
// Usage: rerank stop [--default] [<slug>]
func (c *RerankCommand) runStop(args []string) int {
	slug, err := c.resolveSlug(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping rerank model %s...\n", slug)
	if err := c.svc.StopModelBySlug(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping rerank model: %v\n", err)
		return 1
	}
	return 0
}

// runInfo displays structured information for all rerank models.
func (c *RerankCommand) runInfo() int {
	models, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No rerank models found in database.")
		return 0
	}

	fmt.Println("Rerank Models:")
	fmt.Println(strings.Repeat("─", 60))

	for _, m := range models {
		status, err := c.svc.GetModelStatus(m.Slug)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to get status for %s: %v\n", m.Slug, err)
			continue
		}

		fmt.Printf("  Name:       %s\n", m.Name)
		fmt.Printf("  Slug:       %s", m.Slug)
		if m.Default {
			fmt.Print(" (default)")
		}
		fmt.Println()
		fmt.Printf("  Container:  %s\n", status.Container)
		fmt.Printf("  Port:       %d\n", m.Port)
		fmt.Printf("  Status:     %s\n", status.Status)
		fmt.Println(strings.Repeat("─", 60))
	}

	return 0
}

// PrintHelp prints the rerank command help.
func (c *RerankCommand) PrintHelp() {
	fmt.Println(`rerank - Manage rerank models and containers.

USAGE:
  llm-manager rerank [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start       Start a rerank model
  stop        Stop a rerank model
  info        Show information about rerank models

OPTIONS:
  --default   Select the default rerank model (used with start/stop)

EXAMPLES:
  llm-manager rerank start
      Start the first (or default) rerank model found in the database.

  llm-manager rerank start --default
      Start the rerank model marked as default.

  llm-manager rerank start <slug>
      Start a specific rerank model by its slug.

  llm-manager rerank stop
      Stop the currently running rerank model.

  llm-manager rerank stop --default
      Stop the default rerank model.

  llm-manager rerank stop <slug>
      Stop a specific rerank model by its slug.

  llm-manager rerank info
      Display structured information for all rerank models.

NOTES:
  All rerank commands filter models by SubType=reranker.
  Container names are resolved from the database — no hardcoded names.`)
}
