package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// DefaultStartTimeout is the default timeout for waiting for a container to become healthy.
const DefaultStartTimeout = 180 * time.Second

const healthCheckInterval = 3 * time.Second
const healthCheckClientTimeout = 5 * time.Second

// waitForHealthy polls the /health endpoint of the given URL until it returns 200,
// the context deadline expires, or an error occurs.
// Returns nil on success, error on timeout or failure.
func waitForHealthy(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: healthCheckClientTimeout}
	deadline, ok := ctx.Deadline()
	if !ok {
		return fmt.Errorf("context must have a deadline")
	}

	// Log the timeout so users know the bound
	remaining := time.Until(deadline)
	_ = remaining // used below via the deadline check

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out after waiting for container to become healthy")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
			if err != nil {
				return fmt.Errorf("failed to create health check request: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				// Best-effort: continue polling on transient errors
				continue
			}

			if resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}

			resp.Body.Close()
			// Continue polling on non-200
		}
	}
}
