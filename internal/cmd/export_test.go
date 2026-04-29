package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

func TestExportCommand_Help(t *testing.T) {
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
	cmd := NewExportCommand(root)

	exitCode := cmd.Run([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("export help returned non-zero exit code: %d", exitCode)
	}
}

func TestExportCommand_NoArgs(t *testing.T) {
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
	cmd := NewExportCommand(root)

	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("export with no args should return 0")
	}
}

func TestExportCommand_MissingModel(t *testing.T) {
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
	cmd := NewExportCommand(root)

	exitCode := cmd.Run([]string{"nonexistent-model"})
	if exitCode == 0 {
		t.Error("export nonexistent model should return non-zero")
	}
}

func TestExportCommand_ExistingModel(t *testing.T) {
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

	// Create a test model with new fields
	envVarsJSON := `{"HUGGING_FACE_HUB_TOKEN":"${HF_TOKEN}","VLLM_HOST":"0.0.0.0"}`
	cmdArgsJSON := `["--model","test/model","-max-model-len","8192","-kv-cache-dtype","fp8"]`
	capsJSON := `["reasoning","tool-use"]`

	model := &models.Model{
		Slug:            "export-test",
		Type:            "llm",
		Name:            "Export Test Model",
		HFRepo:          "test/model",
		Container:       "export-test-container",
		Port:            8080,
		EngineType:      "vllm",
		EnvVars:         envVarsJSON,
		CommandArgs:     cmdArgsJSON,
		InputTokenCost:  0.0000003,
		OutputTokenCost: 0.0000004,
		Capabilities:    capsJSON,
		Default:         false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewExportCommand(root)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "exported.yaml")

	exitCode := cmd.Run([]string{"export-test", "--output=" + outputPath})
	if exitCode != 0 {
		t.Errorf("export existing model returned non-zero: %d", exitCode)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("exported file was not created")
	}

	// Verify content
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "slug: export-test") {
		t.Error("exported YAML missing slug")
	}
	if !strings.Contains(content, "name: Export Test Model") {
		t.Error("exported YAML missing name")
	}
	if !strings.Contains(content, "engine: vllm") {
		t.Error("exported YAML missing engine")
	}
	if !strings.Contains(content, "port: 8080") {
		t.Error("exported YAML missing port")
	}
	if !strings.Contains(content, "HUGGING_FACE_HUB_TOKEN") {
		t.Error("exported YAML missing environment variables")
	}
	if !strings.Contains(content, "reasoning") {
		t.Error("exported YAML missing capabilities")
	}
}

func TestExportCommand_DefaultOutputPath(t *testing.T) {
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

	model := &models.Model{
		Slug:    "default-output-test",
		Type:    "llm",
		Name:    "Default Output",
		Port:    8000,
		Default: false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewExportCommand(root)

	// Use temp dir as working directory
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalWd)

	exitCode := cmd.Run([]string{"default-output-test"})
	if exitCode != 0 {
		t.Errorf("export with default path returned non-zero: %d", exitCode)
	}

	// Check that default-output-test.yaml was created in temp dir
	expectedPath := filepath.Join(tmpDir, "default-output-test.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal("default output file was not created")
	}
}

func TestExportCommand_CustomOutputPath(t *testing.T) {
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

	model := &models.Model{
		Slug:    "custom-path-test",
		Type:    "llm",
		Name:    "Custom Path",
		Port:    8000,
		Default: false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewExportCommand(root)

	tmpDir := t.TempDir()
	customDir := filepath.Join(tmpDir, "custom")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("failed to create custom output directory: %v", err)
	}
	outputPath := filepath.Join(customDir, "output.yaml")

	exitCode := cmd.Run([]string{"custom-path-test", "--output=" + outputPath})
	if exitCode != 0 {
		t.Errorf("export with custom path returned non-zero: %d", exitCode)
	}

	// Verify file was created at custom path
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("custom output file was not created")
	}
}

func TestExportCommand_UnknownFlag(t *testing.T) {
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
	cmd := NewExportCommand(root)

	exitCode := cmd.Run([]string{"some-model", "--unknown-flag=value"})
	if exitCode == 0 {
		t.Error("export with unknown flag should return non-zero")
	}
}
