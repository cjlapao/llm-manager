// Package cmd provides the swap subcommand for llm-manager container operations.
package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("swap", func(root *RootCommand) Command { return NewContainerSwapCommand(root) })
}

// ContainerSwapCommand handles the container swap operation.
type ContainerSwapCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewContainerSwapCommand creates a new ContainerSwapCommand.
func NewContainerSwapCommand(root *RootCommand) *ContainerSwapCommand {
	return &ContainerSwapCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the swap command with the given slug argument.
func (c *ContainerSwapCommand) Run(args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: 'swap' requires a model slug\n")
		c.PrintHelp()
		return 1
	}

	slug := args[0]
	return c.runSwap(slug)
}

// runSwap performs a GPU-safe model swap.
func (c *ContainerSwapCommand) runSwap(slug string) int {
	fmt.Printf("Swapping to model: %s\n", slug)

	// Step 1: Stop all LLM containers
	fmt.Println("Stopping all LLM containers...")
	if err := c.svc.StopAllLLMs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping LLM containers: %v\n", err)
		return 1
	}

	// Step 2: Remove active flux and 3D files
	fmt.Println("Removing active model files...")
	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}
	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	// Step 3: Drop OS page cache
	fmt.Println("Dropping OS page cache...")
	if err := c.svc.DropPageCache(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not drop page cache: %v\n", err)
	}

	// Step 4: Start the target model
	fmt.Printf("Starting model: %s\n", slug)
	if err := c.svc.StartContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting container: %v\n", err)
		return 1
	}

	// Step 5: Set the hotspot
	fmt.Printf("Setting hotspot to: %s\n", slug)
	if err := c.cfg.db.SetHotspot(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set hotspot: %v\n", err)
	}

	fmt.Printf("Successfully swapped to: %s\n", slug)
	return 0
}

// PrintHelp prints the swap command help.
func (c *ContainerSwapCommand) PrintHelp() {
	fmt.Println(`swap - GPU-safe model swap (stop all LLMs, drop cache, start target).

USAGE:
  llm-manager container swap <slug>

DESCRIPTION:
  Performs a GPU-safe swap to a new model:
    1. Stops all LLM-type containers
    2. Removes active flux and 3D model files
    3. Drops OS page cache via sync && echo 3 > /proc/sys/vm/drop_caches
    4. Starts the target model via docker compose
    5. Sets the hotspot to the new model

EXAMPLES:
  llm-manager container swap qwen3_6
  llm-manager container swap llama3_1`)
}
