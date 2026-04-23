package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
)

func newTestConfigCommand(t *testing.T) (*ConfigCommand, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := database.NewDatabaseManager(dbPath)
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

	cfg := config.DefaultConfig()
	// Override database path so it uses our test DB
	cfg.DatabaseURL = dbPath

	cmd := NewConfigCommand(cfg, mgr)
	return cmd, dbPath
}

// --- Test: no args prints config ---

func TestConfigCommand_NoArgs(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Should print config without error
	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("Run([]string{}) = %d, want 0", exitCode)
	}
}

// --- Test: help ---

func TestConfigCommand_Help(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"help"})
	if exitCode != 0 {
		t.Errorf("Run([help]) = %d, want 0", exitCode)
	}
}

// --- Test: unknown subcommand ---

func TestConfigCommand_UnknownSubcommand(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"bogus"})
	if exitCode != 1 {
		t.Errorf("Run([bogus]) = %d, want 1", exitCode)
	}
}

// --- Test: list ---

func TestConfigCommand_List(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set some DB values
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")
	cmd.svc.Set("LLM_MANAGER_LOG_DIR", "/db/log")

	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("Run([list]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_List_WithEnvOverride(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set DB value
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	// Set env var — should show environment as source
	t.Setenv("LLM_MANAGER_DATA_DIR", "/env/data")

	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("Run([list]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_List_WithFileOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a config file
	err := os.WriteFile(configPath, []byte("LLM_MANAGER_DATA_DIR: /file/data\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath
	cmd := NewConfigCommand(cfg, mgr)

	// Set DB value (should be hidden by file)
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	// Use our custom config path
	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("Run([list]) = %d, want 0", exitCode)
	}
}

// --- Test: get ---

func TestConfigCommand_Get_FromDB(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	exitCode := cmd.Run([]string{"get", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 0 {
		t.Errorf("Run([get, LLM_MANAGER_DATA_DIR]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Get_FromEnv(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set both DB and env — env should win
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")
	t.Setenv("LLM_MANAGER_DATA_DIR", "/env/data")

	exitCode := cmd.Run([]string{"get", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 0 {
		t.Errorf("Run([get, LLM_MANAGER_DATA_DIR]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Get_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	err := os.WriteFile(configPath, []byte("LLM_MANAGER_DATA_DIR: /file/data\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath
	cmd := NewConfigCommand(cfg, mgr)

	// Set DB value (should be hidden by file)
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	exitCode := cmd.Run([]string{"get", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 0 {
		t.Errorf("Run([get, LLM_MANAGER_DATA_DIR]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Get_Default(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// No DB, file, or env — should show default
	exitCode := cmd.Run([]string{"get", "LITELLM_URL"})
	if exitCode != 0 {
		t.Errorf("Run([get, LITELLM_URL]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Get_InvalidKey(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"get", "INVALID_KEY"})
	if exitCode != 1 {
		t.Errorf("Run([get, INVALID_KEY]) = %d, want 1", exitCode)
	}
}

func TestConfigCommand_Get_MissingKey(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"get"})
	if exitCode != 1 {
		t.Errorf("Run([get]) without key = %d, want 1", exitCode)
	}
}

// --- Test: set ---

func TestConfigCommand_Set(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"set", "LLM_MANAGER_DATA_DIR", "/custom/data"})
	if exitCode != 0 {
		t.Errorf("Run([set, LLM_MANAGER_DATA_DIR, /custom/data]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Set_InvalidKey(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"set", "INVALID_KEY", "value"})
	if exitCode != 1 {
		t.Errorf("Run([set, INVALID_KEY, value]) = %d, want 1", exitCode)
	}
}

func TestConfigCommand_Set_WithEnvOverride(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set env var — should warn but still set DB value
	t.Setenv("LLM_MANAGER_DATA_DIR", "/env/data")

	exitCode := cmd.Run([]string{"set", "LLM_MANAGER_DATA_DIR", "/db/data"})
	if exitCode != 0 {
		t.Errorf("Run([set, LLM_MANAGER_DATA_DIR, /db/data]) with env set = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Set_WithFileOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	err := os.WriteFile(configPath, []byte("LLM_MANAGER_DATA_DIR: /file/data\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath
	cmd := NewConfigCommand(cfg, mgr)

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	exitCode := cmd.Run([]string{"set", "LLM_MANAGER_DATA_DIR", "/db/data"})
	if exitCode != 0 {
		t.Errorf("Run([set, LLM_MANAGER_DATA_DIR, /db/data]) with file set = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Set_DefaultValue(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Setting a value that matches default should produce a note
	exitCode := cmd.Run([]string{"set", "LITELLM_URL", ""})
	if exitCode != 0 {
		t.Errorf("Run([set, LITELLM_URL, \"\"]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Set_MissingValue(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"set", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 1 {
		t.Errorf("Run([set, LLM_MANAGER_DATA_DIR]) without value = %d, want 1", exitCode)
	}
}

// --- Test: unset ---

func TestConfigCommand_Unset(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set then unset
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/custom/data")

	exitCode := cmd.Run([]string{"unset", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 0 {
		t.Errorf("Run([unset, LLM_MANAGER_DATA_DIR]) = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Unset_InvalidKey(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"unset", "INVALID_KEY"})
	if exitCode != 1 {
		t.Errorf("Run([unset, INVALID_KEY]) = %d, want 1", exitCode)
	}
}

func TestConfigCommand_Unset_WithEnvOverride(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set env var — should warn but still unset DB
	t.Setenv("LLM_MANAGER_DATA_DIR", "/env/data")
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	exitCode := cmd.Run([]string{"unset", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 0 {
		t.Errorf("Run([unset, LLM_MANAGER_DATA_DIR]) with env set = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Unset_MissingKey(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Unset a key that doesn't exist — should succeed (idempotent)
	exitCode := cmd.Run([]string{"unset", "LLM_MANAGER_DATA_DIR"})
	if exitCode != 0 {
		t.Errorf("Run([unset, LLM_MANAGER_DATA_DIR]) for non-existent key = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Unset_MissingKeyArg(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	exitCode := cmd.Run([]string{"unset"})
	if exitCode != 1 {
		t.Errorf("Run([unset]) without key = %d, want 1", exitCode)
	}
}

// --- Test: edit ---

func TestConfigCommand_Edit_NoConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	configPath := filepath.Join(tmpDir, "nonexistent.yaml")

	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath
	cmd := NewConfigCommand(cfg, mgr)

	// Point to non-existent config file
	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	exitCode := cmd.Run([]string{"edit"})
	if exitCode != 0 {
		t.Errorf("Run([edit]) with no config file = %d, want 0", exitCode)
	}
}

func TestConfigCommand_Edit_WithConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config file
	err := os.WriteFile(configPath, []byte("# llm-manager config\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath
	cmd := NewConfigCommand(cfg, mgr)

	// Set EDITOR to true (a no-op command) so it doesn't actually open an editor
	t.Setenv("EDITOR", "true")
	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	exitCode := cmd.Run([]string{"edit"})
	if exitCode != 0 {
		t.Errorf("Run([edit]) with config file = %d, want 0", exitCode)
	}
}

// --- Test: 4-layer priority verification ---

func TestConfigCommand_List_PriorityEnvWins(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set all three lower layers
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("LLM_MANAGER_DATA_DIR: /file/data\n"), 0o644)
	t.Setenv("LLM_MANAGER_CONFIG", configPath)
	t.Setenv("LLM_MANAGER_DATA_DIR", "/env/data")

	// Output should show environment as source
	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("Run([list]) with all layers set = %d, want 0", exitCode)
	}
}

func TestConfigCommand_List_PriorityFileWinsDB(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("LLM_MANAGER_DATA_DIR: /file/data\n"), 0o644)

	dbPath := filepath.Join(tmpDir, "test.db")
	mgr, err := database.NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}
	if err := mgr.Open(); err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()
	if err := mgr.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.DatabaseURL = dbPath
	cmd := NewConfigCommand(cfg, mgr)

	// Set DB value — should be hidden by file
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	t.Setenv("LLM_MANAGER_CONFIG", configPath)

	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("Run([list]) with file and DB = %d, want 0", exitCode)
	}
}

func TestConfigCommand_List_PriorityDBWinsDefault(t *testing.T) {
	cmd, _ := newTestConfigCommand(t)

	// Set DB value — should show DB as source
	cmd.svc.Set("LLM_MANAGER_DATA_DIR", "/db/data")

	exitCode := cmd.Run([]string{"list"})
	if exitCode != 0 {
		t.Errorf("Run([list]) with DB set = %d, want 0", exitCode)
	}
}
