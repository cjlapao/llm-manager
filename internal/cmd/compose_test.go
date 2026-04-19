package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
	"github.com/user/llm-manager/internal/service"
)

func TestComposeCommand_Help(t *testing.T) {
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
	cmd := NewComposeCommand(root)

	exitCode := cmd.Run([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("compose help returned non-zero exit code: %d", exitCode)
	}
}

func TestComposeCommand_NoArgs(t *testing.T) {
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
	cmd := NewComposeCommand(root)

	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("compose with no args should return 0")
	}
}

func TestComposeCommand_MissingSlug(t *testing.T) {
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
	cmd := NewComposeCommand(root)

	exitCode := cmd.Run([]string{"--output=test.yml"})
	if exitCode == 0 {
		t.Error("compose without slug should return non-zero")
	}
}

func TestComposeCommand_MissingModel(t *testing.T) {
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
	cmd := NewComposeCommand(root)

	exitCode := cmd.Run([]string{"nonexistent-model"})
	if exitCode == 0 {
		t.Error("compose nonexistent model should return non-zero")
	}
}

func TestComposeCommand_VLLMModel(t *testing.T) {
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

	// Create a test model with vLLM engine
	envVarsJSON := `{"HUGGING_FACE_HUB_TOKEN":"${HF_TOKEN}","VLLM_HOST":"0.0.0.0"}`
	commandArgsJSON := `{"model":"Qwen/Qwen3-Next-80B-A3B-Instruct","max-model-len":"131072","kv-cache-dtype":"fp8"}`

	model := &models.Model{
		Slug:            "compose-vllm-test",
		Type:            "llm",
		Name:            "Compose VLLM Test",
		HFRepo:          "Qwen/Qwen3-Next-80B-A3B-Instruct",
		Container:       "llm-compose-vllm-test",
		Port:            8017,
		EngineType:      "vllm",
		EnvVars:         envVarsJSON,
		CommandArgs:     commandArgsJSON,
		InputTokenCost:  0.0000003,
		OutputTokenCost: 0.0000004,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewComposeCommand(root)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "vllm-compose.yml")

	exitCode := cmd.Run([]string{"compose-vllm-test", "--output=" + outputPath})
	if exitCode != 0 {
		t.Errorf("compose vllm model returned non-zero: %d", exitCode)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("compose file was not created")
	}

	// Verify content
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read compose file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "llm:") {
		t.Error("compose YAML missing 'llm:' service")
	}
	if !strings.Contains(content, "base-pgx-llm.yml") {
		t.Error("compose YAML missing extends file")
	}
	if !strings.Contains(content, "vllm-node") {
		t.Error("compose YAML missing vllm-node service")
	}
	if !strings.Contains(content, "llm-compose-vllm-test") {
		t.Error("compose YAML missing container name")
	}
	if !strings.Contains(content, "8017:8000") {
		t.Error("compose YAML missing port mapping")
	}
	if !strings.Contains(content, "HUGGING_FACE_HUB_TOKEN") {
		t.Error("compose YAML missing environment variable")
	}
	if !strings.Contains(content, "--model") {
		t.Error("compose YAML missing command args")
	}
	if !strings.Contains(content, "--max-model-len") {
		t.Error("compose YAML missing max-model-len command arg")
	}
	if !strings.Contains(content, "--kv-cache-dtype") {
		t.Error("compose YAML missing kv-cache-dtype command arg")
	}
}

func TestComposeCommand_SGLangModel(t *testing.T) {
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

	// Create a test model with SGLang engine
	envVarsJSON := `{"HF_TOKEN":"${HF_TOKEN}"}`
	commandArgsJSON := `{"model":"test/model","port":"8000"}`

	model := &models.Model{
		Slug:            "compose-sglang-test",
		Type:            "llm",
		Name:            "Compose SGLang Test",
		HFRepo:          "test/model",
		Container:       "llm-compose-sglang-test",
		Port:            8000,
		EngineType:      "sglang",
		EnvVars:         envVarsJSON,
		CommandArgs:     commandArgsJSON,
		InputTokenCost:  0,
		OutputTokenCost: 0,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewComposeCommand(root)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "sglang-compose.yml")

	exitCode := cmd.Run([]string{"compose-sglang-test", "--output=" + outputPath})
	if exitCode != 0 {
		t.Errorf("compose sglang model returned non-zero: %d", exitCode)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("compose file was not created")
	}

	// Verify content
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read compose file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "sglang-node") {
		t.Error("compose YAML missing sglang-node service")
	}
	if strings.Contains(content, "vllm-node") {
		t.Error("compose YAML should not contain vllm-node for sglang model")
	}
}

func TestComposeCommand_DefaultOutputPath(t *testing.T) {
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
		Slug:        "default-compose-test",
		Type:        "llm",
		Name:        "Default Compose",
		Container:   "llm-default-compose",
		Port:        8000,
		EngineType:  "vllm",
		EnvVars:     `{}`,
		CommandArgs: `{}`,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewComposeCommand(root)

	// Use temp dir as working directory
	tmpDir := t.TempDir()
	originalWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalWd)

	exitCode := cmd.Run([]string{"default-compose-test"})
	if exitCode != 0 {
		t.Errorf("compose with default path returned non-zero: %d", exitCode)
	}

	// Check that compose.yml was created in temp dir
	expectedPath := filepath.Join(tmpDir, "compose.yml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal("default compose file was not created")
	}
}

func TestComposeCommand_CustomOutputPath(t *testing.T) {
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
		Slug:        "custom-compose-test",
		Type:        "llm",
		Name:        "Custom Path",
		Container:   "llm-custom-compose",
		Port:        8000,
		EngineType:  "vllm",
		EnvVars:     `{}`,
		CommandArgs: `{}`,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewComposeCommand(root)

	tmpDir := t.TempDir()
	customDir := filepath.Join(tmpDir, "custom", "nested")
	outputPath := filepath.Join(customDir, "output.yml")

	// Create parent directory
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("failed to create custom dir: %v", err)
	}

	exitCode := cmd.Run([]string{"custom-compose-test", "--output=" + outputPath})
	if exitCode != 0 {
		t.Errorf("compose with custom path returned non-zero: %d", exitCode)
	}

	// Verify file was created at custom path
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("custom output file was not created")
	}
}

func TestComposeCommand_UnknownFlag(t *testing.T) {
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
	cmd := NewComposeCommand(root)

	exitCode := cmd.Run([]string{"some-model", "--unknown-flag=value"})
	if exitCode == 0 {
		t.Error("compose with unknown flag should return non-zero")
	}
}

func TestComposeCommand_ModelWithoutContainer(t *testing.T) {
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

	// Create a model without container name (should still work)
	model := &models.Model{
		Slug:        "no-container-test",
		Type:        "llm",
		Name:        "No Container",
		Container:   "",
		Port:        8000,
		EngineType:  "vllm",
		EnvVars:     `{}`,
		CommandArgs: `{}`,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewComposeCommand(root)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "no-container.yml")

	exitCode := cmd.Run([]string{"no-container-test", "--output=" + outputPath})
	if exitCode != 0 {
		t.Errorf("compose without container returned non-zero: %d", exitCode)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("compose file was not created")
	}
}

func TestComposeGenerator_Generate(t *testing.T) {
	generator, err := service.NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator() error: %v", err)
	}

	model := &models.Model{
		Slug:            "test-model",
		Type:            "llm",
		Name:            "Test Model",
		HFRepo:          "test/repo",
		Container:       "llm-test",
		Port:            8080,
		EngineType:      "vllm",
		EnvVars:         `{"KEY":"value"}`,
		CommandArgs:     `{"arg1":"val1"}`,
		InputTokenCost:  0.000001,
		OutputTokenCost: 0.000002,
	}

	// Test vLLM generation
	vllmYAML, err := generator.GenerateVLLM(model)
	if err != nil {
		t.Fatalf("GenerateVLLM() error: %v", err)
	}
	if !strings.Contains(vllmYAML, "vllm-node") {
		t.Error("vLLM compose missing vllm-node")
	}

	// Test SGLang generation
	model.EngineType = "sglang"
	sglangYAML, err := generator.GenerateSGLang(model)
	if err != nil {
		t.Fatalf("GenerateSGLang() error: %v", err)
	}
	if !strings.Contains(sglangYAML, "sglang-node") {
		t.Error("SGLang compose missing sglang-node")
	}

	// Test Generate dispatch
	model.EngineType = "vllm"
	generatedYAML, err := generator.Generate(model)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(generatedYAML, "vllm-node") {
		t.Error("Generate() with vllm missing vllm-node")
	}

	model.EngineType = "sglang"
	generatedYAML, err = generator.Generate(model)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(generatedYAML, "sglang-node") {
		t.Error("Generate() with sglang missing sglang-node")
	}
}
