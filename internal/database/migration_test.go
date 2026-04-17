package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
)

func createTestModelsJSON(t *testing.T, path string, models map[string]modelJSON) {
	t.Helper()
	data := modelsJSON{
		Version:     "1",
		HFCacheDir:  "/opt/ai-server/models",
		ModelGroups: models,
	}
	err := os.WriteFile(path, func() []byte {
		b, _ := json.Marshal(data)
		return b
	}(), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test models JSON: %v", err)
	}
}

func TestMigrateFromJSON_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, "models.json")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	testModels := map[string]modelJSON{
		"test-llm": {
			Type:      "llm",
			Name:      "Test LLM",
			HFRepo:    "test/test-llm",
			YML:       "test-llm.yml",
			Container: "test-llm-container",
			Port:      8080,
		},
		"test-embed": {
			Type:      "embed",
			Name:      "Test Embed",
			HFRepo:    "test/test-embed",
			Container: "test-embed-container",
			Port:      8081,
		},
	}

	createTestModelsJSON(t, jsonPath, testModels)

	count, err := mgr.MigrateFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("MigrateFromJSON() returned error: %v", err)
	}

	if count != 2 {
		t.Errorf("Migrated %d models, want 2", count)
	}

	// Verify models were inserted
	var modelCount int64
	mgr.DB().Model(&models.Model{}).Count(&modelCount)
	if modelCount != 2 {
		t.Errorf("Database has %d models, want 2", modelCount)
	}

	// Verify specific model
	var found models.Model
	result := mgr.DB().Where("slug = ?", "test-llm").First(&found)
	if result.Error != nil {
		t.Fatalf("Query for test-llm failed: %v", result.Error)
	}
	if found.Type != "llm" {
		t.Errorf("test-llm Type = %q, want %q", found.Type, "llm")
	}
	if found.Port != 8080 {
		t.Errorf("test-llm Port = %d, want %d", found.Port, 8080)
	}
}

func TestMigrateFromJSON_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, "models.json")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	testModels := map[string]modelJSON{
		"test-llm": {
			Type:      "llm",
			Name:      "Test LLM",
			HFRepo:    "test/test-llm",
			Container: "test-llm",
			Port:      8080,
		},
	}

	createTestModelsJSON(t, jsonPath, testModels)

	// First migration
	count, err := mgr.MigrateFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("First MigrateFromJSON() returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("First migration migrated %d models, want 1", count)
	}

	// Second migration — should skip
	count, err = mgr.MigrateFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("Second MigrateFromJSON() returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("Second migration migrated %d models, want 0 (idempotent)", count)
	}

	// Verify only one model exists
	var modelCount int64
	mgr.DB().Model(&models.Model{}).Count(&modelCount)
	if modelCount != 1 {
		t.Errorf("Database has %d models after second migration, want 1", modelCount)
	}
}

func TestMigrateFromJSON_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	_, err = mgr.MigrateFromJSON(filepath.Join(tmpDir, "nonexistent.json"))
	if err == nil {
		t.Error("MigrateFromJSON() with missing file should return error")
	}
}

func TestMigrateFromJSON_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, "invalid.json")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = os.WriteFile(jsonPath, []byte("{invalid json"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	_, err = mgr.MigrateFromJSON(jsonPath)
	if err == nil {
		t.Error("MigrateFromJSON() with invalid JSON should return error")
	}
}

func TestMigrateFromJSON_EmptyModels(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	jsonPath := filepath.Join(tmpDir, "models.json")

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	testModels := map[string]modelJSON{}
	createTestModelsJSON(t, jsonPath, testModels)

	count, err := mgr.MigrateFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("MigrateFromJSON() returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("Migrated %d models from empty JSON, want 0", count)
	}
}

func TestMigrateFromJSON_WithRealModelsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Copy the real models.json from the project root
	// The test file is at internal/database/migration_test.go
	// Go up 2 levels: database -> internal -> llm-manager (project root)
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("Skipping test: cannot determine test file path")
	}
	projRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(testFile))))
	jsonPath := filepath.Join(projRoot, "models.json")

	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Skipping test: models.json not found: %v", err)
	}

	jsonPath = filepath.Join(tmpDir, "models.json")
	err = os.WriteFile(jsonPath, jsonData, 0o644)
	if err != nil {
		t.Fatalf("Failed to write models.json: %v", err)
	}

	mgr, err := NewDatabaseManager(dbPath)
	if err != nil {
		t.Fatalf("NewDatabaseManager() returned error: %v", err)
	}

	err = mgr.Open()
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer mgr.Close()

	err = mgr.AutoMigrate()
	if err != nil {
		t.Fatalf("AutoMigrate() returned error: %v", err)
	}

	count, err := mgr.MigrateFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("MigrateFromJSON() returned error: %v", err)
	}

	// Should migrate all 17 models
	if count != 17 {
		t.Errorf("Migrated %d models, want 17", count)
	}

	// Verify some specific models
	testCases := []struct {
		slug string
		typ  string
		port int
	}{
		{"qwen3_6", "llm", 8025},
		{"qwen3-embedding", "embed", 8020},
		{"qwen3-reranker", "rerank", 8021},
		{"autocomplete", "autocomplete", 8081},
		{"qwen3_5-2b", "draft", 0},
	}

	for _, tc := range testCases {
		var found models.Model
		result := mgr.DB().Where("slug = ?", tc.slug).First(&found)
		if result.Error != nil {
			t.Errorf("Query for %s failed: %v", tc.slug, result.Error)
			continue
		}
		if found.Type != tc.typ {
			t.Errorf("%s Type = %q, want %q", tc.slug, found.Type, tc.typ)
		}
		if found.Port != tc.port {
			t.Errorf("%s Port = %d, want %d", tc.slug, found.Port, tc.port)
		}
	}
}
