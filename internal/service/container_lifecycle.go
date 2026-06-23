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
)

type ContainerService struct {
	db  database.DatabaseManager
	cfg *config.Config
	svc *EngineService
	mu  sync.Mutex
}

// NewContainerService creates a new ContainerService.
func NewContainerService(db database.DatabaseManager, cfg *config.Config) *ContainerService {
	return &ContainerService{db: db, cfg: cfg, svc: NewEngineService(db)}
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

// ensureCompose regenerates the docker-compose YAML file for a model from the
// current database state. It always overwrites the file to keep it in sync
// with engine config changes (new versions, env vars, volumes, etc.).
func (s *ContainerService) ensureCompose(model *models.Model) error {
	return s.ensureComposeWithOptions(model, StartOverrides{})
}

// ensureComposeWithOptions generates compose YAML with optional CLI overrides
// applied to the auto-calculated flags.
func (s *ContainerService) ensureComposeWithOptions(model *models.Model, overrides StartOverrides) error {
	composeGen, err := NewComposeGenerator()
	if err != nil {
		return fmt.Errorf("failed to create compose generator: %w", err)
	}
	cfg, err := s.svc.BuildComposeConfig(model)
	if err != nil {
		return fmt.Errorf("failed to resolve engine config: %w", err)
	}
	composeYAML, err := composeGen.GenerateWithOptions(model, *cfg, overrides)
	if err != nil {
		return fmt.Errorf("failed to generate compose YAML: %w", err)
	}
	ymlPath := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")
	if err := os.WriteFile(ymlPath, []byte(composeYAML), 0o644); err != nil {
		return fmt.Errorf("failed to write compose file %s: %w", ymlPath, err)
	}
	return nil
}

// ──────────────────────────────────────────────
// Pre-flight checks
// ──────────────────────────────────────────────

// preFlightChecks runs a series of diagnostic checks before attempting to start
// a container. It returns early on the first fatal issue. Checks that can be
// auto-fixed (stale containers, orphaned networks) attempt the fix and return
// nil so the caller retries the start.
func (s *ContainerService) preFlightChecks(slug string, composeFile string, overrides StartOverrides) error {
	// 1. Docker daemon reachable
	if err := s.checkDockerDaemon(); err != nil {
		return err
	}

	// 2. Stale container — handled by aggressive removal before compose up
	// (see StartContainer where we docker stop + docker rm -f the container name)

	// 3. Orphaned docker networks from crashed compose sessions
	if err := s.checkOrphanedNetworks(slug); err != nil {
		fmt.Fprintf(os.Stderr, "  Cleaning orphaned networks: %v\n", err)
		if cleanErr := s.cleanOrphanedNetworks(); cleanErr != nil {
			return fmt.Errorf("failed to clean orphaned networks: %v", cleanErr)
		}
		fmt.Fprintln(os.Stderr, "  ✓ Orphaned networks cleaned")
	}

	// 4. Port conflict
	if err := s.checkPortConflict(slug); err != nil {
		return err
	}

	// 5. NVIDIA runtime
	if err := s.checkNVIDIARuntime(); err != nil {
		return err
	}

	// 5b. GPU memory check (only for LLM-type models with profile data)
	if err := s.checkGPUMemory(slug, overrides); err != nil {
		return err
	}

	// 6. Compose file exists
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("compose file not found: %s (ensureCompose may have failed)", composeFile)
	}

	return nil
}

// checkDockerDaemon verifies that the Docker daemon is reachable.
func (s *ContainerService) checkDockerDaemon() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not running or not accessible (cannot reach docker daemon)\n  Please start Docker Desktop / Docker service and try again.\n  Tip: run 'docker ps' to verify.")
	}
	return nil
}

// checkStaleContainer looks for a container with the exact name that is not
// managed by docker compose. This catches containers started manually via
// "docker run" that conflict with compose's expectations.
func (s *ContainerService) checkStaleContainer(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil // unknown model, skip
	}
	if model.GetContainerName() == "" {
		return nil
	}

	cmd := exec.Command("docker", "inspect", model.GetContainerName())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil // container doesn't exist
	}

	// Container exists — check its state
	var results []map[string]interface{}
	if json.Unmarshal(output, &results) == nil && len(results) > 0 {
		inspectResult := results[0]
		state, _ := inspectResult["State"].(map[string]interface{})
		status, _ := state["Status"].(string)

		if status == "running" {
			// A running container blocks compose from creating a new one.
			// Check if it's compose-managed — if so, it's a leftover from a
			// previous session that didn't shut down cleanly.
			labels, _ := inspectResult["Config"].(map[string]interface{})
			if labels != nil {
				if _, hasComposeLabel := labels["com.docker.compose.project"]; hasComposeLabel {
					return fmt.Errorf("running compose container %q from a previous session", model.GetContainerName())
				}
			}
			return fmt.Errorf("container %q is running (not compose-managed)", model.GetContainerName())
		}

		// Stopped containers (exited, created, dead, etc.) — compose can't
		// recreate them either.
		if status == "exited" || status == "created" || status == "dead" || status == "removing" {
			return fmt.Errorf("stopped compose container %q from a previous session", model.GetContainerName())
		}
	}

	// Container exists but we couldn't determine state — assume stale.
	return fmt.Errorf("container %q exists in unknown state", model.GetContainerName())
}

// removeContainer forcefully removes a container by name.
// Stops it first if running, then removes.
func (s *ContainerService) removeContainer(name string) error {
	// Try to stop it first (if running) — ignore errors, rm -f handles it
	exec.Command("docker", "stop", name).Run()
	// Force remove
	cmd := exec.Command("docker", "rm", "-f", name)
	return cmd.Run()
}

// checkOrphanedNetworks finds docker networks belonging to the project that
// are not attached to any running container (left behind by crashed sessions).
func (s *ContainerService) checkOrphanedNetworks(slug string) error {
	projectName := "llm-" + strings.ReplaceAll(slug, ".", "-")

	cmd := exec.Command("docker", "network", "ls", "--filter", "name="+projectName, "--format", "{{.Name}}")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var orphaned []string

	for _, netName := range lines {
		if netName == "" {
			continue
		}
		// Check if any running container uses this network
		inspectCmd := exec.Command("docker", "network", "inspect", netName,
			"--format", "{{range .Containers}}{{.Name}} {{end}}")
		netOutput, _ := inspectCmd.Output()
		netConns := strings.TrimSpace(string(netOutput))
		if netConns == "" {
			orphaned = append(orphaned, netName)
		}
	}

	if len(orphaned) > 0 {
		return fmt.Errorf("orphaned networks found: %s", strings.Join(orphaned, ", "))
	}
	return nil
}

// cleanOrphanedNetworks removes all dangling docker networks (not just project-scoped).
// This covers the case where a machine hangs and leaves networks from other projects.
func (s *ContainerService) cleanOrphanedNetworks() error {
	cmd := exec.Command("docker", "network", "ls", "--format", "{{.Name}}")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var cleaned []string

	for _, netName := range lines {
		if netName == "" {
			continue
		}
		// Skip built-in networks
		if netName == "bridge" || netName == "host" || netName == "none" {
			continue
		}
		// Check if any running container uses this network
		inspectCmd := exec.Command("docker", "network", "inspect", netName,
			"--format", "{{range .Containers}}{{.Name}} {{end}}")
		netOutput, _ := inspectCmd.Output()
		netConns := strings.TrimSpace(string(netOutput))
		if netConns == "" {
			if rmErr := exec.Command("docker", "network", "rm", netName).Run(); rmErr == nil {
				cleaned = append(cleaned, netName)
			}
		}
	}

	if len(cleaned) > 0 {
		fmt.Fprintf(os.Stderr, "  Removed %d orphaned network(s): %s\n", len(cleaned), strings.Join(cleaned, ", "))
	}
	return nil
}

// checkPortConflict checks if the model's port is already bound by another container.
func (s *ContainerService) checkPortConflict(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil
	}
	if model.Port == 0 {
		return nil
	}

	cmd := exec.Command("docker", "ps",
		"--format", "{{.Names}}\t{{.Ports}}")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Check if port is already published by another container
	portStr := fmt.Sprintf("->%d/tcp", model.Port)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			containerName := parts[0]
			ports := parts[1]
			if strings.Contains(ports, portStr) {
				return fmt.Errorf("port %d is already in use by container %q (%s)\n  Stop the conflicting container first: docker stop %s", model.Port, containerName, ports, containerName)
			}
		}
	}
	return nil
}

// checkNVIDIARuntime verifies that the nvidia container runtime is available.
func (s *ContainerService) checkNVIDIARuntime() error {
	cmd := exec.Command("docker", "info", "--format", "{{.Runtimes}}")
	output, err := cmd.Output()
	if err != nil {
		return nil // docker info failed, skip (daemon check already ran)
	}
	if !strings.Contains(string(output), "nvidia") {
		return fmt.Errorf("NVIDIA Docker runtime not found\n  Install nvidia-container-toolkit and restart Docker:\n    sudo apt install nvidia-container-toolkit\n    sudo systemctl restart docker")
	}
	return nil
}

// checkGPUMemory checks if there is sufficient free memory to load the model.
// Uses CanFitDynamic for the full memory calculation with profile data.
func (s *ContainerService) checkGPUMemory(slug string, overrides StartOverrides) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil
	}
	if model.Type != "llm" && model.Type != "auto-complete" && model.Type != "rag" && model.Type != "speech" {
		return nil // only check for LLM, RAG, and Speech models
	}
	if model.TotalParamsB == nil || model.QuantBytesPerParam == nil {
		return nil // no profile data, skip
	}

	// For speech models, use EstimateSpeechMemory() since they don't need KV cache analysis.
	// All other types continue through the LLM-style profile building below.
	if model.Type == "speech" {
		freeRAM, _ := readMemAvailableMB()
		result, err := EstimateSpeechMemory(model, freeRAM)
		if err != nil {
			return fmt.Errorf("estimate speech memory: %w", err)
		}
		if result == nil {
			return nil // no data — treat same as missing profile data above
		}
		if !result.FitsAtMaxContext {
			availableGB := float64(result.Breakdown.WeightsMB+result.Breakdown.CUDAContextMB+result.Breakdown.PrefixCacheMB) / 1024.0
			return fmt.Errorf("speech model %s requires %.1f GB VRAM (weights %.0f MB + CUDA/context overhead %.0f MB) but only %.0f MB available — startup blocked",
				slug, availableGB,
				float64(result.Breakdown.WeightsMB)/1024,
				float64(result.Breakdown.CUDAContextMB+result.Breakdown.PrefixCacheMB)/1024,
				float64(result.AvailableMB)/1024)
		}
		// Display memory breakdown to match LLM behavior
		fmt.Fprintf(os.Stderr, "\n=== GPU Memory Calculation ===\n")
		fmt.Fprintf(os.Stderr, "  Profile: %.1fB params, %.1f bytes/param\n",
			derefOrZero(model.TotalParamsB), derefOrZero(model.QuantBytesPerParam))
		if model.DefaultContext != nil && *model.DefaultContext > 0 {
			fmt.Fprintf(os.Stderr, "  Default context window: %d tokens\n", *model.DefaultContext)
		}
		if model.MaxContext != nil && *model.MaxContext > 0 {
			fmt.Fprintf(os.Stderr, "  Max context window: %d tokens\n", *model.MaxContext)
		}
		fmt.Fprintf(os.Stderr, "  Weights:   %6d MB (%.1fB × %.1f × 1024)\n",
			result.Breakdown.WeightsMB, derefOrZero(model.TotalParamsB), derefOrZero(model.QuantBytesPerParam))
		fmt.Fprintf(os.Stderr, "  CUDA ctx:  %6d MB\n", result.Breakdown.CUDAContextMB)
		fmt.Fprintf(os.Stderr, "  Prefix cache: %6d MB\n", result.Breakdown.PrefixCacheMB)
		fmt.Fprintf(os.Stderr, "  ──────────────────────────────────\n")
		fmt.Fprintf(os.Stderr, "  Total:     %6d MB\n", result.TotalRealisticMB)
		fmt.Fprintf(os.Stderr, "  GPU util:  %.2f (total / %.0f total)  ⚠ Speech: no KV cache allocated\n",
			result.GPUMemoryUtilization, float64(result.AvailableMB))
		fmt.Fprintf(os.Stderr, "  Docker lim: %dg\n", result.DockerLimitGB)
		fmt.Fprintf(os.Stderr, "  Fits:      %v (need %d MB <= %.0f MB free)\n",
			result.FitsAtMaxContext, result.TotalRealisticMB, float64(result.AvailableMB))
		fmt.Fprintf(os.Stderr, "=================================\n")
		// Store docker limit for later use during container start
		_ = result.DockerLimitGB // set as override so compose generator gets it
		return nil
	}

	// Build profile from DB fields
	attentionLayers := 0
	if model.AttentionLayers != nil {
		attentionLayers = *model.AttentionLayers
	}
	gdnLayers := 0
	if model.GdnLayers != nil {
		gdnLayers = *model.GdnLayers
	}
	numKvHeads := 0
	if model.NumKvHeads != nil {
		numKvHeads = *model.NumKvHeads
	}
	headDim := 0
	if model.HeadDim != nil {
		headDim = *model.HeadDim
	}
	maxContext := 262144
	if model.MaxContext != nil && *model.MaxContext > 0 {
		maxContext = *model.MaxContext
	}
	defaultContext := 262144
	if model.DefaultContext != nil && *model.DefaultContext > 0 {
		defaultContext = *model.DefaultContext
	}

	profile := ModelProfile{
		TotalParamsB:              *model.TotalParamsB,
		ActiveParamsB:             0,
		IsMoe:                     model.IsMoe != nil && *model.IsMoe,
		AttentionLayers:           attentionLayers,
		GdnLayers:                 gdnLayers,
		NumKvHeads:                numKvHeads,
		HeadDim:                   headDim,
		SupportsMtp:               model.SupportsMtp != nil && *model.SupportsMtp,
		SupportsVision:            strings.Contains(model.CommandArgs, "mm-processor-cache-type"),
		DefaultContext:            defaultContext,
		MaxContext:                maxContext,
		QuantBytesPerParam:        *model.QuantBytesPerParam,
		MaxNumSeqs:                derefOrZero(model.MaxNumSeqs),
		SubType:                   model.SubType,
		KvCacheOverheadMultiplier: 1.0,
	}

	// Detect hybrid (GDN) models and apply KV cache overhead multiplier.
	if gdnLayers > 0 {
		if profile.SupportsMtp && !profile.IsMoe {
			profile.KvCacheOverheadMultiplier = 2.00
		} else {
			profile.KvCacheOverheadMultiplier = 1.44
		}
		fmt.Fprintf(os.Stderr, "  [kv-override] hybrid gdn=%d model, kv_cache_x%.2f\n", gdnLayers, profile.KvCacheOverheadMultiplier)
	}

	// Determine MTP tokens: CLI override > DB profile.
	// If CLI sets --speculative-tokens 0, MTP is disabled even if DB has it configured.
	mtpTokens := 0
	if overrides.NumSpeculativeTokens != nil {
		mtpTokens = *overrides.NumSpeculativeTokens
	} else if model.NumSpeculativeTokens != nil && *model.NumSpeculativeTokens > 0 {
		mtpTokens = *model.NumSpeculativeTokens
	}

	// Apply CLI overrides to context length and sequences for the pre-flight check.
	// Default to profile.MaxNumSeqs so the pre-flight matches what vLLM will
	// actually reserve (compose.go also starts from profile.MaxNumSeqs).
	checkContext := defaultContext
	checkSeqs := profile.MaxNumSeqs
	if checkSeqs < 1 {
		checkSeqs = 1
	}
	if overrides.MaxModelLen > 0 {
		checkContext = overrides.MaxModelLen
		fmt.Fprintf(os.Stderr, "  [override] --max-model-len=%d\n", overrides.MaxModelLen)
	}
	if overrides.MaxNumSeqs > 0 {
		checkSeqs = overrides.MaxNumSeqs
		fmt.Fprintf(os.Stderr, "  [override] --max-num-seqs=%d\n", overrides.MaxNumSeqs)
	}
	if overrides.MaxNumBatchedTokens > 0 {
		fmt.Fprintf(os.Stderr, "  [override] --max-num-batched-tokens=%d\n", overrides.MaxNumBatchedTokens)
	}

	// ── Check if there's a GPU memory utilization override ───────────────
	// Priority: CLI override > DB profile value
	var hasOverride bool
	var overrideUtil float64
	if overrides.GPUMemoryUtil != nil {
		hasOverride = true
		overrideUtil = *overrides.GPUMemoryUtil
	} else if model.GpuMemoryUtilization != nil && *model.GpuMemoryUtilization > 0 {
		hasOverride = true
		overrideUtil = *model.GpuMemoryUtilization
	}

	if hasOverride {
		// Bypass auto-calculation — just verify the reserved pool fits in free RAM.
		freeMB, err := readMemAvailableMB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not read free memory: %v\n", err)
			return nil // non-fatal
		}
		poolNeeded := int(overrideUtil * float64(TotalGPUMB))

		// When gpu_memory_utilization is explicitly set, skip the safety margin.
		// The user has chosen the utilization level — just verify the pool fits.
		available := freeMB

		fmt.Fprintf(os.Stderr, "  [gpu-memory] using override: %.2f (pool: %d MB)\n", overrideUtil, poolNeeded)
		fmt.Fprintf(os.Stderr, "  Available RAM: %d MB (free %d MB)\n", available, freeMB)

		if poolNeeded > available {
			fmt.Fprintf(os.Stderr, "  ERROR: insufficient memory for gpu_memory_utilization=%.2f\n", overrideUtil)
			fmt.Fprintf(os.Stderr, "  Need %d MB (%.1f GB), only %d MB available\n",
				poolNeeded, float64(poolNeeded)/1024, available)
			return fmt.Errorf("insufficient memory for model %s (gpu_memory_utilization=%.2f requires %d MB)",
				slug, overrideUtil, poolNeeded)
		}

		// Warn if tight
		headroom := available - poolNeeded
		if headroom < 4096 {
			fmt.Fprintf(os.Stderr, "  Warning: memory is tight — %.1f GB headroom remaining\n",
				float64(headroom)/1024)
		}
		return nil
	}

	// ── Auto-calculate path (no override) ──────────────────────────────────

	// Sync the profile with the overridden values so CalculateMemory() inside
	// CanFitDynamic sees the CLI overrides instead of the DB defaults.
	profile.MaxNumSeqs = checkSeqs

	result, err := CanFitDynamic(profile, getKVDtypeBytes(model.CommandArgs), checkContext, checkSeqs, mtpTokens, s.cfg.SafetyMarginPct)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not calculate GPU memory: %v\n", err)
		return nil // non-fatal
	}

	if !result.Fits {
		fmt.Fprintf(os.Stderr, "  ERROR: insufficient GPU memory\n")
		safetyGB := float64(result.SafetyMarginMB) / 1024
		fmt.Fprintf(os.Stderr, "  Model needs %d MB (%.0f GB), only %d MB available (free %d MB - %.1f GB safety margin)\n",
			result.NeededMB, float64(result.NeededMB)/1024, result.AvailableMB, result.FreeMB, safetyGB)
		fmt.Fprintf(os.Stderr, "  gpu_memory_utilization would be %.2f (vLLM reserves %.2f × %.0f MB = %.0f MB)\n",
			result.GPUMemoryUtilization, result.GPUMemoryUtilization, float64(TotalGPUMB),
			result.GPUMemoryUtilization*float64(TotalGPUMB))
		fmt.Fprintln(os.Stderr, "  Stop other models or reduce context length before starting.")
		return fmt.Errorf("insufficient GPU memory for model %s", slug)
	}

	// Warn if tight
	if result.HeadroomMB < 4096 {
		fmt.Fprintf(os.Stderr, "  Warning: GPU memory is tight — %.1f GB headroom remaining\n",
			float64(result.HeadroomMB)/1024)
	}

	return nil
}

// StartContainer starts a Docker container via docker compose.
// It always regenerates the compose YAML from DB state before starting,
// ensuring engine config changes are picked up.
// Overrides (if nonzero) replace auto-calculated values for the given run.
