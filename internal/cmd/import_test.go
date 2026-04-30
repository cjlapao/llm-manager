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

	exitCode := cmd.Run([]string{"-h"})
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

	// Create existing model so --override has something to work on
	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewImportCommand(root)

	tmpDir := t.TempDir()
	yamlContent := `slug: override-model
name: "Override Model"
type: llm
engine: vllm
container: override-container
port: 8080
hf_repo: "test/override-model"

input_token_cost: 0.0000001
output_token_cost: 0.0000001

capabilities:
  - tool-use
`
	yamlPath := filepath.Join(tmpDir, "override.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	// First import without override to create the model
	exitCode := cmd.Run([]string{yamlPath})
	if exitCode != 0 {
		t.Fatalf("first import failed: %d", exitCode)
	}

	// Then re-import with --override to replace it
	yamlContent2 := `slug: override-model
name: "Override Model Updated"
type: llm
engine: vllm
container: override-container-v2
port: 8081
hf_repo: "test/override-model-v2"

input_token_cost: 0.0000002
output_token_cost: 0.0000003

capabilities:
  - tool-use
`
	yamlPath2 := filepath.Join(tmpDir, "override2.yaml")
	if err := os.WriteFile(yamlPath2, []byte(yamlContent2), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	exitCode = cmd.Run([]string{yamlPath2, "--override"})
	if exitCode != 0 {
		t.Errorf("import with override returned non-zero: %d", exitCode)
	}

	model, err := db.GetModel("override-model")
	if err != nil {
		t.Fatalf("GetModel() error: %v", err)
	}
	if model.Name != "Override Model Updated" {
		t.Errorf("Name = %q, want %q", model.Name, "Override Model Updated")
	}
	if model.Port != 8081 {
		t.Errorf("Port = %d, want %d", model.Port, 8081)
	}
	if model.InputTokenCost != 0.0000002 {
		t.Errorf("InputTokenCost = %v, want %v", model.InputTokenCost, 0.0000002)
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

	exitCode := cmd.Run([]string{yamlPath, "--unknown-flag"})
	if exitCode == 0 {
		t.Error("import with unknown flag should return non-zero")
	}
}
