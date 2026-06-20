package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
)

// storageDBForOverwrite extends mockDB with proper storage for engine types
// and versions, plus working UpdateEngineType / UpdateEngineVersion methods
// required by the overwrite import path.
type storageDBForOverwrite struct {
	*mockDB
}

func newStorageDBForOverwrite() *storageDBForOverwrite {
	return &storageDBForOverwrite{mockDB: newMockDB()}
}

func (m *storageDBForOverwrite) CreateEngineType(et *models.EngineType) error {
	m.engineTypes[et.Slug] = et
	return nil
}

// UpdateEngineType persists field updates on the stored EngineType.
func (m *storageDBForOverwrite) UpdateEngineType(slug string, updates map[string]interface{}) error {
	et, ok := m.engineTypes[slug]
	if !ok {
		return fmt.Errorf("engine type %s not found", slug)
	}
	if name, ok := updates["name"].(string); ok {
		et.Name = name
	}
	if desc, ok := updates["description"].(string); ok {
		et.Description = desc
	}
	if prov, ok := updates["provider"].(string); ok {
		et.Provider = prov
	}
	return nil
}

// UpdateEngineVersion persists field updates on the stored EngineVersion.
func (m *storageDBForOverwrite) UpdateEngineVersion(slug string, updates map[string]interface{}) error {
	for i := range m.engineVersions {
		if m.engineVersions[i].Slug == slug {
			if v, ok := updates["version"].(string); ok {
				m.engineVersions[i].Version = v
			}
			if cn, ok := updates["container_name"].(string); ok {
				m.engineVersions[i].ContainerName = cn
			}
			if img, ok := updates["image"].(string); ok {
				m.engineVersions[i].Image = img
			}
			if ep, ok := updates["entrypoint"].(string); ok {
				m.engineVersions[i].Entrypoint = ep
			}
			if def, ok := updates["is_default"].(bool); ok {
				m.engineVersions[i].IsDefault = def
			}
			if lat, ok := updates["is_latest"].(bool); ok {
				m.engineVersions[i].IsLatest = lat
			}
			if ej, ok := updates["environment_json"].(string); ok {
				m.engineVersions[i].EnvironmentJSON = ej
			}
			if vj, ok := updates["volumes_json"].(string); ok {
				m.engineVersions[i].VolumesJSON = vj
			}
			if el, ok := updates["enable_logging"].(bool); ok {
				m.engineVersions[i].EnableLogging = el
			}
			if sa, ok := updates["syslog_address"].(string); ok {
				m.engineVersions[i].SyslogAddress = sa
			}
			if sf, ok := updates["syslog_facility"].(string); ok {
				m.engineVersions[i].SyslogFacility = sf
			}
			if ne, ok := updates["deploy_enable_nvidia"].(bool); ok {
				m.engineVersions[i].DeployEnableNvidia = ne
			}
			if gc, ok := updates["deploy_gpu_count"].(string); ok {
				m.engineVersions[i].DeployGPUCount = gc
			}
			if ca, ok := updates["command_args"].(string); ok {
				m.engineVersions[i].CommandArgs = ca
			}
			if hj, ok := updates["healthcheck_json"].(string); ok {
				m.engineVersions[i].HealthcheckJSON = hj
			}
			if uj, ok := updates["ulimits_json"].(string); ok {
				m.engineVersions[i].UlimitsJSON = uj
			}
			if ipc, ok := updates["ipc"].(string); ok {
				m.engineVersions[i].IPC = ipc
			}
			return nil
		}
	}
	return fmt.Errorf("engine version %s not found", slug)
}

// CreateEngineVersion stores the version in the slice.
func (m *storageDBForOverwrite) CreateEngineVersion(ev *models.EngineVersion) error {
	m.engineVersions = append(m.engineVersions, *ev)
	return nil
}

// GetEngineVersionByTypeAndSlug looks up by both type slug and version slug.
func (m *storageDBForOverwrite) GetEngineVersionByTypeAndSlug(typeSlug, slug string) (*models.EngineVersion, error) {
	for _, v := range m.engineVersions {
		if v.EngineTypeSlug == typeSlug && v.Slug == slug {
			return &v, nil
		}
	}
	return nil, nil
}

// GetEngineVersionBySlugAndType also uses proper type+slug lookup.
func (m *storageDBForOverwrite) GetEngineVersionBySlugAndType(typeSlug, slug string) (*models.EngineVersion, error) {
	for _, v := range m.engineVersions {
		if v.EngineTypeSlug == typeSlug && v.Slug == slug {
			return &v, nil
		}
	}
	return nil, nil
}

// EngineVersionExistsByTypeAndSlug checks existence by type+slug.
func (m *storageDBForOverwrite) EngineVersionExistsByTypeAndSlug(typeSlug, slug string) (bool, error) {
	for _, v := range m.engineVersions {
		if v.EngineTypeSlug == typeSlug && v.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}

// EngineTypeExists checks existence by slug.
func (m *storageDBForOverwrite) EngineTypeExists(slug string) (bool, error) {
	_, ok := m.engineTypes[slug]
	return ok, nil
}

// CreateOrSkipEngineType is a no-op for overwrite tests (we manage storage ourselves).
func (m *storageDBForOverwrite) CreateOrSkipEngineType(_ *models.EngineType) (bool, error) {
	return false, nil
}

// CreateOrSkipEngineVersion is a no-op for overwrite tests.
func (m *storageDBForOverwrite) CreateOrSkipEngineVersion(_ *models.EngineVersion) (bool, error) {
	return false, nil
}

// ============================================================================
// Test 1: TestImportEngineFile_Overwrite_NewEngine
// ============================================================================

func TestImportEngineFile_Overwrite_NewEngine(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: overwrite-new
  name: Overwrite New Engine
  description: A new engine tested via overwrite import
  provider: vllm
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
    latest: true
  - slug: v020
    version: "0.2.0"
    image: myorg/myengine:0.2.0
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	created, updated, skipped, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("ImportEngineFile returned error: %v", err)
	}
	// created counts engine type (1) + versions (2) = 3; updated=0; skipped=0
	if created != 3 {
		t.Errorf("expected created=3 (1 type + 2 versions), got %d", created)
	}
	if updated != 0 {
		t.Errorf("expected updated=0, got %d", updated)
	}
	if skipped != 0 {
		t.Errorf("expected skipped=0, got %d", skipped)
	}

	et, err := svc.GetEngineTypeBySlug("overwrite-new")
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

// ============================================================================
// Test 2: TestImportEngineFile_Overwrite_UpdateTypeName
// ============================================================================

func TestImportEngineFile_Overwrite_UpdateTypeName(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	// First import: non-overwrite mode — creates engine type with "Old Name"
	yamlContent1 := `
engine:
  slug: typename-update
  name: Old Name
  description: Initial import
  provider: custom
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent1), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	created1, _, skipped1, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: false})
	if err != nil {
		t.Fatalf("first ImportEngineFile returned error: %v", err)
	}
	if created1 != 1 {
		t.Errorf("expected created=1 on first import, got %d", created1)
	}
	if skipped1 != 0 {
		t.Errorf("expected skipped=0 on first import, got %d", skipped1)
	}

	// Second import: overwrite mode — same slug, different name
	yamlContent2 := `
engine:
  slug: typename-update
  name: New Name
  description: Updated description
  provider: custom
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent2), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	created2, updated2, skipped2, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("second ImportEngineFile returned error: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected created=0 on overwrite, got %d", created2)
	}
	// updated counts engine type (1) + version (1) = 2
	if updated2 != 2 {
		t.Errorf("expected updated=2 (1 type + 1 version), got %d", updated2)
	}
	if skipped2 != 0 {
		t.Errorf("expected skipped=0 on overwrite, got %d", skipped2)
	}

	// Verify the DB record has the new name
	et, err := svc.GetEngineTypeBySlug("typename-update")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug returned error: %v", err)
	}
	if et == nil {
		t.Fatal("engine type is nil")
	}
	if et.Name != "New Name" {
		t.Errorf("expected Name == 'New Name', got %q", et.Name)
	}
}

// ============================================================================
// Test 3: TestImportEngineFile_Overwrite_UpsertVersions
// ============================================================================

func TestImportEngineFile_Overwrite_UpsertVersions(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	// First import: 2 versions (v1, v2)
	yamlContent1 := `
engine:
  slug: upsert-versions
  name: Upsert Versions Engine
  description: Test upsert versions
  provider: custom
versions:
  - slug: v1
    version: "0.1.0"
    image: myorg/myengine:v1
    default: true
    latest: true
  - slug: v2
    version: "0.2.0"
    image: myorg/myengine:v2
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent1), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	created1, _, skipped1, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("first ImportEngineFile returned error: %v", err)
	}
	// created counts engine type (1) + versions (2) = 3
	if created1 != 3 {
		t.Errorf("expected created=3 (1 type + 2 versions), got %d", created1)
	}
	if skipped1 != 0 {
		t.Errorf("expected skipped=0, got %d", skipped1)
	}

	// Second import: v1 (updated image) + v3 (new), no v2
	yamlContent2 := `
engine:
  slug: upsert-versions
  name: Upsert Versions Engine
  description: Test upsert versions
  provider: custom
versions:
  - slug: v1
    version: "0.1.1"
    image: myorg/myengine:v1-updated
    default: true
    latest: true
  - slug: v3
    version: "0.3.0"
    image: myorg/myengine:v3
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent2), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	created2, updated2, skipped2, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("second ImportEngineFile returned error: %v", err)
	}
	// created: v3 is new (1), updated: v1 updated (1) + type updated (1) = 2
	if created2 != 1 {
		t.Errorf("expected created=1 (v3), got %d", created2)
	}
	if updated2 != 2 {
		t.Errorf("expected updated=2 (v1 + type), got %d", updated2)
	}
	if skipped2 != 0 {
		t.Errorf("expected skipped=0, got %d", skipped2)
	}

	// Verify v2 still exists unchanged
	v2Found := false
	for _, v := range db.engineVersions {
		if v.Slug == "v2" {
			v2Found = true
			if v.Image != "myorg/myengine:v2" {
				t.Errorf("expected v2 image to be unchanged, got %q", v.Image)
			}
			break
		}
	}
	if !v2Found {
		t.Error("expected v2 to still exist in DB")
	}

	// Verify v1 was updated
	for _, v := range db.engineVersions {
		if v.Slug == "v1" {
			if v.Image != "myorg/myengine:v1-updated" {
				t.Errorf("expected v1 image updated, got %q", v.Image)
			}
			if v.Version != "0.1.1" {
				t.Errorf("expected v1 version updated, got %q", v.Version)
			}
			break
		}
	}

	// Verify v3 was inserted
	v3Found := false
	for _, v := range db.engineVersions {
		if v.Slug == "v3" {
			v3Found = true
			if v.Image != "myorg/myengine:v3" {
				t.Errorf("expected v3 image, got %q", v.Image)
			}
			break
		}
	}
	if !v3Found {
		t.Error("expected v3 to be inserted")
	}
}

// ============================================================================
// Test 4: TestImportEngineFile_Overwrite_UpdateProvider
// ============================================================================

func TestImportEngineFile_Overwrite_UpdateProvider(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	// First import: provider = custom
	yamlContent1 := `
engine:
  slug: provider-update
  name: Provider Update Engine
  description: Test provider update
  provider: custom
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent1), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	created1, _, skipped1, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("first ImportEngineFile returned error: %v", err)
	}
	// created counts engine type (1) + version (1) = 2
	if created1 != 2 {
		t.Errorf("expected created=2 (1 type + 1 version), got %d", created1)
	}
	if skipped1 != 0 {
		t.Errorf("expected skipped=0, got %d", skipped1)
	}

	et1, _ := svc.GetEngineTypeBySlug("provider-update")
	if et1.Provider != "custom" {
		t.Errorf("expected initial Provider == 'custom', got %q", et1.Provider)
	}

	// Second import: provider = vllm (overwrite mode)
	yamlContent2 := `
engine:
  slug: provider-update
  name: Provider Update Engine
  description: Test provider update
  provider: vllm
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent2), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	created2, updated2, skipped2, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("second ImportEngineFile returned error: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected created=0, got %d", created2)
	}
	// updated counts engine type (1) + version (1) = 2
	if updated2 != 2 {
		t.Errorf("expected updated=2 (1 type + 1 version), got %d", updated2)
	}
	if skipped2 != 0 {
		t.Errorf("expected skipped=0, got %d", skipped2)
	}

	et2, err := svc.GetEngineTypeBySlug("provider-update")
	if err != nil {
		t.Fatalf("GetEngineTypeBySlug returned error: %v", err)
	}
	if et2 == nil {
		t.Fatal("engine type is nil")
	}
	if et2.Provider != "vllm" {
		t.Errorf("expected Provider == 'vllm' after overwrite, got %q", et2.Provider)
	}
}

// ============================================================================
// Test 5: TestImportEngineFile_Overwrite_VersionFields
// ============================================================================

func TestImportEngineFile_Overwrite_VersionFields(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	// First import: all version fields with specific values
	yamlContent1 := `
engine:
  slug: version-fields
  name: Version Fields Engine
  description: Test all version fields
  provider: custom
versions:
  - slug: v010
    version: "0.1.0"
    container_name: my-container
    image: myorg/myengine:0.1.0
    entrypoint: ["/bin/start.sh", "--verbose"]
    default: true
    latest: true
    environment:
      HF_TOKEN: secret123
      LOG_LEVEL: debug
    volumes:
      ../models: /root/.cache/huggingface
      ../data: /mnt/data
    logging:
      enable: true
      address: udp://127.0.0.1:514
      facility: local3
    nvidia: true
    gpu_count: "2"
    command_args: ["--model", "test"]
    healthcheck:
      test: /usr/bin/check
      interval: 30
      retries: 3
    ulimits:
      nofile: 65535
    ipc: host
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent1), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	created1, _, skipped1, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("first ImportEngineFile returned error: %v", err)
	}
	// created counts engine type (1) + version (1) = 2
	if created1 != 2 {
		t.Errorf("expected created=2 (1 type + 1 version), got %d", created1)
	}
	if skipped1 != 0 {
		t.Errorf("expected skipped=0, got %d", skipped1)
	}

	// Store original values for comparison
	var original models.EngineVersion
	for _, v := range db.engineVersions {
		if v.Slug == "v010" {
			original = v
			break
		}
	}

	// Second import: overwrite with different values
	yamlContent2 := `
engine:
  slug: version-fields
  name: Version Fields Engine
  description: Test all version fields
  provider: custom
versions:
  - slug: v010
    version: "0.2.0"
    container_name: updated-container
    image: myorg/myengine:0.2.0
    entrypoint: ["/bin/run.sh"]
    default: true
    latest: true
    environment:
      HF_TOKEN: newsecret
      NEW_VAR: added
    volumes:
      ../models: /root/.cache/huggingface
      ../new-data: /mnt/newdata
    logging:
      enable: false
      address: udp://10.0.0.1:514
      facility: local0
    nvidia: false
    gpu_count: "all"
    command_args: ["--new-flag"]
    healthcheck:
      test: /usr/bin/new-check
      interval: 60
      retries: 5
    ulimits:
      nproc: 32768
    ipc: share
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent2), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	created2, updated2, skipped2, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err != nil {
		t.Fatalf("second ImportEngineFile returned error: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected created=0, got %d", created2)
	}
	// updated counts engine type (1) + version (1) = 2
	if updated2 != 2 {
		t.Errorf("expected updated=2 (1 type + 1 version), got %d", updated2)
	}
	if skipped2 != 0 {
		t.Errorf("expected skipped=0, got %d", skipped2)
	}

	// Verify ALL fields were updated
	var updatedVersion models.EngineVersion
	for _, v := range db.engineVersions {
		if v.Slug == "v010" {
			updatedVersion = v
			break
		}
	}

	checks := []struct {
		field  string
		got    string
		expect string
	}{
		{"version", updatedVersion.Version, "0.2.0"},
		{"container_name", updatedVersion.ContainerName, "updated-container"},
		{"image", updatedVersion.Image, "myorg/myengine:0.2.0"},
		{"entrypoint", updatedVersion.Entrypoint, "/bin/run.sh"},
		{"command_args", updatedVersion.CommandArgs, `["--new-flag"]`},
		{"syslog_address", updatedVersion.SyslogAddress, "udp://10.0.0.1:514"},
		{"syslog_facility", updatedVersion.SyslogFacility, "local0"},
		{"deploy_gpu_count", updatedVersion.DeployGPUCount, "all"},
		{"ipc", updatedVersion.IPC, "share"},
	}

	for _, tc := range checks {
		if tc.got != tc.expect {
			t.Errorf("field %s: expected %q, got %q", tc.field, tc.expect, tc.got)
		}
	}

	// Boolean fields
	if updatedVersion.EnableLogging != false {
		t.Errorf("enable_logging: expected false, got %v", updatedVersion.EnableLogging)
	}
	if updatedVersion.EnableLogging == original.EnableLogging {
		t.Error("enable_logging should have changed from true to false")
	}

	if updatedVersion.DeployEnableNvidia != false {
		t.Errorf("deploy_enable_nvidia: expected false, got %v", updatedVersion.DeployEnableNvidia)
	}
	if updatedVersion.DeployEnableNvidia == original.DeployEnableNvidia {
		t.Error("deploy_enable_nvidia should have changed from true to false")
	}

	// Check JSON fields were updated
	env := updatedVersion.GetEnvironment()
	if env["HF_TOKEN"] != "newsecret" {
		t.Errorf("environment HF_TOKEN: expected 'newsecret', got %q", env["HF_TOKEN"])
	}
	if env["NEW_VAR"] != "added" {
		t.Errorf("environment NEW_VAR: expected 'added', got %q", env["NEW_VAR"])
	}

	vols := updatedVersion.GetVolumes()
	if vols["../new-data"] != "/mnt/newdata" {
		t.Errorf("volumes ../new-data: expected '/mnt/newdata', got %q", vols["../new-data"])
	}

	hc := updatedVersion.GetHealthcheck()
	if hc["test"] != "/usr/bin/new-check" {
		t.Errorf("healthcheck test: expected '/usr/bin/new-check', got %v", hc["test"])
	}

	ul := updatedVersion.GetUlimits()
	if ul["nproc"] != float64(32768) {
		t.Errorf("ulimits nproc: expected 32768, got %v", ul["nproc"])
	}
}

// ============================================================================
// Test 6: TestImportEngineFile_NoOverwrite_SkipsExisting
// ============================================================================

func TestImportEngineFile_NoOverwrite_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: skip-existing
  name: Skip Existing Engine
  description: Test non-overwrite skip behavior
  provider: custom
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
    latest: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	// First import: should create
	created1, updated1, skipped1, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: false})
	if err != nil {
		t.Fatalf("first ImportEngineFile returned error: %v", err)
	}
	if created1 != 1 {
		t.Errorf("expected created=1 on first import, got %d", created1)
	}
	if updated1 != 0 {
		t.Errorf("expected updated=0 on first import, got %d", updated1)
	}
	if skipped1 != 0 {
		t.Errorf("expected skipped=0 on first import, got %d", skipped1)
	}

	// Second import: same YAML, non-overwrite mode — should skip
	created2, updated2, skipped2, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: false})
	if err != nil {
		t.Fatalf("second ImportEngineFile returned error: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected created=0 on second import (skip), got %d", created2)
	}
	if updated2 != 0 {
		t.Errorf("expected updated=0 on second import, got %d", updated2)
	}
	if skipped2 != 1 {
		t.Errorf("expected skipped=1 on second import, got %d", skipped2)
	}
}

// ============================================================================
// Test 7: TestImportEngineFile_Overwrite_IgnoredProviderValidation
// ============================================================================

func TestImportEngineFile_Overwrite_IgnoredProviderValidation(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "engine.yaml")

	yamlContent := `
engine:
  slug: invalid-provider
  name: Invalid Provider Engine
  description: Should fail due to invalid provider
  provider: unknown-provider
versions:
  - slug: v010
    version: "0.1.0"
    image: myorg/myengine:0.1.0
    default: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}

	db := newStorageDBForOverwrite()
	svc := NewEngineService(db)

	created, updated, skipped, err := svc.ImportEngineFile(yamlPath, EngineImportOverrides{Overwrite: true})
	if err == nil {
		t.Fatalf("expected error for invalid provider, got nil")
	}
	if created != 0 {
		t.Errorf("expected created=0, got %d", created)
	}
	if updated != 0 {
		t.Errorf("expected updated=0, got %d", updated)
	}
	if skipped != 0 {
		t.Errorf("expected skipped=0, got %d", skipped)
	}
	if !contains(err.Error(), "invalid provider") {
		t.Errorf("error should mention 'invalid provider': %v", err)
	}

	// Verify no engine type was created (no partial updates)
	et, _ := svc.GetEngineTypeBySlug("invalid-provider")
	if et != nil {
		t.Errorf("expected nil engine type for rejected import, got %+v", et)
	}
}
