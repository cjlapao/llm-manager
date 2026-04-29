// Package cmd provides the rag subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("rag", func(root *RootCommand) Command { return NewRagCommand(root) })
}

// RagCommand handles combined embed + rerank operations.
type RagCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewRagCommand creates a new RagCommand.
func NewRagCommand(root *RootCommand) *RagCommand {
	return &RagCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the rag command with the given subcommand and arguments.
func (c *RagCommand) Run(args []string) int {
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
		fmt.Fprintf(os.Stderr, "unknown rag subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// resolveSlug selects a model slug from the arguments for a given subType.
// It handles three cases:
//   - "--default" flag: finds the model with Default=true
//   - positional slug: uses the first positional arg as the slug
//   - no args: selects the first model from the database
func (c *RagCommand) resolveSlug(subType string, args []string) (string, error) {
	models, err := c.cfg.db.ListModelsByTypeSubType("rag", subType)
	if err != nil {
		return "", fmt.Errorf("failed to list %s models: %w", subType, err)
	}
	if len(models) == 0 {
		return "", fmt.Errorf("no %s models found in database", subType)
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
		return "", fmt.Errorf("no default %s model found in database", subType)
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

// runStart starts both embed and rerank models.
// Usage: rag start [--default] [--allow-multiple|-m] [<embed-slug> <rerank-slug>]
func (c *RagCommand) runStart(args []string) int {
	var useDefault bool
	var allowMultiple bool
	var slugs []string
	for _, arg := range args {
		switch arg {
		case "--default":
			useDefault = true
		case "--allow-multiple", "-m":
			allowMultiple = true
		default:
			slugs = append(slugs, arg)
		}
	}

	var embedSlug, rerankSlug string

	if useDefault {
		// Find default embed model
		embedModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
			return 1
		}
		for _, m := range embedModels {
			if m.Default {
				embedSlug = m.Slug
				break
			}
		}
		if embedSlug == "" {
			fmt.Fprintln(os.Stderr, "Error: No default embed model found. Set a model as default in the database.")
			return 1
		}

		// Find default rerank model
		rerankModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
			return 1
		}
		for _, m := range rerankModels {
			if m.Default {
				rerankSlug = m.Slug
				break
			}
		}
		if rerankSlug == "" {
			fmt.Fprintln(os.Stderr, "Error: No default rerank model found. Set a model as default in the database.")
			return 1
		}
	} else if len(slugs) == 2 {
		// Specific slugs provided: <embed-slug> <rerank-slug>
		// Validate embed slug exists
		embedModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
			return 1
		}
		foundEmbed := false
		for _, m := range embedModels {
			if m.Slug == slugs[0] {
				foundEmbed = true
				break
			}
		}
		if !foundEmbed {
			fmt.Fprintf(os.Stderr, "Error: Embed model with slug %q not found in database.\n", slugs[0])
			return 1
		}

		// Validate rerank slug exists
		rerankModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
			return 1
		}
		foundRerank := false
		for _, m := range rerankModels {
			if m.Slug == slugs[1] {
				foundRerank = true
				break
			}
		}
		if !foundRerank {
			fmt.Fprintf(os.Stderr, "Error: Rerank model with slug %q not found in database.\n", slugs[1])
			return 1
		}

		embedSlug = slugs[0]
		rerankSlug = slugs[1]
	} else if len(slugs) == 0 {
		// No args: take first embed and first rerank model from DB
		embedModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
			return 1
		}
		if len(embedModels) == 0 {
			fmt.Fprintln(os.Stderr, "No embed models found in database. Add models with Type=rag and SubType=embedding.")
			return 1
		}
		embedSlug = embedModels[0].Slug

		rerankModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
			return 1
		}
		if len(rerankModels) == 0 {
			fmt.Fprintln(os.Stderr, "No rerank models found in database. Add models with Type=rag and SubType=reranker.")
			return 1
		}
		rerankSlug = rerankModels[0].Slug
	} else {
		fmt.Fprintln(os.Stderr, "Error: Expected 0, 2 positional slugs, or --default flag. Got:", len(slugs))
		return 1
	}

	fmt.Printf("Starting RAG services (embed: %s, rerank: %s)...\n", embedSlug, rerankSlug)

	if err := c.svc.StartModelBySlugWithAllow(embedSlug, allowMultiple); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting embed model %s: %v\n", embedSlug, err)
		return 1
	}

	if err := c.svc.StartModelBySlugWithAllow(rerankSlug, allowMultiple); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting rerank model %s: %v\n", rerankSlug, err)
		// Attempt to roll back the embed container
		c.svc.StopModelBySlug(embedSlug) // best-effort cleanup
		return 1
	}

	fmt.Printf("RAG services started (embed: %s, rerank: %s)\n", embedSlug, rerankSlug)
	return 0
}

// runStop stops both embed and rerank models.
// Usage: rag stop [--default] [--allow-multiple|-m] [<embed-slug> <rerank-slug>]
func (c *RagCommand) runStop(args []string) int {
	var useDefault bool
	var allowMultiple bool
	var slugs []string
	for _, arg := range args {
		switch arg {
		case "--default":
			useDefault = true
		case "--allow-multiple", "-m":
			allowMultiple = true
		default:
			slugs = append(slugs, arg)
		}
	}

	var embedSlug, rerankSlug string

	if useDefault {
		// Find default embed model
		embedModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
			return 1
		}
		for _, m := range embedModels {
			if m.Default {
				embedSlug = m.Slug
				break
			}
		}
		if embedSlug == "" {
			fmt.Fprintln(os.Stderr, "Error: No default embed model found. Set a model as default in the database.")
			return 1
		}

		// Find default rerank model
		rerankModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
			return 1
		}
		for _, m := range rerankModels {
			if m.Default {
				rerankSlug = m.Slug
				break
			}
		}
		if rerankSlug == "" {
			fmt.Fprintln(os.Stderr, "Error: No default rerank model found. Set a model as default in the database.")
			return 1
		}
	} else if len(slugs) == 2 {
		// Specific slugs provided: <embed-slug> <rerank-slug>
		embedSlug = slugs[0]
		rerankSlug = slugs[1]
	} else if len(slugs) == 0 {
		// Find currently running embed container
		embedModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
			return 1
		}
		for _, m := range embedModels {
			status, statusErr := c.svc.GetModelStatus(m.Slug)
			if statusErr != nil {
				continue
			}
			if status.Status == "running" {
				embedSlug = m.Slug
				break
			}
		}
		if embedSlug == "" {
			fmt.Fprintln(os.Stderr, "No running embed container found.")
		}

		// Find currently running rerank container
		rerankModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
			return 1
		}
		for _, m := range rerankModels {
			status, statusErr := c.svc.GetModelStatus(m.Slug)
			if statusErr != nil {
				continue
			}
			if status.Status == "running" {
				rerankSlug = m.Slug
				break
			}
		}
		if rerankSlug == "" {
			fmt.Fprintln(os.Stderr, "No running rerank container found.")
		}
	} else {
		fmt.Fprintln(os.Stderr, "Error: Expected 0, 2 positional slugs, or --default flag. Got:", len(slugs))
		return 1
	}

	hasWork := false

	if embedSlug != "" {
		fmt.Printf("Stopping embed model %s...\n", embedSlug)
		if err := c.svc.StopModelBySlug(embedSlug); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping embed model: %v\n", err)
			return 1
		}
		hasWork = true
	}

	if !allowMultiple {
		// Stop any other running containers of the same subtypes
		if embedSlug != "" {
			if err := c.svc.StopAllBySubType("rag", "embedding"); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to stop other embed containers: %v\n", err)
			}
		}
		if rerankSlug != "" {
			if err := c.svc.StopAllBySubType("rag", "reranker"); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to stop other rerank containers: %v\n", err)
			}
		}
	}

	if rerankSlug != "" {
		fmt.Printf("Stopping rerank model %s...\n", rerankSlug)
		if err := c.svc.StopModelBySlug(rerankSlug); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping rerank model: %v\n", err)
			return 1
		}
		hasWork = true
	}

	if !hasWork {
		fmt.Println("No running RAG containers to stop.")
		return 0
	}

	fmt.Println("RAG services stopped")
	return 0
}

// runInfo displays structured information for all embed and rerank models.
func (c *RagCommand) runInfo() int {
	// Embed models
	embedModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "embedding")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing embed models: %v\n", err)
		return 1
	}

	rerankModels, err := c.cfg.db.ListModelsByTypeSubType("rag", "reranker")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing rerank models: %v\n", err)
		return 1
	}

	if len(embedModels) == 0 && len(rerankModels) == 0 {
		fmt.Println("No RAG models found in database.")
		fmt.Println("Add models with Type=rag and SubType=embedding or SubType=reranker.")
		return 0
	}

	// Embed section
	if len(embedModels) > 0 {
		fmt.Println("Embed Models")
		fmt.Println(strings.Repeat("─", 60))
		for _, m := range embedModels {
			status, statusErr := c.svc.GetModelStatus(m.Slug)
			if statusErr != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not get status for %s: %v\n", m.Slug, statusErr)
				continue
			}

			fmt.Printf("  Name:       %s\n", status.Name)
			fmt.Printf("  Slug:       %s", m.Slug)
			if m.Default {
				fmt.Print(" (default)")
			}
			fmt.Println()
			fmt.Printf("  Container:  %s\n", status.Container)
			fmt.Printf("  Port:       %d\n", status.Port)
			fmt.Printf("  Status:     %s\n", status.Status)
			fmt.Println(strings.Repeat("─", 60))
		}
	}

	// Rerank section
	if len(rerankModels) > 0 {
		fmt.Println("Rerank Models")
		fmt.Println(strings.Repeat("─", 60))
		for _, m := range rerankModels {
			status, statusErr := c.svc.GetModelStatus(m.Slug)
			if statusErr != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not get status for %s: %v\n", m.Slug, statusErr)
				continue
			}

			fmt.Printf("  Name:       %s\n", status.Name)
			fmt.Printf("  Slug:       %s", m.Slug)
			if m.Default {
				fmt.Print(" (default)")
			}
			fmt.Println()
			fmt.Printf("  Container:  %s\n", status.Container)
			fmt.Printf("  Port:       %d\n", status.Port)
			fmt.Printf("  Status:     %s\n", status.Status)
			fmt.Println(strings.Repeat("─", 60))
		}
	}

	return 0
}

// PrintHelp prints the rag command help.
func (c *RagCommand) PrintHelp() {
	fmt.Println(`rag - Manage RAG services (embed + rerank combined).

USAGE:
  llm-manager rag [SUBCOMMAND] [OPTIONS] [SLUGS]

SUBCOMMANDS:
  start   Start embed and rerank model containers
  stop    Stop embed and rerank model containers
  info    Show information about all RAG models

OPTIONS:
  --default          Select the default model (marked as default in database)
  --allow-multiple, -m  Allow multiple containers of the same subtype to run simultaneously

POSITIONAL ARGUMENTS:
  SLUGS       Embed and rerank slugs: <embed-slug> <rerank-slug>

EXAMPLES:
  llm-manager rag start
      Start the first (or default) embed and rerank models found in the database.

  llm-manager rag start --default
      Start the embed and rerank models marked as default.

  llm-manager rag start nomic-embed bge-reranker
      Start specific embed and rerank models by slug.

  llm-manager rag start --allow-multiple
      Start without stopping other running embed/rerank containers.

  llm-manager rag stop
      Stop the currently running embed and rerank containers.

  llm-manager rag stop --default
      Stop the default embed and rerank models.

  llm-manager rag stop nomic-embed bge-reranker
      Stop specific embed and rerank models by slug.

  llm-manager rag info
      Display structured information for all embed and rerank models.

NOTES:
  This command manages both embed (SubType=embedding) and rerank (SubType=reranker)
  containers together as a single RAG stack.
  All container operations use the container name from the model's Container field.`)
}
