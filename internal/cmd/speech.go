// Package cmd provides the speech subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("speech", func(root *RootCommand) Command { return NewSpeechCommand(root) })
}

// SpeechCommand handles speech services using generic model lifecycle methods.
type SpeechCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewSpeechCommand creates a new SpeechCommand.
func NewSpeechCommand(root *RootCommand) *SpeechCommand {
	return &SpeechCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the speech command with the given subcommand and arguments.
func (c *SpeechCommand) Run(args []string) int {
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
		fmt.Fprintf(os.Stderr, "unknown speech subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts all registered speech models via generic model lifecycle.
// Speech models may run simultaneously, so allowMultiple is true.
func (c *SpeechCommand) runStart() int {
	models, err := c.svc.ListSpeechModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing speech models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No speech models configured")
		return 0
	}

	for i, m := range models {
		fmt.Printf("Starting speech model %d/%d: %s...\n", i+1, len(models), m.Slug)
		if err := c.svc.StartModelBySlug(m.Slug); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting %s: %v\n", m.Slug, err)
			return 1
		}
	}

	fmt.Printf("Started %d speech model(s)\n", len(models))
	return 0
}

// runStop stops all running speech containers by subtype.
func (c *SpeechCommand) runStop() int {
	stopped := 0
	for _, subType := range []string{"stt", "tts", "omni"} {
		if err := c.svc.StopAllBySubType("speech", subType); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop %s models: %v\n", subType, err)
		} else {
			stopped++
		}
	}

	fmt.Printf("Stopped speech services (%d subtype group(s))\n", stopped)
	return 0
}

// PrintHelp prints the speech command help.
func (c *SpeechCommand) PrintHelp() {
	fmt.Println(`speech - Manage speech models (STT, TTS, Omni) via generic model lifecycle.

USAGE:
  llm-manager speech [SUBCOMMAND]

SUBCOMMANDS:
  start   Start all registered speech models from the database
  stop    Stop all running speech containers

EXAMPLES:
  llm-manager speech start
  llm-manager speech stop`)
}
