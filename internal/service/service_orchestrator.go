// Package service provides business logic services that wrap the database layer.
package service

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

const (
	StatusChecking = "checking"
	StatusRunning  = "running"
	StatusStopped  = "stopped"
	StatusError    = "error"
)

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
		model:     NewModelService(db, cfg),
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
func (s *ServiceService) StartService(slug string, allowMultiple bool) error {
	return s.container.StartContainer(slug, allowMultiple, StartOverrides{})
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

// ModelStatus holds model metadata and Docker container status.
type ModelStatus struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Container string `json:"container"`
	Port      int    `json:"port"`
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

// parseJSONToMap deserializes a JSON-encoded map from a string field.
// Returns nil when the input is empty or invalid JSON.
func parseJSONToMap(jsonStr string) map[string]string {
	if jsonStr == "" {
		return nil
	}
	m := make(map[string]string)
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil
	}
	return m
}

// parseJSONToArray deserializes a JSON-encoded string array from a field.
// Returns nil when the input is empty or invalid JSON.
func parseJSONToArray(jsonStr string) []string {
	if jsonStr == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return nil
	}
	return arr
}
