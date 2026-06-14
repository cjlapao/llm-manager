// Package cmd provides the stt subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("stt", func(root *RootCommand) Command { return NewSttCommand(root) })
}

// SttCommand handles STT (speech-to-text) model operations.
type SttCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewSttCommand creates a new SttCommand.
func NewSttCommand(root *RootCommand) *SttCommand {
	return &SttCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the stt command with the given subcommand and arguments.
func (c *SttCommand) Run(args []string) int {
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
		fmt.Fprintf(os.Stderr, "unknown stt subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// parseArgs extracts --default flag and optional slug from the remaining args.
// Returns (slug, useDefault, err). If useDefault is true, slug should be empty.
func parseArgs(args []string) (string, bool, error) {
	var slug string
	useDefault := false

	for _, arg := range args {
		switch arg {
		case "--default":
			useDefault = true
		default:
			if strings.HasPrefix(arg, "-") && arg != "--default" {
				return "", false, fmt.Errorf("unknown flag: %s", arg)
			}
			if slug != "" {
				return "", false, fmt.Errorf("too many positional arguments: got %q after already found slug", slug)
			}
			slug = arg
		}
	}

	return slug, useDefault, nil
}

// resolveSlug resolves a target STT model slug from user input.
// If explicitSlug is non-empty, it validates that slug exists in DB and returns it.
// If useDefault is true, it finds the first STT model with Default=true.
// If both are unset, it returns the first available STT model.
// Returns an error if no suitable model can be resolved.
func (c *SttCommand) resolveSlug(explicitSlug string, useDefault bool) (string, error) {
	models, err := c.svc.ListSTTModels()
	if err != nil {
		return "", fmt.Errorf("failed to list STT models: %w", err)
	}

	if len(models) == 0 {
		return "", fmt.Errorf("no STT models configured")
	}

	// Case 1: explicit slug provided
	if explicitSlug != "" {
		if _, err := c.cfg.db.GetModel(explicitSlug); err != nil {
			return "", fmt.Errorf("STT model not found: %s", explicitSlug)
		}
		return explicitSlug, nil
	}

	// Case 2: --default flag → find first default STT model
	if useDefault {
		for _, m := range models {
			if m.Default {
				return m.Slug, nil
			}
		}
		return "", fmt.Errorf("no STT model marked as default; run 'stt info' to see models")
	}

	// Case 3: nothing specified → first available STT model
	return models[0].Slug, nil
}

// runStart starts an STT model container.
// Usage: stt start [--default] [<slug>]
// Peer isolation: stops other running STT containers before starting the target.
func (c *SttCommand) runStart(args []string) int {
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

	// Peer isolation: stop other running STT containers before starting.
	fmt.Println("Stopping other STT containers for peer isolation...")
	if err := c.svc.StopAllBySubType("speech", "stt"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop other STT containers: %v\n", err)
	}

	fmt.Printf("Starting STT model: %s\n", resolvedSlug)
	if err := c.svc.StartModelBySlugWithAllow(resolvedSlug, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting STT model: %v\n", err)
		return 1
	}

	fmt.Printf("STT model '%s' started successfully\n", resolvedSlug)
	return 0
}

// runStop stops an STT model container.
// Usage: stt stop [--default] [<slug>]
// If neither a specific slug nor --default is given, stops all STT containers.
func (c *SttCommand) runStop(args []string) int {
	slug, useDefault, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// No specificity at all → stop all STT containers
	if slug == "" && !useDefault {
		fmt.Println("Stopping all STT containers...")
		if err := c.svc.StopAllBySubType("speech", "stt"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping STT containers: %v\n", err)
			return 1
		}
		fmt.Println("All STT containers stopped")
		return 0
	}

	// Resolve specific slug
	resolvedSlug, err := c.resolveSlug(slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping STT model: %s\n", resolvedSlug)
	if err := c.svc.StopModelBySlug(resolvedSlug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping STT model: %v\n", err)
		return 1
	}

	fmt.Printf("STT model '%s' stopped\n", resolvedSlug)
	return 0
}

// runInfo lists all STT models or shows details for one.
// Usage: stt info               — list all STT models with status
//
//	stt info <slug>         — show full details for one model
func (c *SttCommand) runInfo(args []string) int {
	if len(args) > 0 && args[0] != "" {
		return c.runInfoDetail(args[0])
	}
	return c.runInfoList()
}

// runInfoList displays all STT models in a structured table format.
func (c *SttCommand) runInfoList() int {
	models, err := c.svc.ListSTTModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing STT models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No STT models configured")
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
	fmt.Printf("Total: %d STT model(s)\n", len(models))

	return 0
}

// runInfoDetail shows full details for a single STT model by slug.
func (c *SttCommand) runInfoDetail(slug string) int {
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "STT model not found: %s\n", slug)
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

// PrintHelp prints the stt command help.
func (c *SttCommand) PrintHelp() {
	fmt.Println(`stt - Manage speech-to-text (STT) models via Docker Compose.

USAGE:
  llm-manager stt [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [--default] [<slug>]
        Start an STT model container. Stops other running STT containers
        for peer isolation before starting. If no slug is provided,
        uses '--default' to find the default STT model, or picks the
        first available STT model.
  stop [--default] [<slug>]
        Stop an STT model container. If neither a slug nor '--default'
        is provided, stops all running STT containers.
  info [slug]
        Without a slug, lists all registered STT models with their
        container status. With a slug, shows full model details.
  help  Show this help message.

EXAMPLES:
  llm-manager stt start                          # start first STT model
  llm-manager stt start --default                 # start default STT model
  llm-manager stt start whisper-large-v3         # start by slug
  llm-manager stt stop                           # stop all STT containers
  llm-manager stt stop whisper-large-v3          # stop specific STT model
  llm-manager stt info                           # list all STT models
  llm-manager stt info whisper-large-v3          # show model details`)
}

// truncateWithDots truncates s to maxLen characters, appending "…" if truncated.
func truncateWithDots(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
