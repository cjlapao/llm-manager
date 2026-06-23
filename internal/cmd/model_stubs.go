package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// knownFluxModels returns the list of known flux model slugs.
func knownFluxModels() []string {
	return []string{"flux-schnell", "flux-dev"}
}

// isFluxModel checks if a slug is a known flux model.
func isFluxModel(slug string) bool {
	for _, m := range knownFluxModels() {
		if slug == m {
			return true
		}
	}
	return false
}

// known3DModels returns the list of known 3D model slugs.
func known3DModels() []string {
	return []string{"hunyuan3d", "trellis"}
}

// is3DModel checks if a slug is a known 3D model.
func is3DModel(slug string) bool {
	for _, m := range known3DModels() {
		if slug == m {
			return true
		}
	}
	return false
}

// fluxCheckpoint returns the checkpoint filename for a flux model.
func fluxCheckpoint(slug string) string {
	switch slug {
	case "flux-schnell":
		return "flux1-schnell.safetensors"
	case "flux-dev":
		return "flux1-dev.safetensors"
	}
	return ""
}

// dirFor3DModel returns the directory name for a 3D model.
func dirFor3DModel(slug string) string {
	switch slug {
	case "hunyuan3d":
		return "hunyuan3d"
	case "trellis":
		return "trellis"
	}
	return ""
}

// readActiveFile reads the content of an active model file.
func readActiveFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeActiveFile writes content to an active model file.
func writeActiveFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// countDirFiles recursively counts files and sums total size under a directory.
func countDirFiles(root string) (int64, int64) {
	var files int64
	var size int64
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			files++
			size += info.Size()
		}
		return nil
	})
	return files, size
}

// formatSize formats a byte count as human-readable.
func formatSize(n int64) string {
	const (
		_  = iota
		KB = 1 << (10 * iota)
		MB
		GB
		TB
	)
	switch {
	case n >= TB:
		return fmt.Sprintf("%.1fTB", float64(n)/TB)
	case n >= GB:
		return fmt.Sprintf("%.1fGB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1fMB", float64(n)/MB)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// convertRepoToCacheDir converts an HuggingFace repo name to cache directory name.
func convertRepoToCacheDir(repo string) string {
	return "models--" + strings.ReplaceAll(repo, "/", "--")
}

// PrintHelp prints the model command help.
func (c *ModelCommand) PrintHelp() {
	fmt.Println(`model - Manage LLM models in the registry.

USAGE:
  llm-manager model [SUBCOMMAND] [ARGS]

SUBCOMMANDS:
  list, ls             List all models (with live STATUS, CACHED, and ENGINE columns)
  get <slug>           Show details for a model
  info <slug>          Show LiteLLM model information
  create <slug>        Create a new model
                       USAGE: llm-manager model create <slug> [type] [name] [port]
  update <slug>        Update model fields
                       USAGE: llm-manager model update <slug> [key=value ...]
                       Available fields: name, type, hf_repo, yml, container, port,
                         engine_type, env_vars, command_args, input_token_cost,
                         output_token_cost, capabilities
  delete, del [--all]  Delete a model or all models
                       USAGE: llm-manager model delete <slug>
                             llm-manager model delete --all
  import <file.yaml>   Import a model from a YAML file
                       OPTIONS: --input-cost, --output-cost, --capabilities
  export <slug>        Export a model to a YAML file
                       OPTIONS: --output <file.yaml>
  compose <slug>       Generate docker-compose configuration
  clear-cache <slug>   Remove cached model weights

EXAMPLES:
  llm-manager model list
  llm-manager model get qwen-3-8b
  llm-manager model info qwen-3-8b
  llm-manager model create my-model llm "My Model" 8080
  llm-manager model update qwen-3-8b name="Updated Name"
  llm-manager model delete old-model
  llm-manager model delete --all
  llm-manager model import model.yaml --input-cost 0.000001
  llm-manager model export qwen-3-8b --output backup.yaml
  llm-manager model clear-cache qwen-3-8b`)
}

// printNestedMap recursively prints a map with indentation.
func printNestedMap(m map[string]interface{}, indent string) {
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Printf("%s%s:\n", indent, k)
			printNestedMap(val, indent+"  ")
		case []interface{}:
			fmt.Printf("%s%s:\n", indent, k)
			for _, item := range val {
				fmt.Printf("%s  - %v\n", indent, item)
			}
		default:
			fmt.Printf("%s%s: %v\n", indent, k, v)
		}
	}
}
