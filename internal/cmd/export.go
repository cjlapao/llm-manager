// Package cmd provides the export subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
	"gopkg.in/yaml.v3"
)

func init() {
	RegisterCommand("export", func(root *RootCommand) Command { return NewExportCommand(root) })
}

// ExportCommand handles model export to YAML files.
type ExportCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewExportCommand creates a new ExportCommand.
func NewExportCommand(root *RootCommand) *ExportCommand {
	return &ExportCommand{
		cfg: root,
		svc: service.NewModelService(root.db, root.cfg),
	}
}

// Run executes the export command with the given arguments.
func (c *ExportCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Parse args: first non-flag is the slug, optional --output flag
	var slug string
	var outputPath string

	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			eqIdx := strings.Index(arg, "=")
			if eqIdx >= 0 {
				key := arg[2:eqIdx]
				val := arg[eqIdx+1:]
				switch key {
				case "output":
					outputPath = val
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
		} else if arg == "-h" || arg == "--help" || arg == "help" {
			c.PrintHelp()
			return 0
		} else {
			if slug == "" {
				slug = arg
			} else {
				fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n", arg)
				return 1
			}
		}
	}

	if slug == "" {
		fmt.Fprintf(os.Stderr, "Error: model slug is required\n\n")
		c.PrintHelp()
		return 1
	}

	y, err := c.svc.ExportModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting model: %v\n", err)
		return 1
	}

	// Default output path
	if outputPath == "" {
		outputPath = slug + ".yaml"
	}

	data, err := yaml.Marshal(y)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling YAML: %v\n", err)
		return 1
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file %s: %v\n", outputPath, err)
		return 1
	}

	fmt.Printf("Exported model %s to %s\n", slug, outputPath)
	return 0
}

// PrintHelp prints the export command help.
func (c *ExportCommand) PrintHelp() {
	fmt.Println(`export - Export a model to a YAML file.

USAGE:
  llm-manager model export <slug> [OPTIONS]

ARGUMENTS:
  <slug>    Slug of the model to export

OPTIONS:
  --output <file.yaml>    Output file path (default: <slug>.yaml)

EXAMPLES:
  llm-manager model export qwen3_6
  llm-manager model export qwen3_6 --output my-model.yaml
  llm-manager model export flux-schnell --output /tmp/flux.yaml`)
}
