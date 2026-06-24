package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
)

// splitArgs separates command-line args into positional arguments (anything that
// doesn't start with "-") and flags (anything starting with "-").
func splitArgs(args []string) (positional []string, flags []string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			positional = append(positional, arg)
		}
	}
	return
}

// ── logs ─────────────────────────────────────────────────────────────────

// runLogs shows container logs.
func (c *LlmCommand) runLogs(args []string) int {
	// Split into positional args (slug candidate) and flag args
	positional, allFlags := splitArgs(args)

	slug := ""
	if len(positional) > 0 {
		slug = positional[0]
	}

	isLatest := slug == "latest"
	if slug == "" || isLatest {
		resolved, err := resolveLatestSlug(c.cfg.db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		slug = resolved
		if isLatest {
			fmt.Printf("Resolving 'latest' to model: %s\n", slug)
		} else {
			fmt.Printf("Using latest model: %s\n", slug)
		}
	}

	lines := 50
	follow := false
	for _, arg := range allFlags {
		if arg == "-f" || arg == "--follow" {
			follow = true
		} else {
			if n, _ := fmt.Sscanf(arg, "%d", &lines); n == 0 {
				fmt.Fprintf(os.Stderr, "Warning: invalid log line count %q, using default 50\n", arg)
			}
		}
	}

	containerName, err := c.resolveContainer(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if follow {
		cmd := exec.Command("docker", "logs", "-f", "--tail", fmt.Sprintf("%d", lines), containerName)
		if err := RunInteractive(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error following logs for %s: %v\n", containerName, err)
			return 1
		}
		return 0
	}

	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting logs for %s: %s\n", containerName, strings.TrimSpace(string(output)))
		return 1
	}

	fmt.Print(string(output))
	return 0
}

// resolveContainer resolves a slug or service alias to a Docker container name.
func (c *LlmCommand) resolveContainer(slug string) (string, error) {
	containerName := ResolveServiceAlias(slug)
	if containerName != "" {
		return containerName, nil
	}

	model, err := c.cfg.db.GetModel(slug)
	if err == nil && model.Container != "" {
		return model.GetContainerName(), nil
	}

	fmt.Fprintf(os.Stderr, "Unknown service or model: %s\n\n", slug)
	fmt.Fprint(os.Stderr, "Known services:\n")
	for _, alias := range KnownServiceAliases() {
		fmt.Fprintf(os.Stderr, "  %-15s -> %s\n", alias, ServiceAliases[alias])
	}
	fmt.Fprint(os.Stderr, "\nOr use a model slug that has a container configured.\n")
	return "", fmt.Errorf("unknown service or model: %s", slug)
}

// resolveLatestSlug resolves the "latest" keyword to an actual model slug.
// Returns the resolved slug, or an error if no latest model is set or the resolved
// model doesn't exist in the database.
func resolveLatestSlug(db database.DatabaseManager) (string, error) {
	configSvc := service.NewConfigService(db)
	resolved, err := configSvc.GetLatestModel()
	if err != nil {
		return "", fmt.Errorf("error resolving latest model: %w", err)
	}
	if resolved == "" {
		return "", fmt.Errorf("no latest model has been set. Start a model first with 'llm-manager llm start <slug>'")
	}
	if _, err := db.GetModel(resolved); err != nil {
		return "", fmt.Errorf("resolved model %q is not a known model", resolved)
	}
	return resolved, nil
}

// ── help ───────────────────────────────────────────────────────────────────

// PrintHelp prints the llm command help.
func (c *LlmCommand) PrintHelp() {
	fmt.Println(`llm - Manage LLM model containers (start, stop, restart, swap, ls, status, logs).

USAGE:
  llm-manager llm [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  start [<slug>]     Start a model container (defaults to latest if omitted)
  stop [<slug>]      Stop a model container (defaults to latest if omitted)
  restart [<slug>]   Restart a model container (defaults to latest if omitted)
  swap [<slug>]      GPU-safe model swap (defaults to latest if omitted)
  status [slug]         Show all container status and latest model info
  status <slug>         Show status of a specific container
  ls                    List all LLM models (with live STATUS, CACHED, and ENGINE columns)
  logs [<slug>] [-f] [lines]  Show container logs (-f for follow mode, defaults to latest)

FLAGS:
  --dry-run, -n           Preview startup (memory checks, diagnostics) without
                          modifying Docker
  --allow-multiple, -m    Only for 'start' and 'swap': don't stop other running
                          LLM containers before starting
  --wait, -w              Wait for the container to become healthy before returning
                          (polls /health endpoint up to 180s, stops container on failure)

SERVICE ALIASES (for logs):
  comfyui, flux   -> comfyui-flux
  whisper         -> whisper-stt
  kokoro          -> kokoro-tts
  litellm         -> litellm
  swap-api, swapapi -> swap-api
  open-webui, webui -> open-webui
  mcp             -> mcpo

EXAMPLES:
  llm-manager llm start                   Start using the latest model
  llm-manager llm start qwen3_6
  llm-manager llm start latest
  llm-manager llm start qwen3_6 --allow-multiple
  llm-manager llm start qwen3_6 --wait
  llm-manager llm stop                    Stop the latest model
  llm-manager llm stop qwen3_6
  llm-manager llm restart                 Restart the latest model
  llm-manager llm restart qwen3_6
  llm-manager llm swap                    Swap to the latest model
  llm-manager llm swap qwen3_6
  llm-manager llm status  
  llm-manager llm status qwen3_6  
  llm-manager llm ls
  llm-manager llm logs              Show logs from latest model
  llm-manager llm logs -f           Follow logs from latest model
  llm-manager llm logs qwen3_6 -f   Follow logs for specific model
  llm-manager llm logs comfyui 100`)
}
