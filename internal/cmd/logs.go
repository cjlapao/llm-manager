// Package cmd provides the logs subcommand for llm-manager.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/user/llm-manager/internal/service"
)

// LogsCommand handles viewing container and application logs.
type LogsCommand struct {
	cfg *RootCommand
	svc *service.ContainerService
}

// NewLogsCommand creates a new LogsCommand.
func NewLogsCommand(root *RootCommand) *LogsCommand {
	return &LogsCommand{
		cfg: root,
		svc: service.NewContainerService(root.db, root.cfg),
	}
}

// Run executes the logs command with the given arguments.
func (c *LogsCommand) Run(args []string) int {
	if len(args) == 0 {
		c.PrintHelp()
		return 0
	}

	// Parse flags
	follow := false
	lines := 50
	slug := ""

	for _, arg := range args {
		switch arg {
		case "-f", "--follow":
			follow = true
		default:
			if slug == "" {
				slug = arg
			} else {
				fmt.Sscanf(arg, "%d", &lines)
			}
		}
	}

	if slug == "" {
		c.PrintHelp()
		return 0
	}

	return c.runLogs(slug, lines, follow)
}

// runLogs retrieves and displays container logs.
func (c *LogsCommand) runLogs(slug string, lines int, follow bool) int {
	// Resolve slug to container name
	containerName, err := c.resolveContainer(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if follow {
		// Follow mode: stream logs in real-time
		cmd := exec.Command("docker", "logs", "-f", "--tail", fmt.Sprintf("%d", lines), containerName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error following logs for %s: %v\n", containerName, err)
			return 1
		}
		return 0
	}

	// Snapshot mode: show last N lines
	args := []string{"logs", "--tail", fmt.Sprintf("%d", lines), containerName}
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting logs for %s: %s\n", containerName, strings.TrimSpace(string(output)))
		return 1
	}

	if len(output) == 0 {
		fmt.Printf("No logs available for container: %s\n", containerName)
		return 0
	}

	fmt.Print(string(output))
	return 0
}

// resolveContainer resolves a slug or service alias to a Docker container name.
func (c *LogsCommand) resolveContainer(slug string) (string, error) {
	// Check if it's a service alias
	containerName := resolveServiceAlias(slug)
	if containerName != "" {
		return containerName, nil
	}

	// Check if it's a model slug (look up container from DB)
	model, err := c.cfg.db.GetModel(slug)
	if err == nil && model.Container != "" {
		return model.Container, nil
	}

	// Known service aliases for error message
	knownServices := map[string]string{
		"comfyui":    "comfyui-flux",
		"flux":       "comfyui-flux",
		"embed":      "llm-embed",
		"rerank":     "llm-rerank",
		"whisper":    "whisper-stt",
		"kokoro":     "kokoro-tts",
		"litellm":    "litellm",
		"swap-api":   "swap-api",
		"swapapi":    "swap-api",
		"open-webui": "open-webui",
		"webui":      "open-webui",
		"mcp":        "mcpo",
	}

	fmt.Fprintf(os.Stderr, "Unknown service or model: %s\n\n", slug)
	fmt.Fprint(os.Stderr, "Known services:\n")
	for alias, container := range knownServices {
		fmt.Fprintf(os.Stderr, "  %-15s -> %s\n", alias, container)
	}
	fmt.Fprint(os.Stderr, "\nOr use a model slug that has a container configured.\n")
	return "", fmt.Errorf("unknown service or model: %s", slug)
}

// resolveServiceAlias maps a service alias to a Docker container name.
func resolveServiceAlias(alias string) string {
	switch strings.ToLower(alias) {
	case "comfyui", "flux":
		return "comfyui-flux"
	case "embed":
		return "llm-embed"
	case "rerank":
		return "llm-rerank"
	case "whisper":
		return "whisper-stt"
	case "kokoro":
		return "kokoro-tts"
	case "litellm":
		return "litellm"
	case "swap-api", "swapapi":
		return "swap-api"
	case "open-webui", "webui":
		return "open-webui"
	case "mcp":
		return "mcpo"
	default:
		return ""
	}
}

// PrintHelp prints the logs command help.
func (c *LogsCommand) PrintHelp() {
	fmt.Println(`logs - View container logs for an LLM model or service.

USAGE:
  llm-manager logs <slug|service> [-f] [lines]

ARGUMENTS:
  slug      The model slug or service alias
  -f, --follow  Follow mode: stream logs in real-time (blocks until Ctrl+C)
  lines     Number of log lines to show (default: 50, only in non-follow mode)

SERVICE ALIASES:
  comfyui, flux   -> comfyui-flux
  embed           -> llm-embed
  rerank          -> llm-rerank
  whisper         -> whisper-stt
  kokoro          -> kokoro-tts
  litellm         -> litellm
  swap-api, swapapi -> swap-api
  open-webui, webui -> open-webui
  mcp             -> mcpo

EXAMPLES:
  llm-manager logs qwen3_6
  llm-manager logs qwen3_6 200
  llm-manager logs qwen3_6 -f
  llm-manager logs comfyui -f
  llm-manager logs embed 100`)
}
