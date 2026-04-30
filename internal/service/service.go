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

// OpenCodeModelEntry represents a model entry in opencode's provider.models.
type OpenCodeModelEntry struct {
	Name       string                 `json:"name,omitempty"`
	Limit      *OpenCodeLimit         `json:"limit,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
	Variants   map[string]interface{} `json:"variants,omitempty"`
	Modalities map[string][]string    `json:"modalities,omitempty"`
}

// OpenCodeLimit represents context and output token limits.
type OpenCodeLimit struct {
	Context int     `json:"context"`
	Output  int     `json:"output"`
	Price   *Price  `json:"price,omitempty"`
}

// Price represents input/output token pricing.
type Price struct {
	Input  *float64 `json:"input,omitempty"`
	Output *float64 `json:"output,omitempty"`
}

// variantEntry holds a variant name and its merged parameters.
type variantEntry struct {
	Name   string
	Params map[string]interface{}
}

// ModelService handles business logic for LLM model operations.
type ModelService struct {
	db       database.DatabaseManager
	cfg      *config.Config
	litellm  DeleteModeler
	eng      *EngineService
}

// NewModelService creates a new ModelService.
func NewModelService(db database.DatabaseManager, cfg *config.Config) *ModelService {
	return &ModelService{db: db, cfg: cfg}
}

// SetEngineService sets the optional EngineService for engine version resolution.
func (s *ModelService) SetEngineService(svc *EngineService) {
	s.eng = svc
}

// SetLiteLLMService sets the optional LiteLLM deleter for delete+reimport mode.
func (s *ModelService) SetLiteLLMService(l DeleteModeler) {
	s.litellm = l
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

	// Expand template references (${{ .xxx }}) before validation.
	cfgValues := s.configValues()
	if err := yamlparser.ApplyTemplateVars(y, cfgValues); err != nil {
		return nil, fmt.Errorf("template expansion failed: %w", err)
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
		commandArgsStr, err := json.Marshal(y.CommandArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal command_args: %w", err)
		}
		updates["command_args"] = string(commandArgsStr)
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

// GenerateOpenCodeModel generates opencode-compatible model entry for a single model.
func (s *ModelService) GenerateOpenCodeModel(slug string) ([]byte, error) {
	m, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get model %s: %w", slug, err)
	}

	modelsMap := make(map[string]*OpenCodeModelEntry)
	modelsMap[m.Slug] = s.buildOpenCodeEntry(m)
	return json.MarshalIndent(modelsMap, "", "  ")
}

// GenerateOpenCodeModels generates opencode-compatible model entries from all
// models registered in the database. It returns a JSON object of model entries
// keyed by slug, suitable for pasting directly into a provider's models section.
//
// For each model it:
//   - Includes the base model entry with name, limit, options, and variants
//   - Includes all variants (from litellm_params.variants) but NOT the active alias
//   - Includes input/output token costs if available
//   - Excludes top_k/top_p from base params so they can be set at provider level
func (s *ModelService) GenerateOpenCodeModels() ([]byte, error) {
	models, err := s.db.ListModels()
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	modelsMap := make(map[string]*OpenCodeModelEntry)
	for _, m := range models {
		modelsMap[m.Slug] = s.buildOpenCodeEntry(&m)
	}

	return json.MarshalIndent(modelsMap, "", "  ")
}

// buildOpenCodeEntry builds a single OpenCode model entry from a model record.
func (s *ModelService) buildOpenCodeEntry(m *models.Model) *OpenCodeModelEntry {
	oc := &OpenCodeModelEntry{}

	displayName := m.Name
	if displayName == "" {
		displayName = m.Slug
	}
	oc.Name = displayName

	oc.Options = map[string]interface{}{
		"model": m.Slug,
		"provider": map[string]interface{}{
			"model": m.Slug,
		},
	}

	contextLimit := 262144 // default context window
	outputLimit := uint(4096) // default output limit

	// Try to extract limits from model_info
	if m.ModelInfo != "" {
		var minfo map[string]interface{}
		if err := json.Unmarshal([]byte(m.ModelInfo), &minfo); err == nil {
			if inputTokens, ok := minfo["input_tokens_limits"].([]interface{}); ok && len(inputTokens) > 0 {
				if v, ok := inputTokens[0].(float64); ok {
					contextLimit = int(v)
				}
			}
			if outputTokens, ok := minfo["output_token_limits"].([]interface{}); ok && len(outputTokens) > 0 {
				if v, ok := outputTokens[0].(float64); ok {
					outputLimit = uint(v)
				}
			}
		}
	}

	oc.Limit = &OpenCodeLimit{
		Context: contextLimit,
		Output:  int(outputLimit),
	}

	if m.InputTokenCost > 0 || m.OutputTokenCost > 0 {
		oc.Limit.Price = &Price{}
		if m.InputTokenCost > 0 {
			oc.Limit.Price.Input = &m.InputTokenCost
		}
		if m.OutputTokenCost > 0 {
			oc.Limit.Price.Output = &m.OutputTokenCost
		}
	}

	variants := s.extractVariants(*m)

	var caps []string
	json.Unmarshal([]byte(m.Capabilities), &caps)
	hasReasoning := false
	for _, c := range caps {
		if c == "reasoning" {
			hasReasoning = true
		}
	}

	// Build modalities based on capabilities
	oc.Modalities = map[string][]string{
		"input":  {"text"},
		"output": {"text"},
	}
	for _, c := range caps {
		switch c {
		case "image":
			oc.Modalities["input"] = append(oc.Modalities["input"], "image")
		case "video":
			oc.Modalities["input"] = append(oc.Modalities["input"], "video")
		case "document":
			oc.Modalities["input"] = append(oc.Modalities["input"], "pdf")
		}
	}

	if len(variants) > 0 {
		oc.Variants = make(map[string]interface{})
		for _, v := range variants {
			vEntry := map[string]interface{}{
				"model": m.Slug + "-" + v.Name,
			}
			if hasReasoning {
				if strings.Contains(strings.ToLower(v.Name), "think") {
					vEntry["thinking"] = map[string]interface{}{
						"type":         "enabled",
						"budgetTokens": 16000,
					}
				}
			}
			oc.Variants[v.Name] = vEntry
		}
	}

	return oc
}

// extractVariants parses litellm_params from a model record and returns
// a list of variant entries with their names and specs.
// Excludes top_k and top_p from base params so they can be set at provider level.
func (s *ModelService) extractVariants(m models.Model) []variantEntry {
	var variants []variantEntry

	if m.LiteLLMParams == "" {
		return variants
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(m.LiteLLMParams), &params); err != nil {
		return variants
	}

	variantsMap, ok := params["variants"].(map[string]interface{})
	if !ok || len(variantsMap) == 0 {
		return variants
	}

	// Build base params (without variants key, top_k, top_p)
	baseParams := make(map[string]interface{})
	for k, v := range params {
		if k != "variants" && k != "top_k" && k != "top_p" {
			baseParams[k] = v
		}
	}

	for name, spec := range variantsMap {
		vEntry := variantEntry{
			Name:   name,
			Params: make(map[string]interface{}),
		}

		if specMap, ok := spec.(map[string]interface{}); ok {
			// Merge base params with variant overrides
			for k, v := range baseParams {
				vEntry.Params[k] = v
			}
			// Override with variant-specific values (excluding top_k/top_p)
			for k, v := range specMap {
				if k == "top_k" || k == "top_p" {
					continue
				}
				vEntry.Params[k] = v
			}
		}

		variants = append(variants, vEntry)
	}

	return variants
}

// GenerateCompose generates a docker-compose YAML for the model using the given generator.
// It resolves the engine version from the model's DB record to build the full compose config.
func (s *ModelService) GenerateCompose(slug string, generator *ComposeGenerator, cfg EngineComposeConfig) (string, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return "", fmt.Errorf("model %s not found: %w", slug, err)
	}

	// If caller provided a config, merge it on top of the engine-resolved config.
	// Engine-resolved takes priority; caller overrides are layered on top.
	if cfg.Image == "" {
		resolved, err := s.resolveComposeConfig(model)
		if err != nil {
			return "", err
		}
		cfg = *resolved
	}

	// Apply caller-provided overrides
	if cfg.Image != "" {
		// already set
	}
	if len(cfg.EnvVars) > 0 {
		for k, v := range cfg.EnvVars {
			cfg.EnvVars[k] = v
		}
	}

	composeYAML, err := generator.Generate(model, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate compose file for %s: %w", slug, err)
	}

	return composeYAML, nil
}

// resolveComposeConfig resolves the engine version for a model and returns the
// full EngineComposeConfig with image, entrypoint, env vars, volumes, logging, deploy.
func (s *ModelService) resolveComposeConfig(model *models.Model) (*EngineComposeConfig, error) {
	if s.eng == nil {
		return &EngineComposeConfig{}, nil
	}
	return s.eng.BuildComposeConfig(model)
}

// ContainerService handles Docker container operations.
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
	composeGen, err := NewComposeGenerator()
	if err != nil {
		return fmt.Errorf("failed to create compose generator: %w", err)
	}
	cfg, err := s.svc.BuildComposeConfig(model)
	if err != nil {
		return fmt.Errorf("failed to resolve engine config: %w", err)
	}
	composeYAML, err := composeGen.Generate(model, *cfg)
	if err != nil {
		return fmt.Errorf("failed to generate compose YAML: %w", err)
	}
	ymlPath := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")
	if err := os.WriteFile(ymlPath, []byte(composeYAML), 0o644); err != nil {
		return fmt.Errorf("failed to write compose file %s: %w", ymlPath, err)
	}
	return nil
}

// StartContainer starts a Docker container via docker compose.
// It always regenerates the compose YAML from DB state before starting,
// ensuring engine config changes are picked up.
func (s *ContainerService) StartContainer(slug string, allowMultiple bool) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
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
	if err := s.ensureCompose(model); err != nil {
		return fmt.Errorf("failed to regenerate compose: %w", err)
	}

	composeFile := filepath.Join(s.cfg.LLMDir, slug+".yml")
	projectName := "llm-" + strings.ReplaceAll(slug, ".", "-")
	cmd := exec.Command("docker", "compose", "--project-name", projectName, "-f", composeFile, "up", "-d")
	cmd.Dir = s.cfg.LLMDir
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start container %s: %w", slug, err)
	}

	if err := s.db.UpdateContainerStatus(slug, "running"); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to update container status for %s: %v\n", slug, err)
	}
	return nil
}

// StopContainer stops a Docker container by name.
func (s *ContainerService) StopContainer(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	cmd := exec.Command("docker", "stop", model.Container)
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
	return s.StartContainer(slug, false)
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
		if m.Container == "" {
			continue
		}

		if m.Type != "llm" {
			if runningNames[m.Container] {
				skippedNonLLM++
			}
			continue
		}

		if !runningNames[m.Container] {
			continue // not running, nothing to stop
		}

		stopCmd := exec.Command("docker", "stop", m.Container)
		if err := stopCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ Failed to stop %s: %v\n", m.Container, err)
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

// StartEmbed starts the first available embed model container.
// It delegates to StartModelBySlug after resolving the model from the database.
// Backward-compatible: same signature, returns error on failure.
func (s *ContainerService) StartEmbed() error {
	models, err := s.db.ListModelsByTypeSubType("rag", "embedding")
	if err != nil {
		return fmt.Errorf("failed to list embedding models: %w", err)
	}
	if len(models) == 0 {
		return fmt.Errorf("no embedding models found")
	}
	// Use StartModelBySlugWithAllow with allowMultiple=false so that
	// starting an embed model stops any other running embed containers
	// in the same type+subtype group (per-subtype isolation).
	return s.StartModelBySlugWithAllow(models[0].Slug, false)
}

// StopEmbed stops the first available embed model container.
// It delegates to StopModelBySlug after resolving the model from the database.
// Backward-compatible: same signature, returns error on failure.
func (s *ContainerService) StopEmbed() error {
	models, err := s.db.ListModelsByTypeSubType("rag", "embedding")
	if err != nil {
		return fmt.Errorf("failed to list embedding models: %w", err)
	}
	if len(models) == 0 {
		return nil
	}
	return s.StopModelBySlug(models[0].Slug)
}

// StartRerank starts the first available rerank model container.
// It delegates to StartModelBySlug after resolving the model from the database.
// Backward-compatible: same signature, returns error on failure.
func (s *ContainerService) StartRerank() error {
	models, err := s.db.ListModelsByTypeSubType("rag", "reranker")
	if err != nil {
		return fmt.Errorf("failed to list reranker models: %w", err)
	}
	if len(models) == 0 {
		return fmt.Errorf("no reranker models found")
	}
	// Use StartModelBySlugWithAllow with allowMultiple=false so that
	// starting a rerank model stops any other running rerank containers
	// in the same type+subtype group (per-subtype isolation).
	return s.StartModelBySlugWithAllow(models[0].Slug, false)
}

// StopRerank stops the first available rerank model container.
// It delegates to StopModelBySlug after resolving the model from the database.
// Backward-compatible: same signature, returns error on failure.
func (s *ContainerService) StopRerank() error {
	models, err := s.db.ListModelsByTypeSubType("rag", "reranker")
	if err != nil {
		return fmt.Errorf("failed to list reranker models: %w", err)
	}
	if len(models) == 0 {
		return nil
	}
	return s.StopModelBySlug(models[0].Slug)
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

// StartModelBySlug reads a model from the database by slug and starts its
// Docker container using the container name stored in model.Container.
// It always regenerates the compose YAML from DB state before starting,
// ensuring engine config changes are picked up.
func (s *ContainerService) StartModelBySlug(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	// Always regenerate compose YAML to stay in sync with engine config changes.
	if err := s.ensureCompose(model); err != nil {
		return fmt.Errorf("failed to regenerate compose: %w", err)
	}

	ymlPath := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")
	projectName := "rag-" + strings.ReplaceAll(model.Slug, ".", "-")
	composeDir := filepath.Dir(ymlPath)

	// Check if the container already exists
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", model.Container)
	if _, err := cmd.CombinedOutput(); err != nil {
		// Container doesn't exist — create it
		fmt.Fprintf(os.Stderr, "  Container %s does not exist — creating via compose...\n", model.Container)

		composeUp := exec.Command("docker", "compose", "--project-name", projectName, "-f", ymlPath, "up", "-d")
		composeUp.Dir = composeDir
		if composeOut, composeErr := composeUp.CombinedOutput(); composeErr != nil {
			return fmt.Errorf("failed to create container %s: %s (%w)", model.Container, string(composeOut), composeErr)
		}
		fmt.Printf("Container %s created\n", model.Container)
	} else {
		// Container exists — bring it up with the regenerated compose YAML
		// so engine config changes are picked up.
		composeUp := exec.Command("docker", "compose", "--project-name", projectName, "-f", ymlPath, "up", "-d")
		composeUp.Dir = composeDir
		if composeOut, composeErr := composeUp.CombinedOutput(); composeErr != nil {
			return fmt.Errorf("failed to start container %s: %s (%w)", model.Container, string(composeOut), composeErr)
		}
		fmt.Printf("Container %s started\n", model.Container)
	}

	return nil
}

// StopModelBySlug reads a model from the database by slug, checks the Docker
// container status, and stops it if running.
func (s *ContainerService) StopModelBySlug(slug string) error {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	if model.Container == "" {
		return fmt.Errorf("model %s has no container configured", slug)
	}

	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", model.Container)
	output, err := cmd.Output()
	if err != nil {
		// Container doesn't exist or can't be inspected — nothing to stop
		return nil
	}

	state := strings.TrimSpace(string(output))
	if state == "running" {
		stopCmd := exec.Command("docker", "stop", model.Container)
		if err := stopCmd.Run(); err != nil {
			return fmt.Errorf("failed to stop container %s: %w", slug, err)
		}
		fmt.Printf("Container %s stopped\n", model.Container)
	} else {
		fmt.Printf("Container %s is not running (state: %s)\n", model.Container, state)
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
		if m.Container == "" {
			continue
		}
		if m.Type != modelType || m.SubType != subType {
			continue
		}
		if !runningNames[m.Container] {
			continue
		}
		stopCmd := exec.Command("docker", "stop", m.Container)
		if err := stopCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to stop %s: %v\n", m.Container, err)
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

	if model.Container == "" {
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
	projectName := "rag-" + strings.ReplaceAll(model.Slug, ".", "-")
	composeDir := filepath.Dir(ymlPath)

	// Check if the container already exists
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", model.Container)
	if _, err := cmd.CombinedOutput(); err != nil {
		// Container doesn't exist — create it
		fmt.Fprintf(os.Stderr, "  Container %s does not exist — creating via compose...\n", model.Container)

		composeUp := exec.Command("docker", "compose", "--project-name", projectName, "-f", ymlPath, "up", "-d")
		composeUp.Dir = composeDir
		if composeOut, composeErr := composeUp.CombinedOutput(); composeErr != nil {
			return fmt.Errorf("failed to create container %s: %s (%w)", model.Container, string(composeOut), composeErr)
		}
		fmt.Printf("Container %s created\n", model.Container)
	} else {
		// Container exists — bring it up with the regenerated compose YAML
		// so engine config changes are picked up.
		composeUp := exec.Command("docker", "compose", "--project-name", projectName, "-f", ymlPath, "up", "-d")
		composeUp.Dir = composeDir
		if composeOut, composeErr := composeUp.CombinedOutput(); composeErr != nil {
			return fmt.Errorf("failed to start container %s: %s (%w)", model.Container, string(composeOut), composeErr)
		}
		fmt.Printf("Container %s started\n", model.Container)
	}

	return nil
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
	if model.Container != "" {
		cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", model.Container)
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

	composeFile := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")

	cmd := exec.Command("docker", "compose", "-f", composeFile, "down")
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

	composeFile := filepath.Join(s.cfg.LLMDir, model.Slug+".yml")

	// Stop the container
	stopCmd := exec.Command("docker", "compose", "-f", composeFile, "down")
	stopCmd.Dir = s.cfg.LLMDir
	if output, err := stopCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to stop hotspot container: %s\n", strings.TrimSpace(string(output)))
	}

	// Start the container
	startCmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
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
	return s.container.StartContainer(slug, allowMultiple)
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
