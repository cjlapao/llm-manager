// Package cmd provides the mem subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("mem", func(root *RootCommand) Command { return NewMemCommand(root) })
}

// MemCommand displays GPU VRAM estimation for LLM models.
type MemCommand struct {
	cfg *config.Config
	svc *service.MemService
}

// NewMemCommand creates a new MemCommand.
func NewMemCommand(root *RootCommand) *MemCommand {
	return &MemCommand{
		cfg: root.cfg,
		svc: service.NewMemService(nil, root.cfg),
	}
}

// Run executes the mem command with the given arguments.
func (c *MemCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Handle help explicitly
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		c.PrintHelp()
		return 0
	}

	// First arg is optional model slug
	slug := ""
	for _, arg := range args {
		if arg != "info" && arg != "status" && arg != "vram" && arg != "estimate" {
			slug = arg
			break
		}
	}

	if slug == "" {
		c.PrintHelp()
		return 0
	}

	return c.runEstimate(slug)
}

// runEstimate estimates VRAM usage for a model.
func (c *MemCommand) runEstimate(slug string) int {
	// Create a new MemService with actual DB if available
	// The MemCommand doesn't have direct DB access, so we use a nil db
	// which means it will only read from models.json and HF cache/API
	svc := service.NewMemService(nil, c.cfg)

	results, err := svc.EstimateVRAM(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error estimating VRAM: %v\n", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No models found for slug: %s\n", slug)
		return 1
	}

	// Print header
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tNAME\tQUANT\tWEIGHTS\tKV@4K\tKV@32K\tKV@128K\tKV@262K")
	fmt.Fprintln(w, "----\t----\t-----\t-------\t-----\t------\t-------\t-------")

	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Slug, r.Name, r.Quant,
			service.FormatVRAM(r.Weights),
			service.FormatKV(r.KV4K),
			service.FormatKV(r.KV32K),
			service.FormatKV(r.KV128K),
			service.FormatKV(r.KV262K),
		)
	}
	w.Flush()

	return 0
}

// PrintHelp prints the mem command help.
func (c *MemCommand) PrintHelp() {
	fmt.Println(`mem - Estimate GPU VRAM usage for LLM models.

USAGE:
  llm-manager mem [slug]

ARGUMENTS:
  slug    Optional: estimate VRAM for a specific model.
          If omitted, shows help.

OUTPUT:
  Estimates VRAM based on model architecture, quantization, and context length.
  KV cache sizes are shown for 4K, 32K, 128K, and 262K context lengths.

EXAMPLES:
  llm-manager mem qwen3_6
  llm-manager mem

NOTES:
  Reads model definitions from models.json and architecture config from
  HuggingFace cache or API. Requires HF_TOKEN for private repos.`)
}
