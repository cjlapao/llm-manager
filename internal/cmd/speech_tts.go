// Package cmd provides TTS (Text-to-Speech) CLI command handlers.
package cmd

import (
	"fmt"
	"os"
	"strings"
)

func (c *SpeechCommand) runStartTTS(args []string) int {
	slug, useDefault, err := parseTypedArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	resolved, err := c.resolveTypedModel("tts", slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Println("Stopping other TTS containers for peer isolation...")
	if err := c.svc.StopAllBySubType("speech", "tts"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop other TTS containers: %v\n", err)
	}

	fmt.Printf("Starting TTS model: %s\n", resolved)
	if err := c.svc.StartModelBySlugWithAllow(resolved, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting TTS model: %v\n", err)
		return 1
	}

	fmt.Printf("TTS model '%s' started successfully\n", resolved)
	return 0
}

// runStopTTS stops a TTS model container.
func (c *SpeechCommand) runStopTTS(args []string) int {
	slug, useDefault, err := parseTypedArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if slug == "" && !useDefault {
		fmt.Println("Stopping all TTS containers...")
		if err := c.svc.StopAllBySubType("speech", "tts"); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping TTS containers: %v\n", err)
			return 1
		}
		fmt.Println("All TTS containers stopped")
		return 0
	}

	resolved, err := c.resolveTypedModel("tts", slug, useDefault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Stopping TTS model: %s\n", resolved)
	if err := c.svc.StopModelBySlug(resolved); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping TTS model: %v\n", err)
		return 1
	}

	fmt.Printf("TTS model '%s' stopped\n", resolved)
	return 0
}

// runInfoTTS lists TTS models or shows details for one.
func (c *SpeechCommand) runInfoTTS(args []string) int {
	nonEmptySlug := safeSlug(args, 0)
	if nonEmptySlug != "" {
		return c.runInfoDetailTTS(nonEmptySlug)
	}
	return c.runInfoListTTS()
}

// runInfoListTTS displays all TTS models in a structured table format.
func (c *SpeechCommand) runInfoListTTS() int {
	mss, err := c.svc.ListTTSModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing TTS models: %v\n", err)
		return 1
	}

	if len(mss) == 0 {
		fmt.Println("No TTS models configured")
		return 0
	}

	c.printModelTable(mss)
	fmt.Printf("\nTotal: %d TTS model(s)\n", len(mss))
	return 0
}

// run-infoDetailTTS shows full details for a single TTS model by slug.
func (c *SpeechCommand) runInfoDetailTTS(slug string) int {
	model, err := c.cfg.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TTS model not found: %s\n", slug)
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

// printHelpTTS prints the tts subcommand help text.
func (c *SpeechCommand) printHelpTTS() {
	fmt.Println(`speech [stt|tts|omni] help

USAGE:
  llm-manager speech tts [SUBCOMMAND] [ARGS]

TYPE: tts
SUBCOMMANDS:
  start [--default] [<slug>]    Start a tts model container
  stop [--default] [<slug>]     Stop a tts model container
  ls [slug]                     Without slug: list all tts models. With slug: show full details.
  info [slug]                   Alias for ls.
  help                          Show this help message

EXAMPLES:
  llm-manager speech tts start                    # start first TTS model
  llm-manager speech tts start --default           # start default TTS model
  llm-manager speech tts start xtts-v2             # start by slug
  llm-manager speech tts stop                      # stop all TTS containers
  llm-manager speech tts stop xtts-v2              # stop specific TTS model
  llm-manager speech tts ls                        # list all TTS models
  llm-manager speech tts ls xtts-v2                # show model details`)
}

// ----- Omni type-specific handlers -----

// runStartOmni starts an Omni model container.
