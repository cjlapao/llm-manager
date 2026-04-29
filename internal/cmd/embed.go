// Package cmd provides the embed subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("embed", func(root *RootCommand) Command { return NewEmbedCommand(root) })
}

// EmbedCommand handles the embed container operations.
type EmbedCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewEmbedCommand creates a new EmbedCommand.
func NewEmbedCommand(root *RootCommand) *EmbedCommand {
	return &EmbedCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the embed command with the given subcommand and arguments.
func (c *EmbedCommand) Run(args []string) int {
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
		fmt.Fprintf(os.Stderr, "unknown embed subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// parseArgs splits args into the --default flag and remaining positional slugs.
// Returns (useDefault bool, slugs []string).
func parseArgs(args []string) (bool, []string) {
	var slugs []string
	var useDefault bool
	for _, arg := range args {
		if arg == "--default" {
			useDefault = true
		} else {
			slugs = append(slugs, arg)
		}
	}
	return useDefault, slugs
}

// runStart starts an embed model. Supports --default flag and positional slug.
func (c *EmbedCommand) runStart(args []string) int {
	useDefault, slugs := parseArgs(args)

	var slug string
	var err error

	if useDefault {
		// Find the default embed model
		models, listErr := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", listErr)
			return 1
		}
		for _, m := range models {
			if m.Default {
				slug = m.Slug
				break
			}
		}
		if slug == "" {
			fmt.Fprintln(os.Stderr, "No default embed model found. Set a model as default in the database.")
			return 1
		}
	} else if len(slugs) > 0 {
		slug = slugs[0]
	} else {
		// Take the first embed model found
		models, listErr := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", listErr)
			return 1
		}
		if len(models) == 0 {
			fmt.Fprintln(os.Stderr, "No embed models found in database. Add models with Type=rag and SubType=embedding.")
			return 1
		}
		slug = models[0].Slug
	}

	fmt.Printf("Starting embed model %s...\n", slug)
	if err = c.svc.StartModelBySlug(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting embed model: %v\n", err)
		return 1
	}
	return 0
}

// runStop stops an embed model. Supports --default flag and positional slug.
func (c *EmbedCommand) runStop(args []string) int {
	useDefault, slugs := parseArgs(args)

	var slug string
	var err error

	if useDefault {
		// Find the default embed model
		models, listErr := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", listErr)
			return 1
		}
		for _, m := range models {
			if m.Default {
				slug = m.Slug
				break
			}
		}
		if slug == "" {
			fmt.Fprintln(os.Stderr, "No default embed model found. Set a model as default in the database.")
			return 1
		}
	} else if len(slugs) > 0 {
		slug = slugs[0]
	} else {
		// Find the currently running embed model
		models, listErr := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", listErr)
			return 1
		}
		for _, m := range models {
			status, statusErr := c.svc.GetModelStatus(m.Slug)
			if statusErr != nil {
				continue
			}
			if status.Status == "running" {
				slug = m.Slug
				break
			}
		}
		if slug == "" {
			fmt.Fprintln(os.Stderr, "No running embed container found.")
			return 0
		}
	}

	fmt.Printf("Stopping embed model %s...\n", slug)
	if err = c.svc.StopModelBySlug(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping embed model: %v\n", err)
		return 1
	}
	return 0
}

// runInfo displays structured information for all embed models with their status.
func (c *EmbedCommand) runInfo() int {
	models, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No embed models found. Add models with Type=rag and SubType=embedding.")
		return 0
	}

	fmt.Println("Embed Models")
	fmt.Println(strings.Repeat("─", 40))

	for _, m := range models {
		status, statusErr := c.svc.GetModelStatus(m.Slug)
		if statusErr != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not get status for %s: %v\n", m.Slug, statusErr)
			continue
		}

		fmt.Printf("\n  Name:      %s\n", status.Name)
		fmt.Printf("  Slug:      %s\n", status.Slug)
		fmt.Printf("  Container: %s\n", status.Container)
		fmt.Printf("  Port:      %d\n", status.Port)
		fmt.Printf("  Status:    %s\n", status.Status)
	}

	return 0
}

// PrintHelp prints the embed command help.
func (c *EmbedCommand) PrintHelp() {
	fmt.Println(`embed - Manage embed RAG models.

USAGE:
  llm-manager embed [SUBCOMMAND] [OPTIONS] [SLUG]

SUBCOMMANDS:
  start   Start an embed model container
  stop    Stop a running embed model container
  info    Show information about embed models

OPTIONS:
  --default   Select the default embed model (marked as default in database)

POSITIONAL ARGUMENTS:
  SLUG        Select a specific embed model by slug

EXAMPLES:
  llm-manager embed start              Start the first embed model found
  llm-manager embed start --default    Start the default embed model
  llm-manager embed start nomic-embed  Start a specific embed model by slug
  llm-manager embed stop               Stop the currently running embed container
  llm-manager embed stop --default     Stop the default embed model
  llm-manager embed stop nomic-embed   Stop a specific embed model by slug
  llm-manager embed info               Show information about all embed models

NOTES:
  Embed models are stored in the database with Type=rag and SubType=embedding.
  All container operations use the container name from the model's Container field.`)
}
