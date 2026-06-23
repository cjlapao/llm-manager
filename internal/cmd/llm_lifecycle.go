package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/user/llm-manager/internal/service"
)

// ── start ──────────────────────────────────────────────────────────────────

// runStart starts a model container.
func (c *LlmCommand) runStart(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}

	// Resolve "latest" to the actual model slug
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

	allowMultiple := false
	dryRun := false
	wait := false
	overrides := service.StartOverrides{}
	// flags start after [0] if slug was passed, otherwise nothing
	flagArgs := args
	if len(args) > 0 { flagArgs = args[1:] }
	for _, arg := range flagArgs {
		switch arg {
		case "--dry-run", "-n":
			dryRun = true
		case "--allow-multiple", "-m":
			allowMultiple = true
		case "--wait", "-w":
			wait = true
		case "--max-model-len":
			// next arg is the value
		case "--max-num-seqs":
			// next arg is the value
		case "--max-num-batched-tokens":
			// next arg is the value
		case "--gpu-memory":
			// next arg is the value
		case "--speculative-decoding":
			// next arg is the value
		case "--speculative-tokens":
			// next arg is the value
		case "--speculative-model":
			// next arg is the value
		}
	}

	// Parse numeric overrides (simple positional: --flag value)
	startIdx := 1
	if len(args) < 2 { startIdx = 0 }
	for i := startIdx; i < len(args); i++ {
		switch args[i] {
		case "--max-model-len":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.MaxModelLen = val
				i++
			}
		case "--max-num-seqs":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.MaxNumSeqs = val
				i++
			}
		case "--max-num-batched-tokens":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.MaxNumBatchedTokens = val
				i++
			}
		case "--gpu-memory":
			if i+1 < len(args) {
				var val float64
				fmt.Sscanf(args[i+1], "%f", &val)
				overrides.GPUMemoryUtil = &val
				i++
			}
		case "--speculative-decoding":
			if i+1 < len(args) {
				val := args[i+1]
				overrides.SpeculativeDecoding = &val
				i++
			}
		case "--speculative-tokens":
			if i+1 < len(args) {
				var val int
				fmt.Sscanf(args[i+1], "%d", &val)
				overrides.NumSpeculativeTokens = &val
				i++
			}
		case "--speculative-model":
			if i+1 < len(args) {
				val := args[i+1]
				overrides.SpeculativeModel = &val
				i++
			}
		}
	}

	// Dry-run: only GPU memory check, no Docker modifications
	if dryRun {
		if err := c.svc.StartContainerDryRun(slug, overrides); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println("[dry-run] Dry run complete. No containers were modified.")
		return 0
	}

	// Normal LLM start
	if err := c.svc.StartContainer(slug, allowMultiple, overrides); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting container: %v\n", err)
		return 1
	}

	fmt.Printf("Started container: %s\n", slug)

	// Persist this model as the latest started model
	configSvc := service.NewConfigService(c.cfg.db)
	if err := configSvc.SetLatestModel(slug); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to set latest model: %v\n", err)
	}

	// Optionally wait for health check
	if wait {
		fmt.Println("Waiting for container to become healthy...")
		model, err := c.cfg.db.GetModel(slug)
		if err == nil && model.Port > 0 {
			host := "localhost"
			if c.cfg.cfg.OpenAIAPIURL != "" {
				if parsed, err := url.Parse(c.cfg.cfg.OpenAIAPIURL); err == nil && parsed.Host != "" {
					host = parsed.Host
				}
			}
			healthURL := fmt.Sprintf("http://%s:%d", host, model.Port)

			ctx, cancel := context.WithTimeout(context.Background(), DefaultStartTimeout)
			defer cancel()

			if err := waitForHealthy(ctx, healthURL); err != nil {
				fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
				fmt.Println("Stopping container...")
				stopErr := c.svc.StopContainer(slug)
				if stopErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to stop container after health check failure: %v\n", stopErr)
				}
				fmt.Fprintf(os.Stderr, "Error: container %s failed health check and was stopped\n", slug)
				return 1
			}
			fmt.Println("Container is healthy!")
		}
	}

	return 0
}
