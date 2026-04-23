// Package cmd provides the install subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

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
}

// NewInstallCommand creates a new InstallCommand wired to the given root context.
func NewInstallCommand(root *RootCommand) *InstallCommand {
	configSvc := service.NewConfigService(root.db)
	gen, err := service.NewComposeGenerator()
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

	slug, startFlag := parseInstallArgs(args)
	c.start = startFlag

	switch slug {
	case "", "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		return c.runInstall(slug)
	}
}

// parseInstallArgs splits positional args into the model slug and any flags.
// Returns (slug, includeStart). Empty slug implies none was provided.
func parseInstallArgs(args []string) (string, bool) {
	var slug string
	var hasStart bool

	for _, arg := range args {
		switch arg {
		case "--start", "-s":
			hasStart = true
		case "-h", "--help", "help":
			continue
		default:
			if !strings.HasPrefix(arg, "--") && !strings.HasPrefix(arg, "-") {
				if slug == "" {
					slug = arg
				} else {
					fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n", arg)
					return "", false
				}
			}
		}
	}
	return slug, hasStart
}

// runInstall runs the full installation pipeline for a single model slug.
func (c *InstallCommand) runInstall(slug string) int {
	fmt.Printf("\n%s\n", strings.Repeat("═", 56))
	fmt.Printf(" Installing model: %s\n", slug)
	fmt.Printf("%s\n", strings.Repeat("═", 56))

	// ──────── Stage A: pre-flight validation ────────
	model, ok := c.preflight(slug)
	if !ok {
		return 1
	}

	// ──────── Stage B: back up old YAML & regenerate ────────
	ymlPath := absYML(c.cfg.LLMDir, model.YML)
	baseName := filepath.Base(ymlPath)

	// Back up existing YAML if present.
	if data, err := os.ReadFile(ymlPath); err == nil {
		bak := ymlPath + ".bak"
		if err := os.WriteFile(bak, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to back up %s: %v\n", baseName, err)
			return 1
		}
		fmt.Printf("✓ Backed up %s -> %s.bak\n", baseName, baseName)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "✗ Error reading %s: %v\n", ymlPath, err)
		return 1
	} else {
		fmt.Printf("ℹ No existing %s — clean install\n", baseName)
	}

	// Regenerate docker-compose YAML from DB record.
	composeYAML, err := c.composeGen.Generate(model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to generate docker-compose YAML: %v\n", err)
		return 1
	}

	if err := os.WriteFile(ymlPath, []byte(composeYAML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to write %s: %v\n", ymlPath, err)
		return 1
	}
	fmt.Printf("✓ Regenerated %s from DB record\n", baseName)

	// ──────── Stage C: pull weights from HuggingFace ────────
	hfToken := c.cfg.HfToken
	if hfToken == "" {
		hfToken = os.Getenv("HUGGING_FACE_HUB_TOKEN")
	}
	if model.HFRepo == "" || hfToken == "" {
		fmt.Fprintln(os.Stderr, "✗ Weight pull cannot proceed: HF_REPO or HF_TOKEN not configured")
		return 1
	}

	fmt.Println("Pulling weights from HuggingFace…")
	cmd := exec.Command("hf", "download", model.HFRepo, "--token", hfToken)
	cmd.Env = append(os.Environ(), "HF_HOME="+c.cfg.HFCacheDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Weight pull failed — aborting install.\n")
		return 1
	}
	fmt.Println("Weights pulled successfully")

	// ──────── Stage D: start container (conditional) ────────
	if c.start {
		if model.Container == "" {
			fmt.Fprintln(os.Stderr, "✗ Cannot start — model has no container configured")
			return 1
		}

		if err := c.container.StartContainer(slug); err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to start container: %v\n", err)
			return 1
		}

		fmt.Println("Waiting for container to initialise …")
		time.Sleep(3 * time.Second)

		status, _ := c.container.GetContainerStatus(slug)
		upperStatus := strings.ToUpper(status)
		if upperStatus == "" {
			upperStatus = "UNKNOWN"
		}
		fmt.Printf("Container running (status=%s)\n", upperStatus)
	}

	// ──────── Stage E: sync with LiteLLM ────────
	if err := c.litellm.SyncModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "✗ LiteLLM sync failed: %v\n", err)
		return 1
	}
	fmt.Println("Synced with LiteLLM proxy")

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
	fmt.Println("  ✓ Synced with LiteLLM proxy")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  OpenAI API URL: %s\n", maskEndpoint(c.cfg.OpenAIAPIURL))
	fmt.Printf("  Port:            %d\n", model.Port)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("")

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
		return fmt.Sprintf("%s://%s...", parts[0], host)
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
  llm-manager install <slug> [--start|-s]

ARGUMENTS:
  slug   The model slug to install (must exist in the database)

OPTIONS:
  --start, -s   Also start the container after install (default: skip start)

STEPS:
  1. Validate HF_TOKEN, OPENAI_API_URL, and model configuration
  2. Back up existing docker-compose YAML (if present)
  3. Regenerate docker-compose YAML from database record
  4. Pull model weights from HuggingFace
  5. [--start] Start the Docker container
  6. Sync model with LiteLLM proxy

REQUIREMENTS:
  The model must already exist in the database. Use "llm-manager model import"
  to register a new model first.

ENVIRONMENT VARIABLES:
  HF_TOKEN                 HuggingFace API token (required)
  OPENAI_API_URL           OpenAI-compatible API endpoint (required)
  LLM_MANAGER_LLM_DIR      Directory containing compose files (required)

EXAMPLES:
  llm-manager install qwen3_6          # Install without starting container
  llm-manager install qwen3_6 --start  # Install and start container`)
}
