// Package cmd provides the uninstall subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("uninstall", func(root *RootCommand) Command {
		return NewUninstallCommand(root)
	})
}

// UninstallCommand orchestrates end-to-end uninstallation of a model:
// [pre-flight → stop container → delete compose YAML → clear HF cache → delete from LiteLLM → delete from DB].
type UninstallCommand struct {
	cfg        *config.Config
	db         database.DatabaseManager
	container  *service.ContainerService
	litellm    *service.LiteLLMService
	composeGen *service.ComposeGenerator
	keepCache  bool
	all        bool
}

// NewUninstallCommand creates a new UninstallCommand wired to the given root context.
func NewUninstallCommand(root *RootCommand) *UninstallCommand {
	configSvc := service.NewConfigService(root.db)
	gen, err := service.NewComposeGenerator(root.db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create compose generator: %v\n", err)
	}

	return &UninstallCommand{
		cfg:        root.cfg,
		db:         root.db,
		container:  service.NewContainerService(root.db, root.cfg),
		litellm:    service.NewLiteLLMService(root.db, root.cfg, configSvc),
		composeGen: gen,
	}
}

// Run executes the uninstall command with the given arguments.
func (c *UninstallCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	slug, keepCache, allFlag := parseUninstallArgs(args)

	c.keepCache = keepCache
	c.all = allFlag

	switch {
	case allFlag:
		return c.runUninstallAll()
	case slug == "" || slug == "help" || slug == "-h" || slug == "--help":
		c.PrintHelp()
		return 0
	default:
		return c.runUninstall(slug)
	}
}

// parseUninstallArgs splits positional args into slug, keepCache flag, and all flag.
func parseUninstallArgs(args []string) (string, bool, bool) {
	var slug string
	var hasKeepCache bool
	var hasAll bool

	for _, arg := range args {
		switch arg {
		case "--all":
			hasAll = true
		case "--keep-cached-model", "-k":
			hasKeepCache = true
		case "-h", "--help", "help":
			continue
		default:
			if !strings.HasPrefix(arg, "--") && !strings.HasPrefix(arg, "-") {
				if slug == "" {
					slug = arg
				} else {
					fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n", arg)
					return "", false, false
				}
			}
		}
	}
	return slug, hasKeepCache, hasAll
}

// runUninstallAll uninstalls all registered models.
func (c *UninstallCommand) runUninstallAll() int {
	models, err := c.db.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No models registered to uninstall.")
		return 0
	}

	fmt.Printf("Uninstalling %d model(s)...\n\n", len(models))

	total := 0
	successCount := 0
	failCount := 0

	for _, m := range models {
		fmt.Printf("%s\n", strings.Repeat("─", 56))
		exitCode := c.runUninstall(m.Slug)
		total++
		if exitCode == 0 {
			successCount++
		} else {
			failCount++
		}
		fmt.Println()
	}

	fmt.Printf("%s\n", strings.Repeat("═", 56))
	fmt.Printf("  Uninstall complete: %d/%d models removed\n", successCount, total)
	if failCount > 0 {
		fmt.Printf("  ⚠ %d model(s) failed to uninstall\n", failCount)
	}
	fmt.Printf("%s\n", strings.Repeat("═", 56))

	if failCount > 0 {
		return 1
	}
	return 0
}

// runUninstall runs the full uninstallation pipeline for a single model slug.
func (c *UninstallCommand) runUninstall(slug string) int {
	fmt.Printf("\n%s\n", strings.Repeat("═", 56))
	fmt.Printf(" Uninstalling model: %s\n", slug)
	fmt.Printf("%s\n", strings.Repeat("═", 56))

	// ──────── Stage A: pre-flight validation ────────
	model, ok := c.preflight(slug)
	if !ok {
		return 1
	}

	// ──────── Stage B: stop container ────────
	if model.Container != "" {
		fmt.Printf("Stopping container: %s\n", model.Container)
		if err := c.container.StopContainer(slug); err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to stop container (may not be running): %v\n", err)
		} else {
			fmt.Printf("✓ Container stopped\n")
		}
	}

	// ──────── Stage C: delete compose YAML file ────────
	ymlPath := absYML(c.cfg.LLMDir, model.Slug+".yml")
	if err := os.Remove(ymlPath); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "✗ Failed to delete %s: %v\n", ymlPath, err)
			return 1
		}
		fmt.Printf("ℹ No compose YAML found at %s — skipping\n", ymlPath)
	} else {
		fmt.Printf("✓ Deleted compose YAML: %s\n", filepath.Base(ymlPath))
	}

	// Also remove backup if exists
	bakPath := ymlPath + ".bak"
	if err := os.Remove(bakPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "✗ Failed to delete backup %s: %v\n", bakPath, err)
	} else if err == nil {
		fmt.Printf("✓ Deleted compose YAML backup: %s.bak\n", filepath.Base(ymlPath))
	}

	// ──────── Stage D: clear HF cache (unless --keep-cached-model) ────────
	if !c.keepCache && model.HFRepo != "" {
		if err := c.clearHFCache(model.HFRepo); err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to clear HF cache: %v\n", err)
		} else {
			fmt.Printf("✓ Cleared HuggingFace cache for %s\n", model.HFRepo)
		}
	} else if c.keepCache {
		fmt.Printf("ℹ Skipping HF cache cleanup (--keep-cached-model)\n")
	}

	// ──────── Stage E: delete from LiteLLM ────────
	if err := c.litellm.DeleteModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to delete from LiteLLM: %v\n", err)
		return 1
	}
	fmt.Printf("✓ Deleted model from LiteLLM proxy\n")

	// ──────── Stage F: delete from database ────────
	if err := c.db.DeleteModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to delete model from database: %v\n", err)
		return 1
	}
	fmt.Printf("✓ Deleted model from database\n")

	// ──────── Summary ────────
	fmt.Println()
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Uninstall complete: %s\n", slug)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("  ✓ Container stopped")
	fmt.Println("  ✓ Compose YAML deleted")
	if !c.keepCache {
		fmt.Println("  ✓ HuggingFace cache cleared")
	} else {
		fmt.Println("  ⌀ HuggingFace cache preserved (--keep-cached-model)")
	}
	fmt.Println("  ✓ Deleted from LiteLLM proxy")
	fmt.Println("  ✓ Deleted from database")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("")

	return 0
}

// preflight validates configuration and model requirements before proceeding.
func (c *UninstallCommand) preflight(slug string) (*models.Model, bool) {
	model, err := c.db.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: model %q not found in database.\n", slug)
		fmt.Fprintln(os.Stderr, "Run 'llm-manager model ls' to list registered models.")
		return nil, false
	}

	if c.cfg.LLMDir == "" {
		fmt.Fprintln(os.Stderr, "Error: LLM_DIR is not configured.")
		return nil, false
	}

	return model, true
}

// clearHFCache removes the HuggingFace cache directory for a model's repo.
func (c *UninstallCommand) clearHFCache(hfRepo string) error {
	cacheDir := "models--" + strings.ReplaceAll(hfRepo, "/", "--")

	cachePaths := []string{
		filepath.Join(c.cfg.HFCacheDir, "hub", cacheDir),
		filepath.Join(c.cfg.HFCacheDir, cacheDir),
	}

	for _, dir := range cachePaths {
		if _, err := os.Stat(dir); err == nil {
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("failed to remove %s: %w", dir, err)
			}
		}
	}

	return nil
}

// PrintHelp prints usage help for the uninstall command.
func (c *UninstallCommand) PrintHelp() {
	fmt.Println(`uninstall - Uninstall a model (stop container, delete YAML, clear cache, remove from LiteLLM and DB).

USAGE:
  llm-manager uninstall <slug> [--keep-cached-model|-k]
  llm-manager uninstall --all [--keep-cached-model|-k]

ARGUMENTS:
  slug    The model slug to uninstall (must exist in the database)

OPTIONS:
  --all, -a              Uninstall ALL registered models
  --keep-cached-model, -k  Keep HuggingFace cached weights after uninstall

STEPS:
  1. Stop the Docker container (if running)
  2. Delete docker-compose YAML file (+ backup if present)
  3. Clear HuggingFace cache directory (unless --keep-cached-model)
  4. Delete model from LiteLLM proxy (base, aliases, variants)
  5. Delete model record from database

EXAMPLES:
  llm-manager uninstall qwen3_6              # Uninstall a single model
  llm-manager uninstall qwen3_6 -k           # Uninstall but keep cached weights
  llm-manager uninstall --all                # Uninstall all models
  llm-manager uninstall --all -k             # Uninstall all but keep all caches`)
}
