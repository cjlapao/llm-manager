// Package cmd provides the llm subcommand for llm-manager.
package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("llm", func(root *RootCommand) Command { return NewLlmCommand(root) })
}

// splitArgs separates command-line args into positional arguments (anything that
// doesn't start with "-") and flags (anything starting with "-").
func splitArgs(args []string) (positional []string, flags []string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			positional = append(positional, arg)
		}
	}
	return
}

// LlmCommand manages LLM model containers (start, stop, restart, swap, status, logs).
type LlmCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewLlmCommand creates a new LlmCommand.
func NewLlmCommand(root *RootCommand) *LlmCommand {
	return &LlmCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the llm command with the given subcommand and arguments.
func (c *LlmCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "start":
		return c.runStart(args[1:])
	case "stop":
		return c.runStop(args[1:])
	case "restart":
		return c.runRestart(args[1:])
	case "swap":
		return c.runSwap(args[1:])
	case "status":
		if len(args) < 2 {
			return c.runStatusAll()
		}
		return c.runStatus(args[1])
	case "logs":
		return c.runLogs(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown llm subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// ── start ──────────────────────────────────────────────────────────────────

// runStart starts a model container.
func (c *LlmCommand) runStart(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}

	// Resolve "latest" to the actual model slug
	isLatest := slug == "latest"
	if slug == "" || isLatest {
		resolved, err := resolveLatestSlug(c.cfg.db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		slug = resolved
		if isLatest {
			fmt.Printf("Resolving 'latest' to model: %s\n", slug)
		} else {
			fmt.Printf("Using latest model: %s\n", slug)
		}
	}

	allowMultiple := false
	dryRun := false
	wait := false
	overrides := service.StartOverrides{}
	// flags start after [0] if slug was passed, otherwise nothing
	flagArgs := args
	if len(args) > 0 { flagArgs = args[1:] }
	for _, arg := range flagArgs {
		switch arg {
		case "--dry-run", "-n":
			dryRun = true
		case "--allow-multiple", "-m":
			allowMultiple = true
		case "--wait", "-w":
			wait = true
		case "--max-model-len":
			// next arg is the value
		case "--max-num-seqs":
			// next arg is the value
		case "--max-num-batched-tokens":
			// next arg is the value
		case "--gpu-memory":
			// next arg is the value
		case "--speculative-decoding":
			// next arg is the value
		case "--speculative-tokens":
			// next arg is the value
		case "--speculative-model":
			// next arg is the value
		}
	}

	// Parse numeric overrides (simple positional: --flag value)
	startIdx := 1
	if len(args) < 2 { startIdx = 0 }
	for i := startIdx; i < len(args); i++ {
		switch args[i] {
		case "--max-model-len":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.MaxModelLen = val
				i++
			}
		case "--max-num-seqs":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.MaxNumSeqs = val
				i++
			}
		case "--max-num-batched-tokens":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.MaxNumBatchedTokens = val
				i++
			}
		case "--gpu-memory":
			if i+1 < len(args) {
				var val float64
				fmt.Sscanf(args[i+1], "%f", &val)
				overrides.GPUMemoryUtil = &val
				i++
			}
		case "--speculative-decoding":
			if i+1 < len(args) {
				val := args[i+1]
				overrides.SpeculativeDecoding = &val
				i++
			}
		case "--speculative-tokens":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.NumSpeculativeTokens = &val
				i++
			}
		case "--speculative-model":
			if i+1 < len(args) {
				val := args[i+1]
				overrides.SpeculativeModel = &val
				i++
			}
		}
	}

	// Dry-run: only GPU memory check, no Docker modifications
	if dryRun {
		if err := c.svc.StartContainerDryRun(slug, overrides); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("[dry-run] Dry run complete. No containers were modified.")
		return 0
	}

	// Normal LLM start
	if err := c.svc.StartContainer(slug, allowMultiple, overrides); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting container: %v\n", err)
		return 1
	}

	fmt.Printf("Started container: %s\n", slug)

	// Persist this model as the latest started model
	configSvc := service.NewConfigService(c.cfg.db)
	if err := configSvc.SetLatestModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to set latest model: %v\n", err)
	}

	// Optionally wait for health check
	if wait {
		fmt.Println("Waiting for container to become healthy...")
		model, err := c.cfg.db.GetModel(slug)
		if err == nil && model.Port > 0 {
			host := "localhost"
			if c.cfg.cfg.OpenAIAPIURL != "" {
				if parsed, err := url.Parse(c.cfg.cfg.OpenAIAPIURL); err == nil && parsed.Host != "" {
					host = parsed.Host
				}
			}
			healthURL := fmt.Sprintf("http://%s:%d", host, model.Port)

			ctx, cancel := context.WithTimeout(context.Background(), DefaultStartTimeout)
			defer cancel()

			if err := waitForHealthy(ctx, healthURL); err != nil {
				fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
				fmt.Println("Stopping container...")
				stopErr := c.svc.StopContainer(slug)
				if stopErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to stop container after health check failure: %v\n", stopErr)
				}
				fmt.Fprintf(os.Stderr, "Error: container %s failed health check and was stopped\n", slug)
				return 1
			}
			fmt.Println("Container is healthy!")
		}
	}

	return 0
}

// ── stop ───────────────────────────────────────────────────────────────────

// runStop stops a model container. If no slug is provided, uses the latest-started model.
func (c *LlmCommand) runStop(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}

	isLatest := slug == "latest"
	if slug == "" || isLatest {
		resolved, err := resolveLatestSlug(c.cfg.db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		slug = resolved
		if isLatest {
			fmt.Printf("Resolving 'latest' to model: %s\n", slug)
		} else {
			fmt.Printf("Using latest model: %s\n", slug)
		}
	}

	if err := c.svc.StopContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping container: %v\n", err)
		return 1
	}

	fmt.Printf("Stopped container: %s\n", slug)
	return 0
}

// ── restart ───────────────────────────────────────────────────────────────

// runRestart restarts a model container. If no slug is provided, uses the latest-started model.
func (c *LlmCommand) runRestart(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}

	isLatest := slug == "latest"
	if slug == "" || isLatest {
		resolved, err := resolveLatestSlug(c.cfg.db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		slug = resolved
		if isLatest {
			fmt.Printf("Resolving 'latest' to model: %s\n", slug)
		} else {
			fmt.Printf("Using latest model: %s\n", slug)
		}
	}

	if err := c.svc.RestartContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error restarting container: %v\n", err)
		return 1
	}

	fmt.Printf("Restarted container: %s\n", slug)
	return 0
}

// ── swap ─────────────────────────────────────────────────────────────────

// runSwap performs a GPU-safe model swap. If no slug is provided, uses the latest-started model.
func (c *LlmCommand) runSwap(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}
	isLatest := slug == "latest"
	if slug == "" || isLatest {
		resolved, err := resolveLatestSlug(c.cfg.db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		slug = resolved
		if isLatest {
			fmt.Printf("Resolving 'latest' to model: %s\n", slug)
		} else {
			fmt.Printf("Using latest model: %s\n", slug)
		}
	}

	allowMultiple := false
	swapFlags := args
	if len(args) > 0 { swapFlags = args[1:] }
	for _, arg := range swapFlags {
		if arg == "--allow-multiple" || arg == "-m" {
			allowMultiple = true
		}
	}

	// If --allow-multiple is set, skip the stop-all step
	if allowMultiple {
		fmt.Printf("Swapping to model: %s (--allow-multiple, skipping stop-all)\n", slug)
		if err := c.svc.StartContainer(slug, true, service.StartOverrides{}); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting container: %v\n", err)
			return 1
		}
		if err := c.cfg.db.SetHotspot(slug); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not set hotspot: %v\n", err)
		}
		fmt.Printf("Successfully swapped to: %s\n", slug)
		return 0
	}

	fmt.Printf("Swapping to model: %s\n", slug)

	fmt.Println("Stopping all LLM containers...")
	if err := c.svc.StopAllLLMs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping LLM containers: %v\n", err)
		return 1
	}

	fmt.Println("Removing active model files...")
	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}
	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	fmt.Println("Dropping OS page cache...")
	if err := c.svc.DropPageCache(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not drop page cache: %v\n", err)
	}

	fmt.Printf("Starting model: %s\n", slug)
	if err := c.svc.StartContainer(slug, false, service.StartOverrides{}); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting container: %v\n", err)
		return 1
	}

	fmt.Printf("Setting hotspot to: %s\n", slug)
	if err := c.cfg.db.SetHotspot(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set hotspot: %v\n", err)
	}

	fmt.Printf("Successfully swapped to: %s\n", slug)
	return 0
}

// ── status ─────────────────────────────────────────────────────────────────

// runStatusAll shows a comprehensive status overview.
func (c *LlmCommand) runStatusAll() int {
	fmt.Println("=== Docker Containers ===")

	cmd := exec.Command("docker", "ps",
		"--filter", "name=llm-",
		"--filter", "name=comfyui-flux",
		"--filter", "name=whisper-",
		"--filter", "name=kokoro-",
		"--filter", "name=swap-",
		"--filter", "name=open-webui",
		"--filter", "name=mcpo",
		"--filter", "name=litellm",
		"--format", "  {{.Names}}\t{{.Status}}\t{{.Ports}}")

	output, err := cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		fmt.Print(string(output))
	} else {
		dockerCheck := exec.Command("docker", "info")
		if _, dockerErr := dockerCheck.CombinedOutput(); dockerErr != nil {
			fmt.Println("  Docker is not running or not accessible")
		} else {
			fmt.Println("  (no matching containers running)")
		}
	}

	fmt.Println()

	hotspot, err := c.cfg.db.GetHotspot()
	if err == nil && hotspot != nil {
		model, modelErr := c.cfg.db.GetModel(hotspot.ModelSlug)
		if modelErr == nil {
			fmt.Printf("  Active hotspot model: %s (%s)\n", model.Name, hotspot.ModelSlug)
		} else {
			fmt.Printf("  Active hotspot model: %s\n", hotspot.ModelSlug)
		}
	}

	// Display latest model
	latestModel, err := c.cfg.db.GetConfig("LLM_MANAGER_LATEST_MODEL")
	if err == nil && latestModel != nil && latestModel.Value != "" {
		model, modelErr := c.cfg.db.GetModel(latestModel.Value)
		if modelErr == nil {
			fmt.Printf("  Latest model: %s (%s)\n", model.Name, latestModel.Value)
		} else {
			fmt.Printf("  Latest model: %s (model not found — may be stale)\n", latestModel.Value)
		}
	} else {
		fmt.Println("  Latest model: none set")
	}

	return 0
}

// runStatus shows the status of a specific model/container.
func (c *LlmCommand) runStatus(slug string) int {
	status, err := c.svc.GetContainerStatus(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting container status: %v\n", err)
		return 1
	}

	fmt.Printf("Container %s: %s\n", slug, status)
	return 0
}

// ── logs ─────────────────────────────────────────────────────────────────

// runLogs shows container logs.
func (c *LlmCommand) runLogs(args []string) int {
	// Split into positional args (slug candidate) and flag args
	positional, allFlags := splitArgs(args)

	slug := ""
	if len(positional) > 0 {
		slug = positional[0]
	}

	isLatest := slug == "latest"
	if slug == "" || isLatest {
		resolved, err := resolveLatestSlug(c.cfg.db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		slug = resolved
		if isLatest {
			fmt.Printf("Resolving 'latest' to model: %s\n", slug)
		} else {
			fmt.Printf("Using latest model: %s\n", slug)
		}
	}

	lines := 50
	follow := false
	for _, arg := range allFlags {
		if arg == "-f" || arg == "--follow" {
			follow = true
		} else {
			if n, _ := fmt.Sscanf(arg, "%d", &lines); n == 0 {
				fmt.Fprintf(os.Stderr, "Warning: invalid log line count %q, using default 50\n", arg)
			}
		}
	}

	containerName, err := c.resolveContainer(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if follow {
		cmd := exec.Command("docker", "logs", "-f", "--tail", fmt.Sprintf("%d", lines), containerName)
		if err := RunInteractive(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error following logs for %s: %v\n", containerName, err)
			return 1
		}
		return 0
	}

	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting logs for %s: %s\n", containerName, strings.TrimSpace(string(output)))
		return 1
	}

	fmt.Print(string(output))
	return 0
}

// resolveContainer resolves a slug or service alias to a Docker container name.
func (c *LlmCommand) resolveContainer(slug string) (string, error) {
	containerName := ResolveServiceAlias(slug)
	if containerName != "" {
		return containerName, nil
	}

	model, err := c.cfg.db.GetModel(slug)
	if err == nil && model.Container != "" {
		return model.GetContainerName(), nil
	}

	fmt.Fprintf(os.Stderr, "Unknown service or model: %s\n\n", slug)
	fmt.Fprint(os.Stderr, "Known services:\n")
	for _, alias := range KnownServiceAliases() {
		fmt.Fprintf(os.Stderr, "  %-15s -> %s\n", alias, ServiceAliases[alias])
	}
	fmt.Fprint(os.Stderr, "\nOr use a model slug that has a container configured.\n")
	return "", fmt.Errorf("unknown service or model: %s", slug)
}

// resolveLatestSlug resolves the "latest" keyword to an actual model slug.
// Returns the resolved slug, or an error if no latest model is set or the resolved
// model doesn't exist in the database.
func resolveLatestSlug(db database.DatabaseManager) (string, error) {
	configSvc := service.NewConfigService(db)
	resolved, err := configSvc.GetLatestModel()
	if err != nil {
		return "", fmt.Errorf("error resolving latest model: %w", err)
	}
	if resolved == "" {
		return "", fmt.Errorf("no latest model has been set. Start a model first with 'llm-manager llm start <slug>'")
	}
	if _, err := db.GetModel(resolved); err != nil {
		return "", fmt.Errorf("resolved model %q is not a known model", resolved)
	}
	return resolved, nil
}

// ── help ───────────────────────────────────────────────────────────────────

// PrintHelp prints the llm command help.
func (c *LlmCommand) PrintHelp() {
	fmt.Println(`llm - Manage LLM model containers (start, stop, restart, swap, status, logs).

USAGE:
  llm-manager llm [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [<slug>]     Start a model container (defaults to latest if omitted)
  stop [<slug>]      Stop a model container (defaults to latest if omitted)
  restart [<slug>]   Restart a model container (defaults to latest if omitted)
  swap [<slug>]      GPU-safe model swap (defaults to latest if omitted)
  status [slug]         Show all container status and latest model info
  status <slug>         Show status of a specific container
  logs [<slug>] [-f] [lines]  Show container logs (-f for follow mode, defaults to latest)

FLAGS:
  --dry-run, -n           Preview startup (memory checks, diagnostics) without
                          modifying Docker
  --allow-multiple, -m    Only for 'start' and 'swap': don't stop other running
                          LLM containers before starting
  --wait, -w              Wait for the container to become healthy before returning
                          (polls /health endpoint up to 180s, stops container on failure)

SERVICE ALIASES (for logs):
  comfyui, flux   -> comfyui-flux
  whisper         -> whisper-stt
  kokoro          -> kokoro-tts
  litellm         -> litellm
  swap-api, swapapi -> swap-api
  open-webui, webui -> open-webui
  mcp             -> mcpo

EXAMPLES:
  llm-manager llm start                   Start using the latest model
  llm-manager llm start qwen3_6
  llm-manager llm start latest
  llm-manager llm start qwen3_6 --allow-multiple
  llm-manager llm start qwen3_6 --wait
  llm-manager llm stop                    Stop the latest model
  llm-manager llm stop qwen3_6
  llm-manager llm restart                 Restart the latest model
  llm-manager llm restart qwen3_6
  llm-manager llm swap                    Swap to the latest model
  llm-manager llm swap qwen3_6
  llm-manager llm status
  llm-manager llm status qwen3_6
  llm-manager llm logs              Show logs from latest model
  llm-manager llm logs -f           Follow logs from latest model
  llm-manager llm logs qwen3_6 -f   Follow logs for specific model
  llm-manager llm logs comfyui 100`)
}
