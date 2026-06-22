// Package service provides business logic services that wrap the database layer.
package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// getComposeProjectPrefix returns the docker-compose project-name prefix for
// the given model type, ensuring each type gets its own network namespace.
func getComposeProjectPrefix(modelType string) string {
	return modelType + "-"
}

// StartModelBySlug reads a model from the database by slug and starts its
// Docker container using the container name stored in model.Container.
// It always regenerates the compose YAML from DB state before starting,
// ensuring engine config changes are picked up.
func (s *ContainerService) StartModelBySlug(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.GetContainerName() == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	// Always regenerate compose YAML to stay in sync with engine config changes.
	if err := s.ensureCompose(model); err != nil {
		return fmt.Errorf("failed to regenerate compose: %w", err)
	}

	ymlPath := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")
	projectName := getComposeProjectPrefix(model.Type) + strings.ReplaceAll(model.Slug, ".", "-")
	composeDir := filepath.Dir(ymlPath)

	// Aggressively clean up any existing container with the same name, regardless of state.
	// This covers containers that were manually created, left over from crashed sessions,
	// or created by a different compose project that used the same container name.
	// docker compose up cannot reuse a name that is already in use (even if stopped).
	fmt.Fprintf(os.Stderr, "  Cleaning up stale container %s...\n", model.GetContainerName())
	exec.Command("docker", "stop", model.GetContainerName()).Run()     // stop if running, ignore errors
	exec.Command("docker", "rm", "-f", model.GetContainerName()).Run() // force remove, ignore errors

	// Always create fresh via compose
	composeUp := exec.Command("docker", "compose", "--project-name", projectName, "-f", ymlPath, "up", "-d")
	composeUp.Dir = composeDir
	if composeOut, composeErr := composeUp.CombinedOutput(); composeErr != nil {
		return fmt.Errorf("failed to create container %s: %s (%w)", model.GetContainerName(), string(composeOut), composeErr)
	}
	fmt.Printf("Container %s created\n", model.GetContainerName())

	return nil
}

// StopModelBySlug reads a model from the database by slug, checks the Docker
// container status, and stops it if running.
func (s *ContainerService) StopModelBySlug(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.GetContainerName() == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", model.GetContainerName())
	output, err := cmd.Output()
	if err != nil {
		// Container doesn't exist or can't be inspected — nothing to stop
		return nil
	}

	state := strings.TrimSpace(string(output))
	if state == "running" {
		stopCmd := exec.Command("docker", "stop", model.GetContainerName())
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop container %s: %w", slug, err)
		}
		fmt.Printf("Container %s stopped\n", model.GetContainerName())
	} else {
		fmt.Printf("Container %s is not running (state: %s)\n", model.GetContainerName(), state)
	}

	return nil
}

// StopAllBySubType stops all running containers whose model type and subtype
// match the given values. It is a per-subtype replacement for StopAllLLMs,
// ensuring that starting a new embedding model only stops the old one — it
// never touches LLM chat containers or other subtypes.
func (s *ContainerService) StopAllBySubType(modelType string, subType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list running containers: %w", err)
	}

	runningNames := make(map[string]bool)
	for _, name := range strings.Fields(string(output)) {
		runningNames[name] = true
	}

	models, err := s.db.ListModels()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	var stopped int
	for _, m := range models {
		if m.GetContainerName() == "" {
			continue
		}
		if m.Type != modelType || m.SubType != subType {
			continue
		}
		if !runningNames[m.GetContainerName()] {
			continue
		}
		stopCmd := exec.Command("docker", "stop", m.GetContainerName())
		if err := stopCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to stop %s: %v\n", m.GetContainerName(), err)
		} else {
			stopped++
		}
	}

	fmt.Printf("  Stopped %d %s/%s container(s)\n", stopped, modelType, subType)
	return nil
}

// StartModelBySlugWithAllow reads a model from the database by slug and starts its
// Docker container. When allowMultiple is false, it first stops any other
// running container of the same type+subtype so only one instance of each
// subtype runs at a time. When allowMultiple is true, it starts without
// stopping peers.
func (s *ContainerService) StartModelBySlugWithAllow(slug string, allowMultiple bool) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	// Check GPU memory availability (speech models too) before any container ops.
	if model.TotalParamsB != nil && model.QuantBytesPerParam != nil {
		if err := s.checkGPUMemory(slug, StartOverrides{}); err != nil {
			// checkGPUMemory already prints the detailed breakdown; log reason only here.
			return fmt.Errorf("insufficient GPU memory for %s: %w", slug, err)
		}
	}

	if model.GetContainerName() == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	if !allowMultiple {
		fmt.Printf("Stopping other %s/%s containers...\n", model.Type, model.SubType)
		if err := s.StopAllBySubType(model.Type, model.SubType); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to stop other containers: %v\n", err)
		}
	} else {
		fmt.Printf("Starting without stopping other %s/%s containers (--allow-multiple)\n", model.Type, model.SubType)
	}

	// Always regenerate compose YAML to stay in sync with engine config changes.
	if err := s.ensureCompose(model); err != nil {
		return fmt.Errorf("failed to regenerate compose: %w", err)
	}

	ymlPath := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")
	projectName := getComposeProjectPrefix(model.Type) + strings.ReplaceAll(model.Slug, ".", "-")
	composeDir := filepath.Dir(ymlPath)

	// Aggressively clean up any existing container with the same name, regardless of state.
	// This covers containers that were manually created, left over from crashed sessions,
	// or created by a different compose project that used the same container name.
	// docker compose up cannot reuse a name that is already in use (even if stopped).
	fmt.Fprintf(os.Stderr, "  Cleaning up stale container %s...\n", model.GetContainerName())
	exec.Command("docker", "stop", model.GetContainerName()).Run()     // stop if running, ignore errors
	exec.Command("docker", "rm", "-f", model.GetContainerName()).Run() // force remove, ignore errors

	// Always create fresh via compose
	composeUp := exec.Command("docker", "compose", "--project-name", projectName, "-f", ymlPath, "up", "-d")
	composeUp.Dir = composeDir
	if composeOut, composeErr := composeUp.CombinedOutput(); composeErr != nil {
		return fmt.Errorf("failed to create container %s: %s (%w)", model.GetContainerName(), string(composeOut), composeErr)
	}
	fmt.Printf("Container %s created\n", model.GetContainerName())

	return nil
}

// StartModelWithHealthCheck starts a model container and then polls its
// /health endpoint until it returns HTTP 200.
// This is the recommended way to start RAG models (embedding + reranker)
// on a shared GPU — start the first model, wait for it to be healthy,
// then start the second model to avoid simultaneous vLLM startup contention.
func (s *ContainerService) StartModelWithHealthCheck(slug string, allowMultiple bool) error {
	if err := s.StartModelBySlugWithAllow(slug, allowMultiple); err != nil {
		return err
	}

	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found after start: %w", err)
	}
	if model.Port == 0 {
		fmt.Fprintf(os.Stderr, "  Warning: model %s has no port configured, skipping health check\n", slug)
		return nil
	}

	healthURL := fmt.Sprintf("http://127.0.0.1:%d", model.Port)
	fmt.Fprintf(os.Stderr, "  Waiting for %s to become healthy at %s/health ...\n", slug, healthURL)
	if err := s.waitForModelHealthy(model.Slug, healthURL); err != nil {
		return fmt.Errorf("health check failed for %s at %s: %w", slug, healthURL, err)
	}
	fmt.Fprintf(os.Stderr, "  ✓ %s is healthy\n", slug)
	return nil
}

// waitForModelHealthy polls the /health endpoint of a model container until
// it returns HTTP 200, the context deadline expires, or an error occurs.
// Uses a 180-second timeout with 3-second intervals.
func (s *ContainerService) waitForModelHealthy(slug string, baseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	client := &http.Client{Timeout: healthCheckClientTimeout}
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out after %s waiting for %s to become healthy", healthCheckTimeout, slug)
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
			if err != nil {
				continue
			}

			resp, err := client.Do(req)
			if err != nil {
				// Best-effort: continue polling on transient errors
				continue
			}

			if resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}

			resp.Body.Close()
			// Continue polling on non-200
		}
	}
}

// GetModelStatus returns model metadata along with the Docker container
// running/stopped status for a given slug. Returns "running", "stopped", or
// "unknown" as the status string.
func (s *ContainerService) GetModelStatus(slug string) (ModelStatus, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return ModelStatus{}, fmt.Errorf("model not found: %w", err)
	}

	status := "unknown"
	if model.GetContainerName() != "" {
		cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", model.GetContainerName())
		output, inspectErr := cmd.Output()
		if inspectErr == nil {
			status = strings.TrimSpace(string(output))
		}
	}

	return ModelStatus{
		Name:      model.Name,
		Slug:      model.Slug,
		Container: model.Container,
		Port:      model.Port,
		Status:    status,
	}, nil
}
