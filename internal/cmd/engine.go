// Package cmd provides the engine subcommand for llm-manager.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

func init() {
	RegisterCommand("engine", func(root *RootCommand) Command { return NewEngineCommand(root) })
}

// EngineCommand handles engine type management.
type EngineCommand struct {
	cfg *RootCommand
	svc *service.EngineService
}

// NewEngineCommand creates a new EngineCommand.
func NewEngineCommand(root *RootCommand) *EngineCommand {
	return &EngineCommand{
		cfg: root,
		svc: service.NewEngineService(root.db),
	}
}

// Run executes the engine command with subcommands.
func (c *EngineCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	sub := args[0]
	switch sub {
	case "ls", "list":
		return c.cmdList(args[1:])
	case "get":
		return c.cmdGet(args[1:])
	case "del", "delete":
		return c.cmdDelete(args[1:])
	case "version":
		return c.cmdVersion(args[1:])
	case "help", "-h", "--help":
		c.PrintHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown engine subcommand '%s'\n\n", sub)
		c.PrintHelp()
		return 1
	}
}

// PrintHelp prints the engine command help.
func (c *EngineCommand) PrintHelp() {
	fmt.Println(`engine - Manage LLM inference engine types and versions.

USAGE:
  llm-manager engine <subcommand> [arguments]

SUBCOMMANDS:
  ls [--type <slug>]                List all engine types
  get <slug>                        Show details for an engine type
  del <slug>                        Delete an engine type (refuses if versions exist)
  version ls [--type <slug>]        List versions for an engine type
  version get <type>/<slug>         Show details for an engine version
  version del <type>/<slug>         Delete an engine version (refuses if used by models)
  version show-composition <type>/<slug>  Print generated docker-compose YAML for a version
  version import <file.yml> [--overwrite]  Import engine versions from YAML file
                                           (--overwrite updates existing records)

EXAMPLES:
  llm-manager engine ls
  llm-manager engine get vllm
  llm-manager engine del vllm
  llm-manager engine version ls --type vllm
  llm-manager engine version get vllm/pgx-llm-v1
  llm-manager engine version show-composition vllm/pgx-llm-v1
  llm-manager engine version import ./engines/vllm.yml --overwrite`)
}

// ---------------------------------------------------------------------------
// engine list
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdList(args []string) int {
	var filterType string
	for i := 0; i < len(args); i++ {
		if args[i] == "--type" && i+1 < len(args) {
			filterType = args[i+1]
			i++
		}
	}

	types, err := c.svc.ListAllEngineTypes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing engine types: %v\n", err)
		return 1
	}

	if len(types) == 0 {
		fmt.Println("No engine types found.")
		return 0
	}

	fmt.Printf("%-20s %-30s %-12s %s\n", "SLUG", "NAME", "PROVIDER", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 90))
	for _, t := range types {
		if filterType != "" && t.Slug != filterType {
			continue
		}
		name := t.Name
		if name == "" {
			name = "<unset>"
		}
		desc := t.Description
		if desc == "" {
			desc = "<unset>"
		}
		provider := t.Provider
		if provider == "" {
			provider = "custom"
		}
		fmt.Printf("%-20s %-30s %-12s %s\n", t.Slug, name, provider, desc)
	}
	return 0
}

// ---------------------------------------------------------------------------
// engine get
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdGet(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: engine slug required")
		return 1
	}
	et, err := c.svc.GetEngineTypeBySlug(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Slug:     %s\n", et.Slug)
	fmt.Printf("Name:     %s\n", et.Name)
	fmt.Printf("Provider: %s\n", et.Provider)
	fmt.Printf("Desc:     %s\n", et.Description)
	fmt.Printf("Created:  %s\n", et.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:  %s\n", et.UpdatedAt.Format("2006-01-02 15:04:05"))

	versions, err := c.svc.ListEngineVersionsByType(et.Slug)
	if err == nil && len(versions) > 0 {
		fmt.Printf("\nVersions (%d):\n", len(versions))
		for _, v := range versions {
			def := ""
			if v.IsDefault {
				def = " (default)"
			}
			latest := ""
			if v.IsLatest {
				latest = " (latest)"
			}
			fmt.Printf("  - %s [%s]%s%s\n", v.Slug, v.Version, def, latest)
		}
	} else {
		fmt.Println("\nNo versions defined.")
	}
	return 0
}

// ---------------------------------------------------------------------------
// engine delete
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: engine slug required")
		return 1
	}
	versions, err := c.svc.ListEngineVersionsByType(args[0])
	if err == nil && len(versions) > 0 {
		fmt.Fprintf(os.Stderr, "Error: cannot delete engine type '%s' — %d version(s) exist\n", args[0], len(versions))
		return 1
	}
	err = c.svc.DeleteEngineType(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting: %v\n", err)
		return 1
	}
	fmt.Printf("Engine type '%s' deleted\n", args[0])
	return 0
}

// ---------------------------------------------------------------------------
// engine version
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdVersion(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: engine-version subcommand required (ls, get, del, show-composition)")
		return 1
	}
	sub := args[0]
	switch sub {
	case "ls", "list":
		return c.cmdVersionList(args[1:])
	case "get":
		return c.cmdVersionGet(args[1:])
	case "del", "delete":
		return c.cmdVersionDelete(args[1:])
	case "show-composition":
		return c.cmdVersionShowComposition(args[1:])
	case "import":
		return c.cmdImport(args[1:])
	case "help", "-h", "--help":
		fmt.Println(`engine version - Manage engine versions.

USAGE:
  llm-manager engine version <subcommand> [arguments]

SUBCOMMANDS:
  ls [--type <slug>]                      List versions for an engine type
  get <type>/<slug>                       Show details for an engine version
  del <type>/<slug>                       Delete an engine version
  show-composition <type>/<slug>          Print generated docker-compose YAML
  import <file.yml> [--overwrite]         Import engine versions from YAML file
                                          (--overwrite updates existing records)`)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown engine-version subcommand '%s'\n", sub)
		return 1
	}
}

// ---------------------------------------------------------------------------
// engine version list
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdVersionList(args []string) int {
	var filterType string
	for i := 0; i < len(args); i++ {
		if args[i] == "--type" && i+1 < len(args) {
			filterType = args[i+1]
			i++
		}
	}

	types, err := c.svc.ListAllEngineTypes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing engine types: %v\n", err)
		return 1
	}

	hasOutput := false
	for _, t := range types {
		if filterType != "" && t.Slug != filterType {
			continue
		}
		versions, err := c.svc.ListEngineVersionsByType(t.Slug)
		if err != nil {
			continue
		}
		if len(versions) == 0 {
			continue
		}
		if !hasOutput {
			provider := t.Provider
			if provider == "" {
				provider = "custom"
			}
			fmt.Printf("Engine: %s (provider: %s)\n", t.Slug, provider)
			fmt.Printf("%-20s %-10s %-40s %-10s %-10s\n", "SLUG", "VERSION", "IMAGE", "DEFAULT", "LATEST")
			fmt.Println(strings.Repeat("-", 100))
			hasOutput = true
		}
		for _, v := range versions {
			imageShort := v.Image
			if len(imageShort) > 40 {
				imageShort = imageShort[:37] + "..."
			}
			def := "-"
			if v.IsDefault {
				def = "yes"
			}
			latest := "-"
			if v.IsLatest {
				latest = "yes"
			}
			fmt.Printf("%-20s %-10s %-40s %-10s %-10s\n", v.Slug, v.Version, imageShort, def, latest)
		}
	}
	if !hasOutput {
		fmt.Println("No engine versions found.")
	}
	return 0
}

// ---------------------------------------------------------------------------
// engine version get
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdVersionGet(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: type/<slug> required (e.g., vllm/pgx-llm-v1)")
		return 1
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		fmt.Fprintln(os.Stderr, "Error: format must be <type>/<slug>")
		return 1
	}
	v, err := c.svc.GetEngineVersionByTypeAndSlug(parts[0], parts[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Slug:          %s\n", v.Slug)
	fmt.Printf("Type:          %s\n", v.EngineTypeSlug)
	fmt.Printf("Version:       %s\n", v.Version)
	fmt.Printf("Container:     %s\n", v.ContainerName)
	fmt.Printf("Image:         %s\n", v.Image)
	fmt.Printf("Entrypoint:    %s\n", v.Entrypoint)
	fmt.Printf("Default:       %v\n", v.IsDefault)
	fmt.Printf("Latest:        %v\n", v.IsLatest)
	fmt.Printf("Logging:       %v\n", v.EnableLogging)
	if v.EnableLogging {
		fmt.Printf("  Address:     %s\n", v.SyslogAddress)
		fmt.Printf("  Facility:    %s\n", v.SyslogFacility)
	}
	fmt.Printf("NVIDIA:        %v\n", v.DeployEnableNvidia)
	if v.DeployEnableNvidia {
		fmt.Printf("  GPU Count:   %s\n", v.DeployGPUCount)
	}
	fmt.Printf("Commands:      %s\n", v.CommandArgs)
	fmt.Printf("Environment:   %s\n", v.EnvironmentJSON)
	fmt.Printf("Volumes:       %s\n", v.VolumesJSON)
	fmt.Printf("Created:       %s\n", v.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:       %s\n", v.UpdatedAt.Format("2006-01-02 15:04:05"))
	return 0
}

// ---------------------------------------------------------------------------
// engine version delete
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdVersionDelete(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: type/<slug> required")
		return 1
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		fmt.Fprintln(os.Stderr, "Error: format must be <type>/<slug>")
		return 1
	}
	// Check if any models use this version
	models, err := c.svc.ListModelsByEngineVersion(parts[1])
	if err == nil && len(models) > 0 {
		fmt.Fprintf(os.Stderr, "Error: cannot delete version '%s/%s' — used by %d model(s)\n", parts[0], parts[1], len(models))
		for _, m := range models {
			fmt.Fprintf(os.Stderr, "  - %s (%s)\n", m.Slug, m.Name)
		}
		return 1
	}
	err = c.svc.DeleteEngineVersionByTypeAndSlug(parts[0], parts[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting: %v\n", err)
		return 1
	}
	fmt.Printf("Engine version '%s/%s' deleted\n", parts[0], parts[1])
	return 0
}

// ---------------------------------------------------------------------------
// engine version show-composition
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdVersionShowComposition(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: type/<slug> required")
		return 1
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		fmt.Fprintln(os.Stderr, "Error: format must be <type>/<slug>")
		return 1
	}
	v, err := c.svc.GetEngineVersionByTypeAndSlug(parts[0], parts[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if v == nil {
		fmt.Fprintln(os.Stderr, "Error: engine version not found")
		return 1
	}
	composeYAML, err := c.svc.ShowComposition(nil, v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating composition: %v\n", err)
		return 1
	}
	fmt.Println(composeYAML)
	return 0
}

// ---------------------------------------------------------------------------
// engine version import
// ---------------------------------------------------------------------------

func (c *EngineCommand) cmdImport(args []string) int {
	var overwrite bool
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: YAML file path required")
		fmt.Fprintln(os.Stderr, "\nUSAGE:")
		fmt.Fprintln(os.Stderr, "  llm-manager engine import <file.yml> [--overwrite]")
		fmt.Fprintln(os.Stderr, "\nPre-validates the file is an engine-type config before importing.")
		fmt.Fprintln(os.Stderr, "For general auto-detect import, use:")
		fmt.Fprintln(os.Stderr, "  llm-manager import <file.yml>")
		return 1
	}

	// Parse flags
	var yamlPath string
	for _, arg := range args {
		if arg == "--overwrite" {
			overwrite = true
		} else if strings.HasPrefix(arg, "--") {
			fmt.Fprintf(os.Stderr, "Error: unknown flag %s (supported: --overwrite)\n", arg)
			return 1
		} else {
			yamlPath = arg
		}
	}

	if yamlPath == "" {
		fmt.Fprintln(os.Stderr, "Error: YAML file path required")
		return 1
	}

	// Pre-validate: read file and check it's an engine-type YAML
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", yamlPath, err)
		return 1
	}

	if !service.IsEngineYAML(data) {
		fmt.Fprintf(os.Stderr, "Error: %s does not contain a valid engine configuration (missing engine:key with slug)\n", yamlPath)
		fmt.Fprintln(os.Stderr, "Use 'llm-manager import <file.yml>' for general import that auto-detects type.")
		return 1
	}

	fmt.Printf("Importing engine from %s...\n", yamlPath)
	overrides := service.EngineImportOverrides{
		Overwrite: overwrite,
	}
	created, updated, skipped, err := c.svc.ImportEngineFile(yamlPath, overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing engine: %v\n", err)
		return 1
	}

	fmt.Printf("Imported engine: %d version(s) created, %d updated, %d skipped\n", created, updated, skipped)
	return 0
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toJSONStr(m map[string]string) string {
	if m == nil || len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func toJSONArr(a []string) string {
	if a == nil || len(a) == 0 {
		return ""
	}
	b, _ := json.Marshal(a)
	return string(b)
}
