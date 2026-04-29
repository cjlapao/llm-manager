// Package cmd provides the install subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

// envWithOverrides returns a copy of env with the given key=value overrides applied.
// Any existing entries with matching keys are removed so the overrides always win.
func envWithOverrides(env []string, overrides ...string) []string {
	if len(overrides) == 0 {
		return env
	}
	keys := make(map[string]bool)
	for _, ov := range overrides {
		idx := strings.IndexByte(ov, '=')
		if idx >= 0 {
			keys[ov[:idx]] = true
		}
	}
	result := make([]string, 0, len(env)+len(overrides))
	for _, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			result = append(result, e)
			continue
		}
		if !keys[e[:idx]] {
			result = append(result, e)
		}
	}
	return append(result, overrides...)
}

func init() {
	RegisterCommand("install", func(root *RootCommand) Command {
		return NewInstallCommand(root)
	})
}

// InstallCommand orchestrates end-to-end installation of an existing model:
// [pre-flight → back / regen YAML → pull weights → [start container]→ litel ↗ m sync].
type InstallCommand struct {
	cfg        *config.Config
	db         database.DatabaseManager
	container  *service.ContainerService
	litellm    *service.LiteLLMService
	composeGen *service.ComposeGenerator
	start      bool
	clean      bool
	all        bool
}

// NewInstallCommand creates a new InstallCommand wired to the given root context.
func NewInstallCommand(root *RootCommand) *InstallCommand {
	configSvc := service.NewConfigService(root.db)
	gen, err := service.NewComposeGenerator(root.db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create compose generator: %v\n", err)
	}

	return &InstallCommand{
		cfg:        root.cfg,
		db:         root.db,
		container:  service.NewContainerService(root.db, root.cfg),
		litellm:    service.NewLiteLLMService(root.db, root.cfg, configSvc),
		composeGen: gen,
	}
}

// Run executes the install command with the given arguments.
func (c *InstallCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	slug, startFlag, cleanFlag, allFlag := parseInstallArgs(args)
	c.start = startFlag
	c.clean = cleanFlag
	c.all = allFlag

	if allFlag && slug != "" {
		fmt.Fprintln(os.Stderr, "Error: cannot specify both --all and a model slug")
		return 1
	}

	switch {
	case allFlag:
		return c.runInstallAll(startFlag, cleanFlag)
	case slug == "" || slug == "help" || slug == "-h" || slug == "--help":
		c.PrintHelp()
		return 0
	default:
		return c.runInstall(slug)
	}
}

// parseInstallArgs splits positional args into the model slug, start flag, clean flag, and all flag.
// Returns (slug, includeStart, includeClean, includeAll). Empty slug implies none was provided.
func parseInstallArgs(args []string) (string, bool, bool, bool) {
	var slug string
	var hasStart bool
	var hasClean bool
	var hasAll bool

	for _, arg := range args {
		switch arg {
		case "--start", "-s":
			hasStart = true
		case "--clean":
			hasClean = true
		case "--all":
			hasAll = true
		case "-h", "--help", "help":
			continue
		default:
			if !strings.HasPrefix(arg, "--") && !strings.HasPrefix(arg, "-") {
				if slug == "" {
					slug = arg
				} else {
					fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n", arg)
					return "", false, false, false
				}
			}
		}
	}
	return slug, hasStart, hasClean, hasAll
}

// runInstall runs the full installation pipeline for a single model slug.
func (c *InstallCommand) runInstall(slug string) int {
	fmt.Printf("\n%s\n", strings.Repeat("═", 56))
	fmt.Printf(" Installing model: %s\n", slug)
	fmt.Printf("%s\n", strings.Repeat("═", 56))

	if exitCode := c.runSingle(slug); exitCode != 0 {
		return exitCode
	}

	// ──────── Summary ────────
	fmt.Println()
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Install complete: %s\n", slug)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("  ✓ Backup & regeneration completed")
	fmt.Println("  ✓ Weight pull from HuggingFace completed")
	if c.start {
		fmt.Println("  ✓ Container started")
	} else {
		fmt.Println("  ⌀ Container start skipped — use --start to begin")
	}
	model, _ := c.db.GetModel(slug)
	if model == nil || model.LiteLLMParams == "" {
		fmt.Println("  ℹ LiteLLM sync skipped — model has no litellm_params")
	} else {
		fmt.Println("  ✓ Synced with LiteLLM proxy")
	}
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  OpenAI API URL: %s\n", maskEndpoint(c.cfg.OpenAIAPIURL))
	_, _ = c.db.GetModel(slug)
	fmt.Printf("  Port:            %d\n", model.Port)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("")

	return 0
}

// runInstallAll installs all registered models in the database.
func (c *InstallCommand) runInstallAll(startFlag, cleanFlag bool) int {
	models, err := c.db.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No models registered. Use 'llm-manager model import' to add models.")
		return 0
	}

	fmt.Printf("Installing %d model(s)...\n", len(models))
	fmt.Println(strings.Repeat("─", 60))

	total := len(models)
	succeeded := 0
	failed := 0

	for i, m := range models {
		fmt.Printf("\n[%d/%d] Installing: %s (%s)\n", i+1, total, m.Slug, m.Name)
		c.clean = cleanFlag
		c.start = startFlag
		exitCode := c.runSingle(m.Slug)
		if exitCode == 0 {
			succeeded++
		} else {
			failed++
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Install complete: %d/%d succeeded, %d failed\n", succeeded, total, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

// runSingle runs the install pipeline for a single model. Returns 0 on success.
func (c *InstallCommand) runSingle(slug string) int {
	model, ok := c.preflight(slug)
	if !ok {
		return 1
	}

	ymlPath := absYML(c.cfg.LLMDir, model.Slug+".yml")
	baseName := filepath.Base(ymlPath)

	if data, err := os.ReadFile(ymlPath); err == nil {
		bak := ymlPath + ".bak"
		if err := os.WriteFile(bak, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Failed to back up %s: %v\n", baseName, err)
			return 1
		}
		fmt.Printf("  ✓ Backed up %s -> %s.bak\n", baseName, baseName)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  ✗ Error reading %s: %v\n", ymlPath, err)
		return 1
	} else {
		fmt.Printf("  ℹ No existing %s — clean install\n", baseName)
	}

	composeYAML, err := c.composeGen.Generate(model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Failed to generate docker-compose YAML: %v\n", err)
		return 1
	}

	if err := os.WriteFile(ymlPath, []byte(composeYAML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Failed to write %s: %v\n", ymlPath, err)
		return 1
	}
	fmt.Printf("  ✓ Regenerated %s from DB record\n", baseName)

	hfToken := c.cfg.HfToken
	if hfToken == "" {
		hfToken = os.Getenv("HUGGING_FACE_HUB_TOKEN")
	}
	if model.HFRepo == "" || hfToken == "" {
		fmt.Fprintln(os.Stderr, "  ✗ Weight pull cannot proceed: HF_REPO or HF_TOKEN not configured")
		return 1
	}

	fmt.Println("  Pulling weights from HuggingFace…")
	cmd := exec.Command("hf", "download", model.HFRepo, "--token="+hfToken)
	cmd.Env = envWithOverrides(os.Environ(),
		"HF_HOME="+c.cfg.HFCacheDir,
		"HF_TOKEN="+hfToken,
		"HUGGING_FACE_HUB_TOKEN="+hfToken,
	)

	if err := RunInteractive(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Weight pull failed\n")
		return 1
	}
	fmt.Println("  ✓ Weights pulled successfully")

	if c.start {
		if model.Container == "" {
			fmt.Fprintln(os.Stderr, "  ✗ Cannot start — model has no container configured")
			return 1
		}

		if err := c.container.StartContainer(slug, false); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Failed to start container: %v\n", err)
			return 1
		}

		fmt.Println("  ✓ Container started")
	}

	if c.clean {
		if err := c.litellm.CleanDuplicates(slug); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ LiteLLM cleanup failed: %v\n", err)
			return 1
		}
	}
	skipSync := model.LiteLLMParams == ""
	if !skipSync {
		if err := c.litellm.SyncModel(model.Slug); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ LiteLLM sync failed: %v\n", err)
			return 1
		}
		fmt.Println("  ✓ Synced with LiteLLM proxy")
	} else {
		fmt.Println("  ℹ LiteLLM sync skipped — model has no litellm_params")
	}
	return 0
}

// preflight validates configuration and model requirements before proceeding.
// It returns the full *models.Model on success (or zero-valued nil on bail-out).
func (c *InstallCommand) preflight(slug string) (*models.Model, bool) {
	model, err := c.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: model %q not found in database.\n", slug)
		fmt.Fprintln(os.Stderr, "Run 'llm-manager model ls' to list registered models.")
		return nil, false
	}

	if model.HFRepo == "" {
		fmt.Fprintf(os.Stderr, "Error: model %q has no HF_REPO configured.\n", slug)
		fmt.Fprintln(os.Stderr, "Please update the model record before installing.")
		return nil, false
	}

	if model.Container == "" {
		fmt.Fprintf(os.Stderr, "Error: model %q has no container configured.", slug)
		return nil, false
	}

	if c.cfg.HfToken == "" {
		fmt.Fprintln(os.Stderr, "Error: HF_TOKEN is not configured.")
		fmt.Fprintln(os.Stderr, "Set it via: llm-manager config set HF_TOKEN <token>")
		return nil, false
	}
	if c.cfg.OpenAIAPIURL == "" {
		fmt.Fprintln(os.Stderr, "Error: OPENAI_API_URL is not configured.")
		fmt.Fprintln(os.Stderr, "Set it via: llm-manager config set OPENAI_API_URL <url>")
		return nil, false
	}
	if c.cfg.LLMDir == "" {
		fmt.Fprintln(os.Stderr, "Error: LLM_DIR is not configured.")
		return nil, false
	}

	return model, true
}

// maskEndpoint hides the host:port part but shows the scheme.
func maskEndpoint(u string) string {
	u = strings.TrimRight(u, "/")
	parts := strings.SplitN(u, "://", 2)
	if len(parts) == 2 {
		host := parts[1]
		if slashIdx := strings.Index(host, "/"); slashIdx >= 0 {
			host = host[:slashIdx]
		}
		if colonIdx := strings.LastIndexByte(host, ':'); colonIdx > 0 {
			host = host[:colonIdx]
		}
		return fmt.Sprintf("%s://%s/...", parts[0], host)
	}
	return u
}

// absYML resolves a potentially relative YAML path to its absolute form.
func absYML(llmDir, relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	}
	return filepath.Join(llmDir, relPath)
}

// PrintHelp prints usage help for the install command.
func (c *InstallCommand) PrintHelp() {
	fmt.Println(`install - Install or reinstall a model from its database record.

USAGE:
  llm-manager install <slug> [--start|-s] [--clean]
  llm-manager install --all [--start|-s] [--clean]

ARGUMENTS:
  slug   The model slug to install (must exist in the database)
  --all  Install all registered models in the database

OPTIONS:
  --start, -s   Also start the container after install (default: skip start)
  --clean       Before syncing, scan LiteLLM and delete all duplicate/stale
                deployments where litellm_params.model matches this slug

STEPS:
  1. Validate HF_TOKEN, OPENAI_API_URL, and model configuration
  2. Back up existing docker-compose YAML (if present)
  3. Regenerate docker-compose YAML from database record
  4. Pull model weights from HuggingFace
  5. [--start] Start the Docker container
  6. [--clean] Clean duplicate/stale LiteLLM deployments
  7. Sync model with LiteLLM proxy

REQUIREMENTS:
  The model must already exist in the database. Use "llm-manager model import"
  to register a new model first.

ENVIRONMENT VARIABLES:
  HF_TOKEN                 HuggingFace API token (required)
  OPENAI_API_URL           OpenAI-compatible API endpoint (required)
  LLM_MANAGER_LLM_DIR      Directory containing compose files (required)

EXAMPLES:
  llm-manager install qwen3_6          # Install without starting container
  llm-manager install qwen3_6 --start  # Install and start container
  llm-manager install qwen3_6 --clean  # Clean duplicates before syncing
  llm-manager install --all            # Install all registered models
  llm-manager install --all --start    # Install all and start containers`)
}
