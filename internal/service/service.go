// Package service provides business logic services that wrap the database layer.
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/pkg/yamlparser"
)

// ModelService handles business logic for LLM model operations.
type ModelService struct {
	db database.DatabaseManager
}

// NewModelService creates a new ModelService.
func NewModelService(db database.DatabaseManager) *ModelService {
	return &ModelService{db: db}
}

// ListModels returns all models from the database.
func (s *ModelService) ListModels() ([]models.Model, error) {
	return s.db.ListModels()
}

// GetModel returns a single model by slug.
func (s *ModelService) GetModel(slug string) (*models.Model, error) {
	return s.db.GetModel(slug)
}

// CreateModel creates a new model record.
func (s *ModelService) CreateModel(model *models.Model) error {
	return s.db.CreateModel(model)
}

// UpdateModel updates a model by slug.
func (s *ModelService) UpdateModel(slug string, updates map[string]interface{}) error {
	return s.db.UpdateModel(slug, updates)
}

// DeleteModel deletes a model by slug.
func (s *ModelService) DeleteModel(slug string) error {
	return s.db.DeleteModel(slug)
}

// UpdateModelWithYAML updates a model's fields using values from a YAML file.
// Unlike ImportModel, this does NOT check for duplicate slug.
func (s *ModelService) UpdateModelWithYAML(slug string, yamlPath string) (*models.Model, error) {
	y, err := yamlparser.ParseYAML(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if errs := yamlparser.Validate(y); len(errs) > 0 {
		var msg strings.Builder
		msg.WriteString("validation errors:\n")
		for _, e := range errs {
			fmt.Fprintf(&msg, "  - %s\n", e)
		}
		return nil, fmt.Errorf("invalid model YAML:%s", msg.String())
	}

	// Build updates map
	updates := map[string]interface{}{}
	if y.Name != "" {
		updates["name"] = y.Name
	}
	if y.HFRepo != "" {
		updates["hf_repo"] = y.HFRepo
	}
	if y.Container != "" {
		updates["container"] = y.Container
	}
	if y.Port > 0 {
		updates["port"] = y.Port
	}
	if y.Engine != "" {
		updates["engine_type"] = y.Engine
	}

	// Convert maps to JSON strings
	if len(y.EnvVars) > 0 {
		envVarsJSON, err := json.Marshal(y.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal env_vars: %w", err)
		}
		updates["env_vars"] = string(envVarsJSON)
	}
	if len(y.CommandArgs) > 0 {
		commandArgsJSON, err := json.Marshal(y.CommandArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command_args: %w", err)
		}
		updates["command_args"] = string(commandArgsJSON)
	}
	if len(y.Capabilities) > 0 {
		capabilitiesJSON, err := json.Marshal(y.Capabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
		}
		updates["capabilities"] = string(capabilitiesJSON)
	}
	if y.InputTokenCost != nil {
		updates["input_token_cost"] = *y.InputTokenCost
	}
	if y.OutputTokenCost != nil {
		updates["output_token_cost"] = *y.OutputTokenCost
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update from YAML")
	}

	if err := s.db.UpdateModel(slug, updates); err != nil {
		return nil, fmt.Errorf("failed to update model %s: %w", slug, err)
	}

	return s.db.GetModel(slug)
}

// GenerateCompose generates a docker-compose YAML for the model using the given generator.
func (s *ModelService) GenerateCompose(slug string, generator *ComposeGenerator) (string, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return "", fmt.Errorf("model %s not found: %w", slug, err)
	}

	composeYAML, err := generator.Generate(model)
	if err != nil {
		return "", fmt.Errorf("failed to generate compose file for %s: %w", slug, err)
	}

	return composeYAML, nil
}

// ContainerService handles Docker container operations.
type ContainerService struct {
	db  database.DatabaseManager
	cfg *config.Config
	mu  sync.Mutex
}

// NewContainerService creates a new ContainerService.
func NewContainerService(db database.DatabaseManager, cfg *config.Config) *ContainerService {
	return &ContainerService{db: db, cfg: cfg}
}

// ListContainers returns all containers from the database.
func (s *ContainerService) ListContainers() ([]models.Container, error) {
	return s.db.ListContainers()
}

// GetContainerStatus returns the cached status for a container.
func (s *ContainerService) GetContainerStatus(slug string) (string, error) {
	return s.db.GetContainerStatus(slug)
}

// UpdateContainerStatus updates the cached status for a container.
func (s *ContainerService) UpdateContainerStatus(slug string, status string) error {
	return s.db.UpdateContainerStatus(slug, status)
}

// RefreshContainerStatus queries Docker and updates the database.
func (s *ContainerService) RefreshContainerStatus(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.queryDockerStatus(slug)
	if err != nil {
		return fmt.Errorf("failed to query Docker for %s: %w", slug, err)
	}

	if err := s.db.UpdateContainerStatus(slug, status); err != nil {
		return fmt.Errorf("failed to update container status: %w", err)
	}

	return nil
}

// StartContainer starts a Docker container via docker compose.
func (s *ContainerService) StartContainer(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	ymlPath := model.YML
	if !strings.HasPrefix(ymlPath, "/") {
		ymlPath = s.cfg.LLMDir + "/" + ymlPath
	}

	cmd := exec.Command("docker", "compose", "-f", ymlPath, "up", "-d")
	cmd.Dir = s.cfg.LLMDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start container %s: %s (%w)", slug, string(output), err)
	}

	if err := s.db.UpdateContainerStatus(slug, "running"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for %s: %v\n", slug, err)
	}
	return nil
}

// StopContainer stops a Docker container via docker compose.
func (s *ContainerService) StopContainer(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	ymlPath := model.YML
	if !strings.HasPrefix(ymlPath, "/") {
		ymlPath = s.cfg.LLMDir + "/" + ymlPath
	}

	cmd := exec.Command("docker", "compose", "-f", ymlPath, "down")
	cmd.Dir = s.cfg.LLMDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop container %s: %s (%w)", slug, string(output), err)
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
	return s.StartContainer(slug)
}

// GetContainerLogs retrieves logs for a container.
func (s *ContainerService) GetContainerLogs(slug string, lines int) (string, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return "", fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
		return "", fmt.Errorf("model %s has no container configured", slug)
	}

	args := []string{"logs", "--tail", fmt.Sprintf("%d", lines), model.Container}
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

	if model.Container == "" {
		return "unknown", nil
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", model.Container)
	output, err := cmd.Output()
	if err != nil {
		return "unknown", nil
	}

	return strings.TrimSpace(string(output)), nil
}

// StopAllLLMs stops all LLM-type containers (those with a yml file).
func (s *ContainerService) StopAllLLMs() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	models, err := s.db.ListModels()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	var failures []string
	for _, m := range models {
		if m.Type != "llm" || m.YML == "" {
			continue
		}

		ymlPath := m.YML
		if !strings.HasPrefix(ymlPath, "/") {
			ymlPath = s.cfg.LLMDir + "/" + ymlPath
		}

		cmd := exec.Command("docker", "compose", "-f", ymlPath, "down")
		cmd.Dir = s.cfg.LLMDir
		if output, err := cmd.CombinedOutput(); err != nil {
			failures = append(failures, fmt.Sprintf("  %s (%s): %s", m.Slug, m.YML, strings.TrimSpace(string(output))))
		}
	}

	if len(failures) > 0 {
		var msg strings.Builder
		msg.WriteString("failed to stop the following containers:\n")
		for _, f := range failures {
			msg.WriteString(f + "\n")
		}
		return fmt.Errorf("some containers failed to stop:\n%s", msg.String())
	}

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

// StartComfyUI starts ComfyUI via profile-based docker compose.
func (s *ContainerService) StartComfyUI() error {
	composePath := filepath.Join(s.cfg.InstallDir, "docker-compose.yml")
	cmd := exec.Command("docker", "compose", "-f", composePath, "--profile", "comfyui", "up", "-d", "comfyui")
	cmd.Dir = s.cfg.InstallDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start ComfyUI: %s (%w)", string(output), err)
	}
	return nil
}

// StopComfyUI stops the ComfyUI container.
func (s *ContainerService) StopComfyUI() error {
	// Check if container is running first
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", "comfyui-flux")
	output, err := cmd.Output()
	if err != nil {
		// Container doesn't exist or can't be inspected
		return nil
	}

	state := strings.TrimSpace(string(output))
	if state == "running" {
		stopCmd := exec.Command("docker", "stop", "comfyui-flux")
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop ComfyUI container: %w", err)
		}
	}

	return nil
}

// StartEmbed starts the embed container via docker start.
func (s *ContainerService) StartEmbed() error {
	cmd := exec.Command("docker", "start", "llm-embed")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start embed container: %s (%w)", string(output), err)
	}
	fmt.Println("Embed container up on port 8020")
	return nil
}

// StopEmbed stops the embed container if running.
func (s *ContainerService) StopEmbed() error {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", "llm-embed")
	output, err := cmd.Output()
	if err != nil {
		// Container doesn't exist or can't be inspected
		return nil
	}

	state := strings.TrimSpace(string(output))
	if state == "running" {
		stopCmd := exec.Command("docker", "stop", "llm-embed")
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop embed container: %w", err)
		}
	}

	return nil
}

// StartRerank starts the rerank container via docker start.
func (s *ContainerService) StartRerank() error {
	cmd := exec.Command("docker", "start", "llm-rerank")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start rerank container: %s (%w)", string(output), err)
	}
	fmt.Println("Rerank container up on port 8021")
	return nil
}

// StopRerank stops the rerank container if running.
func (s *ContainerService) StopRerank() error {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", "llm-rerank")
	output, err := cmd.Output()
	if err != nil {
		// Container doesn't exist or can't be inspected
		return nil
	}

	state := strings.TrimSpace(string(output))
	if state == "running" {
		stopCmd := exec.Command("docker", "stop", "llm-rerank")
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop rerank container: %w", err)
		}
	}

	return nil
}

// StartSpeech starts whisper-stt and kokoro-tts via profile-based docker compose.
func (s *ContainerService) StartSpeech() error {
	composePath := filepath.Join(s.cfg.InstallDir, "docker-compose.yml")
	cmd := exec.Command("docker", "compose", "-f", composePath, "--profile", "speech", "up", "-d", "whisper-stt", "kokoro-tts")
	cmd.Dir = s.cfg.InstallDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start speech services: %s (%w)", string(output), err)
	}
	return nil
}

// StopSpeech stops whisper-stt and kokoro-tts containers.
func (s *ContainerService) StopSpeech() error {
	for _, name := range []string{"whisper-stt", "kokoro-tts"} {
		cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", name)
		output, err := cmd.Output()
		if err != nil {
			// Container doesn't exist or can't be inspected
			continue
		}

		state := strings.TrimSpace(string(output))
		if state == "running" {
			stopCmd := exec.Command("docker", "stop", name)
			if err := stopCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to stop %s: %v\n", name, err)
			}
		}
	}

	return nil
}

// HotspotService handles hotspot (most recent model) operations.
type HotspotService struct {
	db  database.DatabaseManager
	cfg *config.Config
	mu  sync.Mutex
}

// NewHotspotService creates a new HotspotService.
func NewHotspotService(db database.DatabaseManager) *HotspotService {
	return &HotspotService{db: db}
}

// NewHotspotServiceWithConfig creates a new HotspotService with config.
func NewHotspotServiceWithConfig(db database.DatabaseManager, cfg *config.Config) *HotspotService {
	return &HotspotService{db: db, cfg: cfg}
}

// GetCurrentHotspot returns the active hotspot model slug.
func (s *HotspotService) GetCurrentHotspot() (*models.Hotspot, error) {
	return s.db.GetHotspot()
}

// SetHotspot sets the active hotspot model.
func (s *HotspotService) SetHotspot(slug string) error {
	// Verify model exists
	if _, err := s.db.GetModel(slug); err != nil {
		return fmt.Errorf("model %s not found: %w", slug, err)
	}

	return s.db.SetHotspot(slug)
}

// ClearHotspot removes the active hotspot.
func (s *HotspotService) ClearHotspot() error {
	return s.db.ClearHotspot()
}

// StopHotspot stops the hotspot container and clears the hotspot.
func (s *HotspotService) StopHotspot() error {
	hotspot, err := s.db.GetHotspot()
	if err != nil {
		return fmt.Errorf("failed to get hotspot: %w", err)
	}
	if hotspot == nil {
		return fmt.Errorf("no active hotspot model")
	}

	model, err := s.db.GetModel(hotspot.ModelSlug)
	if err != nil {
		return fmt.Errorf("hotspot model not found: %w", err)
	}

	ymlPath := model.YML
	if !strings.HasPrefix(ymlPath, "/") {
		ymlPath = s.cfg.LLMDir + "/" + ymlPath
	}

	cmd := exec.Command("docker", "compose", "-f", ymlPath, "down")
	cmd.Dir = s.cfg.LLMDir
	if output, err := cmd.CombinedOutput(); err != nil {
		// Non-fatal: container may already be stopped
		_ = output
	}

	if err := s.db.ClearHotspot(); err != nil {
		return fmt.Errorf("failed to clear hotspot: %w", err)
	}

	return nil
}

// RestartHotspot restarts the hotspot container in-place.
func (s *HotspotService) RestartHotspot() error {
	hotspot, err := s.db.GetHotspot()
	if err != nil {
		return fmt.Errorf("failed to get hotspot: %w", err)
	}
	if hotspot == nil {
		return fmt.Errorf("no active hotspot model")
	}

	model, err := s.db.GetModel(hotspot.ModelSlug)
	if err != nil {
		return fmt.Errorf("hotspot model not found: %w", err)
	}

	ymlPath := model.YML
	if !strings.HasPrefix(ymlPath, "/") {
		ymlPath = s.cfg.LLMDir + "/" + ymlPath
	}

	// Stop the container
	stopCmd := exec.Command("docker", "compose", "-f", ymlPath, "down")
	stopCmd.Dir = s.cfg.LLMDir
	if output, err := stopCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop hotspot container: %s\n", strings.TrimSpace(string(output)))
	}

	// Start the container
	startCmd := exec.Command("docker", "compose", "-f", ymlPath, "up", "-d")
	startCmd.Dir = s.cfg.LLMDir
	if output, err := startCmd.CombinedOutput(); err != nil {
		// Start failed — keep the hotspot but warn
		fmt.Fprintf(os.Stderr, "  Warning: failed to restart hotspot container: %s\n", strings.TrimSpace(string(output)))
		fmt.Fprintf(os.Stderr, "  Hotspot model %s still set as active (container not running)\n", hotspot.ModelSlug)
		return fmt.Errorf("failed to restart hotspot container: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// ServiceService provides high-level service orchestration.
type ServiceService struct {
	db        database.DatabaseManager
	cfg       *config.Config
	model     *ModelService
	container *ContainerService
}

// NewServiceService creates a new ServiceService.
func NewServiceService(db database.DatabaseManager, cfg *config.Config) *ServiceService {
	return &ServiceService{
		db:        db,
		cfg:       cfg,
		model:     NewModelService(db),
		container: NewContainerService(db, cfg),
	}
}

// ListServices returns all models with their container status.
func (s *ServiceService) ListServices() ([]ServiceStatus, error) {
	models, err := s.db.ListModels()
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var services []ServiceStatus
	for _, m := range models {
		status, _ := s.db.GetContainerStatus(m.Slug)
		if status == "" {
			status = "stopped"
		}

		services = append(services, ServiceStatus{
			Slug:      m.Slug,
			Name:      m.Name,
			Type:      m.Type,
			Port:      m.Port,
			Container: m.Container,
			Status:    status,
		})
	}

	return services, nil
}

// StartService starts a model's container.
func (s *ServiceService) StartService(slug string) error {
	return s.container.StartContainer(slug)
}

// StopService stops a model's container.
func (s *ServiceService) StopService(slug string) error {
	return s.container.StopContainer(slug)
}

// RestartService restarts a model's container.
func (s *ServiceService) RestartService(slug string) error {
	return s.container.RestartContainer(slug)
}

// ServiceStatus represents a model with its runtime status.
type ServiceStatus struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Port      int    `json:"port"`
	Container string `json:"container"`
	Status    string `json:"status"`
}

// MemoryService provides system memory and resource information.
type MemoryService struct {
	cfg *config.Config
}

// NewMemoryService creates a new MemoryService.
func NewMemoryService(cfg *config.Config) *MemoryService {
	return &MemoryService{cfg: cfg}
}

// GetSystemInfo returns system resource information.
func (s *MemoryService) GetSystemInfo() (SystemInfo, error) {
	info := SystemInfo{
		DataDir:    s.cfg.DataDir,
		LogDir:     s.cfg.LogDir,
		LLMDir:     s.cfg.LLMDir,
		InstallDir: s.cfg.InstallDir,
		HFCacheDir: s.cfg.HFCacheDir,
	}

	// Get disk usage for data directory
	if diskUsage, err := getDiskUsage(s.cfg.DataDir); err == nil {
		info.DataDirUsage = diskUsage
	}

	// Get disk usage for LLM directory
	if diskUsage, err := getDiskUsage(s.cfg.LLMDir); err == nil {
		info.LLMDirUsage = diskUsage
	}

	return info, nil
}

// SystemInfo holds system resource information.
type SystemInfo struct {
	DataDir      string        `json:"data_dir"`
	DataDirUsage DiskUsageInfo `json:"data_dir_usage,omitempty"`
	LogDir       string        `json:"log_dir"`
	LLMDir       string        `json:"llm_dir"`
	LLMDirUsage  DiskUsageInfo `json:"llm_dir_usage,omitempty"`
	InstallDir   string        `json:"install_dir"`
	HFCacheDir   string        `json:"hf_cache_dir"`
}

// DiskUsageInfo holds disk usage statistics.
type DiskUsageInfo struct {
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	UsedPct float64 `json:"used_pct"`
}

// getDiskUsage returns disk usage for the given path.
func getDiskUsage(path string) (DiskUsageInfo, error) {
	cmd := exec.Command("df", "--output=target,size,used,avail,pcent", path)
	output, err := cmd.Output()
	if err != nil {
		return DiskUsageInfo{}, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return DiskUsageInfo{}, fmt.Errorf("unexpected df output")
	}

	// Parse the second line (first line is header)
	parts := strings.Fields(lines[1])
	if len(parts) < 5 {
		return DiskUsageInfo{}, fmt.Errorf("unexpected df output format")
	}

	// Parse percentage (remove %)
	pct := 0.0
	for _, r := range parts[4] {
		if r >= '0' && r <= '9' {
			pct = pct*10 + float64(r-'0')
		}
	}

	return DiskUsageInfo{
		Total:   0, // Would need statfs for bytes
		UsedPct: pct,
	}, nil
}
