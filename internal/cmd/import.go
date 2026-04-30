// Package cmd provides the import subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("import", func(root *RootCommand) Command { return NewImportCommand(root) })
}

// ImportCommand handles import of models or engines from YAML files.
type ImportCommand struct {
	cfg *RootCommand
	svc *service.ModelService
	eng *service.EngineService
}

// NewImportCommand creates a new ImportCommand.
func NewImportCommand(root *RootCommand) *ImportCommand {
	svc := service.NewModelService(root.db, root.cfg)
	svc.SetEngineService(service.NewEngineService(root.db))
	return &ImportCommand{
		cfg: root,
		svc: svc,
		eng: service.NewEngineService(root.db),
	}
}

// Run executes the import command with the given arguments.
func (c *ImportCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Handle help before flag parsing
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			c.PrintHelp()
			return 0
		}
	}

	// Parse flags: --override and --folder supported
	var override bool
	var folderPath string
	filePaths := []string{}

	for _, arg := range args {
		if arg == "--override" {
			override = true
		} else if strings.HasPrefix(arg, "--folder") {
			// --folder=<path> or --folder <path>
			eqIdx := strings.Index(arg, "=")
			if eqIdx >= 0 {
				folderPath = arg[eqIdx+1:]
			} else {
				fmt.Fprintf(os.Stderr, "Error: --folder requires a path\n")
				return 1
			}
		} else if strings.HasPrefix(arg, "--") {
			fmt.Fprintf(os.Stderr, "Error: unknown flag %s (supported: --override, --folder)\n", arg)
			return 1
		} else {
			filePaths = append(filePaths, arg)
		}
	}

	if folderPath != "" && len(filePaths) > 0 {
		fmt.Fprintf(os.Stderr, "Error: cannot specify both --folder and file paths\n")
		return 1
	}

	if folderPath != "" {
		return c.runImportFolder(folderPath, override)
	}

	if len(filePaths) > 0 {
		// Import each file individually
		for _, path := range filePaths {
			if exitCode := c.runImportFile(path, override); exitCode != 0 {
				return exitCode
			}
		}
		return 0
	}

	fmt.Fprintln(os.Stderr, "Error: YAML file path or --folder is required")
	c.PrintHelp()
	return 1
}

// runImportFolder imports all YAML files from a directory.
// Recognized engine and model files are imported; unrecognized files are skipped with a warning.
func (c *ImportCommand) runImportFolder(folderPath string, override bool) int {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory %s: %v\n", folderPath, err)
		return 1
	}

	var yamlFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			yamlFiles = append(yamlFiles, filepath.Join(folderPath, entry.Name()))
		}
	}

	if len(yamlFiles) == 0 {
		fmt.Printf("No .yml or .yaml files found in %s\n", folderPath)
		return 0
	}

	fmt.Printf("Found %d YAML file(s) in %s\n", len(yamlFiles), folderPath)
	fmt.Println(strings.Repeat("-", 60))

	imported := 0
	skipped := 0

	for _, path := range yamlFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s — read error: %v\n", filepath.Base(path), err)
			skipped++
			continue
		}

		if service.IsEngineYAML(data) {
			fmt.Printf("  %s — engine config\n", filepath.Base(path))
			created, skippedCount, err := c.eng.ImportEngineFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    ✗ Error importing engine: %v\n", err)
				skipped++
				continue
			}
			fmt.Printf("    ✓ Engine imported: %d created, %d skipped\n", created, skippedCount)
			imported++
		} else {
			// Try model import — if it fails validation, skip silently
			err = c.tryImportModel(path, override)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %s — skipped: %v\n", filepath.Base(path), err)
				skipped++
				continue
			}
			imported++
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Import complete: %d imported, %d skipped\n", imported, skipped)
	if skipped > 0 {
		return 1
	}
	return 0
}

// tryImportModel attempts to import a model YAML file.
// Returns nil on success, or an error describing why it was skipped.
func (c *ImportCommand) tryImportModel(yamlPath string, override bool) error {
	// Quick check: does this look like a model YAML?
	// We peek at the raw bytes to avoid full parse cost.
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	// Must have a "slug" field at the top level (model indicator)
	var raw map[string]interface{}
	if err := c.parseQuick(data, &raw); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	if _, hasSlug := raw["slug"]; !hasSlug {
		return fmt.Errorf("not a recognized model or engine config")
	}

	overrides := service.ImportOverrides{
		Override: override,
	}
	_, err = c.svc.ImportModel(yamlPath, overrides)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	fmt.Printf("  ✓ %s — model imported\n", filepath.Base(yamlPath))
	return nil
}

// runImportFile imports a single YAML file (auto-detects engine vs model).
func (c *ImportCommand) runImportFile(yamlPath string, override bool) int {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", yamlPath, err)
		return 1
	}

	if service.IsEngineYAML(data) {
		return c.runEngineImport(yamlPath)
	}

	return c.runModelImport(yamlPath, override)
}

// runEngineImport imports an engine configuration from a YAML file.
func (c *ImportCommand) runEngineImport(yamlPath string) int {
	fmt.Printf("Importing engine from %s...\n", yamlPath)

	created, skipped, err := c.eng.ImportEngineFile(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing engine: %v\n", err)
		return 1
	}

	fmt.Printf("Imported engine: %d version(s) created, %d skipped\n", created, skipped)
	return 0
}

// runModelImport imports a model from a YAML file.
func (c *ImportCommand) runModelImport(yamlPath string, override bool) int {
	overrides := service.ImportOverrides{
		Override: override,
	}

	model, err := c.svc.ImportModel(yamlPath, overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing model: %v\n", err)
		return 1
	}

	fmt.Printf("Imported model: %s (%s)\n", model.Slug, model.Name)
	return 0
}

// parseQuick is a minimal YAML-aware quick parser for top-level key detection.
// Uses the same yaml.v3 import as the rest of the codebase.
func (c *ImportCommand) parseQuick(data []byte, target interface{}) error {
	// Reuse the yaml parser from the service layer
	return service.ParseQuickYAML(data, target)
}

// PrintHelp prints the import command help.
func (c *ImportCommand) PrintHelp() {
	fmt.Print(`import - Import a model or engine from a YAML file.

USAGE:
  llm-manager import <file.yml> [--override]
  llm-manager import --folder <directory> [--override]

DESCRIPTION:
  Auto-detects whether the YAML file contains an engine configuration or a
  model configuration based on its structure.

  Engine config: has a top-level "engine:" key with "slug" sub-key.
  Model config: has model fields like "slug:", "name:", "type:", etc.

ARGUMENTS:
  <file.yml>    Path to a single YAML file to import
  --folder      Import all .yml/.yaml files from a directory

OPTIONS:
  --override    Delete existing DB + LiteLLM records before re-importing
                (model import only; silently ignored for engine import)

EXAMPLES:
  llm-manager import model.yaml
  llm-manager import model.yaml --override
  llm-manager import ./engines/vllm.yml
  llm-manager import --folder ./models/
  llm-manager import --folder ./models/ --override
  llm-manager engine import ./engines/vllm.yml    (pre-validates engine type)
`)
}
