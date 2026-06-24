// Package cmd provides STT (Speech-to-Text) CLI command handlers.
package cmd

import (
	"fmt"
	"os"
	"strings"
)

func (c *SpeechCommand) runStartSTT(args []string) int {
	slug, useDefault, err := parseTypedArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	resolved, err := c.resolveTypedModel("stt", slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Println("Stopping other STT containers for peer isolation...")
	if err := c.svc.StopAllBySubType("speech", "stt"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop other STT containers: %v\n", err)
	}

	fmt.Printf("Starting STT model: %s\n", resolved)
	if err := c.svc.StartModelBySlugWithAllow(resolved, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting STT model: %v\n", err)
		return 1
	}

	fmt.Printf("STT model '%s' started successfully\n", resolved)
	return 0
}

// runStopSTT stops an STT model container.
func (c *SpeechCommand) runStopSTT(args []string) int {
	slug, useDefault, err := parseTypedArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if slug == "" && !useDefault {
		fmt.Println("Stopping all STT containers...")
		if err := c.svc.StopAllBySubType("speech", "stt"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping STT containers: %v\n", err)
			return 1
		}
		fmt.Println("All STT containers stopped")
		return 0
	}

	resolved, err := c.resolveTypedModel("stt", slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping STT model: %s\n", resolved)
	if err := c.svc.StopModelBySlug(resolved); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping STT model: %v\n", err)
		return 1
	}

	fmt.Printf("STT model '%s' stopped\n", resolved)
	return 0
}

// runInfoSTT lists STT models or shows details for one.
func (c *SpeechCommand) runInfoSTT(args []string) int {
	nonEmptySlug := safeSlug(args, 0)
	if nonEmptySlug != "" {
		return c.runInfoDetailSTT(nonEmptySlug)
	}
	return c.runInfoListSTT()
}

// runInfoListSTT displays all STT models in a structured table format.
func (c *SpeechCommand) runInfoListSTT() int {
	mss, err := c.svc.ListSTTModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing STT models: %v\n", err)
		return 1
	}

	if len(mss) == 0 {
		fmt.Println("No STT models configured")
		return 0
	}

	c.printModelTable(mss)
	fmt.Printf("\nTotal: %d STT model(s)\n", len(mss))
	return 0
}

// runInfoDetailSTT shows full details for a single STT model by slug.
func (c *SpeechCommand) runInfoDetailSTT(slug string) int {
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "STT model not found: %s\n", slug)
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

// printHelpSTT prints the stt subcommand help text.
func (c *SpeechCommand) printHelpSTT() {
	fmt.Println(`speech [stt|tts|omni] help

USAGE:
  llm-manager speech stt [SUBCOMMAND] [ARGS]

TYPE: stt
SUBCOMMANDS:
  start [--default] [<slug>]    Start a stt model container
  stop [--default] [<slug>]     Stop a stt model container
  ls [slug]                     Without slug: list all stt models. With slug: show full details.
  info [slug]                   Alias for ls.
  help                          Show this help message

EXAMPLES:
  llm-manager speech stt start                     # start first STT model
  llm-manager speech stt start --default            # start default STT model
  llm-manager speech stt start whisper-large-v3     # start by slug
  llm-manager speech stt stop                       # stop all STT containers
  llm-manager speech stt stop whisper-large-v3      # stop specific STT model
  llm-manager speech stt ls                         # list all STT models
  llm-manager speech stt ls whisper-large-v3        # show model details`)
}

// ----- TTS type-specific handlers -----

// runStartTTS starts a TTS model container.
