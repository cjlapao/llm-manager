// Package cmd provides the embed subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

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
		return c.runStart()
	case "stop":
		return c.runStop()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown embed subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts the embed container via docker start.
func (c *EmbedCommand) runStart() int {
	fmt.Println("Starting embed container...")
	if err := c.svc.StartEmbed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting embed container: %v\n", err)
		return 1
	}
	return 0
}

// runStop stops the embed container if running.
func (c *EmbedCommand) runStop() int {
	fmt.Println("Stopping embed container...")
	if err := c.svc.StopEmbed(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping embed container: %v\n", err)
		return 1
	}
	fmt.Println("Embed container stopped")
	return 0
}

// PrintHelp prints the embed command help.
func (c *EmbedCommand) PrintHelp() {
	fmt.Println(`embed - Manage the embed container (llm-embed).

USAGE:
  llm-manager embed [SUBCOMMAND]

SUBCOMMANDS:
  start   Start embed container (docker start llm-embed)
  stop    Stop embed container (docker stop llm-embed if running)

EXAMPLES:
  llm-manager embed start
  llm-manager embed stop

NOTES:
  The embed container runs on port 8020.`)
}
