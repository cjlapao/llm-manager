// Package cmd provides the rerank subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

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
		return c.runStart()
	case "stop":
		return c.runStop()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown rerank subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts the rerank container via docker start.
func (c *RerankCommand) runStart() int {
	fmt.Println("Starting rerank container...")
	if err := c.svc.StartRerank(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting rerank container: %v\n", err)
		return 1
	}
	return 0
}

// runStop stops the rerank container if running.
func (c *RerankCommand) runStop() int {
	fmt.Println("Stopping rerank container...")
	if err := c.svc.StopRerank(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping rerank container: %v\n", err)
		return 1
	}
	fmt.Println("Rerank container stopped")
	return 0
}

// PrintHelp prints the rerank command help.
func (c *RerankCommand) PrintHelp() {
	fmt.Println(`rerank - Manage the rerank container (llm-rerank).

USAGE:
  llm-manager rerank [SUBCOMMAND]

SUBCOMMANDS:
  start   Start rerank container (docker start llm-rerank)
  stop    Stop rerank container (docker stop llm-rerank if running)

EXAMPLES:
  llm-manager rerank start
  llm-manager rerank stop

NOTES:
  The rerank container runs on port 8021.`)
}
