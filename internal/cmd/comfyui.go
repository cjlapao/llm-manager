// Package cmd provides the comfyui subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("comfyui", func(root *RootCommand) Command { return NewComfyUICommand(root) })
}

// ComfyUICommand handles ComfyUI container operations.
type ComfyUICommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewComfyUICommand creates a new ComfyUICommand.
func NewComfyUICommand(root *RootCommand) *ComfyUICommand {
	return &ComfyUICommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the comfyui command with the given subcommand and arguments.
func (c *ComfyUICommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "start":
		return c.runStart()
	case "stop":
		return c.runStop()
	case "flux":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'flux' requires a subcommand: start, stop, status\n")
			return 1
		}
		switch args[1] {
		case "start":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Error: 'flux start' requires a model slug\n")
				return 1
			}
			allowMultiple := false
			for _, arg := range args[2:] {
				if arg == "--allow-multiple" || arg == "-m" {
					allowMultiple = true
				}
			}
			return c.runFluxStart(args[2], allowMultiple)
		case "stop":
			return c.runFluxStop()
		case "status":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Error: 'flux status' requires a model slug\n")
				return 1
			}
			return c.runFluxStatus(args[2])
		default:
			fmt.Fprintf(os.Stderr, "unknown flux subcommand: %s\n\n", args[1])
			fmt.Fprintln(os.Stderr, "Usage: llm-manager comfyui flux [start|stop|status] [ARGS]")
			return 1
		}
	case "3d":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: '3d' requires a subcommand: start, stop, status\n")
			return 1
		}
		switch args[1] {
		case "start":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Error: '3d start' requires a model slug\n")
				return 1
			}
			allowMultiple := false
			for _, arg := range args[2:] {
				if arg == "--allow-multiple" || arg == "-m" {
					allowMultiple = true
				}
			}
			return c.run3DStart(args[2], allowMultiple)
		case "stop":
			return c.run3DStop()
		case "status":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Error: '3d status' requires a model slug\n")
				return 1
			}
			return c.run3DStatus(args[2])
		default:
			fmt.Fprintf(os.Stderr, "unknown 3d subcommand: %s\n\n", args[1])
			fmt.Fprintln(os.Stderr, "Usage: llm-manager comfyui 3d [start|stop|status] [ARGS]")
			return 1
		}
	case "status":
		return c.runStatus()
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown comfyui subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runStart starts ComfyUI via profile-based compose.
func (c *ComfyUICommand) runStart() int {
	fmt.Println("Starting ComfyUI...")
	if err := c.svc.StartComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting ComfyUI: %v\n", err)
		return 1
	}
	fmt.Println("ComfyUI started")
	return 0
}

// runStop stops the ComfyUI container.
func (c *ComfyUICommand) runStop() int {
	fmt.Println("Stopping ComfyUI...")
	if err := c.svc.StopComfyUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping ComfyUI: %v\n", err)
		return 1
	}
	fmt.Println("ComfyUI stopped")
	return 0
}

// runFluxStart handles starting a flux model.
func (c *ComfyUICommand) runFluxStart(slug string, allowMultiple bool) int {
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

// runFluxStop handles stopping a flux model.
func (c *ComfyUICommand) runFluxStop() int {
	fmt.Println("Stopping flux model...")

	if err := c.svc.DeactivateFlux(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active flux file: %v\n", err)
	}

	fmt.Println("Flux model deactivated.")
	return 0
}

// runFluxStatus shows the status of a flux model.
func (c *ComfyUICommand) runFluxStatus(slug string) int {
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

// run3DStart handles starting a 3D model.
func (c *ComfyUICommand) run3DStart(slug string, allowMultiple bool) int {
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

// run3DStop handles stopping a 3D model.
func (c *ComfyUICommand) run3DStop() int {
	fmt.Println("Stopping 3D model...")

	if err := c.svc.Deactivate3D(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove active 3d file: %v\n", err)
	}

	fmt.Println("3D model deactivated.")
	return 0
}

// run3DStatus shows the status of a 3D model.
func (c *ComfyUICommand) run3DStatus(slug string) int {
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

// runStatus shows current ComfyUI state including flux and 3D models.
func (c *ComfyUICommand) runStatus() int {
	fmt.Println("=== ComfyUI Status ===")

	// Check ComfyUI container
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", "comfyui-flux")
	if output, err := cmd.Output(); err == nil {
		state := strings.TrimSpace(string(output))
		fmt.Printf("  ComfyUI container: %s\n", state)
	} else {
		fmt.Println("  ComfyUI container: not running")
	}

	activeFlux := readActiveFile(fluxActiveFilePath(c.cfg.cfg.InstallDir))
	if activeFlux != "" {
		fmt.Printf("  Active Flux model: %s\n", activeFlux)
	} else {
		fmt.Println("  Active Flux model: none")
	}

	active3D := readActiveFile(threeDActiveFilePath(c.cfg.cfg.InstallDir))
	if active3D != "" {
		fmt.Printf("  Active 3D model: %s\n", active3D)
	} else {
		fmt.Println("  Active 3D model: none")
	}

	return 0
}

// PrintHelp prints the comfyui command help.
func (c *ComfyUICommand) PrintHelp() {
	fmt.Println(`comfyui - Manage ComfyUI and image generation models.

USAGE:
  llm-manager comfyui [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start                  Start ComfyUI (docker compose --profile comfyui up -d)
  stop                   Stop ComfyUI container
  flux start <slug>      Activate a flux model and start ComfyUI
  flux stop              Deactivate current flux model
  flux status <slug>     Show flux model status
  3d start <slug>        Activate a 3D model and start ComfyUI
  3d stop                Deactivate current 3D model
  3d status <slug>       Show 3D model status
  status                 Show ComfyUI container and model state

EXAMPLES:
  llm-manager comfyui start
  llm-manager comfyui stop
  llm-manager comfyui flux start flux-schnell
  llm-manager comfyui flux start flux-schnell --allow-multiple
  llm-manager comfyui flux stop
  llm-manager comfyui flux status flux-schnell
  llm-manager comfyui 3d start hunyuan3d
  llm-manager comfyui 3d stop
  llm-manager comfyui 3d status hunyuan3d
  llm-manager comfyui status`)
}
