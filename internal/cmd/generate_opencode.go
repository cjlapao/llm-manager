// Package cmd provides the generate subcommand for llm-manager.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("generate", func(root *RootCommand) Command { return NewGenerateCommand(root) })
}

// GenerateCommand handles code/config generation from model data.
type GenerateCommand struct {
	cfg *RootCommand
	svc *service.ModelService
}

// NewGenerateCommand creates a new GenerateCommand.
func NewGenerateCommand(root *RootCommand) *GenerateCommand {
	return &GenerateCommand{
		cfg: root,
		svc: service.NewModelService(root.db, root.cfg),
	}
}

// Run executes the generate command with the given subcommand and arguments.
func (c *GenerateCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	switch args[0] {
	case "opencode":
		return c.runOpenCode(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown generate subcommand: %s\n\n", args[0])
		c.PrintHelp()
		return 1
	}
}

// runOpenCode generates opencode-compatible model configuration JSON.
func (c *GenerateCommand) runOpenCode(args []string) int {
	if c.cfg.cfg.OpenAIAPIURL == "" {
		fmt.Fprintf(os.Stderr, "Error: OPENAI_API_URL is not configured\n")
		fmt.Fprintf(os.Stderr, "Set LITELLM_URL in config or environment\n")
		return 1
	}

	varSlug := false
	var slug string

	for _, arg := range args {
		if arg == "--all" {
			varSlug = true
		} else if !strings.HasPrefix(arg, "--") {
			slug = arg
		}
	}

	var data []byte
	var err error

	if slug != "" && varSlug {
		fmt.Fprintln(os.Stderr, "Error: cannot specify both --all and a slug")
		return 1
	}

	if slug != "" {
		data, err = c.svc.GenerateOpenCodeModel(slug)
	} else {
		data, err = c.svc.GenerateOpenCodeModels()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating opencode config: %v\n", err)
		return 1
	}

	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(data, &prettyJSON); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		return 1
	}

	output, err := json.MarshalIndent(prettyJSON, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		return 1
	}

	fmt.Println(string(output))
	return 0
}

// PrintHelp prints the generate command help.
func (c *GenerateCommand) PrintHelp() {
	fmt.Println(`generate - Generate configuration from model data.

USAGE:
  llm-manager generate opencode [SLUG] [--all]

ARGUMENTS:
  SLUG    Generate config for a single model slug
          (defaults to all models if omitted or --all is specified)

OPTIONS:
  --all   Generate config for all models (same as omitting SLUG)

OUTPUT:
  Produces a JSON object of model entries suitable for pasting
  directly into a provider's models section in opencode.json.

EXAMPLES:
  llm-manager generate opencode              # All models
  llm-manager generate opencode --all        # All models (explicit)
  llm-manager generate opencode qwen3_6      # Single model`)
}
