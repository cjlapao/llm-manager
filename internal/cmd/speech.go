// Package cmd provides the speech subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("speech", func(root *RootCommand) Command { return NewSpeechCommand(root) })
}

// SpeechCommand handles combined speech model operations (STT + TTS + Omni).
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
	default:
		fmt.Fprintf(os.Stderr, "unknown speech subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// parseArgs extracts the --allow-multiple (-m) flag and up to 3 positional
// slug arguments from the provided slice. Extras beyond 3 are silently ignored.
func parseArgs(args []string) (allowMultiple bool, slugs []string) {
	for _, arg := range args {
		switch arg {
		case "--allow-multiple", "-m":
			allowMultiple = true
		default:
			if !strings.HasPrefix(arg, "-") {
				if len(slugs) < 3 {
					slugs = append(slugs, arg)
				}
				// Silently ignore extras beyond 3
			}
		}
	}
	return allowMultiple, slugs
}

// safeSlug returns the slug at the given index if it exists, else empty string.
func safeSlug(slugs []string, idx int) string {
	if idx < len(slugs) {
		return slugs[idx]
	}
	return ""
}

// resolveSTT returns the first STT model's slug (default-first, then any).
func (c *SpeechCommand) resolveSTT() string {
	models, err := c.svc.ListSTTModels()
	if err != nil || len(models) == 0 {
		return ""
	}
	for _, m := range models {
		if m.Default {
			return m.Slug
		}
	}
	return models[0].Slug
}

// resolveTTS returns the first TTS model's slug (default-first, then any).
func (c *SpeechCommand) resolveTTS() string {
	models, err := c.svc.ListTTSModels()
	if err != nil || len(models) == 0 {
		return ""
	}
	for _, m := range models {
		if m.Default {
			return m.Slug
		}
	}
	return models[0].Slug
}

// resolveOmni returns the first Omni model's slug (default-first, then any).
func (c *SpeechCommand) resolveOmni() string {
	models, err := c.svc.ListOmniModels()
	if err != nil || len(models) == 0 {
	 return ""
	}
	for _, m := range models {
		if m.Default {
			return m.Slug
		}
	}
	return models[0].Slug
}

// startedContainer tracks a container that was successfully started, used for rollback.
type startedContainer struct {
	subType string
	slug    string
}

// runStart starts combined speech containers (up to 3: stt, tts, omni).
// Usage: speech start [--allow-multiple|-m] [stt-slug] [tts-slug] [omni-slug]
func (c *SpeechCommand) runStart(args []string) int {
	allowMultiple, slugs := parseArgs(args)

	// Resolve actual slugs — positional order: 0=stt, 1=tts, 2=omni
	tgt := struct {
		stt  string
		tts  string
		omni string
	}{
		stt:  safeSlug(slugs, 0),
		tts:  safeSlug(slugs, 1),
		omni: safeSlug(slugs, 2),
	}

	// Fill in missing defaults
	if tgt.stt == "" {
		tgt.stt = c.resolveSTT()
	}
	if tgt.tts == "" {
		tgt.tts = c.resolveTTS()
	}
	if tgt.omni == "" {
		tgt.omni = c.resolveOmni()
	}

	// Build ordered list — stt first, then tts, then omni
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

	// Validate all slugs exist in DB before starting anything
	for _, s := range steps {
		if _, err := c.cfg.db.GetModel(s.slug); err != nil {
			fmt.Fprintf(os.Stderr, "Model not found: %s\n", s.slug)
			return 1
		}
	}

	// Track successful starts for rollback
	var started []startedContainer

	for i, s := range steps {
		isLast := i == len(steps)-1
		fmt.Printf("Starting %s model: %s\n", s.label, s.slug)

		// Per-subtype peer isolation (unless --allow-multiple)
		if !allowMultiple {
			fmt.Printf("Stopping other %s containers...\n", s.label)
			if err := c.svc.StopAllBySubType("speech", s.subType); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to stop other %s containers: %v\n", s.label, err)
			}
		}

		err := c.svc.StartModelBySlugWithAllow(s.slug, !allowMultiple)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting %s model: %v\n", s.label, err)
			// Rollback everything started so far (reverse order)
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

// runStop stops combined speech containers.
// Usage: speech stop [stt-slug] [tts-slug] [omni-slug]
func (c *SpeechCommand) runStop(args []string) int {
	_, slugs := parseArgs(args)

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

	// No slugs at all → stop everything by subtype
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

	// Stop specific models in order
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

	sectionHeader := func(title string) {
		fmt.Println(strings.Repeat("-", 95))
		fmt.Printf("%-25s %-20s %-18s %6s %s\n", "Name", "Slug", "Container", "Port", "Status(Default)")
		fmt.Println(strings.Repeat("-", 95))
	}
	footerLine := func() {
		fmt.Println(strings.Repeat("-", 95))
	}

	printSection := func(title string, models []models.Model) {
		fmt.Printf("\n%s:\n", title)
		if len(models) == 0 {
			fmt.Println("  (none)")
			return
		}
		sectionHeader(title)
		for _, m := range models {
			status := "unknown"
			s, err := c.svc.GetModelStatus(m.Slug)
			if err == nil {
				status = s.Status
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
		footerLine()
	}

	printSection("STT Models", sttModels)
	printSection("TTS Models", ttsModels)
	printSection("Omni Models", omniModels)

	total := len(sttModels) + len(ttsModels) + len(omniModels)
	fmt.Printf("Total: %d speech model(s)\n", total)

	return 0
}

// PrintHelp prints the speech command help.
func (c *SpeechCommand) PrintHelp() {
	fmt.Println(`speech - Manage combined speech models (STT + TTS + Omni) via Docker Compose.

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

EXAMPLES:
  llm-manager speech start                              # start defaults
  llm-manager speech start whisper-large-v3             # start STT only
  llm-manager speech start whisper-large-v3 xtts-v2    # start STT+TTS
  llm-manager speech start -m whisper xtts pixtral     # start all 3
  llm-manager speech stop                               # stop all
  llm-manager speech stop whisper-large-v3 xtts-v2     # stop specific
  llm-manager speech info                                # list all`)
}