// Package cmd provides the import subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("import", func(root *RootCommand) Command { return NewImportCommand(root) })
}

// ImportCommand handles model import from YAML files.
type ImportCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewImportCommand creates a new ImportCommand.
func NewImportCommand(root *RootCommand) *ImportCommand {
	return &ImportCommand{
		cfg: root,
		svc: service.NewModelService(root.db, root.cfg),
	}
}

// Run executes the import command with the given arguments.
func (c *ImportCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Flags that take a value from the next argument (no =)
	valueFlags := map[string]bool{
		"folder": true,
		"type":   true,
		"engine": true,
	}

	var yamlPath string
	var folderPath string
	overrides := service.ImportOverrides{}

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			// Parse flag
			eqIdx := strings.Index(arg, "=")
			if eqIdx >= 0 {
				// Flag with = (e.g., --folder=/path or --input-cost=0.001)
				key := arg[2:eqIdx]
				val := arg[eqIdx+1:]
				if !c.handleFlag(key, val, &yamlPath, &folderPath, &overrides) {
					return 1
				}
	} else {
			key := arg[2:]
			switch key {
			case "help":
				c.PrintHelp()
				return 0
			case "override":
				overrides.Override = true
			default:
				if valueFlags[key] {
					i++
					if i >= len(args) {
						fmt.Fprintf(os.Stderr, "Error: --%s requires a value\n", key)
						return 1
					}
					switch key {
					case "folder":
						folderPath = args[i]
					case "type":
						if !c.handleFlag("type", args[i], &yamlPath, &folderPath, &overrides) {
							return 1
						}
					case "engine":
						if !c.handleFlag("engine", args[i], &yamlPath, &folderPath, &overrides) {
							return 1
						}
					}
				} else {
					fmt.Fprintf(os.Stderr, "Error: unknown flag --%s\n", key)
					return 1
				}
			}
		}
	} else if arg == "-h" || arg == "--help" || arg == "help" {
			c.PrintHelp()
			return 0
		} else {
			// Non-flag argument = YAML file path
			if yamlPath == "" {
				yamlPath = arg
			} else {
				fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n", arg)
				return 1
			}
		}
		i++
	}

	if folderPath != "" && yamlPath != "" {
		fmt.Fprintf(os.Stderr, "Error: cannot specify both --folder and a file path\n")
		return 1
	}

	if folderPath != "" {
		return c.runImportFolder(folderPath, overrides)
	}

	if yamlPath == "" {
		fmt.Fprintf(os.Stderr, "Error: YAML file path is required\n\n")
		c.PrintHelp()
		return 1
	}

	model, err := c.svc.ImportModel(yamlPath, overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing model: %v\n", err)
		return 1
	}

	fmt.Printf("Imported model: %s (%s)\n", model.Slug, model.Name)
	return 0
}

// handleFlag processes a flag=value pair. Returns false on error.
func (c *ImportCommand) handleFlag(key, val string, yamlPath, folderPath *string, overrides *service.ImportOverrides) bool {
	switch key {
	case "input-cost", "input-token-cost":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid input-cost value: %s\n", val)
			return false
		}
		overrides.InputCost = &v
	case "output-cost", "output-token-cost":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid output-cost value: %s\n", val)
			return false
		}
		overrides.OutputCost = &v
	case "capabilities":
		var caps []string
		for _, cap := range strings.Split(val, ",") {
			t := strings.TrimSpace(cap)
			if t != "" {
				caps = append(caps, t)
			}
		}
		overrides.Capabilities = caps
	case "type":
		validTypes := []string{"llm", "rag", "speech", "comfyui"}
		valid := false
		for _, t := range validTypes {
			if val == t {
				valid = true
				break
			}
		}
		if !valid {
			fmt.Fprintf(os.Stderr, "Error: invalid --type value: %s (must be one of: llm, rag, speech, comfyui)\n", val)
			return false
		}
		overrides.Type = val
	case "engine":
		validEngines := []string{"vllm", "sglang", "llama.cpp"}
		valid := false
		for _, e := range validEngines {
			if val == e {
				valid = true
				break
			}
		}
		if !valid {
			fmt.Fprintf(os.Stderr, "Error: invalid --engine value: %s (must be one of: vllm, sglang, llama.cpp)\n", val)
			return false
		}
		overrides.Engine = val
	case "folder":
		*folderPath = val
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown flag --%s\n", key)
		return false
	}
	return true
}

// runImportFolder imports all valid YAML files from a directory.
func (c *ImportCommand) runImportFolder(folderPath string, overrides service.ImportOverrides) int {
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

	successCount := 0
	failCount := 0

	for _, path := range yamlFiles {
		fmt.Printf("\nImporting: %s\n", filepath.Base(path))
		model, err := c.svc.ImportModel(path, overrides)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Failed: %v\n", err)
			failCount++
			continue
		}
		fmt.Printf("  ✓ Imported: %s (%s)\n", model.Slug, model.Name)
		successCount++
	}

	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Import complete: %d succeeded, %d failed\n", successCount, failCount)
	if failCount > 0 {
		return 1
	}
	return 0
}

// PrintHelp prints the import command help.
func (c *ImportCommand) PrintHelp() {
	fmt.Print(`import - Import a model from a YAML file.

USAGE:
  llm-manager model import <file.yaml> [OPTIONS]
  llm-manager model import --folder <directory> [OPTIONS]

ARGUMENTS:
  <file.yaml>    Path to the YAML file to import
  --folder       Import all .yml/.yaml files from a directory

OPTIONS:
  --input-cost <float>        Override input token cost from YAML
  --output-cost <float>       Override output token cost from YAML
  --capabilities <comma,list> Override capabilities list from YAML
  --type <llm|rag|speech|comfyui> Override model type (default: llm)
  --engine <vllm|sglang|llama.cpp> Override engine type (default: vllm)
  --override                  Delete existing DB + LiteLLM records before re-importing

EXAMPLES:
  llm-manager model import model.yaml
  llm-manager model import qwen3.yaml --input-cost 0.000001 --output-cost 0.000002
  llm-manager model import model.yaml --capabilities reasoning,tool-use,multi-turn
  llm-manager model import qwen3.yaml --override
  llm-manager model import --folder models/
  llm-manager model import --folder models/ --override
  llm-manager model import model.yaml --type rag --engine sglang
`)
}
