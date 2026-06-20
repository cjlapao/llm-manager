package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
)

// TestIsValidProvider verifies that isValidProvider correctly identifies valid
// and invalid provider strings. Valid providers: "vllm", "sglang", "llama.cpp",
// "custom". Invalid: "unknown", "", "VLLM" (case-sensitive).
func TestIsValidProvider(t *testing.T) {
	validProviders := []string{"vllm", "sglang", "llama.cpp", "custom"}
	for _, p := range validProviders {
		if !isValidProvider(p) {
			t.Errorf("isValidProvider(%q) = false, want true", p)
		}
	}

	invalidProviders := []string{"unknown", "", "VLLM", "SGLANG", "Llama.CPP", "CUSTOM", "vLLM", "0", "vllm-extra"}
	for _, p := range invalidProviders {
		if isValidProvider(p) {
			t.Errorf("isValidProvider(%q) = true, want false", p)
		}
	}
}

// TestIsValidProvider_ModelLevel verifies that models.IsValidProvider has the
// same valid set as the service-level isValidProvider.
func TestIsValidProvider_ModelLevel(t *testing.T) {
	validProviders := []string{"vllm", "sglang", "llama.cpp", "custom"}
	for _, p := range validProviders {
		if !models.IsValidProvider(p) {
			t.Errorf("models.IsValidProvider(%q) = false, want true", p)
		}
	}

	invalidProviders := []string{"unknown", "", "VLLM", "vLLM"}
	for _, p := range invalidProviders {
		if models.IsValidProvider(p) {
			t.Errorf("models.IsValidProvider(%q) = true, want false", p)
		}
	}
}

// storageDB extends mockDB by actually storing engine types and versions.
// This is needed because the base mockDB's CreateEngineType is a no-op.
type storageDB struct {
	*mockDB
}

func newStorageDB() *storageDB {
	return &storageDB{mockDB: newMockDB()}
}

func (m *storageDB) CreateEngineType(et *models.EngineType) error {
	m.engineTypes[et.Slug] = et
	return nil
}

// TestImportEngineFile_ProviderFromYAML verifies that when a YAML file
// contains provider: vllm, the imported EngineType has Provider == "vllm".
func TestImportEngineFile_ProviderFromYAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: test-vllm
  name: Test vLLM
  description: A test engine with vllm provider
  provider: vllm
versions:
  - slug: v010
    version: "0.1.0"
    image: vllm/vllm-openai:latest
    default: true
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDB()
	svc := NewEngineService(db)

	created, _, _, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{})
	if err != nil {
		t.Fatalf("ImportEngineFile returned error: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 created, got %d", created)
	}

	et, err := svc.GetEngineTypeBySlug("test-vllm")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug returned error: %v", err)
	}
	if et == nil {
		t.Fatal("engine type is nil")
	}
	if et.Provider != "vllm" {
		t.Errorf("expected Provider == 'vllm', got %q", et.Provider)
	}
}

// TestImportEngineFile_ProviderDefaultsToCustom verifies that when a YAML file
// omits the provider field, the imported EngineType defaults to "custom".
func TestImportEngineFile_ProviderDefaultsToCustom(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: test-custom
  name: Test Custom
  description: A test engine without explicit provider
versions:
  - slug: v010
    version: "0.1.0"
    image: myimage:latest
    default: true
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDB()
	svc := NewEngineService(db)

	created, _, _, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{})
	if err != nil {
		t.Fatalf("ImportEngineFile returned error: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 created, got %d", created)
	}

	et, err := svc.GetEngineTypeBySlug("test-custom")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug returned error: %v", err)
	}
	if et == nil {
		t.Fatal("engine type is nil")
	}
	if et.Provider != "custom" {
		t.Errorf("expected Provider == 'custom' (default), got %q", et.Provider)
	}
}

// TestImportEngineFile_InvalidProviderRejected verifies that when a YAML file
// contains an invalid provider, ImportEngineFile returns an error and does
// not create the engine type.
func TestImportEngineFile_InvalidProviderRejected(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: test-invalid
  name: Test Invalid
  description: An engine with invalid provider
  provider: unknown-provider
versions:
  - slug: v010
    version: "0.1.0"
    image: myimage:latest
    default: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDB()
	svc := NewEngineService(db)

	created, _, skipped, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{})
	if err == nil {
		t.Fatalf("expected error for invalid provider, got nil")
	}
	if created != 0 {
		t.Errorf("expected 0 created, got %d", created)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if !strings.Contains(err.Error(), "invalid provider") {
		t.Errorf("error should mention 'invalid provider': %v", err)
	}

	// Verify no engine type was created
	et, _ := svc.GetEngineTypeBySlug("test-invalid")
	if et != nil {
		t.Errorf("expected nil engine type for rejected import, got %+v", et)
	}
}

// TestImportEngineFile_AllValidProviders verifies that each valid provider
// value is accepted and stored correctly.
func TestImportEngineFile_AllValidProviders(t *testing.T) {
	validProviders := []string{"vllm", "sglang", "llama.cpp", "custom"}

	for _, provider := range validProviders {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			yamlPath := filepath.Join(dir, "engine.yaml")

			content := `
engine:
  slug: test-prov
  name: Test Provider
`
			if provider != "custom" {
				content += "  provider: " + provider + "\n"
			}
			content += `versions:
  - slug: v010
    version: "0.1.0"
    image: myimage:latest
    default: true
`

			if err := os.WriteFile(yamlPath, []byte(content), 0644); err != nil {
				t.Fatalf("failed to write YAML file: %v", err)
			}

			db := newStorageDB()
			svc := NewEngineService(db)

			created, _, _, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{})
			if err != nil {
				t.Fatalf("ImportEngineFile returned error: %v", err)
			}
			if created != 1 {
				t.Errorf("expected 1 created, got %d", created)
			}

			et, err := svc.GetEngineTypeBySlug("test-prov")
			if err != nil {
				t.Fatalf("GetEngineTypeBySlug returned error: %v", err)
			}
			if et == nil {
				t.Fatal("engine type is nil")
			}
			expectedProvider := provider
			if et.Provider != expectedProvider {
				t.Errorf("expected Provider == %q, got %q", expectedProvider, et.Provider)
			}
		})
	}
}

// TestImportEngineFile_EmptyProviderDefaultsToCustom verifies that an empty
// provider string in the YAML defaults to "custom".
func TestImportEngineFile_EmptyProviderDefaultsToCustom(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: test-empty-prov
  name: Test Empty Provider
  provider: ""
versions:
  - slug: v010
    version: "0.1.0"
    image: myimage:latest
    default: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDB()
	svc := NewEngineService(db)

	created, _, skipped, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{})
	if err != nil {
		t.Fatalf("ImportEngineFile returned error: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 created, got %d", created)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	et, err := svc.GetEngineTypeBySlug("test-empty-prov")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug returned error: %v", err)
	}
	if et == nil {
		t.Fatal("engine type is nil")
	}
	if et.Provider != "custom" {
		t.Errorf("expected Provider == 'custom' (default from empty), got %q", et.Provider)
	}
}
