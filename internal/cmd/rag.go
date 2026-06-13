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

// RagCommand handles RAG model operations (embedding + reranker).
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
	case "list":
		return c.runList()
	case "info":
		return c.runInfo(args)
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown rag subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts RAG model containers.
// Usage: rag start [embed-slug] [rerank-slug]
// If slugs are omitted, the first available model of each type is started.
func (c *RagCommand) runStart(args []string) int {
	// Resolve embed slug: use provided arg or first available
	embedSlug := ""
	rerankSlug := ""

	if len(args) > 0 {
		embedSlug = args[0]
	}
	if len(args) > 1 {
		rerankSlug = args[1]
	}

	// Resolve to actual slugs if empty
	if embedSlug == "" {
		ms, err := c.svc.ListRAGEmbeddingModels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing embedding models: %v\n", err)
			return 1
		}
		if len(ms) == 0 {
			fmt.Fprintln(os.Stderr, "No embedding models available")
			return 1
		}
		embedSlug = ms[0].Slug
	}
	if rerankSlug == "" {
		ms, err := c.svc.ListRAGRerankerModels()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing reranker models: %v\n", err)
			return 1
		}
		if len(ms) == 0 {
			fmt.Fprintln(os.Stderr, "No reranker models available")
			return 1
		}
		rerankSlug = ms[0].Slug
	}

	// Validate both slugs exist
	if _, err := c.cfg.db.GetModel(embedSlug); err != nil {
		fmt.Fprintf(os.Stderr, "Embedding model not found: %s\n", embedSlug)
		return 1
	}
	if _, err := c.cfg.db.GetModel(rerankSlug); err != nil {
		fmt.Fprintf(os.Stderr, "Reranker model not found: %s\n", rerankSlug)
		return 1
	}

	// Start embedding model first, wait for it to be healthy before
	// starting the reranker. This avoids simultaneous vLLM startup
	// contention on a shared GPU.
	fmt.Printf("Starting embedding model: %s\n", embedSlug)
	if err := c.svc.StartModelWithHealthCheck(embedSlug, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting embedding model: %v\n", err)
		return 1
	}

	// Start reranker model after embedding is healthy
	fmt.Printf("Starting reranker model: %s\n", rerankSlug)
	if err := c.svc.StartModelWithHealthCheck(rerankSlug, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting reranker model: %v\n", err)
		return 1
	}

	fmt.Println("RAG models started")
	return 0
}

// runStop stops RAG model containers.
// Usage: rag stop [embed-slug] [rerank-slug]
// If no slugs provided, stops all running RAG containers.
func (c *RagCommand) runStop(args []string) int {
	embedSlug := ""
	rerankSlug := ""

	if len(args) > 0 {
		embedSlug = args[0]
	}
	if len(args) > 1 {
		rerankSlug = args[1]
	}

	// If no slugs provided, stop all running containers of both subtypes
	if embedSlug == "" && rerankSlug == "" {
		fmt.Println("Stopping all running RAG containers...")
		if err := c.svc.StopAllBySubType("rag", "embedding"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping embedding containers: %v\n", err)
			return 1
		}
		if err := c.svc.StopAllBySubType("rag", "reranker"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping reranker containers: %v\n", err)
			return 1
		}
		fmt.Println("All RAG containers stopped")
		return 0
	}

	// Stop specific embedding model
	if embedSlug != "" {
		if _, err := c.cfg.db.GetModel(embedSlug); err != nil {
			fmt.Fprintf(os.Stderr, "Embedding model not found: %s\n", embedSlug)
			return 1
		}
		fmt.Printf("Stopping embedding model: %s\n", embedSlug)
		if err := c.svc.StopModelBySlug(embedSlug); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping embedding model: %v\n", err)
			return 1
		}
	}

	// Stop specific reranker model
	if rerankSlug != "" {
		if _, err := c.cfg.db.GetModel(rerankSlug); err != nil {
			fmt.Fprintf(os.Stderr, "Reranker model not found: %s\n", rerankSlug)
			return 1
		}
		fmt.Printf("Stopping reranker model: %s\n", rerankSlug)
		if err := c.svc.StopModelBySlug(rerankSlug); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping reranker model: %v\n", err)
			return 1
		}
	}

	fmt.Println("RAG models stopped")
	return 0
}

// runList lists all RAG models (embedding and reranker) with their container status.
func (c *RagCommand) runList() int {
	embedModels, err := c.svc.ListRAGEmbeddingModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing embedding models: %v\n", err)
		return 1
	}

	rerankModels, err := c.svc.ListRAGRerankerModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing reranker models: %v\n", err)
		return 1
	}

	fmt.Println("Embedding Models:")
	if len(embedModels) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, m := range embedModels {
			status := "unknown"
			s, err := c.svc.GetModelStatus(m.Slug)
			if err == nil {
				status = s.Status
			}
			fmt.Printf("  %-30s %-30s [%s]\n", m.Slug, m.Name, status)
		}
	}

	fmt.Println()
	fmt.Println("Reranker Models:")
	if len(rerankModels) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, m := range rerankModels {
			status := "unknown"
			s, err := c.svc.GetModelStatus(m.Slug)
			if err == nil {
				status = s.Status
			}
			fmt.Printf("  %-30s %-30s [%s]\n", m.Slug, m.Name, status)
		}
	}

	return 0
}

// runInfo shows details for a specific RAG model.
// Usage: rag info <slug>
func (c *RagCommand) runInfo(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: 'info' requires a model slug")
		return 1
	}

	slug := args[0]
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Model not found: %s\n", slug)
		return 1
	}

	fmt.Printf("Name:      %s\n", model.Name)
	fmt.Printf("Slug:      %s\n", model.Slug)
	fmt.Printf("Type:      %s\n", model.Type)
	fmt.Printf("SubType:   %s\n", model.SubType)
	fmt.Printf("Container: %s\n", model.Container)
	fmt.Printf("Port:      %d\n", model.Port)
	fmt.Printf("Engine:    %s\n", model.EngineType)

	status := "unknown"
	s, err := c.svc.GetModelStatus(slug)
	if err == nil {
		status = s.Status
	}
	fmt.Printf("Status:    %s\n", status)

	if model.Capabilities != "" {
		fmt.Printf("Capabilities: %s\n", strings.ReplaceAll(model.Capabilities, "\"", ""))
	}

	return 0
}

// PrintHelp prints the rag command help.
func (c *RagCommand) PrintHelp() {
	fmt.Println(`rag - Manage RAG models (embedding + reranker) via Docker Compose.

USAGE:
  llm-manager rag [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [embed-slug] [rerank-slug]
        Start RAG containers. If slugs are omitted, starts the first
        available model of each type. Embedding is stopped before
        starting (one at a time per subtype).
  stop [embed-slug] [rerank-slug]
        Stop RAG containers. If no slugs are provided, stops all
        running RAG containers. Otherwise stops only the specified
        models.
  list  List all RAG models with their container status.
  info <slug>
        Show details for a specific RAG model.
  help  Show this help message.

EXAMPLES:
  llm-manager rag start
  llm-manager rag start bge-m3 bge-reranker
  llm-manager rag stop
  llm-manager rag stop bge-m3 bge-reranker
  llm-manager rag list
  llm-manager rag info bge-m3`)
}
