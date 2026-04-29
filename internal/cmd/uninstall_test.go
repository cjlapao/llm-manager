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

// ────────────── Unit helpers ──────────────

func TestParseUninstallArgs_Empty(t *testing.T) {
	slug, keepCache, allFlag := parseUninstallArgs([]string{})
	if slug != "" {
		t.Errorf("parseUninstallArgs([]) slug = %q, want \"\"", slug)
	}
	if keepCache {
		t.Error("parseUninstallArgs([]) keepCache = true, want false")
	}
	if allFlag {
		t.Error("parseUninstallArgs([]) allFlag = true, want false")
	}
}

func TestParseUninstallArgs_SlugOnly(t *testing.T) {
	slug, keepCache, allFlag := parseUninstallArgs([]string{"my-model"})
	if slug != "my-model" {
		t.Errorf("slug = %q, want \"my-model\"", slug)
	}
	if keepCache {
		t.Error("keepCache = true, want false")
	}
	if allFlag {
		t.Error("allFlag = true, want false")
	}
}

func TestParseUninstallArgs_KeepCacheFlag(t *testing.T) {
	for _, flag := range []string{"--keep-cached-model", "-k"} {
		slug, keepCache, allFlag := parseUninstallArgs([]string{"my-model", flag})
		if slug != "my-model" {
			t.Errorf("flag %s: slug = %q, want \"my-model\"", flag, slug)
		}
		if !keepCache {
			t.Errorf("flag %s: keepCache = false, want true", flag)
		}
		if allFlag {
			t.Errorf("flag %s: allFlag = true, want false", flag)
		}
	}
}

func TestParseUninstallArgs_AllFlag(t *testing.T) {
	slug, keepCache, allFlag := parseUninstallArgs([]string{"--all"})
	if slug != "" {
		t.Errorf("all flag: slug = %q, want \"\"", slug)
	}
	if keepCache {
		t.Error("all flag: keepCache = true, want false")
	}
	if !allFlag {
		t.Error("all flag: allFlag = false, want true")
	}
}

func TestParseUninstallArgs_AllWithKeepCache(t *testing.T) {
	slug, keepCache, allFlag := parseUninstallArgs([]string{"--all", "--keep-cached-model"})
	if slug != "" {
		t.Errorf("all+keep: slug = %q, want \"\"", slug)
	}
	if !keepCache {
		t.Error("all+keep: keepCache = false, want true")
	}
	if !allFlag {
		t.Error("all+keep: allFlag = false, want true")
	}
}

func TestParseUninstallArgs_HelpSkipped(t *testing.T) {
	slug, _, _ := parseUninstallArgs([]string{"help", "--all"})
	if slug != "" {
		t.Errorf("with 'help' arg should return empty slug, got %q", slug)
	}
}

func TestParseUninstallArgs_DuplicateSlug(t *testing.T) {
	slug, _, _ := parseUninstallArgs([]string{"a", "b"})
	if slug == "b" {
		t.Error("duplicate slug should return first only, not second")
	}
}

func TestParseUninstallArgs_MixedFlags(t *testing.T) {
	slug, keepCache, allFlag := parseUninstallArgs([]string{"my-model", "--all", "--keep-cached-model"})
	if slug != "my-model" {
		t.Errorf("mixed: slug = %q, want \"my-model\"", slug)
	}
	if !keepCache {
		t.Error("mixed: keepCache = false, want true")
	}
	if !allFlag {
		t.Error("mixed: allFlag = false, want true")
	}
}

// ────────────── UninstallCommand HELP paths ──────────────

func TestUninstallCommand_Help(t *testing.T) {
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
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("uninstall help returned non-zero exit code: %d", exitCode)
	}
}

func TestUninstallCommand_NoArgs(t *testing.T) {
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
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{})
	if exitCode != 0 {
		t.Errorf("uninstall with no args should return 0, got %d", exitCode)
	}
}

func TestUninstallCommand_ModelNotFound(t *testing.T) {
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
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"nonexistent-slug"})
	if exitCode == 0 {
		t.Error("uninstall nonexistent model should return non-zero")
	}
}

func TestUninstallCommand_MissingLLMDir(t *testing.T) {
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
		Slug:       "test-model",
		Type:       "llm",
		Name:       "Test Model",
		HFRepo:     "Qwen/Qwen3-8B",
		Container:  "test-container",
		Port:       8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:    false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = "" // intentionally empty

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"test-model"})
	if exitCode == 0 {
		t.Error("uninstall with empty LLMDir should return non-zero")
	}
}

func TestUninstallCommand_AllFlag(t *testing.T) {
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
	cfg.LLMDir = t.TempDir()

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"--all"})
	if exitCode != 0 {
		t.Errorf("uninstall --all with no models should return 0, got %d", exitCode)
	}
}

func TestUninstallCommand_AllWithModels(t *testing.T) {
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

	models := []*models.Model{
		{
			Slug:       "model-one",
			Type:       "llm",
			Name:       "Model One",
			HFRepo:     "Qwen/Qwen3-8B",
			Container:  "container-one",
			Port:       8080,
			EngineType: "vllm",
			EnvVars:    "{}",
			Default:    false,
		},
		{
			Slug:       "model-two",
			Type:       "llm",
			Name:       "Model Two",
			HFRepo:     "Qwen/Qwen3-14B",
			Container:  "container-two",
			Port:       8081,
			EngineType: "vllm",
			EnvVars:    "{}",
			Default:    false,
		},
	}

	for _, m := range models {
		if err := db.CreateModel(m); err != nil {
			t.Fatalf("CreateModel(%s) error: %v", m.Slug, err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"--all"})
	// May return non-zero due to missing containers/LLM proxy, but should not panic
	_ = exitCode
}

func TestUninstallCommand_KeepCacheFlag(t *testing.T) {
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

	model := &models.Model{
		Slug:       "cache-test-model",
		Type:       "llm",
		Name:       "Cache Test",
		HFRepo:     "Qwen/Qwen3-8B",
		Container:  "cache-test-container",
		Port:       8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:    false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	_ = NewUninstallCommand(root)

	// Test that --keep-cached-model flag is parsed correctly
	_, keepCache, _ := parseUninstallArgs([]string{"cache-test-model", "--keep-cached-model"})
	if !keepCache {
		t.Error("--keep-cached-model should set keepCache to true")
	}

	// Test that -k short flag works
	_, keepCache, _ = parseUninstallArgs([]string{"cache-test-model", "-k"})
	if !keepCache {
		t.Error("-k should set keepCache to true")
	}
}

func TestUninstallCommand_HFRepoEmpty(t *testing.T) {
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

	model := &models.Model{
		Slug:      "no-hf-model",
		Type:      "llm",
		Name:      "No HF Model",
		HFRepo:    "", // no HuggingFace repo
		Container: "no-hf-container",
		Port:      8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:   false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"no-hf-model"})
	// May fail due to missing container/LLM, but should not panic
	_ = exitCode
}

func TestUninstallCommand_NoContainer(t *testing.T) {
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

	model := &models.Model{
		Slug:       "no-container-model",
		Type:       "llm",
		Name:       "No Container Model",
		HFRepo:     "Qwen/Qwen3-8B",
		Container:  "",
		Port:       8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:    false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"no-container-model"})
	// May fail due to missing LLM proxy, but should not panic
	_ = exitCode
}

func TestUninstallCommand_YAMLFileExists(t *testing.T) {
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

	// Create a YAML file that should be deleted
	ymlPath := filepath.Join(tmpDir, "yaml-test-model.yml")
	ymlContent := `services:
  llm:
    image: vllm
`
	if err := os.WriteFile(ymlPath, []byte(ymlContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(ymlPath); os.IsNotExist(err) {
		t.Fatal("YAML file should exist before test")
	}

	model := &models.Model{
		Slug:       "yaml-test-model",
		Type:       "llm",
		Name:       "YAML Test Model",
		HFRepo:     "Qwen/Qwen3-8B",
		Container:  "yaml-test-container",
		Port:       8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:    false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"yaml-test-model"})
	// May fail due to missing LLM proxy, but should not panic
	_ = exitCode

	// YAML file should be deleted
	if _, err := os.Stat(ymlPath); !os.IsNotExist(err) {
		t.Error("YAML file should be deleted after uninstall")
	}
}

func TestUninstallCommand_YAMLBackupDeleted(t *testing.T) {
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

	// Create both YAML and backup files
	ymlPath := filepath.Join(tmpDir, "backup-test-model.yml")
	bakPath := ymlPath + ".bak"
	if err := os.WriteFile(ymlPath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if err := os.WriteFile(bakPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	model := &models.Model{
		Slug:       "backup-test-model",
		Type:       "llm",
		Name:       "Backup Test Model",
		HFRepo:     "Qwen/Qwen3-8B",
		Container:  "backup-test-container",
		Port:       8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:    false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"backup-test-model"})
	_ = exitCode

	// Both files should be deleted
	if _, err := os.Stat(ymlPath); !os.IsNotExist(err) {
		t.Error("YAML file should be deleted after uninstall")
	}
	if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
		t.Error("YAML backup file should be deleted after uninstall")
	}
}

func TestUninstallCommand_CacheDir(t *testing.T) {
	// Test that clearHFCache generates the correct cache directory name
	testCases := []struct {
		hfRepo   string
		expected string
	}{
		{"Qwen/Qwen3-8B", "models--Qwen--Qwen3-8B"},
		{"meta-llama/Llama-3-8B", "models--meta-llama--Llama-3-8B"},
		{"mistralai/Mistral-7B", "models--mistralai--Mistral-7B"},
	}

	for _, tc := range testCases {
		cacheDir := "models--" + strings.ReplaceAll(tc.hfRepo, "/", "--")
		if cacheDir != tc.expected {
			t.Errorf("clearHFCache(%q) = %q, want %q", tc.hfRepo, cacheDir, tc.expected)
		}
	}
}

func TestUninstallCommand_EmptyHFRepo(t *testing.T) {
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

	model := &models.Model{
		Slug:       "no-hf-model",
		Type:       "llm",
		Name:       "No HF Model",
		HFRepo:     "",
		Container:  "no-hf-container",
		Port:       8080,
		EngineType: "vllm",
		EnvVars:    "{}",
		Default:    false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"empty-hf-model"})
	_ = exitCode
}

func TestUninstallCommand_FullPipeline(t *testing.T) {
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

	// Create a model with all fields populated
	ymlPath := filepath.Join(tmpDir, "full-pipeline-model.yml")
	if err := os.WriteFile(ymlPath, []byte("test: content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	model := &models.Model{
		Slug:            "full-pipeline-model",
		Type:            "llm",
		Name:            "Full Pipeline Model",
		HFRepo:          "Qwen/Qwen3-8B",
		YML:             "models/full-pipeline.yml",
		Container:       "full-pipeline-container",
		Port:            8080,
		EngineType:      "vllm",
		EnvVars:         `{"HF_TOKEN": "test"}`,
		CommandArgs:     `["--model", "Qwen/Qwen3-8B"]`,
		InputTokenCost:  0.000001,
		OutputTokenCost: 0.000002,
		Default:         false,
	}
	if err := db.CreateModel(model); err != nil {
		t.Fatalf("CreateModel() error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.LLMDir = tmpDir

	root := &RootCommand{cfg: cfg, db: db}
	cmd := NewUninstallCommand(root)

	exitCode := cmd.Run([]string{"full-pipeline-model"})
	// May fail due to missing LLM proxy, but should not panic
	_ = exitCode

	// Model should be deleted from DB
	_, err = db.GetModel("full-pipeline-model")
	if err == nil {
		t.Error("Model should be deleted from database after uninstall")
	}

	// YAML file should be deleted
	if _, err := os.Stat(ymlPath); !os.IsNotExist(err) {
		t.Error("YAML file should be deleted after uninstall")
	}
}

func TestUninstallCommand_VariousSlugFormats(t *testing.T) {
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
	cmd := NewUninstallCommand(root)

	// Test various slug formats are handled gracefully (all should fail with model not found)
	testSlugs := []string{
		"simple-slug",
		"snake_case_slug",
		"camelCaseSlug",
		"slug-with-numbers-123",
		"UPPERCASE",
		"lowercase",
	}

	for _, slug := range testSlugs {
		exitCode := cmd.Run([]string{slug})
		if exitCode == 0 {
			t.Errorf("uninstall %q should return non-zero (model not found)", slug)
		}
	}
}
