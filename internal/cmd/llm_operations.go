package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/user/llm-manager/internal/service"
)

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
