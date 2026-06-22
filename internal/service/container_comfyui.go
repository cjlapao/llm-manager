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

// StartComfyUI starts ComfyUI via dynamic Docker Compose provisioning.
//
// It resolves the ComfyUI engine type and latest version from the database,
// validates the volume path, generates a docker-compose YAML, starts the
// container, and polls the health endpoint (root path /) until it returns HTTP 200.
//
// The generated compose file is written to $INSTALL_DIR/.llm-manager/comfyui-compose.yml.
func (s *ContainerService) StartComfyUI() error {
	// 1. Resolve the ComfyUI engine type and latest version from the database
	//    using the existing EngineService (s.svc) set in NewContainerService.
	latest, err := s.svc.ResolveLatestVersion("comfyui")
	if err != nil {
		return fmt.Errorf("no ComfyUI engine version found: %w", err)
	}

	// 2. Extract image name, tag, container name from the engine version.
	image := latest.Image // e.g. "comfyanonymous/ComfyUI"
	tag := "latest"
	if latest.Version != "" {
		tag = latest.Version
	}
	containerName := latest.ContainerName // e.g. "comfyui-flux"
	if containerName == "" {
		containerName = "comfyui-flux"
	}

	// 3. Resolve volume path from volumes_json.
	//    Pick the first host path deterministically; fall back to a default.
	volumes := latest.GetVolumes()
	volumePath := pickFirstVolumePath(volumes)
	if volumePath == "" {
		volumePath = filepath.Join(s.cfg.InstallDir, ".comfyui", "models")
	}

	// 4. Validate/create the volume path on the host filesystem.
	if err := os.MkdirAll(volumePath, 0o755); err != nil {
		return fmt.Errorf("failed to create volume path %s: %w", volumePath, err)
	}

	// 5. Resolve host port — default to 8188 (ComfyUI's default).
	hostPort := 8188

	// 6. Generate the Docker Compose YAML.
	composeGen, err := NewComposeGenerator()
	if err != nil {
		return fmt.Errorf("failed to create compose generator: %w", err)
	}
	composeYAML, err := composeGen.GenerateComfyUICompose(ComfyUIComposeTemplateData{
		ImageName:     image,
		ImageTag:      tag,
		HostPort:      hostPort,
		VolumePath:    volumePath,
		ContainerName: containerName,
	})
	if err != nil {
		return fmt.Errorf("failed to generate ComfyUI compose YAML: %w", err)
	}

	// 7. Write the YAML to a known location.
	composeDir := filepath.Join(s.cfg.InstallDir, ".llm-manager")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		return fmt.Errorf("failed to create compose directory %s: %w", composeDir, err)
	}
	composePath := filepath.Join(composeDir, "comfyui-compose.yml")
	if err := os.WriteFile(composePath, []byte(composeYAML), 0o644); err != nil {
		return fmt.Errorf("failed to write compose file %s: %w", composePath, err)
	}

	// 8. Clean up any stale container with the same name before provisioning.
	//    This matches the pattern used in StartContainer and StartModelBySlug.
	fmt.Fprintf(os.Stderr, "  Cleaning up stale container %s...\n", containerName)
	exec.Command("docker", "rm", "-f", containerName).Run() // ignore errors — container may not exist

	// 9. Execute docker compose --project-name comfyui -f <path> up -d.
	cmd := exec.Command("docker", "compose", "--project-name", "comfyui", "-f", composePath, "up", "-d")
	cmd.Dir = s.cfg.InstallDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start ComfyUI: %v\nDocker output:\n%s", err, strings.TrimSpace(string(output)))
	}

	// 10. Poll health endpoint at http://127.0.0.1:<host_port>/ (root path) for HTTP 200.
	healthURL := fmt.Sprintf("http://127.0.0.1:%d", hostPort)
	if err := s.waitForComfyUIHealthy(healthURL); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	// 11. Update container status to RUNNING in database.
	if err := s.db.UpdateContainerStatus("comfyui", "running"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for comfyui: %v\n", err)
	}

	return nil
}

// waitForComfyUIHealthy polls the root path (/) of the ComfyUI container
// until it returns HTTP 200, the context deadline expires, or an error occurs.
// Uses a 180-second timeout with 3-second intervals, following the pattern
// from cmd/health.go's waitForHealthy().
func (s *ContainerService) waitForComfyUIHealthy(baseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out after 180s waiting for ComfyUI to become healthy")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
			if err != nil {
				return fmt.Errorf("failed to create health check request: %w", err)
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

// pickFirstVolumePath returns the first host path from a volumes map.
// The map keys are container paths; the values are host paths.
// Returns an empty string if the map is nil or empty.
func pickFirstVolumePath(volumes map[string]string) string {
	if volumes == nil {
		return ""
	}
	for _, hostPath := range volumes {
		return hostPath
	}
	return ""
}

// StopComfyUI stops the ComfyUI container by name and updates the database status.
//
// It uses docker stop on the container (default: comfyui-flux) and sets the
// container status to STOPPED in the database. If the container does not exist,
// it still updates the DB status to stopped (best-effort) and returns nil.
func (s *ContainerService) StopComfyUI() error {
	containerName := "comfyui-flux"

	// Check if container is running first.
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		// Container doesn't exist or can't be inspected — update DB anyway
		// so the status reflects that we attempted to stop it.
		fmt.Fprintf(os.Stderr, "  Container %s not found, updating status to stopped\n", containerName)
		if stopErr := s.db.UpdateContainerStatus("comfyui", "stopped"); stopErr != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for comfyui: %v\n", stopErr)
		}
		return nil
	}

	state := strings.TrimSpace(string(output))
	if state == "running" {
		stopCmd := exec.Command("docker", "stop", containerName)
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop ComfyUI container %s: %w", containerName, err)
		}
	}

	// Update container status in database.
	if err := s.db.UpdateContainerStatus("comfyui", "stopped"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for comfyui: %v\n", err)
	}

	return nil
}

// ActivateFlux writes the flux model name to the active flux model file.
func (s *ContainerService) ActivateFlux(model string) error {
	dir := filepath.Join(s.cfg.InstallDir, "comfyui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create comfyui directory: %w", err)
	}

	path := filepath.Join(dir, ".active-model")
	if err := os.WriteFile(path, []byte(model), 0o644); err != nil {
		return fmt.Errorf("failed to write active flux file: %w", err)
	}

	return nil
}

// DeactivateFlux removes the active flux model file.
func (s *ContainerService) DeactivateFlux() error {
	path := filepath.Join(s.cfg.InstallDir, "comfyui", ".active-model")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove active flux file: %w", err)
	}
	return nil
}

// Activate3D writes the 3D model name to the active 3D model file.
func (s *ContainerService) Activate3D(model string) error {
	dir := filepath.Join(s.cfg.InstallDir, "comfyui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create comfyui directory: %w", err)
	}

	path := filepath.Join(dir, ".active-3d")
	if err := os.WriteFile(path, []byte(model), 0o644); err != nil {
		return fmt.Errorf("failed to write active 3d file: %w", err)
	}

	return nil
}

// Deactivate3D removes the active 3D model file.
func (s *ContainerService) Deactivate3D() error {
	path := filepath.Join(s.cfg.InstallDir, "comfyui", ".active-3d")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove active 3d file: %w", err)
	}
	return nil
}

