// Package cmd provides the rag subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/service"
)

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
		return c.runStart()
	case "stop":
		return c.runStop()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown rag subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts both embed and rerank containers.
func (c *RagCommand) runStart() int {
	fmt.Println("Starting RAG services (embed + rerank)...")

	if err := c.svc.StartEmbed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting embed: %v\n", err)
		return 1
	}

	if err := c.svc.StartRerank(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting rerank: %v\n", err)
		return 1
	}

	fmt.Println("RAG services started")
	return 0
}

// runStop stops both embed and rerank containers.
func (c *RagCommand) runStop() int {
	fmt.Println("Stopping RAG services (embed + rerank)...")

	if err := c.svc.StopEmbed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping embed: %v\n", err)
		return 1
	}

	if err := c.svc.StopRerank(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping rerank: %v\n", err)
		return 1
	}

	fmt.Println("RAG services stopped")
	return 0
}

// PrintHelp prints the rag command help.
func (c *RagCommand) PrintHelp() {
	fmt.Println(`rag - Manage RAG services (embed + rerank combined).

USAGE:
  llm-manager rag [SUBCOMMAND]

SUBCOMMANDS:
  start   Start embed and rerank containers
  stop    Stop embed and rerank containers

EXAMPLES:
  llm-manager rag start
  llm-manager rag stop

NOTES:
  This command manages both embed (port 8020) and rerank (port 8021)
  containers together as a single RAG stack.`)
}
