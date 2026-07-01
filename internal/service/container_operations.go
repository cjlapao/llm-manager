package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
func (s *ContainerService) StartContainer(slug string, allowMultiple bool, overrides StartOverrides) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.GetContainerName() == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	if !allowMultiple {
		fmt.Println("Stopping all other LLM containers...")
		if err := s.StopAllLLMs(); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to stop other LLM containers: %v\n", err)
		}
	} else {
		fmt.Println("Starting without stopping other LLM containers (--allow-multiple)")
	}

	// Always regenerate compose YAML to stay in sync with engine config changes.
	if err := s.ensureComposeWithOptions(model, overrides); err != nil {
		return fmt.Errorf("failed to regenerate compose: %w", err)
	}

	composeFile := filepath.Join(s.cfg.LLMDir, slug+".yml")

	// Run pre-flight checks before attempting to start.
	if err := s.preFlightChecks(slug, composeFile, overrides); err != nil {
		return err
	}

	projectName := "llm-" + strings.ReplaceAll(slug, ".", "-")

	// Ensure a clean slate: bring down any leftover compose state for this project.
	// This handles the case where a previous session left stopped containers or
	// orphaned networks that block docker compose up from recreating them.
	downCmd := exec.Command("docker", "compose", "--project-name", projectName, "-f", composeFile, "down")
	downCmd.Dir = s.cfg.LLMDir
	_ = downCmd.Run() // non-fatal — just cleans up stale state

	// Aggressively remove any container with the expected name, regardless of state.
	// This covers containers that were manually created, left over from crashed sessions,
	// or created by a different compose project that used the same container name.
	// docker compose down won't touch these.
	if model.GetContainerName() != "" {
		exec.Command("docker", "stop", model.GetContainerName()).Run()     // stop if running, ignore errors
		exec.Command("docker", "rm", "-f", model.GetContainerName()).Run() // force remove, ignore errors
	}

	// Debug: log memory calculation values
	if model.TotalParamsB != nil && model.QuantBytesPerParam != nil {
		freeGPUmb := ReadFreeGPUMemory()
		fmt.Fprintf(os.Stderr, "  Memory profile: %.1fB params, %.1f bytes/param, free GPU: %d MB\n",
			*model.TotalParamsB, *model.QuantBytesPerParam, freeGPUmb)
		fmt.Fprintf(os.Stderr, "  Estimated weights: %.0f MB\n",
			*model.TotalParamsB**model.QuantBytesPerParam*1024)
	}

	cmd := exec.Command("docker", "compose", "--project-name", projectName, "-f", composeFile, "up", "-d")
	cmd.Dir = s.cfg.LLMDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container %s: %v\nDocker output:\n%s", slug, err, strings.TrimSpace(string(output)))
	}

	if err := s.db.UpdateContainerStatus(slug, "running"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for %s: %v\n", slug, err)
	}

	// Activate LiteLLM alias for this model (deactivates any previous model's alias)
	if s.litellm != nil && model.Type == "llm" {
		if err := s.litellm.ActivateModel(slug); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to activate LiteLLM alias for %s: %v\n", slug, err)
		}
	}

	return nil
}

// StartContainerDryRun is a dry-run variant of StartContainer. It performs all
// preparation and pre-flight checks (most importantly GPU memory) without
// touching Docker at all — no compose down, no rm, no up, no DB status update.
// It is safe to call when the machine is offline or Docker is unreachable.
// Returns nil on success, error only if the model is not found or GPU memory
// is insufficient.
func (s *ContainerService) StartContainerDryRun(slug string, overrides StartOverrides) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.GetContainerName() == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	fmt.Println("[dry-run] Pre-flight: checking GPU memory...")
	if err := s.checkGPUMemory(slug, overrides); err != nil {
		return err
	}
	fmt.Println("[dry-run] Pre-flight: GPU memory OK")

	return nil
}

// StopContainer stops a Docker container by name.
func (s *ContainerService) StopContainer(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.GetContainerName() == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	cmd := exec.Command("docker", "stop", model.GetContainerName())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", slug, err)
	}

	if err := s.db.UpdateContainerStatus(slug, "stopped"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for %s: %v\n", slug, err)
	}
	return nil
}

// RestartContainer restarts a Docker container.
func (s *ContainerService) RestartContainer(slug string) error {
	if err := s.StopContainer(slug); err != nil {
		return err
	}
	return s.StartContainer(slug, false, StartOverrides{})
}

// GetContainerLogs retrieves logs for a container.
func (s *ContainerService) GetContainerLogs(slug string, lines int) (string, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return "", fmt.Errorf("model not found: %w", err)
	}

	if model.GetContainerName() == "" {
		return "", fmt.Errorf("model %s has no container configured", slug)
	}

	args := []string{"logs", "--tail", fmt.Sprintf("%d", lines), model.GetContainerName()}
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs for %s: %s (%w)", slug, string(output), err)
	}

	return string(output), nil
}

// queryDockerStatus queries Docker for the actual status of a container.
func (s *ContainerService) queryDockerStatus(slug string) (string, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return "unknown", nil
	}

	if model.GetContainerName() == "" {
		return "unknown", nil
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", model.GetContainerName())
	output, err := cmd.Output()
	if err != nil {
		return "unknown", nil
	}

	return strings.TrimSpace(string(output)), nil
}

// StopAllLLMs stops all running LLM-type containers by name.
// It cross-references docker ps with DB models, stops LLM containers,
// and skips non-LLM containers (embed, comfyui, etc.).
func (s *ContainerService) StopAllLLMs() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all running container names
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list running containers: %w", err)
	}

	runningNames := make(map[string]bool)
	for _, name := range strings.Fields(string(output)) {
		runningNames[name] = true
	}

	// Get all models from DB
	models, err := s.db.ListModels()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	var stopped int
	var skippedNonLLM int
	for _, m := range models {
		if m.GetContainerName() == "" {
			continue
		}

		if m.Type != "llm" {
			if runningNames[m.GetContainerName()] {
				skippedNonLLM++
			}
			continue
		}

		if !runningNames[m.GetContainerName()] {
			continue // not running, nothing to stop
		}

		stopCmd := exec.Command("docker", "stop", m.GetContainerName())
		if err := stopCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ Failed to stop %s: %v\n", m.GetContainerName(), err)
		} else {
			stopped++
		}
	}

	fmt.Printf("  Stopped %d LLM container(s), skipped %d non-LLM container(s)\n", stopped, skippedNonLLM)
	return nil
}

// DropPageCache drops the OS page cache by writing to /proc/sys/vm/drop_caches.
func (s *ContainerService) DropPageCache() error {
	// sync first — best-effort, log warning on failure
	syncCmd := exec.Command("sync")
	if err := syncCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to sync: %v\n", err)
	}

	// echo 3 > /proc/sys/vm/drop_caches — best-effort, log warning on failure
	dropCmd := exec.Command("sh", "-c", "echo 3 | tee /proc/sys/vm/drop_caches > /dev/null 2>&1 || true")
	if err := dropCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to drop page cache: %v\n", err)
	}

	return nil
}
