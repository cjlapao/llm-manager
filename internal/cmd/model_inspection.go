package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// inspectContainerState queries a Docker container's live state.
func inspectContainerState(containerName string) string {
	if containerName == "" {
		return "unset"
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	status := strings.TrimSpace(string(output))
	if status == "exited" {
		codeCmd := exec.Command("docker", "inspect", "-f", "{{.State.ExitCode}}", containerName)
		codeOutput, codeErr := codeCmd.Output()
		if codeErr == nil {
			code, _ := strconv.Atoi(strings.TrimSpace(string(codeOutput)))
			if code != 0 {
				return fmt.Sprintf("error (%d)", code)
			}
		}
		return "stopped"
	}
	return status
}

// runInfo displays model information organized into grouped sections.
func (c *ModelCommand) runInfo(slug string) int {
	model, err := c.svc.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model: %v\n", err)
		return 1
	}

	pad := strings.Repeat("-", 60)
	slab := func(indent int) string { return strings.Repeat("  ", indent) }

	fmt.Printf("Model: %s (%s)\n", model.Slug, model.Name)
	fmt.Println(pad)

	// base properties
	hasBase := model.Slug != "" || model.HFRepo != "" || model.Port > 0
	if hasBase {
		fmt.Println("\nbase properties:")
		fmt.Printf("%-24s%s\n", "slug:", model.Slug)
		fmt.Printf("%-24s%s\n", "name:", model.Name)
		fmt.Printf("%-24s%s\n", "type:", model.Type)
		if model.SubType != "" {
			fmt.Printf("%-24s%s\n", "subtype:", model.SubType)
		}
		if model.EngineType != "" {
			fmt.Printf("%-24s%s\n", "engine:", model.EngineType)
		}
		if model.HFRepo != "" {
			fmt.Printf("%-24s%s\n", "hf_repo:", model.HFRepo)
		}
		fmt.Printf("%-24s%d\n", "port:", model.Port)
		if model.InputTokenCost > 0 && model.OutputTokenCost > 0 {
			fmt.Printf("%-24s%.8f / %.8f\n", "cost:", model.InputTokenCost, model.OutputTokenCost)
		} else if model.InputTokenCost > 0 {
			fmt.Printf("%-24sinput:   %.8f\n", "", model.InputTokenCost)
		} else if model.OutputTokenCost > 0 {
			fmt.Printf("%-24soutput:  %.8f\n", "", model.OutputTokenCost)
		}

		caps := []string{}
		json.Unmarshal([]byte(model.Capabilities), &caps)
		if len(caps) > 0 {
			fmt.Printf("%-24s%+v\n", "capabilities:", strings.Join(caps, ", "))
		}
	}

	// docker
	hasDocker := model.Container != "" || model.EnvVars != "" || model.CommandArgs != ""
	if hasDocker {
		fmt.Println("\ndocker:")
		if model.Container != "" {
			fmt.Printf("%s%-20s%s\n", "  ", "container:", model.Container)
		}
		if model.EnvVars != "" {
			var envVars map[string]string
			json.Unmarshal([]byte(model.EnvVars), &envVars)
			if len(envVars) > 0 {
				fmt.Printf("%senvironment:\n", slab(1))
				for k, v := range envVars {
					fmt.Printf("  %s%s=%s\n", slab(2), k, v)
				}
			}
		}
		if model.CommandArgs != "" {
			var cmdArgs []string
			json.Unmarshal([]byte(model.CommandArgs), &cmdArgs)
			if len(cmdArgs) > 0 {
				fmt.Printf("%scommand:\n", slab(1))
				for _, arg := range cmdArgs {
					fmt.Printf("    - %s\n", arg)
				}
			}
		}
	}

	// profile / memory
	hasProfile := model.TotalParamsB != nil || model.ActiveParamsB != nil || model.QuantBytesPerParam != nil
	if hasProfile {
		fmt.Println("\nprofile:")
		if model.TotalParamsB != nil {
			fmt.Printf("%-24s%.1fB\n", "total_params:", *model.TotalParamsB)
		}
		if model.ActiveParamsB != nil {
			fmt.Printf("%-24s%.1fB\n", "active_params:", *model.ActiveParamsB)
		}
		if model.IsMoe != nil {
			fmt.Printf("%-24s%v\n", "is_moe:", *model.IsMoe)
		}
		if model.AttentionLayers != nil {
			fmt.Printf("%-24s%d\n", "attention_layers:", *model.AttentionLayers)
		}
		if model.GdnLayers != nil {
			fmt.Printf("%-24s%d\n", "gdn_layers:", *model.GdnLayers)
		}
		if model.NumKvHeads != nil {
			fmt.Printf("%-24s%d\n", "num_kv_heads:", *model.NumKvHeads)
		}
		if model.HeadDim != nil {
			fmt.Printf("%-24s%d\n", "head_dim:", *model.HeadDim)
		}
		if model.SupportsMtp != nil {
			fmt.Printf("%-24s%v\n", "supports_mtp:", *model.SupportsMtp)
		}
		if model.DefaultContext != nil {
			fmt.Printf("%-24s%d\n", "default_context:", *model.DefaultContext)
		}
		if model.MaxContext != nil {
			fmt.Printf("%-24s%d\n", "max_context:", *model.MaxContext)
		}
		if model.QuantBytesPerParam != nil {
			fmt.Printf("%-24s%.1f\n", "quant_bytes_per_param:", *model.QuantBytesPerParam)
		}
	}

	// litellm
	hasLiteLLM := model.LiteLLMParams != "" || model.ModelInfo != ""
	if hasLiteLLM {
		fmt.Println("\nlitellm:")
		if model.LiteLLMParams != "" {
			var litellmParams map[string]interface{}
			if err := json.Unmarshal([]byte(model.LiteLLMParams), &litellmParams); err == nil {
				fmt.Println("  litellm_params:")
				printNestedMap(litellmParams, "    ")
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse litellm_params: %v\n", err)
			}
		}
		if model.ModelInfo != "" {
			var modelInfo map[string]interface{}
			if err := json.Unmarshal([]byte(model.ModelInfo), &modelInfo); err == nil {
				fmt.Println("  model_info:")
				printNestedMap(modelInfo, "    ")
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse model_info: %v\n", err)
			}
		}
	}

	// No data at all
	if !hasBase && !hasDocker && !hasProfile && !hasLiteLLM {
		fmt.Println("\nno model information available.")
	}

	fmt.Println()
	return 0
}

// runClearCache removes the entire HF cache directory for a model.
func (c *ModelCommand) runClearCache(slug string) int {
	model, err := c.svc.GetModel(slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: model %s not found: %v\n", slug, err)
		return 1
	}

	if model.HFRepo == "" {
		fmt.Fprintf(os.Stderr, "Error: model %s has no HF repo configured\n", slug)
		return 1
	}

	cacheDir := convertRepoToCacheDir(model.HFRepo)
	cachePaths := []string{
		filepath.Join(c.cfg.cfg.HFCacheDir, "hub", cacheDir),
		filepath.Join(c.cfg.cfg.HFCacheDir, cacheDir),
	}

	var deletedPaths []string
	for _, dir := range cachePaths {
		if _, statErr := os.Stat(dir); statErr == nil {
			fileCount, dirSize := countDirFiles(dir)

			fmt.Printf("Removing cache for %s (%s):\n", slug, model.HFRepo)
			fmt.Printf("  Path: %s\n", dir)
			fmt.Printf("  Files: %d (%s)\n", fileCount, formatSize(dirSize))

			if rmErr := os.RemoveAll(dir); rmErr != nil {
				fmt.Fprintf(os.Stderr, "  Error: failed to remove %s: %v\n", dir, rmErr)
				continue
			}

			deletedPaths = append(deletedPaths, dir)
			fmt.Printf("  ✓ Removed\n")
		}
	}

	if len(deletedPaths) == 0 {
		fmt.Printf("No cache found for %s (%s)\n", slug, model.HFRepo)
		return 0
	}

	fmt.Printf("\nCache cleared for %d path(s)\n", len(deletedPaths))
	return 0
}
