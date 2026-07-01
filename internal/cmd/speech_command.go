// Package cmd provides the speech subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("speech", func(root *RootCommand) Command { return NewSpeechCommand(root) })
}

// SpeechCommand handles unified speech model operations (STT + TTS + Omni).
type SpeechCommand struct {
	cfg    *RootCommand
	svc    *service.ContainerService
	litellm service.LiteLLMActivator
}

// NewSpeechCommand creates a new SpeechCommand.
func NewSpeechCommand(root *RootCommand) *SpeechCommand {
	containerSvc := service.NewContainerService(root.db, root.cfg)
	configSvc := service.NewConfigService(root.db)
	litellmSvc := service.NewLiteLLMService(root.db, root.cfg, configSvc)
	containerSvc.SetLiteLLMService(litellmSvc)
	return &SpeechCommand{
		cfg:    root,
		svc:    containerSvc,
		litellm: litellmSvc,
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
	case "ls":
		return c.runInfo()
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
		return "", fmt.Errorf("no %s model marked as default; run 'speech %s ls' to see models", subType, subType)
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
	case "ls":
		return c.runInfoSTT(args[1:])
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
	case "ls":
		return c.runInfoTTS(args[1:])
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
	case "ls":
		return c.runInfoOmni(args[1:])
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

	// Activate LiteLLM aliases for each started speech model
	if c.litellm != nil {
		fmt.Println()
		for _, s := range started {
			fmt.Printf("Activating %s alias for: %s\n", s.subType, s.slug)
			if err := c.litellm.ActivateSpeechRAGModel(s.slug, s.subType); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to activate %s alias for %s: %v\n", s.subType, s.slug, err)
			}
		}
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

// runInfo lists all speech models in a single unified table sorted by slug.
func (c *SpeechCommand) runInfo() int {
	modelList, err := c.svc.ListSpeechModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing speech models: %v\n", err)
		return 1
	}

	if len(modelList) == 0 {
		fmt.Println("(none)")
		return 0
	}

	// Sort by slug for consistent ordering
	sort.Slice(modelList, func(i, j int) bool {
		return modelList[i].Slug < modelList[j].Slug
	})

	c.printModelTable(modelList)
	fmt.Printf("\nTotal: %d speech model(s)\n", len(modelList))
	return 0
}

// getContainerStatus returns the running status of a model by slug.
func (c *SpeechCommand) getContainerStatus(slug string) string {
	s, err := c.svc.GetModelStatus(slug)
	if err != nil {
		return "unknown"
	}
	return s.Status
}

// printModelTable prints a model list as a formatted table using tabwriter.
// Column layout matches `models ls`: SLUG, TYPE, SUBTYPE, NAME, PORT, STATUS, CACHED, ENGINE.
func (c *SpeechCommand) printModelTable(mList []models.Model) {
	if len(mList) == 0 {
		fmt.Println("(none)")
		return
	}

	containerSvc := service.NewContainerService(c.cfg.db, c.cfg.cfg)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintln(w, "SLUG\tTYPE\tSUBTYPE\tNAME\tPORT\tSTATUS\tCACHED\tENGINE")
	fmt.Fprintln(w, "----\t----\t-------\t----\t----\t------\t------\t------")

	for _, m := range mList {
		status := inspectContainerState(m.GetContainerName())

		// Check cached
		var cached string
		if m.HFRepo != "" && containerSvc != nil {
			cacheInfo := containerSvc.HFCacheSize(m.HFRepo)
			if cacheInfo.Cached {
				cached = service.FormatVRAM(uint64(cacheInfo.Size))
			} else {
				cached = "no"
			}
		} else {
			cached = "\u2014"
		}

		engine := m.EngineType
		if engine == "" {
			engine = "vllm"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			m.Slug,
			m.Type,
			m.SubType,
			truncateWithDots(m.Name, 24),
			m.Port,
			status,
			cached,
			engine)
	}
	w.Flush()
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
  ls           List all registered speech models in a single table
               (STT, TTS, Omni), sorted by slug, with container status.
  info         Alias for ls.
  help         Show this help message.

TYPE-SPECIFIC COMMANDS:
  llm-manager speech stt [start|stop|ls|info|help]    Manage STT (speech-to-text) models
  llm-manager speech tts [start|stop|ls|info|help]    Manage TTS (text-to-speech) models
  llm-manager speech omni [start|stop|ls|info|help]   Manage Omni (multimodal) models

EXAMPLES:
  llm-manager speech start                              # start defaults
  llm-manager speech start whisper-large-v3             # start STT only
  llm-manager speech start whisper-large-v3 xtts-v2    # start STT+TTS
  llm-manager speech start -m whisper xtts pixtral     # start all 3
  llm-manager speech stop                               # stop all
  llm-manager speech stop whisper-large-v3 xtts-v2     # stop specific
  llm-manager speech ls                                 # list all
  llm-manager speech stt ls                             # list STT models
  llm-manager speech tts ls                             # list TTS models
  llm-manager speech omni ls                            # list Omni models
  llm-manager speech stt start                           # start first STT
  llm-manager speech tts start --default                # start default TTS
  llm-manager speech omni start                          # start first Omni`)
}
