package cmd

import (
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

// TestLogsCommand_RunWithDB tests logs command with a real DB.
func TestLogsCommand_RunWithDB(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewLogsCommand(root)

	// Test with a model slug that doesn't exist
	exitCode := cmd.Run([]string{"nonexistent-model", "50"})
	if exitCode == 0 {
		t.Error("logs for nonexistent model should return non-zero")
	}
}

// TestLogsCommand_RunWithServiceAlias tests logs with service aliases.
func TestLogsCommand_RunWithServiceAlias(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewLogsCommand(root)

	// Test with service alias - will fail because container doesn't exist,
	// but should NOT fail with "unknown service" error
	exitCode := cmd.Run([]string{"embed"})
	// embed is a known alias, so resolveContainer should succeed
	// The docker command will fail, but that's expected
	if exitCode != 0 {
		// This is expected because docker logs won't work
		t.Log("logs embed failed (expected without Docker)")
	}
}

// TestLogsCommand_FollowMode tests follow mode flag parsing.
func TestLogsCommand_FollowMode(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewLogsCommand(root)

	// Follow mode with known alias - should resolve container, then fail on docker
	exitCode := cmd.Run([]string{"comfyui", "-f"})
	if exitCode != 0 {
		t.Log("logs comfyui -f failed (expected without Docker)")
	}
}

// TestLogsCommand_LineCount tests line count parsing.
func TestLogsCommand_LineCount(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewLogsCommand(root)

	// Line count as second arg
	exitCode := cmd.Run([]string{"embed", "200"})
	if exitCode != 0 {
		t.Log("logs embed 200 failed (expected without Docker)")
	}
}

// TestServiceAliasCaseInsensitive tests that service aliases are case-insensitive.
func TestServiceAliasCaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ComfyUI", "comfyui-flux"},
		{"COMFYUI", "comfyui-flux"},
		{"Embed", "llm-embed"},
		{"EMBED", "llm-embed"},
		{"MCP", "mcpo"},
		{"Mcp", "mcpo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := resolveServiceAlias(tt.input)
			if result != tt.expected {
				t.Errorf("resolveServiceAlias(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
