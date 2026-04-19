// Package cmd provides the import subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

// ImportCommand handles model import from YAML files.
type ImportCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewImportCommand creates a new ImportCommand.
func NewImportCommand(root *RootCommand) *ImportCommand {
	return &ImportCommand{
		cfg: root,
		svc: service.NewModelService(root.db),
	}
}

// Run executes the import command with the given arguments.
func (c *ImportCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Parse args: first non-flag is the YAML file path
	var yamlPath string
	overrides := service.ImportOverrides{}

	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			// Parse flag
			eqIdx := strings.Index(arg, "=")
			if eqIdx >= 0 {
				key := arg[2:eqIdx]
				val := arg[eqIdx+1:]
				switch key {
				case "input-cost", "input-token-cost":
					v, err := strconv.ParseFloat(val, 64)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error: invalid input-cost value: %s\n", val)
						return 1
					}
					overrides.InputCost = &v
				case "output-cost", "output-token-cost":
					v, err := strconv.ParseFloat(val, 64)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error: invalid output-cost value: %s\n", val)
						return 1
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
				case "help":
					c.PrintHelp()
					return 0
				default:
					fmt.Fprintf(os.Stderr, "Error: unknown flag --%s\n", key)
					return 1
				}
			} else {
				// Flag without value (e.g., --help)
				key := arg[2:]
				if key == "help" {
					c.PrintHelp()
					return 0
				}
			}
		} else {
			// Non-flag argument = YAML file path
			if yamlPath == "" {
				yamlPath = arg
			} else {
				fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n", arg)
				return 1
			}
		}
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

// PrintHelp prints the import command help.
func (c *ImportCommand) PrintHelp() {
	fmt.Println(`import - Import a model from a YAML file.

USAGE:
  llm-manager model import <file.yaml> [OPTIONS]

ARGUMENTS:
  <file.yaml>    Path to the YAML file to import

OPTIONS:
  --input-cost <float>        Override input token cost from YAML
  --output-cost <float>       Override output token cost from YAML
  --capabilities <comma,list> Override capabilities list from YAML

EXAMPLES:
  llm-manager model import model.yaml
  llm-manager model import qwen3.yaml --input-cost 0.000001 --output-cost 0.000002
  llm-manager model import model.yaml --capabilities reasoning,tool-use,multi-turn
  llm-manager model import qwen3.yaml --output-token-cost 0.0000004`)
}
