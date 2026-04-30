// Package cmd provides the llm subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("llm", func(root *RootCommand) Command { return NewLlmCommand(root) })
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
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'start' requires a model slug\n")
			return 1
		}
		return c.runStart(args[1:])
	case "stop":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'stop' requires a model slug\n")
			return 1
		}
		return c.runStop(args[1:])
	case "restart":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'restart' requires a model slug\n")
			return 1
		}
		return c.runRestart(args[1])
	case "swap":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'swap' requires a target model slug\n")
			return 1
		}
		return c.runSwap(args[1:])
	case "status":
		if len(args) < 2 {
			return c.runStatusAll()
		}
		return c.runStatus(args[1])
	case "logs":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'logs' requires a model slug\n")
			return 1
		}
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

// runStart starts a model container, handling flux/3D special cases.
func (c *LlmCommand) runStart(args []string) int {
	slug := args[0]
	allowMultiple := false
	for _, arg := range args[1:] {
		if arg == "--allow-multiple" || arg == "-m" {
			allowMultiple = true
		}
	}

	// Check if it's a flux model
	if isFluxModel(slug) {
		return c.runFluxStart(slug, allowMultiple)
	}

	// Check if it's a 3D model
	if is3DModel(slug) {
		return c.run3DStart(slug, allowMultiple)
	}

	// Normal LLM start
	if err := c.svc.StartContainer(slug, allowMultiple); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting container: %v\n", err)
		return 1
	}

	fmt.Printf("Started container: %s\n", slug)
	return 0
}

// runFluxStart handles starting a flux model.
func (c *LlmCommand) runFluxStart(slug string, allowMultiple bool) int {
	fmt.Printf("Starting flux model: %s\n", slug)

	if !allowMultiple {
		fmt.Println("Stopping all LLM containers...")
		if err := c.svc.StopAllLLMs(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop LLM containers: %v\n", err)
		}
	} else {
		fmt.Println("Skipping stop of other LLM containers (--allow-multiple)")
	}

	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	checkpoint := fluxCheckpoint(slug)
	if err := c.svc.ActivateFlux(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error activating flux model: %v\n", err)
		return 1
	}

	fmt.Println("Starting ComfyUI...")
	if err := c.svc.StartComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start ComfyUI: %v\n", err)
	}

	fmt.Printf("Flux model %s activated.\n", slug)
	fmt.Printf("  Checkpoint: %s\n", checkpoint)
	return 0
}

// run3DStart handles starting a 3D model.
func (c *LlmCommand) run3DStart(slug string, allowMultiple bool) int {
	fmt.Printf("Starting 3D model: %s\n", slug)

	if !allowMultiple {
		fmt.Println("Stopping all LLM containers...")
		if err := c.svc.StopAllLLMs(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop LLM containers: %v\n", err)
		}
	} else {
		fmt.Println("Skipping stop of other LLM containers (--allow-multiple)")
	}

	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}

	dir := dirFor3DModel(slug)
	weightsPath := c.cfg.cfg.InstallDir + "/comfyui/models/" + dir
	if err := c.svc.Activate3D(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error activating 3D model: %v\n", err)
		return 1
	}

	fmt.Println("Starting ComfyUI...")
	if err := c.svc.StartComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start ComfyUI: %v\n", err)
	}

	fmt.Printf("3D model %s activated.\n", slug)
	fmt.Printf("  Weights path: %s\n", weightsPath)
	return 0
}

// ── stop ───────────────────────────────────────────────────────────────────

// runStop stops a model container, handling flux/3D special cases.
func (c *LlmCommand) runStop(args []string) int {
	slug := args[0]

	if isFluxModel(slug) {
		return c.runFluxStop(slug)
	}

	if is3DModel(slug) {
		return c.run3DStop(slug)
	}

	if err := c.svc.StopContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping container: %v\n", err)
		return 1
	}

	fmt.Printf("Stopped container: %s\n", slug)
	return 0
}

// runFluxStop handles stopping a flux model.
func (c *LlmCommand) runFluxStop(slug string) int {
	fmt.Printf("Stopping flux model: %s\n", slug)

	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}

	fmt.Println("Flux model deactivated.")
	return 0
}

// run3DStop handles stopping a 3D model.
func (c *LlmCommand) run3DStop(slug string) int {
	fmt.Printf("Stopping 3D model: %s\n", slug)

	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	fmt.Println("3D model deactivated.")
	return 0
}

// ── restart ────────────────────────────────────────────────────────────────

// runRestart restarts a model container.
func (c *LlmCommand) runRestart(slug string) int {
	if err := c.svc.RestartContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error restarting container: %v\n", err)
		return 1
	}

	fmt.Printf("Restarted container: %s\n", slug)
	return 0
}

// ── swap ───────────────────────────────────────────────────────────────────

// runSwap performs a GPU-safe model swap.
func (c *LlmCommand) runSwap(args []string) int {
	slug := args[0]
	allowMultiple := false
	for _, arg := range args[1:] {
		if arg == "--allow-multiple" || arg == "-m" {
			allowMultiple = true
		}
	}

	// If --allow-multiple is set, skip the stop-all step
	if allowMultiple {
		fmt.Printf("Swapping to model: %s (--allow-multiple, skipping stop-all)\n", slug)
		if err := c.svc.StartContainer(slug, true); err != nil {
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
	if err := c.svc.StartContainer(slug, false); err != nil {
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

	activeFlux := readActiveFile(fluxActiveFilePath(c.cfg.cfg.InstallDir))
	if activeFlux != "" {
		fmt.Printf("  Active Flux model: %s\n", activeFlux)
	}

	active3D := readActiveFile(threeDActiveFilePath(c.cfg.cfg.InstallDir))
	if active3D != "" {
		fmt.Printf("  Active 3D model: %s\n", active3D)
	}

	hotspot, err := c.cfg.db.GetHotspot()
	if err == nil && hotspot != nil {
		model, modelErr := c.cfg.db.GetModel(hotspot.ModelSlug)
		if modelErr == nil {
			fmt.Printf("  Active hotspot model: %s (%s)\n", model.Name, hotspot.ModelSlug)
		} else {
			fmt.Printf("  Active hotspot model: %s\n", hotspot.ModelSlug)
		}
	}

	return 0
}

// runStatus shows the status of a specific model/container.
func (c *LlmCommand) runStatus(slug string) int {
	if isFluxModel(slug) {
		return c.runFluxStatus(slug)
	}

	if is3DModel(slug) {
		return c.run3DStatus(slug)
	}

	status, err := c.svc.GetContainerStatus(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting container status: %v\n", err)
		return 1
	}

	fmt.Printf("Container %s: %s\n", slug, status)
	return 0
}

// runFluxStatus shows the status of a flux model.
func (c *LlmCommand) runFluxStatus(slug string) int {
	activeFlux := readActiveFile(fluxActiveFilePath(c.cfg.cfg.InstallDir))
	comfyuiRunning := false

	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", "comfyui-flux")
	if output, err := cmd.Output(); err == nil {
		state := strings.TrimSpace(string(output))
		if state == "running" {
			comfyuiRunning = true
		}
	}

	if activeFlux == slug && comfyuiRunning {
		fmt.Printf("Flux model %s: active\n", slug)
		fmt.Printf("  Checkpoint: %s\n", fluxCheckpoint(slug))
		fmt.Println("  ComfyUI container: running")
	} else if comfyuiRunning {
		fmt.Printf("Flux model %s: standby\n", slug)
		fmt.Println("  ComfyUI container: running")
		if activeFlux != "" {
			fmt.Printf("  Active flux model: %s\n", activeFlux)
		}
	} else {
		fmt.Printf("Flux model %s: stopped\n", slug)
		fmt.Println("  ComfyUI container: not running")
	}

	return 0
}

// run3DStatus shows the status of a 3D model.
func (c *LlmCommand) run3DStatus(slug string) int {
	active3D := readActiveFile(threeDActiveFilePath(c.cfg.cfg.InstallDir))

	if active3D == slug {
		dir := dirFor3DModel(slug)
		weightsPath := c.cfg.cfg.InstallDir + "/comfyui/models/" + dir
		fmt.Printf("3D model %s: active\n", slug)
		fmt.Printf("  Weights path: %s\n", weightsPath)
	} else {
		fmt.Printf("3D model %s: stopped\n", slug)
		if active3D != "" {
			fmt.Printf("  Active 3D model: %s\n", active3D)
		}
	}

	return 0
}

// ── logs ───────────────────────────────────────────────────────────────────

// runLogs shows container logs.
func (c *LlmCommand) runLogs(args []string) int {
	slug := args[0]
	lines := 50
	follow := false
	for i := 1; i < len(args); i++ {
		if args[i] == "-f" || args[i] == "--follow" {
			follow = true
		} else {
			fmt.Sscanf(args[i], "%d", &lines)
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
		return model.Container, nil
	}

	fmt.Fprintf(os.Stderr, "Unknown service or model: %s\n\n", slug)
	fmt.Fprint(os.Stderr, "Known services:\n")
	for _, alias := range KnownServiceAliases() {
		fmt.Fprintf(os.Stderr, "  %-15s -> %s\n", alias, ServiceAliases[alias])
	}
	fmt.Fprint(os.Stderr, "\nOr use a model slug that has a container configured.\n")
	return "", fmt.Errorf("unknown service or model: %s", slug)
}

// ── help ───────────────────────────────────────────────────────────────────

// PrintHelp prints the llm command help.
func (c *LlmCommand) PrintHelp() {
	fmt.Println(`llm - Manage LLM model containers (start, stop, restart, swap, status, logs).

USAGE:
  llm-manager llm [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start <slug>        Start a model container (handles flux/3D models)
  stop <slug>         Stop a model container (handles flux/3D models)
  restart <slug>      Restart a model container
  swap <slug>         GPU-safe model swap (stop all LLMs, drop cache, start target)
  status [slug]       Show all container status, flux, 3D, and hotspot info
  status <slug>       Show status of a specific container/flux/3D model
  logs <slug> [-f] [lines]  Show container logs (-f for follow mode)

FLAGS:
  --allow-multiple    Only for 'start' and 'swap': don't stop other running
                      LLM containers before starting

SERVICE ALIASES (for logs):
  comfyui, flux   -> comfyui-flux
  embed           -> llm-embed
  rerank          -> llm-rerank
  whisper         -> whisper-stt
  kokoro          -> kokoro-tts
  litellm         -> litellm
  swap-api, swapapi -> swap-api
  open-webui, webui -> open-webui
  mcp             -> mcpo

EXAMPLES:
  llm-manager llm start qwen3_6
  llm-manager llm start qwen3_6 --allow-multiple
  llm-manager llm start flux-schnell
  llm-manager llm stop qwen3_6
  llm-manager llm restart qwen3_6
  llm-manager llm swap qwen3_6
  llm-manager llm status
  llm-manager llm status qwen3_6
  llm-manager llm logs qwen3_6 -f
  llm-manager llm logs comfyui 100`)
}
