// Package cmd provides Omni (multimodal) CLI command handlers.
package cmd

import (
	"fmt"
	"os"
	"strings"
)

func (c *SpeechCommand) runStartOmni(args []string) int {
	slug, err := parseOmniArg(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// No --default for Omni — just use explicit slug or first available
	resolved, err := c.resolveTypedModel("omni", slug, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Println("Stopping other Omni containers for peer isolation...")
	if err := c.svc.StopAllBySubType("speech", "omni"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop other Omni containers: %v\n", err)
	}

	fmt.Printf("Starting Omni model: %s\n", resolved)
	if err := c.svc.StartModelBySlugWithAllow(resolved, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting Omni model: %v\n", err)
		return 1
	}

	fmt.Printf("Omni model '%s' started successfully\n", resolved)
	return 0
}

// runStopOmni stops an Omni model container.
func (c *SpeechCommand) runStopOmni(args []string) int {
	slug, err := parseOmniArg(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if slug == "" {
		fmt.Println("Stopping all Omni containers...")
		if err := c.svc.StopAllBySubType("speech", "omni"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping Omni containers: %v\n", err)
			return 1
		}
		fmt.Println("All Omni containers stopped")
		return 0
	}

	resolved, err := c.resolveTypedModel("omni", slug, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping Omni model: %s\n", resolved)
	if err := c.svc.StopModelBySlug(resolved); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping Omni model: %v\n", err)
		return 1
	}

	fmt.Printf("Omni model '%s' stopped\n", resolved)
	return 0
}

// runInfoOmni lists Omni models or shows details for one.
func (c *SpeechCommand) runInfoOmni(args []string) int {
	nonEmptySlug := safeSlug(args, 0)
	if nonEmptySlug != "" {
		return c.runInfoDetailOmni(nonEmptySlug)
	}
	return c.runInfoListOmni()
}

// runInfoListOmni displays all Omni models in a structured table format.
func (c *SpeechCommand) runInfoListOmni() int {
	mss, err := c.svc.ListOmniModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing Omni models: %v\n", err)
		return 1
	}

	if len(mss) == 0 {
		fmt.Println("No Omni models configured")
		return 0
	}

	c.printModelTable("Omni Models", mss, "%d Omni model(s)")
	return 0
}

// runInfoDetailOmni shows full details for a single Omni model by slug.
func (c *SpeechCommand) runInfoDetailOmni(slug string) int {
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Omni model not found: %s\n", slug)
		return 1
	}

	status := c.getContainerStatus(slug)

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

// printHelpOmni prints the omni subcommand help text.
func (c *SpeechCommand) printHelpOmni() {
	fmt.Println(`speech [stt|tts|omni] help

USAGE:
  llm-manager speech omni [SUBCOMMAND] [ARGS]

TYPE: omni
SUBCOMMANDS:
  start [<slug>]                 Start an omni model container
  stop [<slug>]                  Stop an omni model container
  info [slug]                    Without slug: list all omni models. With slug: show full details.
  help                           Show this help message

EXAMPLES:
  llm-manager speech omni start                # start first Omni model
  llm-manager speech omni start pixtral-voice   # start by slug
  llm-manager speech omni stop                 # stop all Omni containers
  llm-manager speech omni stop pixtral-voice    # stop specific Omni model
  llm-manager speech omni info                  # list all Omni models
  llm-manager speech omni info pixtral-voice    # show model details`)
}

// ----- Combined commands (unchanged from current code) -----

// runStart starts combined speech containers (up to 3: stt, tts, omni).
// Usage: speech start [--allow-multiple|-m] [stt-slug] [tts-slug] [omni-slug]
