// Package cmd provides the compose subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("compose", func(root *RootCommand) Command { return NewComposeCommand(root) })
}

// ComposeCommand handles docker-compose file generation.
type ComposeCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewComposeCommand creates a new ComposeCommand.
func NewComposeCommand(root *RootCommand) *ComposeCommand {
	svc := service.NewModelService(root.db, root.cfg)
	svc.SetEngineService(service.NewEngineService(root.db))
	return &ComposeCommand{
		cfg: root,
		svc: svc,
	}
}

// Run executes the compose command with the given arguments.
func (c *ComposeCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Handle help flags first
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			c.PrintHelp()
			return 0
		}
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
				default:
					fmt.Fprintf(os.Stderr, "Error: unknown flag --%s\n", key)
					return 1
				}
			} else {
				fmt.Fprintf(os.Stderr, "Error: flag --%s requires a value\n", arg[2:])
				return 1
			}
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

	// Generate compose YAML
	generator, err := service.NewComposeGenerator()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing compose generator: %v\n", err)
		return 1
	}

	cfg := service.EngineComposeConfig{}
	composeYAML, err := c.svc.GenerateCompose(slug, generator, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating compose file: %v\n", err)
		return 1
	}

	// Default output path
	if outputPath == "" {
		outputPath = "compose.yml"
	}

	// Ensure parent directory exists
	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			return 1
		}
	}

	if err := os.WriteFile(outputPath, []byte(composeYAML), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file %s: %v\n", outputPath, err)
		return 1
	}

	fmt.Printf("Generated docker-compose.yml for model %s → %s\n", slug, outputPath)
	return 0
}

// PrintHelp prints the compose command help.
func (c *ComposeCommand) PrintHelp() {
	fmt.Println(`compose - Generate a docker-compose.yml file for a model.

USAGE:
  llm-manager model compose <slug> [OPTIONS]

ARGUMENTS:
  <slug>    Slug of the model to generate compose file for

OPTIONS:
  --output <file.yml>    Output file path (default: compose.yml)

EXAMPLES:
  llm-manager model compose qwen3_6
  llm-manager model compose qwen3_6 --output qwen3_6-compose.yml
  llm-manager model compose flux-schnell --output /tmp/flux-compose.yml`)
}
