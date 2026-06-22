package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// EstimateMemory returns a MemoryResult for a model based on its profile data.
// Returns nil and no error if the model has no profile data.
func (s *ContainerService) EstimateMemory(slug string) (*MemoryResult, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	return EstimateMemory(model)
}

// ListRAGEmbeddingModels returns all RAG embedding models from the database.
func (s *ContainerService) ListRAGEmbeddingModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("rag", "embedding")
}

// ListRAGRerankerModels returns all RAG reranker models from the database.
func (s *ContainerService) ListRAGRerankerModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("rag", "reranker")
}

// ListSpeechModels returns all speech models from the database (across all subtypes).
// It queries each known speech subtype individually and merges the results.
func (s *ContainerService) ListSpeechModels() ([]models.Model, error) {
	var all []models.Model
	for _, subType := range []string{"stt", "tts", "omni"} {
		models, err := s.db.ListModelsByTypeSubType("speech", subType)
		if err != nil {
			return nil, fmt.Errorf("failed to list speech/%s models: %w", subType, err)
		}
		all = append(all, models...)
	}
	return all, nil
}

// ListSTTModels returns all speech STT models from the database.
func (s *ContainerService) ListSTTModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("speech", "stt")
}

// ListTTSModels returns all speech TTS models from the database.
func (s *ContainerService) ListTTSModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("speech", "tts")
}

// ListOmniModels returns all speech omni models from the database.
func (s *ContainerService) ListOmniModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("speech", "omni")
}
