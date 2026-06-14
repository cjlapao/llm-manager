// Package cmd provides the tts subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("tts", func(root *RootCommand) Command { return NewTtsCommand(root) })
}

// TtsCommand handles TTS (text-to-speech) model operations.
type TtsCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewTtsCommand creates a new TtsCommand.
func NewTtsCommand(root *RootCommand) *TtsCommand {
	return &TtsCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the tts command with the given subcommand and arguments.
func (c *TtsCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "start":
		return c.runStart(args[1:])
	case "stop":
		return c.runStop(args[1:])
	case "info":
		return c.runInfo(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown tts subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// resolveSlug resolves a target TTS model slug from user input.
// If explicitSlug is non-empty, it validates that slug exists in DB and returns it.
// If useDefault is true, it finds the first TTS model with Default=true.
// If both are unset, it returns the first available TTS model.
// Returns an error if no suitable model can be resolved.
func (c *TtsCommand) resolveSlug(explicitSlug string, useDefault bool) (string, error) {
	models, err := c.svc.ListTTSModels()
	if err != nil {
		return "", fmt.Errorf("failed to list TTS models: %w", err)
	}

	if len(models) == 0 {
		return "", fmt.Errorf("no TTS models configured")
	}

	// Case 1: explicit slug provided — validate it is actually a TTS model
	if explicitSlug != "" {
		model, err := c.cfg.db.GetModel(explicitSlug)
		if err != nil {
			return "", fmt.Errorf("TTS model not found: %s", explicitSlug)
		}
		if model.Type != "speech" || model.SubType != "tts" {
			return "", fmt.Errorf("model %q is not a TTS model (type=%q, subType=%q)",
				explicitSlug, model.Type, model.SubType)
		}
		return explicitSlug, nil
	}

	// Case 2: --default flag → find first default TTS model
	if useDefault {
		for _, m := range models {
			if m.Default {
				return m.Slug, nil
			}
		}
		return "", fmt.Errorf("no TTS model marked as default; run 'tts info' to see models")
	}

	// Case 3: nothing specified → first available TTS model
	return models[0].Slug, nil
}

// runStart starts a TTS model container.
// Usage: tts start [--default] [<slug>]
// Peer isolation: stops other running TTS containers before starting the target.
func (c *TtsCommand) runStart(args []string) int {
	slug, useDefault, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	resolvedSlug, err := c.resolveSlug(slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Peer isolation: stop other running TTS containers before starting.
	fmt.Println("Stopping other TTS containers for peer isolation...")
	if err := c.svc.StopAllBySubType("speech", "tts"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop other TTS containers: %v\n", err)
	}

	fmt.Printf("Starting TTS model: %s\n", resolvedSlug)
	if err := c.svc.StartModelBySlugWithAllow(resolvedSlug, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting TTS model: %v\n", err)
		return 1
	}

	fmt.Printf("TTS model '%s' started successfully\n", resolvedSlug)
	return 0
}

// runStop stops a TTS model container.
// Usage: tts stop [--default] [<slug>]
// If neither a specific slug nor --default is given, stops all TTS containers.
func (c *TtsCommand) runStop(args []string) int {
	slug, useDefault, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// No specificity at all → stop all TTS containers
	if slug == "" && !useDefault {
		fmt.Println("Stopping all TTS containers...")
		if err := c.svc.StopAllBySubType("speech", "tts"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping TTS containers: %v\n", err)
			return 1
		}
		fmt.Println("All TTS containers stopped")
		return 0
	}

	// Resolve specific slug
	resolvedSlug, err := c.resolveSlug(slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping TTS model: %s\n", resolvedSlug)
	if err := c.svc.StopModelBySlug(resolvedSlug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping TTS model: %v\n", err)
		return 1
	}

	fmt.Printf("TTS model '%s' stopped\n", resolvedSlug)
	return 0
}

// runInfo lists all TTS models or shows details for one.
// Usage: tts info               — list all TTS models with status
//
//	tts info <slug>         — show full details for one model
func (c *TtsCommand) runInfo(args []string) int {
	if len(args) > 0 && args[0] != "" {
		return c.runInfoDetail(args[0])
	}
	return c.runInfoList()
}

// runInfoList displays all TTS models in a structured table format.
func (c *TtsCommand) runInfoList() int {
	models, err := c.svc.ListTTSModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing TTS models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No TTS models configured")
		return 0
	}

	// Header
	fmt.Println(strings.Repeat("-", 95))
	fmt.Printf("%-25s %-20s %-18s %6s %s\n", "Name", "Slug", "Container", "Port", "Status(Default)")
	fmt.Println(strings.Repeat("-", 95))

	for _, m := range models {
		status := "unknown"
		if m.Container != "" {
			s, err := c.svc.GetModelStatus(m.Slug)
			if err == nil {
				status = s.Status
			}
		}
		portStr := fmt.Sprintf("%d", m.Port)
		if m.Port == 0 {
			portStr = "-"
		}
		defTag := ""
		if m.Default {
			defTag = "(default)"
		}
		fmt.Printf("%-25s %-20s %-18s %6s %s%s\n",
			truncateWithDots(m.Name, 24),
			truncateWithDots(m.Slug, 19),
			truncateWithDots(m.Container, 17),
			portStr,
			status,
			defTag)
	}
	fmt.Println(strings.Repeat("-", 95))
	fmt.Printf("Total: %d TTS model(s)\n", len(models))

	return 0
}

// runInfoDetail shows full details for a single TTS model by slug.
func (c *TtsCommand) runInfoDetail(slug string) int {
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TTS model not found: %s\n", slug)
		return 1
	}

	status := "unknown"
	s, err := c.svc.GetModelStatus(slug)
	if err == nil {
		status = s.Status
	}

	fmt.Printf("Name:          %s\n", model.Name)
	fmt.Printf("Slug:          %s\n", model.Slug)
	fmt.Printf("Type:          %s\n", model.Type)
	fmt.Printf("SubType:       %s\n", model.SubType)
	fmt.Printf("Engine:        %s\n", model.EngineType)
	fmt.Printf("Container:     %s\n", model.Container)
	fmt.Printf("Port:          %d\n", model.Port)
	fmt.Printf("HuggingFace:   %s\n", model.HFRepo)
	fmt.Printf("Status:        %s\n", status)
	if model.Default {
		fmt.Printf("(default model)\n")
	}

	if model.Capabilities != "" {
		fmt.Printf("Capabilities:  %s\n", strings.ReplaceAll(model.Capabilities, "\"", ""))
	}

	if model.CommandArgs != "" {
		fmt.Printf("CommandArgs:   %s\n", truncateWithDots(model.CommandArgs, 80))
	}

	return 0
}

// PrintHelp prints the tts command help.
func (c *TtsCommand) PrintHelp() {
	fmt.Println(`tts - Manage text-to-speech (TTS) models via Docker Compose.

USAGE:
  llm-manager tts [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [--default] [<slug>]
        Start a TTS model container. Stops other running TTS containers
        for peer isolation before starting. If no slug is provided,
        uses '--default' to find the default TTS model, or picks the
        first available TTS model.
  stop [--default] [<slug>]
        Stop a TTS model container. If neither a slug nor '--default'
        is provided, stops all running TTS containers.
  info [slug]
        Without a slug, lists all registered TTS models with their
        container status. With a slug, shows full model details.
  help  Show this help message.

EXAMPLES:
  llm-manager tts start                          # start first TTS model
  llm-manager tts start --default                 # start default TTS model
  llm-manager tts start xtts-v2                  # start by slug
  llm-manager tts stop                           # stop all TTS containers
  llm-manager tts stop xtts-v2                   # stop specific TTS model
  llm-manager tts info                           # list all TTS models
  llm-manager tts info xtts-v2                   # show model details`)
}
