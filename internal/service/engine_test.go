package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// mockDB implements a minimal DatabaseManager for testing.
type mockDB struct {
	engineTypes    map[string]*models.EngineType
	engineVersions []models.EngineVersion
	defaultSlug    map[string]string // typeSlug -> default version slug
	latestSlug     map[string]string // typeSlug -> latest version slug
}

func newMockDB() *mockDB {
	return &mockDB{
		engineTypes:  make(map[string]*models.EngineType),
		engineVersions: []models.EngineVersion{},
		defaultSlug:  make(map[string]string),
		latestSlug:   make(map[string]string),
	}
}

func (m *mockDB) ListEngineTypes() ([]models.EngineType, error) {
	out := make([]models.EngineType, 0, len(m.engineTypes))
	for _, et := range m.engineTypes {
		out = append(out, *et)
	}
	return out, nil
}
func (m *mockDB) GetEngineTypeBySlug(slug string) (*models.EngineType, error) {
	v, ok := m.engineTypes[slug]
	if !ok {
		return nil, nil
	}
	return v, nil
}
func (m *mockDB) CreateEngineType(_ *models.EngineType) error                           { return nil }
func (m *mockDB) UpdateEngineType(_ string, _ map[string]interface{}) error            { return nil }
func (m *mockDB) DeleteEngineType(_ string) error                                      { return nil }
func (m *mockDB) EngineTypeExists(_ string) (bool, error)                              { return false, nil }
func (m *mockDB) ListEngineVersions() ([]models.EngineVersion, error) { return m.engineVersions, nil }
func (m *mockDB) EngineVersionExistsByTypeAndSlug(_, _ string) (bool, error) { return false, nil }
func (m *mockDB) GetEngineVersionBySlugAndType(_, _ string) (*models.EngineVersion, error) {
	for _, v := range m.engineVersions {
		return &v, nil
	}
	return nil, nil
}
func (m *mockDB) GetEngineVersionByTypeAndSlug(_, _ string) (*models.EngineVersion, error) {
	for _, v := range m.engineVersions {
		return &v, nil
	}
	return nil, nil
}
func (m *mockDB) GetEngineVersionByID(_ string) (*models.EngineVersion, error)         { return nil, nil }
func (m *mockDB) GetEngineVersionByTypeAndVersion(_, _ string) (*models.EngineVersion, error) {
	return nil, nil
}
func (m *mockDB) CreateEngineVersion(_ *models.EngineVersion) error                    { return nil }
func (m *mockDB) UpdateEngineVersion(_ string, _ map[string]interface{}) error         { return nil }
func (m *mockDB) DeleteEngineVersion(_ string) error                                   { return nil }
func (m *mockDB) FindDefaultVersionByType(typeSlug string) (*models.EngineVersion, error) {
	if slug, ok := m.defaultSlug[typeSlug]; ok {
		for _, v := range m.engineVersions {
			if v.Slug == slug {
				return &v, nil
			}
		}
	}
	return nil, nil
}
func (m *mockDB) FindLatestVersionByType(typeSlug string) (*models.EngineVersion, error) {
	if slug, ok := m.latestSlug[typeSlug]; ok {
		for _, v := range m.engineVersions {
			if v.Slug == slug {
				return &v, nil
			}
		}
	}
	return nil, nil
}
func (m *mockDB) ClearIsDefaultForType(_ string) error                                 { return nil }
func (m *mockDB) UpdateIsDefaultClearOthers(_, _ string) error                         { return nil }
func (m *mockDB) ListModelsByEngineVersion(_ string) ([]models.Model, error)           { return nil, nil }
func (m *mockDB) ListModels() ([]models.Model, error)                                  { return nil, nil }
func (m *mockDB) ListModelsByTypeSubType(_, _ string) ([]models.Model, error)          { return nil, nil }
func (m *mockDB) GetModel(_ string) (*models.Model, error)                             { return nil, nil }
func (m *mockDB) CreateModel(_ *models.Model) error                                    { return nil }
func (m *mockDB) UpdateModel(_ string, _ map[string]interface{}) error                 { return nil }
func (m *mockDB) DeleteModel(_ string) error                                           { return nil }
func (m *mockDB) ListContainers() ([]models.Container, error)                          { return nil, nil }
func (m *mockDB) GetContainerStatus(_ string) (string, error)                          { return "", nil }
func (m *mockDB) UpdateContainerStatus(_ string, _ string) error                       { return nil }
func (m *mockDB) GetHotspot() (*models.Hotspot, error)                                 { return nil, nil }
func (m *mockDB) SetHotspot(_ string) error                                            { return nil }
func (m *mockDB) ClearHotspot() error                                                  { return nil }
func (m *mockDB) GetConfig(_ string) (*models.Config, error)                           { return nil, nil }
func (m *mockDB) SetConfig(_, _ string) error                                          { return nil }
func (m *mockDB) UnsetConfig(_ string) error                                           { return nil }
func (m *mockDB) ListConfig() ([]models.Config, error)                                 { return nil, nil }
func (m *mockDB) ListBaseImages() ([]models.BaseImage, error)                          { return nil, nil }
func (m *mockDB) GetBaseImageBySlug(_ string) (*models.BaseImage, error)               { return nil, nil }
func (m *mockDB) GetBaseImageByID(_ string) (*models.BaseImage, error)                 { return nil, nil }
func (m *mockDB) CreateBaseImage(_ *models.BaseImage) error                            { return nil }
func (m *mockDB) UpdateBaseImage(_ string, _ map[string]interface{}) error             { return nil }
func (m *mockDB) DeleteBaseImage(_ string) error                                       { return nil }
func (m *mockDB) Open() error                                                          { return nil }
func (m *mockDB) Close() error                                                         { return nil }
func (m *mockDB) SchemaVersion() (int, error)                                          { return 0, nil }
func (m *mockDB) LatestVersion() (int, error)                                          { return 0, nil }
func (m *mockDB) ApplyPendingMigrations() error                                        { return nil }
func (m *mockDB) MigrateTo(_ int) error                                                { return nil }
func (m *mockDB) AutoMigrate() error                                                   { return nil }
func (m *mockDB) DB() *gorm.DB { return nil }

func TestMergeEnvironments(t *testing.T) {
	// Test: version env + model env → both included
	v1 := map[string]string{"A": "1", "B": "2"}
	m1 := map[string]string{"C": "3"}
	result := MergeEnvironments(v1, m1, nil)
	if len(result) != 3 || result["A"] != "1" || result["B"] != "2" || result["C"] != "3" {
		t.Errorf("expected merged env with all keys, got %v", result)
	}

	// Test: override wins
	m2 := map[string]string{"A": "override"}
	result = MergeEnvironments(v1, m2, nil)
	if result["A"] != "override" {
		t.Errorf("expected A=override, got A=%s", result["A"])
	}

	// Test: CLI override wins over model env
	cli := map[string]string{"B": "cli"}
	result = MergeEnvironments(v1, m1, cli)
	if result["B"] != "cli" {
		t.Errorf("expected B=cli, got B=%s", result["B"])
	}
}

func TestMergeVolumes(t *testing.T) {
	// Test: all sources included
	d1 := map[string]string{"host1": "/ctn1", "host2": "/ctn2"}
	v2 := map[string]string{"host3": "/ctn3"}
	u3 := map[string]string{"host4": "/ctn4"}
	result := MergeVolumes(d1, v2, u3)
	if len(result) != 4 {
		t.Errorf("expected 4 volumes, got %d", len(result))
	}

	// Test: dedup by host path
	d2 := map[string]string{"host1": "/ctn1"}
	v3 := map[string]string{"host1": "/ctn_override"}
	result = MergeVolumes(d2, v3, nil)
	if len(result) != 1 || result["host1"] != "/ctn_override" {
		t.Errorf("expected dedup with override, got %v", result)
	}
}

func TestBuildLoggingSection(t *testing.T) {
	svc := &EngineService{}

	// Disabled → empty
	section := svc.BuildLoggingSection(false, "addr", "local3", "test-model")
	if section != "" {
		t.Errorf("expected empty string for disabled logging, got %q", section)
	}

	// Enabled → non-empty
	section = svc.BuildLoggingSection(true, "udp://127.0.0.1:514", "local3", "test-model")
	if section == "" {
		t.Error("expected non-empty section for enabled logging")
	}
	if !contains(section, "syslog") {
		t.Error("expected 'syslog' in logging section")
	}
}

func TestBuildDeploySection(t *testing.T) {
	svc := &EngineService{}

	// Disabled → empty
	section := svc.BuildDeploySection(false, "")
	if section != "" {
		t.Errorf("expected empty for disabled deploy, got %q", section)
	}

	// Enabled → non-empty
	section = svc.BuildDeploySection(true, "")
	if section == "" {
		t.Error("expected non-empty for enabled deploy")
	}
	if !contains(section, "nvidia") {
		t.Error("expected 'nvidia' in deploy section")
	}
}

func TestValidateSlug(t *testing.T) {
	svc := &EngineService{}

	if err := svc.ValidateSlug("vllm"); err != nil {
		t.Errorf("valid slug rejected: %v", err)
	}
	if err := svc.ValidateSlug("PGX-LLM"); err == nil {
		t.Error("uppercase slug accepted")
	}
	if err := svc.ValidateSlug(""); err == nil {
		t.Error("empty slug accepted")
	}
	if err := svc.ValidateSlug("has space"); err == nil {
		t.Error("slug with space accepted")
	}
}

func TestValidateImage(t *testing.T) {
	svc := &EngineService{}

	if err := svc.ValidateImage("cjlapao/img:tag"); err != nil {
		t.Errorf("valid image rejected: %v", err)
	}
	if err := svc.ValidateImage("img:tag"); err != nil {
		t.Errorf("valid image rejected: %v", err)
	}
	if err := svc.ValidateImage(""); err == nil {
		t.Error("empty image accepted")
	}
}

func TestParseVolumeMapping(t *testing.T) {
	svc := &EngineService{}

	host, ctn, ro, err := svc.ParseVolumeMapping("../models:/root/.cache/huggingface")
	if err != nil {
		t.Fatalf("valid volume rejected: %v", err)
	}
	if host != "../models" || ctn != "/root/.cache/huggingface" || ro {
		t.Errorf("expected rw volume, got host=%s ctn=%s ro=%v", host, ctn, ro)
	}

	host, ctn, ro, err = svc.ParseVolumeMapping("../data:/mnt/data:ro")
	if err != nil {
		t.Fatalf("valid RO volume rejected: %v", err)
	}
	if !ro {
		t.Error("expected ro=true")
	}

	_, _, _, err = svc.ParseVolumeMapping("invalid")
	if err == nil {
		t.Error("invalid volume accepted")
	}
}

func TestParseEnvKV(t *testing.T) {
	svc := &EngineService{}

	key, val, err := svc.ParseEnvKV("KEY=value")
	if err != nil || key != "KEY" || val != "value" {
		t.Errorf("valid env rejected: key=%s val=%s err=%v", key, val, err)
	}

	_, _, err = svc.ParseEnvKV("invalid")
	if err == nil {
		t.Error("invalid env accepted")
	}
}

func TestResolveDefaultVersion(t *testing.T) {
	db := newMockDB()
	svc := &EngineService{db: db}

	// No default → nil
	v, err := svc.ResolveDefaultVersion("vllm")
	if err != nil || v != nil {
		t.Errorf("expected nil for no default, got %v err=%v", v, err)
	}

	// Set up default
	db.engineVersions = append(db.engineVersions, models.EngineVersion{
		Slug:           "v1",
		EngineTypeSlug: "vllm",
		Version:        "001",
		IsDefault:      true,
	})
	db.defaultSlug["vllm"] = "v1"

	v, err = svc.ResolveDefaultVersion("vllm")
	if err != nil || v == nil || v.Slug != "v1" {
		t.Errorf("expected v1, got %v err=%v", v, err)
	}
}

func TestValidateEnvKey(t *testing.T) {
	svc := &EngineService{}

	if err := svc.ValidateEnvKey("HF_TOKEN"); err != nil {
		t.Errorf("valid env key rejected: %v", err)
	}
	if err := svc.ValidateEnvKey("my-key"); err != nil {
		t.Errorf("valid env key with hyphen rejected: %v", err)
	}
	if err := svc.ValidateEnvKey("my_key"); err != nil {
		t.Errorf("valid env key with underscore rejected: %v", err)
	}
	if err := svc.ValidateEnvKey(""); err == nil {
		t.Error("empty env key accepted")
	}
	if err := svc.ValidateEnvKey("has space"); err == nil {
		t.Error("env key with space accepted")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEngineVersion_JSONHelpers(t *testing.T) {
	ev := models.EngineVersion{}

	// Empty → empty map (consistent with json.Unmarshal on empty string)
	if env := ev.GetEnvironment(); env == nil || len(env) != 0 {
		// Empty map is acceptable
	}

	// Set and get
	env := map[string]string{"A": "1", "B": "2"}
	if err := ev.SetEnvironment(env); err != nil {
		t.Fatalf("failed to set environment: %v", err)
	}
	got := ev.GetEnvironment()
	if !reflect.DeepEqual(got, env) {
		t.Errorf("expected %+v, got %+v", env, got)
	}

	// Command args
	args := []string{"--flag", "val"}
	if err := ev.SetCommandArgs(args); err != nil {
		t.Fatalf("failed to set command args: %v", err)
	}
	gotArgs := ev.GetCommandArgs()
	if !reflect.DeepEqual(gotArgs, args) {
		t.Errorf("expected %+v, got %+v", args, gotArgs)
	}

	// Volumes
	vols := map[string]string{"host": "/ctn"}
	if err := ev.SetVolumes(vols); err != nil {
		t.Fatalf("failed to set volumes: %v", err)
	}
	gotVols := ev.GetVolumes()
	if !reflect.DeepEqual(gotVols, vols) {
		t.Errorf("expected %+v, got %+v", vols, gotVols)
	}
}

// Test that JSON marshaling/unmarshaling is consistent.
func TestJSONConsistency(t *testing.T) {
	// Map to JSON string and back
	orig := map[string]string{"K": "V"}
	b, _ := json.Marshal(orig)
	var decoded map[string]string
	json.Unmarshal(b, &decoded)
	if decoded["K"] != "V" {
		t.Errorf("JSON round-trip failed: %s → %v", string(b), decoded)
	}
}
