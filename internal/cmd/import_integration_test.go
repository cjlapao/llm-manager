package cmd_test

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

// ── Test fixtures ──────────────────────────────────────────────────────────

const testEngineYAML = `
engine:
  slug: vllm
  name: vLLM
  description: "vLLM inference engine"
versions:
  - slug: test-v1
    version: "001"
    container_name: vllm-node
    image: cjlapao/pgx-vllm:latest
    entrypoint: ["python3", "-m", "vllm.entrypoints.openai.api_server"]
    default: true
    latest: true
    environment:
      HF_HUB_OFFLINE: "0"
    volumes:
      ../models: "/root/.cache/huggingface"
    command_args: ["--tensor-parallel-size", "{{.GPU_COUNT}}"]
    port: 8000
`

const testModelYAML = `
slug: test-model
name: "Test Model"
engine: vllm
engine_version: test-v1
hf_repo: "test/model"
container: llm-test-model
port: 9000
type: llm
subtype: chat
environment:
  VLLM_HOST: "0.0.0.0"
command:
  - "--model"
  - "${{ .hf_repo }}"
  - "--max-model-len"
  - "8192"
capabilities:
  - reasoning
  - tool-use
`

const badEngineNoSlug = `
engine:
  name: "No Slug"
  description: "Missing slug"
versions: []
`

const badModelNoEngine = `
slug: bad-model
name: "No Engine"
hf_repo: "test"
type: llm
`

const garbageYAML = `this is not a valid model or engine config
random content: 12345
`

const duplicateModelYAML = `
slug: test-model
name: "Duplicate Model"
engine: vllm
hf_repo: "test/duplicate"
container: llm-duplicate
port: 9001
type: llm
command: ["--model"]
`

// ── Helpers ────────────────────────────────────────────────────────────────

func mustDB(t *testing.T) database.DatabaseManager {
	t.Helper()
	db, err := database.NewDatabaseManager(":memory:")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}
	if err := db.Open(); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if err := db.ApplyPendingMigrations(); err != nil {
		t.Fatalf("ApplyPendingMigrations() error: %v", err)
	}
	return db
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error: %v", path, err)
	}
	return path
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected %q to NOT contain %q", s, substr)
	}
}

// ── 1. Import engine and model ─────────────────────────────────────────────

func TestIntegration_ImportEngineAndModel(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Write engine YAML
	enginePath := writeFile(t, dir, "vllm.yml", testEngineYAML)

	// Import engine
	engSvc := service.NewEngineService(db)
	created, skipped, err := engSvc.ImportEngineFile(enginePath)
	if err != nil {
		t.Fatalf("ImportEngineFile error: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 engine created, got %d", created)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify engine type in DB
	et, err := db.GetEngineTypeBySlug("vllm")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug(vllm) error: %v", err)
	}
	if et.Name != "vLLM" {
		t.Errorf("Engine name = %q, want %q", et.Name, "vLLM")
	}

	// Verify engine version in DB
	ev, err := db.GetEngineVersionBySlugAndType("vllm", "test-v1")
	if err != nil {
		t.Fatalf("GetEngineVersionBySlugAndType error: %v", err)
	}
	if ev.Image != "cjlapao/pgx-vllm:latest" {
		t.Errorf("Image = %q, want %q", ev.Image, "cjlapao/pgx-vllm:latest")
	}
	if !ev.IsDefault {
		t.Error("Expected IsDefault = true")
	}

	// Write model YAML
	modelPath := writeFile(t, dir, "test-model.yml", testModelYAML)

	// Import model
	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(engSvc)
	model, err := modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("ImportModel error: %v", err)
	}
	if model.Slug != "test-model" {
		t.Errorf("Slug = %q, want %q", model.Slug, "test-model")
	}
	if model.EngineType != "vllm" {
		t.Errorf("EngineType = %q, want %q", model.EngineType, "vllm")
	}
	if model.EngineVersionSlug != "test-v1" {
		t.Errorf("EngineVersionSlug = %q, want %q", model.EngineVersionSlug, "test-v1")
	}
}

// ── 2. Import folder with mixed files ──────────────────────────────────────

func TestIntegration_ImportFolderMixed(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	writeFile(t, dir, "vllm.yml", testEngineYAML)
	writeFile(t, dir, "test-model.yml", testModelYAML)
	writeFile(t, dir, "garbage.txt", garbageYAML)

	// Simulate folder import by scanning and processing each file
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}

	imported := 0
	skipped := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			skipped++
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			skipped++
			continue
		}

		if service.IsEngineYAML(data) {
			_, _, err := service.NewEngineService(db).ImportEngineFile(filepath.Join(dir, entry.Name()))
			if err == nil {
				imported++
			} else {
				skipped++
			}
		} else {
			// Try model import
			err := modelSvcImport(db, filepath.Join(dir, entry.Name()))
			if err == nil {
				imported++
			} else {
				skipped++
			}
		}
	}

	if imported != 2 {
		t.Errorf("expected 2 imported, got %d", imported)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", skipped)
	}
}

// modelSvcImport is a helper to import a model from path using DB.
func modelSvcImport(db database.DatabaseManager, path string) error {
	cfg := config.DefaultConfig()
	cfg.LLMDir = "/tmp/test"
	svc := service.NewModelService(db, cfg)
	svc.SetEngineService(service.NewEngineService(db))
	_, err := svc.ImportModel(path, service.ImportOverrides{})
	return err
}

// ── 3. Import with override ────────────────────────────────────────────────

func TestIntegration_ImportOverride(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Import model first
	modelPath := writeFile(t, dir, "test-model.yml", testModelYAML)
	engSvc := service.NewEngineService(db)
	_, _, _ = engSvc.ImportEngineFile(writeFile(t, dir, "vllm.yml", testEngineYAML))

	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(engSvc)

	// First import
	_, err := modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("First import error: %v", err)
	}

	// Second import WITHOUT override should fail (duplicate)
	_, err = modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err == nil {
		t.Error("expected error for duplicate import without --override")
	}

	// Third import WITH override should succeed
	_, err = modelSvc.ImportModel(modelPath, service.ImportOverrides{Override: true})
	if err != nil {
		t.Fatalf("Import with --override should succeed: %v", err)
	}

	// Fourth import of different model with --override should also succeed
	dupPath := writeFile(t, dir, "dup.yml", duplicateModelYAML)
	_, err = modelSvc.ImportModel(dupPath, service.ImportOverrides{Override: true})
	if err != nil {
		t.Fatalf("Import different model with --override should succeed: %v", err)
	}
}

// ── 4. Compose generation ──────────────────────────────────────────────────

func TestIntegration_ComposeGeneration(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Import engine + model
	engSvc := service.NewEngineService(db)
	_, _, err := engSvc.ImportEngineFile(writeFile(t, dir, "vllm.yml", testEngineYAML))
	if err != nil {
		t.Fatalf("Import engine error: %v", err)
	}

	modelPath := writeFile(t, dir, "test-model.yml", testModelYAML)
	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(engSvc)
	_, err = modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("Import model error: %v", err)
	}

	// Generate compose via the service
	generator, err := service.NewComposeGenerator()
	if err != nil {
		t.Fatalf("NewComposeGenerator error: %v", err)
	}

	composeYAML, err := modelSvc.GenerateCompose("test-model", generator, service.EngineComposeConfig{})
	if err != nil {
		t.Fatalf("GenerateCompose error: %v", err)
	}

	// Verify compose content
	assertContains(t, composeYAML, "services:")
	assertContains(t, composeYAML, "llm-test-model:")
	assertContains(t, composeYAML, "cjlapao/pgx-vllm:latest")
	assertContains(t, composeYAML, "llm-test-model")
	assertContains(t, composeYAML, "9000:8000")
	assertContains(t, composeYAML, "HF_HUB_OFFLINE")
	assertContains(t, composeYAML, "VLLM_HOST")
	assertContains(t, composeYAML, "../models:/root/.cache/huggingface")
	assertContains(t, composeYAML, "--model")
	assertContains(t, composeYAML, "--max-model-len")
	assertContains(t, composeYAML, "8192")

	// Verify NO old-style content
	assertNotContains(t, composeYAML, "extends:")
	assertNotContains(t, composeYAML, "base-pgx-llm.yml")
}

// ── 5. CRUD engine type ────────────────────────────────────────────────────

func TestIntegration_CRUD_EngineType(t *testing.T) {
	db := mustDB(t)

	// Create
	et := &models.EngineType{Slug: "test-cre", Name: "TestCreate", Description: "Testing"}
	if err := db.CreateEngineType(et); err != nil {
		t.Fatalf("CreateEngineType error: %v", err)
	}

	// List
	types, err := db.ListEngineTypes()
	if err != nil {
		t.Fatalf("ListEngineTypes error: %v", err)
	}
	if len(types) != 1 {
		t.Errorf("expected 1 engine type, got %d", len(types))
	}

	// Get
	got, err := db.GetEngineTypeBySlug("test-cre")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug error: %v", err)
	}
	if got.Name != "TestCreate" {
		t.Errorf("Name = %q, want %q", got.Name, "TestCreate")
	}

	// Delete (no versions) — should succeed
	if err := db.DeleteEngineType("test-cre"); err != nil {
		t.Fatalf("DeleteEngineType error: %v", err)
	}

	// Re-create for delete-with-versions test
	if err := db.CreateEngineType(&models.EngineType{Slug: "test-del", Name: "TestDel", Description: "x"}); err != nil {
		t.Fatalf("CreateEngineType error: %v", err)
	}
	if err := db.CreateEngineVersion(&models.EngineVersion{
		Slug:          "del-v1", EngineTypeSlug: "test-del", Version: "001",
		Image: "test/img:latest", ContainerName: "node",
	}); err != nil {
		t.Fatalf("CreateEngineVersion error: %v", err)
	}

	// Delete with versions should fail
	err = db.DeleteEngineType("test-del")
	if err == nil {
		t.Error("expected error deleting engine type with versions")
	}
}

// ── 6. CRUD engine version ─────────────────────────────────────────────────

func TestIntegration_CRUD_EngineVersion(t *testing.T) {
	db := mustDB(t)

	// Create engine type
	db.CreateEngineType(&models.EngineType{Slug: "crd", Name: "CRD", Description: "x"})

	// Create version
	ev := &models.EngineVersion{
		Slug: "crd-v1", EngineTypeSlug: "crd", Version: "001",
		Image: "test/img:latest", ContainerName: "node",
		EnvironmentJSON: `{"KEY":"val"}`, VolumesJSON: `{}`, CommandArgs: `[]`,
	}
	if err := db.CreateEngineVersion(ev); err != nil {
		t.Fatalf("CreateEngineVersion error: %v", err)
	}

	// List
	versions, err := db.ListEngineVersions()
	if err != nil {
		t.Fatalf("ListEngineVersions error: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}

	// Get
	got, err := db.GetEngineVersionBySlugAndType("crd", "crd-v1")
	if err != nil {
		t.Fatalf("GetEngineVersionBySlugAndType error: %v", err)
	}
	if got.Image != "test/img:latest" {
		t.Errorf("Image = %q, want %q", got.Image, "test/img:latest")
	}

	// Delete (no models) — should succeed
	if err := db.DeleteEngineVersion("crd-v1"); err != nil {
		t.Fatalf("DeleteEngineVersion error: %v", err)
	}
}

// ── 7. CRUD model ──────────────────────────────────────────────────────────

func TestIntegration_CRUD_Model(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Setup engine
	engSvc := service.NewEngineService(db)
	_, _, _ = engSvc.ImportEngineFile(writeFile(t, dir, "vllm.yml", testEngineYAML))

	// Import model
	modelPath := writeFile(t, dir, "crud-model.yml", `
slug: crud-model
name: "CRUD Model"
engine: vllm
hf_repo: "test/crud"
container: llm-crud
port: 9002
type: llm
command: ["--model"]
capabilities:
  - reasoning
`)
	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(engSvc)
	_, err := modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("ImportModel error: %v", err)
	}

	// List
	models, err := db.ListModels()
	if err != nil {
		t.Fatalf("ListModels error: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("expected 1 model, got %d", len(models))
	}

	// Get
	got, err := db.GetModel("crud-model")
	if err != nil {
		t.Fatalf("GetModel error: %v", err)
	}
	if got.Name != "CRUD Model" {
		t.Errorf("Name = %q, want %q", got.Name, "CRUD Model")
	}

	// Delete
	if err := db.DeleteModel("crud-model"); err != nil {
		t.Fatalf("DeleteModel error: %v", err)
	}

	// Verify deleted
	_, err = db.GetModel("crud-model")
	if err == nil {
		t.Error("expected error after delete")
	}
}

// ── 8. Bad data: engine ────────────────────────────────────────────────────

func TestIntegration_BadData_Engine(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Missing slug
	path := writeFile(t, dir, "no-slug.yml", badEngineNoSlug)
	_, _, err := service.NewEngineService(db).ImportEngineFile(path)
	if err == nil {
		t.Error("expected error for engine without slug")
	}

	// Duplicate slug (idempotent — should skip, not error)
	validEngineYAML := `
engine:
  slug: dup-test
  name: "Dup Test"
  description: "x"
versions:
  - slug: v1
    version: "001"
    image: test/img:latest
    entrypoint: ["python"]
    port: 8000
`
	pathDup := writeFile(t, dir, "dup-valid.yml", validEngineYAML)
	_, _, err = service.NewEngineService(db).ImportEngineFile(pathDup)
	if err != nil {
		t.Fatalf("first import should succeed: %v", err)
	}
	// Second import of same slug should not error (idempotent)
	_, _, err = service.NewEngineService(db).ImportEngineFile(pathDup)
	if err != nil {
		t.Errorf("duplicate import should not error, got: %v", err)
	}

	// Invalid image (no / or : in image ref)
	invalidImgYAML := `
engine:
  slug: bad-img
  name: "Bad Image"
  description: "x"
versions:
  - slug: v1
    version: "001"
    image: badimage
    entrypoint: ["python"]
    port: 8000
`
	path2 := writeFile(t, dir, "bad-img.yml", invalidImgYAML)
	_, _, err = service.NewEngineService(db).ImportEngineFile(path2)
	if err == nil {
		t.Error("expected error for invalid image format")
	}
}

// ── 9. Bad data: model ─────────────────────────────────────────────────────

func TestIntegration_BadData_Model(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Setup engine
	engSvc := service.NewEngineService(db)
	_, _, _ = engSvc.ImportEngineFile(writeFile(t, dir, "vllm.yml", testEngineYAML))

	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(engSvc)

	// No engine field
	noEng := `
slug: no-engine
name: "No Engine"
hf_repo: "test"
container: llm-no-eng
port: 8000
type: llm
command: ["--model"]
`
	path := writeFile(t, dir, "no-eng.yml", noEng)
	_, err := modelSvc.ImportModel(path, service.ImportOverrides{})
	if err == nil {
		t.Error("expected error for model without engine")
	}

	// Invalid engine type
	badEngType := `
slug: bad-eng-type
name: "Bad Engine"
engine: invalid-engine
hf_repo: "test"
container: llm-bad
port: 8000
type: llm
command: ["--model"]
`
	path2 := writeFile(t, dir, "bad-eng.yml", badEngType)
	_, err = modelSvc.ImportModel(path2, service.ImportOverrides{})
	if err == nil {
		t.Error("expected error for invalid engine type")
	}

	// Duplicate slug without override
	path3 := writeFile(t, dir, "dup.yml", duplicateModelYAML)
	// First import
	_, err = modelSvc.ImportModel(path3, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("first import should succeed: %v", err)
	}
	// Second import without override
	_, err = modelSvc.ImportModel(path3, service.ImportOverrides{})
	if err == nil {
		t.Error("expected error for duplicate without --override")
	}
}

// ── 10. Bad data: compose ──────────────────────────────────────────────────

func TestIntegration_BadData_Compose(t *testing.T) {
	db := mustDB(t)

	generator, _ := service.NewComposeGenerator()
	modelSvc := service.NewModelService(db, config.DefaultConfig())

	// Non-existent model
	_, err := modelSvc.GenerateCompose("nonexistent", generator, service.EngineComposeConfig{})
	if err == nil {
		t.Error("expected error for non-existent model")
	}
}

// ── 11. Nil engine service fallback ────────────────────────────────────────

func TestIntegration_NilEngineService(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Create engine type and version
	db.CreateEngineType(&models.EngineType{Slug: "vllm", Name: "vLLM", Description: "x"})
	db.CreateEngineVersion(&models.EngineVersion{
		Slug: "default-v1", EngineTypeSlug: "vllm", Version: "001",
		Image: "cjlapao/pgx-vllm:latest", ContainerName: "vllm-node",
		Entrypoint: "python3 -m vllm.entrypoints.openai.api_server",
		IsDefault: true, IsLatest: true,
		EnvironmentJSON: `{"HF_HUB_OFFLINE":"0"}`, VolumesJSON: `{}`, CommandArgs: `[]`,
	})

	// Import model WITHOUT specifying engine_version — should resolve to default
	modelPath := writeFile(t, dir, "auto-eng.yml", `
slug: auto-eng-model
name: "Auto Engine"
engine: vllm
hf_repo: "test/auto"
container: llm-auto
port: 9003
type: llm
command: ["--model"]
capabilities:
  - reasoning
`)

	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(service.NewEngineService(db))
	m, err := modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("ImportModel error: %v", err)
	}
	if m.EngineVersionSlug != "default-v1" {
		t.Errorf("EngineVersionSlug = %q, want %q", m.EngineVersionSlug, "default-v1")
	}
}

// ── 12. Volume/env deduplication ───────────────────────────────────────────

func TestIntegration_Deduplication(t *testing.T) {
	db := mustDB(t)
	dir := t.TempDir()

	// Engine version with overlapping volumes and env vars
	overlappingEngine := `
engine:
  slug: vllm
  name: "Dedup Test"
  description: "x"
versions:
  - slug: v1
    version: "001"
    container_name: node
    image: test/img:latest
    entrypoint: ["python"]
    default: true
    latest: true
    environment:
      HF_HUB_OFFLINE: "0"
      SHARED_KEY: "from-engine"
    volumes:
      ../models: "/root/.cache/huggingface"
      ../shared: "/data/shared"
    command_args: ["--engine-arg"]
    port: 8000
`
	_, _, err := service.NewEngineService(db).ImportEngineFile(writeFile(t, dir, "dedup.yml", overlappingEngine))
	if err != nil {
		t.Fatalf("ImportEngine error: %v", err)
	}

	// Model with overlapping env vars
	modelPath := writeFile(t, dir, "dedup-model.yml", `
slug: dedup-model
name: "Dedup Model"
engine: vllm
engine_version: v1
hf_repo: "test/dedup"
container: llm-dedup
port: 9004
type: llm
environment:
  SHARED_KEY: "from-model"
  MODEL_ONLY: "yes"
command:
  - "--model"
  - "--model-arg"
capabilities:
  - reasoning
`)

	modelSvc := service.NewModelService(db, config.DefaultConfig())
	modelSvc.SetEngineService(service.NewEngineService(db))
	_, err = modelSvc.ImportModel(modelPath, service.ImportOverrides{})
	if err != nil {
		t.Fatalf("ImportModel error: %v", err)
	}

	// Generate compose and verify merge priority
	generator, _ := service.NewComposeGenerator()
	composeYAML, err := modelSvc.GenerateCompose("dedup-model", generator, service.EngineComposeConfig{})
	if err != nil {
		t.Fatalf("GenerateCompose error: %v", err)
	}

	// Model env var should override engine env var
	assertContains(t, composeYAML, "SHARED_KEY=from-model")
	// Model-only env var should be present
	assertContains(t, composeYAML, "MODEL_ONLY=yes")
	// Engine-only env var should be present
	assertContains(t, composeYAML, "HF_HUB_OFFLINE=0")
	// Model command args should override engine command args
	assertContains(t, composeYAML, "--model")
	assertContains(t, composeYAML, "--model-arg")
	assertNotContains(t, composeYAML, "--engine-arg")
	// Volumes should be present (engine version volumes)
	assertContains(t, composeYAML, "../models:/root/.cache/huggingface")
	assertContains(t, composeYAML, "../shared:/data/shared")
}
