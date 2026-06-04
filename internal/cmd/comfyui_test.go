package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

// newTestComfyUICommand creates a minimal ComfyUICommand with an in-memory
// SQLite DB and a temp install directory for testing.
func newTestComfyUICommand(t *testing.T) (*ComfyUICommand, string) {
	t.Helper()
	tmpDir := t.TempDir()

	mgr, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := &config.Config{
		InstallDir: tmpDir,
	}

	root := &RootCommand{
		db:  mgr,
		cfg: cfg,
	}

	cmd := NewComfyUICommand(root)
	return cmd, tmpDir
}

// ── ComfyUICommand.Run subcommand tests ──────────────────────────────────────

func TestComfyUICommand_Run_Help(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"help"})
	if exitCode != 0 {
		t.Errorf("Run([help]) = %d, want 0", exitCode)
	}
}

func TestComfyUICommand_Run_HelpFlag(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			exitCode := cmd.Run([]string{flag})
			if exitCode != 0 {
				t.Errorf("Run([%s]) = %d, want 0", flag, exitCode)
			}
		})
	}
}

func TestComfyUICommand_Run_NoArgs(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("Run([]) = %d, want 0", exitCode)
	}
}

// ── Flux subcommand error paths ──────────────────────────────────────────────

func TestComfyUICommand_Run_Flux_MissingSubcommand(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux"})
	if exitCode != 1 {
		t.Errorf("Run([flux]) = %d, want 1", exitCode)
	}
}

func TestComfyUICommand_Run_Flux_Start_MissingSlug(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "start"})
	if exitCode != 1 {
		t.Errorf("Run([flux, start]) = %d, want 1", exitCode)
	}
}

func TestComfyUICommand_Run_Flux_Status_MissingSlug(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "status"})
	if exitCode != 1 {
		t.Errorf("Run([flux, status]) = %d, want 1", exitCode)
	}
}

func TestComfyUICommand_Run_Flux_UnknownSubcommand(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "bogus"})
	if exitCode != 1 {
		t.Errorf("Run([flux, bogus]) = %d, want 1", exitCode)
	}
}

// ── 3D subcommand error paths ────────────────────────────────────────────────

func TestComfyUICommand_Run_3D_MissingSubcommand(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d"})
	if exitCode != 1 {
		t.Errorf("Run([3d]) = %d, want 1", exitCode)
	}
}

func TestComfyUICommand_Run_3D_Start_MissingSlug(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "start"})
	if exitCode != 1 {
		t.Errorf("Run([3d, start]) = %d, want 1", exitCode)
	}
}

func TestComfyUICommand_Run_3D_Status_MissingSlug(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "status"})
	if exitCode != 1 {
		t.Errorf("Run([3d, status]) = %d, want 1", exitCode)
	}
}

func TestComfyUICommand_Run_3D_UnknownSubcommand(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "bogus"})
	if exitCode != 1 {
		t.Errorf("Run([3d, bogus]) = %d, want 1", exitCode)
	}
}

// ── Unknown top-level subcommand ─────────────────────────────────────────────

func TestComfyUICommand_Run_UnknownSubcommand(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"bogus"})
	if exitCode != 1 {
		t.Errorf("Run([bogus]) = %d, want 1", exitCode)
	}
}

// ── Flux subcommand integration tests (activate/deactivate model files) ──────

// TestComfyUICommand_Run_Flux_Start_ActivatesModel verifies flux start creates
// the .active-model file with the correct slug.
func TestComfyUICommand_Run_Flux_Start_ActivatesModel(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "start", "flux-schnell"})
	// Exit code is 1 because Docker isn't available, but activation should succeed
	if exitCode != 1 {
		t.Logf("Run([flux, start, flux-schnell]) = %d (expected 1 without Docker)", exitCode)
	}

	activePath := filepath.Join(tmpDir, "comfyui", ".active-model")
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("Failed to read active flux file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "flux-schnell" {
		t.Errorf("Active flux file = %q, want %q", strings.TrimSpace(string(data)), "flux-schnell")
	}
}

// TestComfyUICommand_Run_Flux_Stop_DeactivatesModel verifies flux stop removes
// the .active-model file.
func TestComfyUICommand_Run_Flux_Stop_DeactivatesModel(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "start", "flux-schnell"})
	_ = exitCode

	activePath := filepath.Join(tmpDir, "comfyui", ".active-model")
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		t.Fatal("flux start did not create active model file")
	}

	exitCode = cmd.Run([]string{"flux", "stop"})
	if exitCode != 0 {
		t.Errorf("Run([flux, stop]) = %d, want 0", exitCode)
	}

	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Error("flux stop did not remove active model file")
	}
}

// TestComfyUICommand_Run_Flux_Start_ActivatesModel_UnknownSlug verifies flux
// start creates the .active-model file even for unknown model slugs.
func TestComfyUICommand_Run_Flux_Start_ActivatesModel_UnknownSlug(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "start", "unknown-flux-model"})
	if exitCode != 1 {
		t.Logf("Run([flux, start, unknown]) = %d (expected 1 without Docker)", exitCode)
	}

	activePath := filepath.Join(tmpDir, "comfyui", ".active-model")
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("Failed to read active flux file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "unknown-flux-model" {
		t.Errorf("Active flux file = %q, want %q", strings.TrimSpace(string(data)), "unknown-flux-model")
	}
}

// TestComfyUICommand_Run_Flux_Start_AllowMultiple verifies --allow-multiple
// flag is accepted without error.
func TestComfyUICommand_Run_Flux_Start_AllowMultiple(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "start", "flux-schnell", "--allow-multiple"})
	if exitCode != 1 {
		t.Logf("Run([flux, start, flux-schnell, --allow-multiple]) = %d (expected 1 without Docker)", exitCode)
	}

	activePath := filepath.Join(tmpDir, "comfyui", ".active-model")
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("Failed to read active flux file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "flux-schnell" {
		t.Errorf("Active flux file = %q, want %q", strings.TrimSpace(string(data)), "flux-schnell")
	}
}

// TestComfyUICommand_Run_Flux_Start_AllowMultipleShort verifies -m flag is
// accepted without error.
func TestComfyUICommand_Run_Flux_Start_AllowMultipleShort(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "start", "flux-schnell", "-m"})
	if exitCode != 1 {
		t.Logf("Run([flux, start, flux-schnell, -m]) = %d (expected 1 without Docker)", exitCode)
	}

	activePath := filepath.Join(tmpDir, "comfyui", ".active-model")
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("Failed to read active flux file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "flux-schnell" {
		t.Errorf("Active flux file = %q, want %q", strings.TrimSpace(string(data)), "flux-schnell")
	}
}

// TestComfyUICommand_Run_Flux_Status_NoComfyUI verifies flux status works
// when ComfyUI is not running (no Docker).
func TestComfyUICommand_Run_Flux_Status_NoComfyUI(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"flux", "status", "flux-schnell"})
	if exitCode != 0 {
		t.Errorf("Run([flux, status, flux-schnell]) = %d, want 0", exitCode)
	}
}

// ── 3D subcommand integration tests (activate/deactivate model files) ────────

// TestComfyUICommand_Run_3D_Start_ActivatesModel verifies 3d start creates
// the .active-3d file with the correct slug.
func TestComfyUICommand_Run_3D_Start_ActivatesModel(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "start", "hunyuan3d"})
	if exitCode != 1 {
		t.Logf("Run([3d, start, hunyuan3d]) = %d (expected 1 without Docker)", exitCode)
	}

	activePath := filepath.Join(tmpDir, "comfyui", ".active-3d")
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("Failed to read active 3d file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hunyuan3d" {
		t.Errorf("Active 3d file = %q, want %q", strings.TrimSpace(string(data)), "hunyuan3d")
	}
}

// TestComfyUICommand_Run_3D_Stop_DeactivatesModel verifies 3d stop removes
// the .active-3d file.
func TestComfyUICommand_Run_3D_Stop_DeactivatesModel(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "start", "hunyuan3d"})
	_ = exitCode

	activePath := filepath.Join(tmpDir, "comfyui", ".active-3d")
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		t.Fatal("3d start did not create active 3d file")
	}

	exitCode = cmd.Run([]string{"3d", "stop"})
	if exitCode != 0 {
		t.Errorf("Run([3d, stop]) = %d, want 0", exitCode)
	}

	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Error("3d stop did not remove active 3d file")
	}
}

// TestComfyUICommand_Run_3D_Start_AllowMultiple verifies --allow-multiple
// flag is accepted without error for 3d start.
func TestComfyUICommand_Run_3D_Start_AllowMultiple(t *testing.T) {
	cmd, tmpDir := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "start", "hunyuan3d", "--allow-multiple"})
	if exitCode != 1 {
		t.Logf("Run([3d, start, hunyuan3d, --allow-multiple]) = %d (expected 1 without Docker)", exitCode)
	}

	activePath := filepath.Join(tmpDir, "comfyui", ".active-3d")
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("Failed to read active 3d file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hunyuan3d" {
		t.Errorf("Active 3d file = %q, want %q", strings.TrimSpace(string(data)), "hunyuan3d")
	}
}

// TestComfyUICommand_Run_3D_Status_NoComfyUI verifies 3d status works
// when ComfyUI is not running (no Docker).
func TestComfyUICommand_Run_3D_Status_NoComfyUI(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"3d", "status", "hunyuan3d"})
	if exitCode != 0 {
		t.Errorf("Run([3d, status, hunyuan3d]) = %d, want 0", exitCode)
	}
}

// ── Status subcommand ──────────────────────────────────────────────────────

// TestComfyUICommand_Run_Status_NoDocker verifies the status subcommand
// works when Docker is not available.
func TestComfyUICommand_Run_Status_NoDocker(t *testing.T) {
	cmd, _ := newTestComfyUICommand(t)

	exitCode := cmd.Run([]string{"status"})
	if exitCode != 0 {
		t.Errorf("Run([status]) = %d, want 0", exitCode)
	}
}
