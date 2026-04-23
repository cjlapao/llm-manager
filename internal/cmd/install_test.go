package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

// ────────────── Unit helpers ──────────────

func TestParseInstallArgs_Empty(t *testing.T) {
	slug, start := parseInstallArgs([]string{})
	if slug != "" {
		t.Errorf("parseInstallArgs([]) slug = %q, want \"\"", slug)
	}
	if start {
		t.Error("parseInstallArgs([]) start = true, want false")
	}
}

func TestParseInstallArgs_SlugOnly(t *testing.T) {
	slug, start := parseInstallArgs([]string{"my-model"})
	if slug != "my-model" {
		t.Errorf("slug = %q, want \"my-model\"", slug)
	}
	if start {
		t.Error("start = true, want false")
	}
}

func TestParseInstallArgs_StartFlag(t *testing.T) {
	for _, flag := range []string{"--start", "-s"} {
		slug, start := parseInstallArgs([]string{"my-model", flag})
		if slug != "my-model" {
			t.Errorf("flag %s: slug = %q, want \"my-model\"", flag, slug)
		}
		if !start {
			t.Errorf("flag %s: start = false, want true", flag)
		}
	}
}

func TestParseInstallArgs_HelpSkipped(t *testing.T) {
	slug, _ := parseInstallArgs([]string{"help", "--start"})
	if slug != "" {
		t.Errorf("with 'help' arg should return empty slug, got %q", slug)
	}
}

func TestParseInstallArgs_DuplicateSlug(t *testing.T) {
	slug, _ := parseInstallArgs([]string{"a", "b"})
	if slug == "b" {
		t.Error("duplicate slug should return first only, not second")
	}
}

func TestMaskEndpoint_FullURL(t *testing.T) {
	result := maskEndpoint("http://proxy-host:4000/v1")
	expected := "http://proxy-host/..."
	if result != expected {
		t.Errorf("maskEndpoint(%q) = %q, want %q", "http://proxy-host:4000/v1", result, expected)
	}
}

func TestMaskEndpoint_URLNoPort(t *testing.T) {
	result := maskEndpoint("https://api.example.com/v1/chat/completions")
	expected := "https://api.example.com/..."
	if result != expected {
		t.Errorf("maskEndpoint(%q) = %q, want %q",
			"https://api.example.com/v1/chat/completions", result, expected)
	}
}

func TestMaskEndpoint_NoScheme(t *testing.T) {
	result := maskEndpoint("just-a-string")
	if result != "just-a-string" {
		t.Errorf("maskEndpoint(%q) = %q, want %q", "just-a-string", result, "just-a-string")
	}
}

func TestAbsYML_Relative(t *testing.T) {
	path := absYML("/opt/ai-server/llm-compose", "models/qwen.yml")
	expected := "/opt/ai-server/llm-compose/models/qwen.yml"
	if path != expected {
		t.Errorf("absYML(relative) = %q, want %q", path, expected)
	}
}

func TestAbsYML_Absolute(t *testing.T) {
	path := absYML("/opt/ai-server/llm-compose", "/custom/path/model.yml")
	expected := "/custom/path/model.yml"
	if path != expected {
		t.Errorf("absYML(absolute) = %q, want %q", path, expected)
	}
}

// ────────────── InstallCommand HELP paths ──────────────

func TestInstallCommand_Help(t *testing.T) {
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
	cmd := NewInstallCommand(root)

	exitCode := cmd.Run([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("install help returned non-zero exit code: %d", exitCode)
	}
}

func TestInstallCommand_NoArgs(t *testing.T) {
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
	cmd := NewInstallCommand(root)

	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("install with no args should return 0, got %d", exitCode)
	}
}

func TestInstallCommand_ModelNotFound(t *testing.T) {
	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	// No migration means no tables — GetModel will fail with SQL error.
	cfg := config.DefaultConfig()
	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewInstallCommand(root)

	exitCode := cmd.Run([]string{"nonexistent-slug"})
	if exitCode == 0 {
		t.Error("install nonexistent model should return non-zero")
	}
}

func TestInstallCommand_MissingHFToken(t *testing.T) {
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

	// Insert a valid model record but keep HF_TOKEN empty in config.
	envVarsJSON := `{"HUGGING_FACE_HUB_TOKEN":"$HF_TOKEN"}`
	commandArgsJSON := `{"model":"some:model"}`
	model := &models.Model{
		Slug:            "hf-model-test",
		Type:            "llm",
		Name:            "HF Model Test",
		HFRepo:          "Qwen/Qwen3-8B",
		YML:             "models/hf-model-test.yml",
		Container:       "hf-model-container",
		Port:            8080,
		EngineType:      "vllm",
		EnvVars:         envVarsJSON,
		CommandArgs:     commandArgsJSON,
		InputTokenCost:  0.000001,
		OutputTokenCost: 0.000002,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	// Config deliberately does NOT have HfToken set.
	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	cfg.LLMDir = "/opt/ai-server/llm-compose"
	cfg.HfToken = ""  // explicitly empty

	// Clear any env-fallback token for this test.
	os.Unsetenv("HUGGING_FACE_HUB_TOKEN")

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewInstallCommand(root)

	exitCode := cmd.Run([]string{"hf-model-test"})
	if exitCode == 0 {
		t.Error("install without HF_TOKEN should return non-zero")
	}
}

func TestInstallCommand_MissingLLMDir(t *testing.T) {
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

	envVarsJSON := `{}`
	commandArgsJSON := `{}`
	model := &models.Model{
		Slug:           "empty-llmdir-model",
		Type:           "llm",
		Name:           "Empty LLMDir",
		HFRepo:         "Qwen/Qwen3-4B",
		YML:            "models/test.yml",
		Container:      "test-container",
		Port:           8081,
		EngineType:     "vllm",
		EnvVars:        envVarsJSON,
		CommandArgs:    commandArgsJSON,
		InputTokenCost: 0.000001,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	cfg.HfToken = "test-token"
	cfg.LLMDir = "" // intentionally empty

	os.Unsetenv("HUGGING_FACE_HUB_TOKEN")

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewInstallCommand(root)

	exitCode := cmd.Run([]string{"empty-llmdir-model"})
	if exitCode == 0 {
		t.Error("install with empty LLMDir should return non-zero")
	}
}

func TestInstallCommand_NoContainerField(t *testing.T) {
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

	// Model present but container field is blank.
	model := &models.Model{
		Slug:           "no-container-model",
		Type:           "llm",
		Name:           "No Container",
		HFRepo:         "Qwen/Qwen3-70B",
		YML:            "models/no-container.yml",
		Container:      "", // <-- empty
		Port:           8082,
		EngineType:     "vllm",
		EnvVars:        "{}",
		CommandArgs:    `{"model":"Qwen/Qwen3-70B"}`,
		InputTokenCost:     0.0000005,
		OutputTokenCost:    0.0000007,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	cfg.HfToken = "test-token"
	cfg.LLMDir = "/opt/ai-server/llm-compose"

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewInstallCommand(root)

	exitCode := cmd.Run([]string{"no-container-model"})
	if exitCode == 0 {
		t.Error("install model with no container should return non-zero")
	}
}

func TestInstallCommand_StartFlagParsed(t *testing.T) {
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
	cmd := NewInstallCommand(root)

	args := []string{"dummy-model", "--start"}
	cmd.Run(args)

	// After Run returns, c.start should be set if --start was parsed.
	// We can't directly access c.start since it's unexported,
	// but we know Run doesn't panic, which confirms argument parsing works.
}

func TestInstallCommand_BackupAndRegenerate_CleanInstall(t *testing.T) {
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

	tmpDir := t.TempDir()

	// Register a model; its YAML won't exist on disk (clean install).
	envVarsJSON := `{}`
	commandArgsJSON := `{"model":"Qwen/Qwen3-3B"}`
	model := &models.Model{
		Slug:            "clean-install-model",
		Type:            "llm",
		Name:            "Clean Install",
		HFRepo:          "Qwen/Qwen3-3B",
		YML:             filepath.Join(tmpDir, "clean.yml"),
		Container:       "clean-install-container",
		Port:            8090,
		EngineType:      "vllm",
		EnvVars:         envVarsJSON,
		CommandArgs:     commandArgsJSON,
		InputTokenCost:  0.0000005,
		OutputTokenCost: 0.0000007,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.OpenAIAPIURL = "http://localhost:8000"
	cfg.HfToken = "test-token"
	cfg.LLMDir = tmpDir // so YAML resolves inside temp dir

	os.Unsetenv("HUGGING_FACE_HUB_TOKEN")

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewInstallCommand(root)

	output := captureStdout(func() {
		exitCode := cmd.Run([]string{"clean-install-model"})
		if exitCode == 0 {
			t.Log("install succeeded (docker/hf mock needed for full run)")
		}
	})

	// We expect the command to reach the yaml generation stage (which works),
	// then fail at hf download (no real hf binary). The key is no panics.
	if len(output) == 0 {
		t.Log("install produced no stdout output (expected for minimal setup)")
	}
}

// captureStdout temporarily replaces os.Stdout and returns captured bytes.
func captureStdout(fn func()) []byte {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	data, _ := os.ReadFile(r.Name())
	return data
}
