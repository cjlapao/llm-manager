// Package cmd provides the comfyui subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/service"
)

// ComfyUICommand handles ComfyUI container operations.
type ComfyUICommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewComfyUICommand creates a new ComfyUICommand.
func NewComfyUICommand(root *RootCommand) *ComfyUICommand {
	return &ComfyUICommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the comfyui command with the given subcommand and arguments.
func (c *ComfyUICommand) Run(args []string) int {
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
		fmt.Fprintf(os.Stderr, "unknown comfyui subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts ComfyUI via profile-based compose.
func (c *ComfyUICommand) runStart() int {
	fmt.Println("Starting ComfyUI...")
	if err := c.svc.StartComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting ComfyUI: %v\n", err)
		return 1
	}
	fmt.Println("ComfyUI started")
	return 0
}

// runStop stops the ComfyUI container.
func (c *ComfyUICommand) runStop() int {
	fmt.Println("Stopping ComfyUI...")
	if err := c.svc.StopComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping ComfyUI: %v\n", err)
		return 1
	}
	fmt.Println("ComfyUI stopped")
	return 0
}

// PrintHelp prints the comfyui command help.
func (c *ComfyUICommand) PrintHelp() {
	fmt.Println(`comfyui - Manage ComfyUI via profile-based docker compose.

USAGE:
  llm-manager comfyui [SUBCOMMAND]

SUBCOMMANDS:
  start   Start ComfyUI (docker compose --profile comfyui up -d comfyui)
  stop    Stop ComfyUI container (docker stop comfyui-flux)

EXAMPLES:
  llm-manager comfyui start
  llm-manager comfyui stop`)
}
