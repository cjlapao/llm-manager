// Package cmd provides the omni subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("omni", func(root *RootCommand) Command { return NewOmniCommand(root) })
}

// OmniCommand handles Omni (multimodal speech) model operations.
type OmniCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewOmniCommand creates a new OmniCommand.
func NewOmniCommand(root *RootCommand) *OmniCommand {
	return &OmniCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the omni command with the given subcommand and arguments.
func (c *OmniCommand) Run(args []string) int {
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
		fmt.Fprintf(os.Stderr, "unknown omni subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// resolveSlug resolves a target Omni model slug from user input.
// If explicitSlug is non-empty, it validates that slug exists in DB and returns it.
// If useDefault is true, it finds the first Omni model with Default=true.
// If both are unset, it returns the first available Omni model.
// Returns an error if no suitable model can be resolved.
func (c *OmniCommand) resolveSlug(explicitSlug string, useDefault bool) (string, error) {
	models, err := c.svc.ListOmniModels()
	if err != nil {
		return "", fmt.Errorf("failed to list Omni models: %w", err)
	}

	if len(models) == 0 {
		return "", fmt.Errorf("no Omni models configured")
	}

	// Case 1: explicit slug provided — validate it is actually an Omni model
	if explicitSlug != "" {
		model, err := c.cfg.db.GetModel(explicitSlug)
		if err != nil {
			return "", fmt.Errorf("Omni model not found: %s", explicitSlug)
		}
		if model.Type != "speech" || model.SubType != "omni" {
			return "", fmt.Errorf("model %q is not an Omni model (type=%q, subType=%q)",
				explicitSlug, model.Type, model.SubType)
		}
		return explicitSlug, nil
	}

	// Case 2: --default flag → find first default Omni model
	if useDefault {
		for _, m := range models {
			if m.Default {
				return m.Slug, nil
			}
		}
		return "", fmt.Errorf("no Omni model marked as default; run 'omni info' to see models")
	}

	// Case 3: nothing specified → first available Omni model
	return models[0].Slug, nil
}

// runStart starts an Omni model container.
// Usage: omni start [--default] [<slug>]
// Peer isolation: stops other running Omni containers before starting the target.
func (c *OmniCommand) runStart(args []string) int {
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

	// Peer isolation: stop other running Omni containers before starting.
	fmt.Println("Stopping other Omni containers for peer isolation...")
	if err := c.svc.StopAllBySubType("speech", "omni"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop other Omni containers: %v\n", err)
	}

	fmt.Printf("Starting Omni model: %s\n", resolvedSlug)
	if err := c.svc.StartModelBySlugWithAllow(resolvedSlug, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting Omni model: %v\n", err)
		return 1
	}

	fmt.Printf("Omni model '%s' started successfully\n", resolvedSlug)
	return 0
}

// runStop stops an Omni model container.
// Usage: omni stop [--default] [<slug>]
// If neither a specific slug nor --default is given, stops all Omni containers.
func (c *OmniCommand) runStop(args []string) int {
	slug, useDefault, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// No specificity at all → stop all Omni containers
	if slug == "" && !useDefault {
		fmt.Println("Stopping all Omni containers...")
		if err := c.svc.StopAllBySubType("speech", "omni"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping Omni containers: %v\n", err)
			return 1
		}
		fmt.Println("All Omni containers stopped")
		return 0
	}

	// Resolve specific slug
	resolvedSlug, err := c.resolveSlug(slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping Omni model: %s\n", resolvedSlug)
	if err := c.svc.StopModelBySlug(resolvedSlug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping Omni model: %v\n", err)
		return 1
	}

	fmt.Printf("Omni model '%s' stopped\n", resolvedSlug)
	return 0
}

// runInfo lists all Omni models or shows details for one.
// Usage: omni info               — list all Omni models with status
//
//	omni info <slug>         — show full details for one model
func (c *OmniCommand) runInfo(args []string) int {
	if len(args) > 0 && args[0] != "" {
		return c.runInfoDetail(args[0])
	}
	return c.runInfoList()
}

// runInfoList displays all Omni models in a structured table format.
func (c *OmniCommand) runInfoList() int {
	models, err := c.svc.ListOmniModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing Omni models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No Omni models configured")
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
	fmt.Printf("Total: %d Omni model(s)\n", len(models))

	return 0
}

// runInfoDetail shows full details for a single Omni model by slug.
func (c *OmniCommand) runInfoDetail(slug string) int {
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Omni model not found: %s\n", slug)
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

// PrintHelp prints the omni command help.
func (c *OmniCommand) PrintHelp() {
	fmt.Println(`omni - Manage multimodal Omni models via Docker Compose.

USAGE:
  llm-manager omni [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [--default] [<slug>]
        Start an Omni model container. Stops other running Omni containers
        for peer isolation before starting. If no slug is provided,
        uses '--default' to find the default Omni model, or picks the
        first available Omni model.
  stop [--default] [<slug>]
        Stop an Omni model container. If neither a slug nor '--default'
        is provided, stops all running Omni containers.
  info [slug]
        Without a slug, lists all registered Omni models with their
        container status. With a slug, shows full model details.
  help  Show this help message.

EXAMPLES:
  llm-manager omni start                          # start first Omni model
  llm-manager omni start --default                 # start default Omni model
  llm-manager omni start pixtral-voice             # start by slug
  llm-manager omni stop                           # stop all Omni containers
  llm-manager omni stop pixtral-voice             # stop specific Omni model
  llm-manager omni info                           # list all Omni models
  llm-manager omni info pixtral-voice             # show model details`)
}
