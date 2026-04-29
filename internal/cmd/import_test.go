package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

func TestImportCommand_Help(t *testing.T) {
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
	cmd := NewImportCommand(root)

	exitCode := cmd.Run([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("import help returned non-zero exit code: %d", exitCode)
	}
}

func TestImportCommand_NoArgs(t *testing.T) {
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
	cmd := NewImportCommand(root)

	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("import with no args should return 0")
	}
}

func TestImportCommand_InvalidFile(t *testing.T) {
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
	cmd := NewImportCommand(root)

	exitCode := cmd.Run([]string{"/nonexistent/file.yaml"})
	if exitCode == 0 {
		t.Error("import with nonexistent file should return non-zero")
	}
}

func TestImportCommand_ValidYAML(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewImportCommand(root)

	tmpDir := t.TempDir()
	yamlContent := `slug: test-import-model
name: "Test Import Model"
engine: vllm
hf_repo: "test/import-model"
container: test-import-container
port: 8080

environment:
  VLLM_HOST: "0.0.0.0"

command:
  - "--model test/import-model"
  - "-max-model-len 8192"

input_token_cost: 0.0000003
output_token_cost: 0.0000004

capabilities:
  - reasoning
  - tool-use
`
	yamlPath := filepath.Join(tmpDir, "import.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	exitCode := cmd.Run([]string{yamlPath})
	if exitCode != 0 {
		t.Errorf("import valid YAML returned non-zero exit code: %d", exitCode)
	}

	// Verify model was created
	model, err := db.GetModel("test-import-model")
	if err != nil {
		t.Fatalf("GetModel() error: %v", err)
	}
	if model.Name != "Test Import Model" {
		t.Errorf("Name = %q, want %q", model.Name, "Test Import Model")
	}
	if model.EngineType != "vllm" {
		t.Errorf("EngineType = %q, want %q", model.EngineType, "vllm")
	}
	if model.InputTokenCost != 0.0000003 {
		t.Errorf("InputTokenCost = %v, want %v", model.InputTokenCost, 0.0000003)
	}
	if model.OutputTokenCost != 0.0000004 {
		t.Errorf("OutputTokenCost = %v, want %v", model.OutputTokenCost, 0.0000004)
	}
}

func TestImportCommand_DuplicateSlug(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}

	// Create existing model
	existing := &models.Model{
		Slug:    "existing-model",
		Type:    "llm",
		Name:    "Existing Model",
		Port:    8000,
		Default: false,
	}
	if err := db.CreateModel(existing); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewImportCommand(root)

	tmpDir := t.TempDir()
	yamlContent := `slug: existing-model
name: "Duplicate Model"
engine: vllm
port: 8080
`
	yamlPath := filepath.Join(tmpDir, "duplicate.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	exitCode := cmd.Run([]string{yamlPath})
	if exitCode == 0 {
		t.Error("import duplicate slug should return non-zero")
	}
}

func TestImportCommand_InvalidYAML(t *testing.T) {
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
	cmd := NewImportCommand(root)

	tmpDir := t.TempDir()
	yamlContent := `name: "Missing Slug"
engine: vllm
port: 8080
`
	yamlPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	exitCode := cmd.Run([]string{yamlPath})
	if exitCode == 0 {
		t.Error("import invalid YAML (missing slug) should return non-zero")
	}
}

func TestImportCommand_WithOverrides(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewImportCommand(root)

	tmpDir := t.TempDir()
	yamlContent := `slug: override-model
name: "Override Model"
engine: vllm
port: 8080

input_token_cost: 0.0000001
output_token_cost: 0.0000001

capabilities:
  - from-yaml
`
	yamlPath := filepath.Join(tmpDir, "override.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	exitCode := cmd.Run([]string{
		yamlPath,
		"--input-cost=0.0000099",
		"--output-token-cost=0.00000088",
		"--capabilities=override1,override2",
	})
	if exitCode != 0 {
		t.Errorf("import with overrides returned non-zero: %d", exitCode)
	}

	model, err := db.GetModel("override-model")
	if err != nil {
		t.Fatalf("GetModel() error: %v", err)
	}
	if model.InputTokenCost != 0.0000099 {
		t.Errorf("InputTokenCost = %v, want 0.0000099 (overridden)", model.InputTokenCost)
	}
	if model.OutputTokenCost != 0.00000088 {
		t.Errorf("OutputTokenCost = %v, want 0.00000088 (overridden)", model.OutputTokenCost)
	}
}

func TestImportCommand_InvalidFlagValue(t *testing.T) {
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
	cmd := NewImportCommand(root)

	tmpDir := t.TempDir()
	yamlContent := `slug: valid-model
name: "Valid"
engine: vllm
port: 8080
`
	yamlPath := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	exitCode := cmd.Run([]string{yamlPath, "--input-cost=not-a-number"})
	if exitCode == 0 {
		t.Error("import with invalid cost should return non-zero")
	}
}
