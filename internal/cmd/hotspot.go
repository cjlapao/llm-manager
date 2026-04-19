// Package cmd provides the hotspot subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/service"
)

// HotspotCommand manages the hotspot (most recently used model).
type HotspotCommand struct {
	cfg *RootCommand
	svc *service.HotspotService
}

// NewHotspotCommand creates a new HotspotCommand.
func NewHotspotCommand(root *RootCommand) *HotspotCommand {
	return &HotspotCommand{
		cfg: root,
		svc: service.NewHotspotServiceWithConfig(root.db, root.cfg),
	}
}

// Run executes the hotspot command with the given subcommand and arguments.
func (c *HotspotCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "show", "get":
		return c.runShow()
	case "set":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'set' requires a model slug\n")
			return 1
		}
		return c.runSet(args[1])
	case "clear", "unset":
		return c.runClear()
	case "stop":
		return c.runStop()
	case "restart":
		return c.runRestart()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown hotspot subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runShow displays the current hotspot.
func (c *HotspotCommand) runShow() int {
	hotspot, err := c.svc.GetCurrentHotspot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting hotspot: %v\n", err)
		return 1
	}

	if hotspot == nil {
		fmt.Println("No hotspot set. Use 'llm-manager hotspot set <slug>' to set one.")
		return 0
	}

	// Try to get model details
	model, modelErr := c.cfg.db.GetModel(hotspot.ModelSlug)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tSLUG\tACTIVE\tUPDATED")
	fmt.Fprintln(w, "-----\t----\t------\t-------")
	if modelErr == nil {
		fmt.Fprintf(w, "%s\t%s\t%v\t%s\n",
			model.Name, hotspot.ModelSlug, hotspot.Active, hotspot.UpdatedAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprintf(w, "N/A\t%s\t%v\t%s\n",
			hotspot.ModelSlug, hotspot.Active, hotspot.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	w.Flush()

	return 0
}

// runSet sets the hotspot to a specific model.
func (c *HotspotCommand) runSet(slug string) int {
	if err := c.svc.SetHotspot(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting hotspot: %v\n", err)
		return 1
	}

	fmt.Printf("Hotspot set to: %s\n", slug)
	return 0
}

// runClear removes the hotspot.
func (c *HotspotCommand) runClear() int {
	if err := c.svc.ClearHotspot(); err != nil {
		fmt.Fprintf(os.Stderr, "Error clearing hotspot: %v\n", err)
		return 1
	}

	fmt.Println("Hotspot cleared")
	return 0
}

// runStop stops the hotspot container and clears the hotspot.
func (c *HotspotCommand) runStop() int {
	hotspot, err := c.svc.GetCurrentHotspot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting hotspot: %v\n", err)
		return 1
	}

	if hotspot == nil {
		fmt.Println("No active hotspot model.")
		return 0
	}

	if err := c.svc.StopHotspot(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping hotspot: %v\n", err)
		return 1
	}

	fmt.Printf("Hotspot container stopped and cleared (was: %s)\n", hotspot.ModelSlug)
	return 0
}

// runRestart restarts the hotspot container in-place.
func (c *HotspotCommand) runRestart() int {
	hotspot, err := c.svc.GetCurrentHotspot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting hotspot: %v\n", err)
		return 1
	}

	if hotspot == nil {
		fmt.Println("No active hotspot model.")
		return 0
	}

	if err := c.svc.RestartHotspot(); err != nil {
		fmt.Fprintf(os.Stderr, "Error restarting hotspot: %v\n", err)
		return 1
	}

	fmt.Printf("Hotspot restarted successfully (model: %s)\n", hotspot.ModelSlug)
	return 0
}

// PrintHelp prints the hotspot command help.
func (c *HotspotCommand) PrintHelp() {
	fmt.Println(`hotspot - Manage the most recently used model (hotspot).

USAGE:
  llm-manager hotspot [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  show, get      Show current hotspot
  set <slug>     Set the hotspot to a model
  clear, unset   Clear the hotspot
  stop           Stop the hotspot container and clear the hotspot
  restart        Restart the hotspot container in-place

EXAMPLES:
  llm-manager hotspot show
  llm-manager hotspot set qwen3_6
  llm-manager hotspot clear
  llm-manager hotspot stop
  llm-manager hotspot restart

NOTES:
  The hotspot tracks your most recently used model for quick access.
  Only one hotspot can be active at a time.`)
}
