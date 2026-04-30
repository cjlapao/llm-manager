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

// ContainerCommand handles low-level Docker container operations.
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
		// Deprecated: status of a specific model now lives in llm command
		fmt.Fprintf(os.Stderr, "Note: 'container status <slug>' is deprecated — use 'llm-manager llm status <slug>' instead\n")
		return c.runStatus(args[1])
	case "logs":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'logs' requires a slug\n")
			return 1
		}
		if args[1] == "-h" || args[1] == "--help" || args[1] == "help" {
			fmt.Println(`logs - View container logs for a model or service.

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
	case "start", "stop", "restart", "swap":
		fmt.Fprintf(os.Stderr, "Note: 'container %s' is deprecated — use 'llm-manager llm %s' instead\n", args[0], args[0])
		llm := NewLlmCommand(c.cfg)
		subArgs := append([]string{args[0]}, args[1:]...)
		return llm.Run(subArgs)
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
func (c *ContainerCommand) runFluxStatus(slug string) int {
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

// runLogs shows container logs.
func (c *ContainerCommand) runLogs(slug string, lines int, follow bool) int {
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

// PrintHelp prints the container command help.
func (c *ContainerCommand) PrintHelp() {
	fmt.Println(`container - Low-level Docker container operations (list, logs, status refresh).

DEPRECATED SUBCOMMANDS:
  start, stop, restart, swap are now under 'llm-manager llm'.
  These commands still work but show a deprecation notice.

USAGE:
  llm-manager container [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list, ls          List all containers
  status [slug]     Show all container status, flux, 3D, and hotspot info
  status refresh    Refresh status for all containers
  logs <slug> [-f] [lines]  Show container logs (-f for follow mode)

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
  llm-manager container status refresh
  llm-manager container logs qwen3_6 -f
  llm-manager container logs comfyui 100

FOR CONTAINER MANAGEMENT, USE:
  llm-manager llm start <slug>
  llm-manager llm stop <slug>
  llm-manager llm restart <slug>
  llm-manager llm swap <slug>
  llm-manager llm status <slug>`)
}
