// Package cmd provides the speech subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/service"
)

// SpeechCommand handles speech services (whisper-stt + kokoro-tts).
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

// runStart starts whisper-stt and kokoro-tts via profile-based compose.
func (c *SpeechCommand) runStart() int {
	fmt.Println("Starting speech services (whisper-stt + kokoro-tts)...")
	if err := c.svc.StartSpeech(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting speech services: %v\n", err)
		return 1
	}
	fmt.Println("Speech services started")
	return 0
}

// runStop stops whisper-stt and kokoro-tts containers.
func (c *SpeechCommand) runStop() int {
	fmt.Println("Stopping speech services...")
	if err := c.svc.StopSpeech(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping speech services: %v\n", err)
		return 1
	}
	fmt.Println("Speech services stopped")
	return 0
}

// PrintHelp prints the speech command help.
func (c *SpeechCommand) PrintHelp() {
	fmt.Println(`speech - Manage speech services (whisper-stt + kokoro-tts) via profile-based compose.

USAGE:
  llm-manager speech [SUBCOMMAND]

SUBCOMMANDS:
  start   Start whisper-stt and kokoro-tts (docker compose --profile speech up -d)
  stop    Stop whisper-stt and kokoro-tts containers

EXAMPLES:
  llm-manager speech start
  llm-manager speech stop`)
}
