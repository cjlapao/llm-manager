// Package cmd provides the llm subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("llm", func(root *RootCommand) Command { return NewLlmCommand(root) })
}

// LlmCommand manages LLM model containers (start, stop, restart, swap, status, logs).
type LlmCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewLlmCommand creates a new LlmCommand.
func NewLlmCommand(root *RootCommand) *LlmCommand {
	return &LlmCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the llm command with the given subcommand and arguments.
func (c *LlmCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "start":
		return c.runStart(args[1:])
	case "stop":
		return c.runStop(args[1:])
	case "restart":
		return c.runRestart(args[1:])
	case "swap":
		return c.runSwap(args[1:])
	case "status":
		if len(args) < 2 {
			return c.runStatusAll()
		}
		return c.runStatus(args[1])
	case "logs":
		return c.runLogs(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown llm subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

