// Package cmd provides the speech subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("speech", func(root *RootCommand) Command { return NewSpeechCommand(root) })
}

// SpeechCommand handles unified speech model operations (STT + TTS + Omni).
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
		return c.runStart(args[1:])
	case "stop":
		return c.runStop(args[1:])
	case "info":
		return c.runInfo()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	case "stt":
		return c.runStartSTTDispatcher(args[1:])
	case "tts":
		return c.runStartTTSDispatcher(args[1:])
	case "omni":
		return c.runStartOmniDispatcher(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown speech subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// ----- Argument parsers -----

// parseSpeechArgs extracts the --allow-multiple (-m) flag and up to 3 positional
// slug arguments from the provided slice. Extras beyond 3 are silently ignored.
func parseSpeechArgs(args []string) (allowMultiple bool, slugs []string) {
	for _, arg := range args {
		switch arg {
		case "--allow-multiple", "-m":
			allowMultiple = true
		default:
			if !strings.HasPrefix(arg, "-") {
				if len(slugs) < 3 {
					slugs = append(slugs, arg)
				}
			}
		}
	}
	return allowMultiple, slugs
}

// parseTypedArgs extracts --default flag and optional slug from args.
// Used by STT/TTS typed handlers.
func parseTypedArgs(args []string) (slug string, useDefault bool, err error) {
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

// parseOmniArg extracts an optional single positional slug from args.
// Used by Omnic typed handler (no --default support).
func parseOmniArg(args []string) (slug string, err error) {
	if len(args) > 1 {
		return "", fmt.Errorf("too many positional arguments: %v", args)
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], nil
}

// ----- Helpers -----

// safeSlug returns the slug at the given index if it exists, else empty string.
func safeSlug(slugs []string, idx int) string {
	if idx < len(slugs) {
		return slugs[idx]
	}
	return ""
}

// resolveSTT returns the first STT model's slug (default-first, then any).
func (c *SpeechCommand) resolveSTT() string {
	ms, err := c.svc.ListSTTModels()
	if err != nil || len(ms) == 0 {
		return ""
	}
	for _, m := range ms {
		if m.Default {
			return m.Slug
		}
	}
	return ms[0].Slug
}

// resolveTTS returns the first TTS model's slug (default-first, then any).
func (c *SpeechCommand) resolveTTS() string {
	ms, err := c.svc.ListTTSModels()
	if err != nil || len(ms) == 0 {
		return ""
	}
	for _, m := range ms {
		if m.Default {
			return m.Slug
		}
	}
	return ms[0].Slug
}

// resolveOmni returns the first Omni model's slug (default-first, then any).
func (c *SpeechCommand) resolveOmni() string {
	ms, err := c.svc.ListOmniModels()
	if err != nil || len(ms) == 0 {
		return ""
	}
	for _, m := range ms {
		if m.Default {
			return m.Slug
		}
	}
	return ms[0].Slug
}

// truncateWithDots truncates s to maxLen characters, appending "\u2026" if truncated.
func truncateWithDots(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "\u2026"
}

// resolverTypedModel resolves a model slug for the given subtype.
// explicitSlug: user-provided slug (may be empty), useDefault: whether to pick default.
func (c *SpeechCommand) resolveTypedModel(subType string, explicitSlug string, useDefault bool) (string, error) {
	var mss []models.Model
	var err error
	switch subType {
	case "stt":
		mss, err = c.svc.ListSTTModels()
		if err != nil {
			return "", fmt.Errorf("failed to list STT models: %w", err)
		}
	case "tts":
		mss, err = c.svc.ListTTSModels()
		if err != nil {
			return "", fmt.Errorf("failed to list TTS models: %w", err)
		}
	case "omni":
		mss, err = c.svc.ListOmniModels()
		if err != nil {
			return "", fmt.Errorf("failed to list Omni models: %w", err)
		}
	default:
		return "", fmt.Errorf("unknown subtype: %s", subType)
	}

	if len(mss) == 0 {
		return "", fmt.Errorf("no %s models configured", subType)
	}

	if explicitSlug != "" {
		model, err := c.cfg.db.GetModel(explicitSlug)
		if err != nil {
			titleSub := strings.Title(subType)
			return "", fmt.Errorf("%s model not found: %s", titleSub, explicitSlug)
		}
		if model.Type != "speech" || model.SubType != subType {
			return "", fmt.Errorf("model %q is not a %s model (type=%q, subType=%q)",
				explicitSlug, strings.Title(subType), model.Type, model.SubType)
		}
		return explicitSlug, nil
	}

	if useDefault {
		for _, m := range mss {
			if m.Default {
				return m.Slug, nil
			}
		}
		return "", fmt.Errorf("no %s model marked as default; run 'speech %s info' to see models", subType, subType)
	}

	return mss[0].Slug, nil
}

// startedContainer tracks a container that was successfully started, used for rollback.
type startedContainer struct {
	subType string
	slug    string
}

// rollbackStarted stops all containers in the started slice in reverse order.
func (c *SpeechCommand) rollbackStarted(started []startedContainer) {
	for i := len(started) - 1; i >= 0; i-- {
		s := started[i]
		label := strings.ToUpper(s.subType)
		fmt.Printf("Rolling back %s model: %s\n", label, s.slug)
		if err := c.svc.StopModelBySlug(s.slug); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: rollback stop of %s failed: %v\n", s.slug, err)
		}
	}
}

// ----- Type dispatchers for stt/tts/omni routing -----

// runStartSTTDispatcher is the entry point for "speech stt <sub>" commands.
// routes to the appropriate method based on the second-level subcommand.
func (c *SpeechCommand) runStartSTTDispatcher(args []string) int {
	if len(args) == 0 {
		c.printHelpSTT()
		return 0
	}
	switch args[0] {
	case "start":
		return c.runStartSTT(args[1:])
	case "stop":
		return c.runStopSTT(args[1:])
	case "info":
		return c.runInfoSTT(args[1:])
	case "help", "-h", "--help":
		c.printHelpSTT()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown stt subcommand: %s\n\n", args[0])
		c.printHelpSTT()
		return 1
	}
}

// runStartTTSDispatcher is the entry point for "speech tts <sub>" commands.
func (c *SpeechCommand) runStartTTSDispatcher(args []string) int {
	if len(args) == 0 {
		c.printHelpTTS()
		return 0
	}
	switch args[0] {
	case "start":
		return c.runStartTTS(args[1:])
	case "stop":
		return c.runStopTTS(args[1:])
	case "info":
		return c.runInfoTTS(args[1:])
	case "help", "-h", "--help":
		c.printHelpTTS()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown tts subcommand: %s\n\n", args[0])
		c.printHelpTTS()
		return 1
	}
}

// runStartOmniDispatcher is the entry point for "speech omni <sub>" commands.
func (c *SpeechCommand) runStartOmniDispatcher(args []string) int {
	if len(args) == 0 {
		c.printHelpOmni()
		return 0
	}
	switch args[0] {
	case "start":
		return c.runStartOmni(args[1:])
	case "stop":
		return c.runStopOmni(args[1:])
	case "info":
		return c.runInfoOmni(args[1:])
	case "help", "-h", "--help":
		c.printHelpOmni()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown omni subcommand: %s\n\n", args[0])
		c.printHelpOmni()
		return 1
	}
}

// ----- STT type-specific handlers -----

// runStartSTT starts an STT model container.
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

	c.printModelTable("STT Models", mss, "%d STT model(s)")
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
  info [slug]                   Without slug: list all stt models. With slug: show full details.
  help                          Show this help message

EXAMPLES:
  llm-manager speech stt start                     # start first STT model
  llm-manager speech stt start --default            # start default STT model
  llm-manager speech stt start whisper-large-v3     # start by slug
  llm-manager speech stt stop                       # stop all STT containers
  llm-manager speech stt stop whisper-large-v3      # stop specific STT model
  llm-manager speech stt info                        # list all STT models
  llm-manager speech stt info whisper-large-v3      # show model details`)
}

// ----- TTS type-specific handlers -----

// runStartTTS starts a TTS model container.
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

	c.printModelTable("TTS Models", mss, "%d TTS model(s)")
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
  info [slug]                   Without slug: list all tts models. With slug: show full details.
  help                          Show this help message

EXAMPLES:
  llm-manager speech tts start                    # start first TTS model
  llm-manager speech tts start --default           # start default TTS model
  llm-manager speech tts start xtts-v2             # start by slug
  llm-manager speech tts stop                      # stop all TTS containers
  llm-manager speech tts stop xtts-v2              # stop specific TTS model
  llm-manager speech tts info                      # list all TTS models
  llm-manager speech tts info xtts-v2              # show model details`)
}

// ----- Omni type-specific handlers -----

// runStartOmni starts an Omni model container.
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
func (c *SpeechCommand) runStart(args []string) int {
	allowMultiple, slugs := parseSpeechArgs(args)

	tgt := struct {
		stt  string
		tts  string
		omni string
	}{
		stt:  safeSlug(slugs, 0),
		tts:  safeSlug(slugs, 1),
		omni: safeSlug(slugs, 2),
	}

	if tgt.stt == "" {
		tgt.stt = c.resolveSTT()
	}
	if tgt.tts == "" {
		tgt.tts = c.resolveTTS()
	}
	if tgt.omni == "" {
		tgt.omni = c.resolveOmni()
	}

	type step struct {
		subType string
		slug    string
		label   string
	}
	var steps []step
	if tgt.stt != "" {
		steps = append(steps, step{"stt", tgt.stt, "STT"})
	}
	if tgt.tts != "" {
		steps = append(steps, step{"tts", tgt.tts, "TTS"})
	}
	if tgt.omni != "" {
		steps = append(steps, step{"omni", tgt.omni, "Omni"})
	}

	if len(steps) == 0 {
		fmt.Fprintln(os.Stderr, "No speech models configured; nothing to start")
		return 1
	}

	for _, s := range steps {
		if _, err := c.cfg.db.GetModel(s.slug); err != nil {
			fmt.Fprintf(os.Stderr, "Model not found: %s\n", s.slug)
			return 1
		}
	}

	var started []startedContainer

	for i, s := range steps {
		isLast := i == len(steps)-1
		fmt.Printf("Starting %s model: %s\n", s.label, s.slug)

		if !allowMultiple {
			fmt.Printf("Stopping other %s containers...\n", s.label)
			if err := c.svc.StopAllBySubType("speech", s.subType); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to stop other %s containers: %v\n", s.label, err)
			}
		}

		err := c.svc.StartModelBySlugWithAllow(s.slug, !allowMultiple)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting %s model: %v\n", s.label, err)
			c.rollbackStarted(started)
			return 1
		}

		fmt.Printf("%s model '%s' started\n", s.label, s.slug)
		started = append(started, startedContainer{subType: s.subType, slug: s.slug})

		if !isLast {
			fmt.Println("---")
		}
	}

	numWords := "models"
	if len(steps) == 1 {
		numWords = "model"
	}
	fmt.Printf("Speech %s (%d) started\n", numWords, len(steps))
	return 0
}

// runStop stops combined speech containers.
// Usage: speech stop [stt-slug] [tts-slug] [omni-slug]
func (c *SpeechCommand) runStop(args []string) int {
	_, slugs := parseSpeechArgs(args)

	tgt := struct {
		stt  string
		tts  string
		omni string
	}{
		stt:  safeSlug(slugs, 0),
		tts:  safeSlug(slugs, 1),
		omni: safeSlug(slugs, 2),
	}

	hasAny := tgt.stt != "" || tgt.tts != "" || tgt.omni != ""

	if !hasAny {
		fmt.Println("Stopping all speech containers...")
		for _, subType := range []string{"stt", "tts", "omni"} {
			if err := c.svc.StopAllBySubType("speech", subType); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping %s containers: %v\n", subType, err)
			}
		}
		fmt.Println("All speech containers stopped")
		return 0
	}

	type step struct {
		label string
		slug  string
	}
	var steps []step
	if tgt.stt != "" {
		steps = append(steps, step{"STT", tgt.stt})
	}
	if tgt.tts != "" {
		steps = append(steps, step{"TTS", tgt.tts})
	}
	if tgt.omni != "" {
		steps = append(steps, step{"Omni", tgt.omni})
	}

	for _, s := range steps {
		fmt.Printf("Stopping %s model: %s\n", s.label, s.slug)
		if _, err := c.cfg.db.GetModel(s.slug); err != nil {
			fmt.Fprintf(os.Stderr, "%s model not found: %s\n", s.label, s.slug)
			continue
		}
		if err := c.svc.StopModelBySlug(s.slug); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping %s model: %v\n", s.label, err)
		}
	}

	fmt.Printf("Specified speech model(s) stopped\n")
	return 0
}

// runInfo displays all speech models grouped by subtype (STT, TTS, Omni)
// in a table format showing name, slug, container, port, and status.
func (c *SpeechCommand) runInfo() int {
	sttModels, err := c.svc.ListSTTModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing STT models: %v\n", err)
		return 1
	}

	ttsModels, err := c.svc.ListTTSModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing TTS models: %v\n", err)
		return 1
	}

	omniModels, err := c.svc.ListOmniModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing Omni models: %v\n", err)
		return 1
	}

	printSection := func(title string, mList []models.Model) {
		fmt.Printf("\n%s:\n", title)
		if len(mList) == 0 {
			fmt.Println("  (none)")
			return
		}
		c.printModelTable(title, mList, "")
	}

	printSection("STT Models", sttModels)
	printSection("TTS Models", ttsModels)
	printSection("Omni Models", omniModels)

	total := len(sttModels) + len(ttsModels) + len(omniModels)
	fmt.Printf("Total: %d speech model(s)\n", total)
	return 0
}

// ----- Shared output helpers -----

// getContainerStatus returns the running status of a model by slug.
func (c *SpeechCommand) getContainerStatus(slug string) string {
	s, err := c.svc.GetModelStatus(slug)
	if err != nil {
		return "unknown"
	}
	return s.Status
}

// printModelTable prints a model group as a formatted table.
// If footerFmt is non-empty it is printed as a summary line after the table.
func (c *SpeechCommand) printModelTable(title string, mList []models.Model, footerFmt string) {
	sep := strings.Repeat("-", 95)
	fmt.Println(sep)
	fmt.Printf("%-25s %-20s %-18s %6s %s\n", "Name", "Slug", "Container", "Port", "Status(Default)")
	fmt.Println(sep)
	for _, m := range mList {
		status := "unknown"
		if m.Container != "" {
			status = c.getContainerStatus(m.Slug)
		}
		portStr := "-"
		if m.Port != 0 {
			portStr = fmt.Sprintf("%d", m.Port)
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
	fmt.Println(sep)
	if footerFmt != "" {
		fmt.Printf(footerFmt+"\n", len(mList))
	}
	_ = title // consumed by caller via section header
}

// PrintHelp prints the speech command top-level help.
func (c *SpeechCommand) PrintHelp() {
	fmt.Println(`speech - Manage speech models (STT + TTS + Omni) via Docker Compose.

USAGE:
  llm-manager speech [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [--allow-multiple|-m] [stt-slug] [tts-slug] [omni-slug]
        Start speech containers. Up to three positional slugs map to
        subtypes in order: STT, TTS, Omni. Omitted slugs auto-resolve
        to the default model of that subtype. --allow-multiple skips
        per-subtype peer isolation.
  stop [stt-slug] [tts-slug] [omni-slug]
        Stop speech containers. Up to three positional slugs target
        specific models. If no slugs are provided, stops all running
        speech containers across all subtypes.
  info  List all registered speech models (STT, TTS, Omni) grouped
        by subtype, with their container status.
  help  Show this help message.

TYPE-SPECIFIC COMMANDS:
  llm-manager speech stt [start|stop|info|help]    Manage STT (speech-to-text) models
  llm-manager speech tts [start|stop|info|help]    Manage TTS (text-to-speech) models
  llm-manager speech omni [start|stop|info|help]   Manage Omni (multimodal) models

EXAMPLES:
  llm-manager speech start                              # start defaults
  llm-manager speech start whisper-large-v3             # start STT only
  llm-manager speech start whisper-large-v3 xtts-v2    # start STT+TTS
  llm-manager speech start -m whisper xtts pixtral     # start all 3
  llm-manager speech stop                               # stop all
  llm-manager speech stop whisper-large-v3 xtts-v2     # stop specific
  llm-manager speech info                                # list all
  llm-manager speech stt start                           # start first STT
  llm-manager speech tts start --default                # start default TTS
  llm-manager speech omni start                          # start first Omni`)
}
