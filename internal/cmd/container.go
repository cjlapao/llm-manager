// Package cmd provides the container subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("container", func(root *RootCommand) Command { return NewContainerCommand(root) })
}

// ContainerCommand handles container operations.
type ContainerCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewContainerCommand creates a new ContainerCommand.
func NewContainerCommand(root *RootCommand) *ContainerCommand {
	return &ContainerCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the container command with the given subcommand and arguments.
func (c *ContainerCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "list", "ls":
		return c.runList()
	case "status":
		if len(args) < 2 {
			return c.runStatusAll()
		}
		if args[1] == "refresh" {
			return c.runRefreshAll()
		}
		return c.runStatus(args[1])
	case "start":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'start' requires a slug (e.g., llm-manager container start qwen3_6 --allow-multiple)\n")
			return 1
		}
		return c.runStart(args[1:])
	case "stop":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'stop' requires a slug (e.g., llm-manager container stop qwen3_6)\n")
			return 1
		}
		return c.runStop(args[1:])
	case "restart":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'restart' requires a slug\n")
			return 1
		}
		return c.runRestart(args[1])
	case "logs":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'logs' requires a slug\n")
			return 1
		}
		if args[1] == "-h" || args[1] == "--help" || args[1] == "help" {
			fmt.Println(`logs - View container logs for an LLM model or service.

USAGE:
  llm-manager container logs <slug> [-f] [lines]

ARGUMENTS:
  slug      The model slug or service alias
  -f, --follow  Follow mode: stream logs in real-time (blocks until Ctrl+C)
  lines     Number of log lines to show (default: 50, only in non-follow mode)

SERVICE ALIASES:
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
  llm-manager container logs qwen3_6
  llm-manager container logs qwen3_6 200
  llm-manager container logs qwen3_6 -f
  llm-manager container logs comfyui -f
  llm-manager container logs embed 100`)
			return 0
		}
		lines := 50
		follow := false
		for i := 2; i < len(args); i++ {
			if args[i] == "-f" || args[i] == "--follow" {
				follow = true
			} else {
				fmt.Sscanf(args[i], "%d", &lines)
			}
		}
		return c.runLogs(args[1], lines, follow)
	case "swap":
		return NewContainerSwapCommand(c.cfg).Run(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown container subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runList displays all containers.
func (c *ContainerCommand) runList() int {
	containers, err := c.svc.ListContainers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing containers: %v\n", err)
		return 1
	}

	if len(containers) == 0 {
		fmt.Println("No containers found.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tNAME\tSTATUS\tPORT\tGPU")
	fmt.Fprintln(w, "----\t----\t------\t----\t---")
	for _, ct := range containers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%v\n",
			ct.Slug, ct.Name, ct.Status, ct.Port, ct.GPUUsed)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d containers\n", len(containers))
	return 0
}

// runStatusAll shows a comprehensive status overview of all containers, flux, 3D, and hotspot.
func (c *ContainerCommand) runStatusAll() int {
	fmt.Println("=== Docker Containers ===")

	// Show docker ps for llm-* and comfyui-flux containers
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
		// Check if Docker is actually available
		dockerCheck := exec.Command("docker", "info")
		if _, dockerErr := dockerCheck.CombinedOutput(); dockerErr != nil {
			fmt.Println("  Docker is not running or not accessible")
		} else {
			fmt.Println("  (no matching containers running)")
		}
	}

	fmt.Println()

	// Active Flux model
	activeFlux := readActiveFile(fluxActiveFilePath(c.cfg.cfg.InstallDir))
	if activeFlux != "" {
		fmt.Printf("  Active Flux model: %s\n", activeFlux)
	}

	// Active 3D model
	active3D := readActiveFile(threeDActiveFilePath(c.cfg.cfg.InstallDir))
	if active3D != "" {
		fmt.Printf("  Active 3D model: %s\n", active3D)
	}

	// Active hotspot model
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

// fluxActiveFilePath returns the path to the active flux model file.
func fluxActiveFilePath(installDir string) string {
	return installDir + "/comfyui/.active-model"
}

// threeDActiveFilePath returns the path to the active 3D model file.
func threeDActiveFilePath(installDir string) string {
	return installDir + "/comfyui/.active-3d"
}

// runStatus shows the status of a specific model/container.
func (c *ContainerCommand) runStatus(slug string) int {
	// Check if it's a flux model
	if isFluxModel(slug) {
		return c.runFluxStatus(slug)
	}

	// Check if it's a 3D model
	if is3DModel(slug) {
		return c.run3DStatus(slug)
	}

	// Standard container status
	status, err := c.svc.GetContainerStatus(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting container status: %v\n", err)
		return 1
	}

	fmt.Printf("Container %s: %s\n", slug, status)
	return 0
}

// runFluxStatus shows the status of a flux model.
func (c *ContainerCommand) runFluxStatus(slug string) int {
	activeFlux := readActiveFile(fluxActiveFilePath(c.cfg.cfg.InstallDir))
	comfyuiRunning := false

	// Check if comfyui-flux container is running
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
func (c *ContainerCommand) run3DStatus(slug string) int {
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

// runRefreshAll refreshes status for all containers from Docker.
func (c *ContainerCommand) runRefreshAll() int {
	containers, err := c.svc.ListContainers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing containers: %v\n", err)
		return 1
	}

	refreshed := 0
	for _, ct := range containers {
		if err := c.svc.RefreshContainerStatus(ct.Slug); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", ct.Slug, err)
			continue
		}
		refreshed++
	}

	fmt.Printf("Refreshed status for %d containers\n", refreshed)
	return 0
}

// runStart starts a container with flux/3D handling.
func (c *ContainerCommand) runStart(args []string) int {
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
func (c *ContainerCommand) runFluxStart(slug string, allowMultiple bool) int {
	fmt.Printf("Starting flux model: %s\n", slug)

	if !allowMultiple {
		// Stop all LLM containers
		fmt.Println("Stopping all LLM containers...")
		if err := c.svc.StopAllLLMs(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop LLM containers: %v\n", err)
		}
	} else {
		fmt.Println("Skipping stop of other LLM containers (--allow-multiple)")
	}

	// Deactivate 3D model
	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	// Activate flux model
	checkpoint := fluxCheckpoint(slug)
	if err := c.svc.ActivateFlux(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error activating flux model: %v\n", err)
		return 1
	}

	// Start ComfyUI
	fmt.Println("Starting ComfyUI...")
	if err := c.svc.StartComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start ComfyUI: %v\n", err)
	}

	fmt.Printf("Flux model %s activated.\n", slug)
	fmt.Printf("  Checkpoint: %s\n", checkpoint)
	return 0
}

// run3DStart handles starting a 3D model.
func (c *ContainerCommand) run3DStart(slug string, allowMultiple bool) int {
	fmt.Printf("Starting 3D model: %s\n", slug)

	if !allowMultiple {
		// Stop all LLM containers
		fmt.Println("Stopping all LLM containers...")
		if err := c.svc.StopAllLLMs(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop LLM containers: %v\n", err)
		}
	} else {
		fmt.Println("Skipping stop of other LLM containers (--allow-multiple)")
	}

	// Remove active flux file
	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}

	// Activate 3D model
	dir := dirFor3DModel(slug)
	weightsPath := c.cfg.cfg.InstallDir + "/comfyui/models/" + dir
	if err := c.svc.Activate3D(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error activating 3D model: %v\n", err)
		return 1
	}

	// Start ComfyUI
	fmt.Println("Starting ComfyUI...")
	if err := c.svc.StartComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start ComfyUI: %v\n", err)
	}

	fmt.Printf("3D model %s activated.\n", slug)
	fmt.Printf("  Weights path: %s\n", weightsPath)
	return 0
}

// runStop stops a container with flux/3D handling.
func (c *ContainerCommand) runStop(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: 'stop' requires a slug\n")
		return 1
	}

	slug := args[0]
	// Check if it's a flux model
	if isFluxModel(slug) {
		return c.runFluxStop(slug)
	}

	// Check if it's a 3D model
	if is3DModel(slug) {
		return c.run3DStop(slug)
	}

	// Normal LLM stop
	if err := c.svc.StopContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping container: %v\n", err)
		return 1
	}

	fmt.Printf("Stopped container: %s\n", slug)
	return 0
}

// runFluxStop handles stopping a flux model.
func (c *ContainerCommand) runFluxStop(slug string) int {
	fmt.Printf("Stopping flux model: %s\n", slug)

	// Remove active flux file
	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}

	fmt.Println("Flux model deactivated.")
	return 0
}

// run3DStop handles stopping a 3D model.
func (c *ContainerCommand) run3DStop(slug string) int {
	fmt.Printf("Stopping 3D model: %s\n", slug)

	// Remove active 3D file
	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	fmt.Println("3D model deactivated.")
	return 0
}

// runRestart restarts a container.
func (c *ContainerCommand) runRestart(slug string) int {
	if err := c.svc.RestartContainer(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Error restarting container: %v\n", err)
		return 1
	}

	fmt.Printf("Restarted container: %s\n", slug)
	return 0
}

// runLogs shows container logs.
func (c *ContainerCommand) runLogs(slug string, lines int, follow bool) int {
	// Resolve slug to container name
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

	args := []string{"logs", "--tail", fmt.Sprintf("%d", lines), containerName}
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting logs for %s: %s\n", containerName, strings.TrimSpace(string(output)))
		return 1
	}

	fmt.Print(string(output))
	return 0
}

// resolveContainer resolves a slug or service alias to a Docker container name.
func (c *ContainerCommand) resolveContainer(slug string) (string, error) {
	// Check if it's a service alias
	containerName := ResolveServiceAlias(slug)
	if containerName != "" {
		return containerName, nil
	}

	// Check if it's a model slug (look up container from DB)
	model, err := c.cfg.db.GetModel(slug)
	if err == nil && model.Container != "" {
		return model.Container, nil
	}

	// Known service aliases for error message
	fmt.Fprintf(os.Stderr, "Unknown service or model: %s\n\n", slug)
	fmt.Fprint(os.Stderr, "Known services:\n")
	for _, alias := range KnownServiceAliases() {
		fmt.Fprintf(os.Stderr, "  %-15s -> %s\n", alias, ServiceAliases[alias])
	}
	fmt.Fprint(os.Stderr, "\nOr use a model slug that has a container configured.\n")
	return "", fmt.Errorf("unknown service or model: %s", slug)
}

// PrintHelp prints the container command help.
func (c *ContainerCommand) PrintHelp() {
	fmt.Println(`container - Manage Docker containers for LLM models.

USAGE:
  llm-manager container [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list, ls          List all containers
  status [slug]     Show all container status, flux, 3D, and hotspot info
  status <slug>     Show status of a specific container/flux/3D model
  status refresh    Refresh status for all containers
  start <slug>      Start a container (handles flux/3D models)
  stop <slug>       Stop a container (handles flux/3D models)
  restart <slug>    Restart a container
  swap <slug>       GPU-safe model swap (stop all LLMs, drop cache, start target)
  logs <slug> [-f] [lines]  Show container logs (-f for follow mode)

FLAGS:
  --allow-multiple   Only for 'start': don't stop other running LLM containers before starting

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
  llm-manager container list
  llm-manager container status
  llm-manager container status qwen3_6
  llm-manager container start qwen3_6
  llm-manager container start qwen3_6 --allow-multiple
  llm-manager container start flux-schnell
  llm-manager container stop flux-schnell
  llm-manager container swap qwen3_6
  llm-manager container logs qwen3_6 -f
  llm-manager container logs comfyui 100`)
}
