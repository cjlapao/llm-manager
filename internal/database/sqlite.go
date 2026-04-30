package database

import (
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/user/llm-manager/internal/database/migrations"
	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqliteManager implements DatabaseManager using SQLite.
type sqliteManager struct {
	dsn string
	db  *gorm.DB
	mg  *migrations.Engine
}

// NewDatabaseManager creates a new SQLite-backed DatabaseManager.
func NewDatabaseManager(dsn string) (DatabaseManager, error) {
	return &sqliteManager{dsn: dsn}, nil
}

// Open opens the database connection.
func (m *sqliteManager) Open() error {
	db, err := gorm.Open(sqlite.Open(m.dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	m.db = db

	// Create the migrations engine
	m.mg, err = migrations.NewEngine(db)
	if err != nil {
		return fmt.Errorf("failed to create migration engine: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (m *sqliteManager) Close() error {
	if m.db == nil {
		return nil
	}
	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// SchemaVersion returns the current schema version from applied migrations.
func (m *sqliteManager) SchemaVersion() (int, error) {
	if m.mg == nil {
		return 0, fmt.Errorf("migration engine not initialized")
	}
	return m.mg.CurrentVersion()
}

// LatestVersion returns the latest migration version known in code.
func (m *sqliteManager) LatestVersion() (int, error) {
	if m.mg == nil {
		return 0, fmt.Errorf("migration engine not initialized")
	}
	return m.mg.LatestVersion(), nil
}

// ApplyPendingMigrations runs all pending up-migrations.
func (m *sqliteManager) ApplyPendingMigrations() error {
	if m.mg == nil {
		return fmt.Errorf("migration engine not initialized")
	}
	fmt.Println("Checking for pending migrations...")
	if err := m.ensureLegacyColumns(); err != nil {
		return fmt.Errorf("legacy column check failed: %w", err)
	}
	if err := m.mg.ApplyUp(); err != nil {
		return fmt.Errorf("pending migrations failed: %w", err)
	}
	return nil
}

// ensureLegacyColumns adds any columns that may be missing from pre-migration
// databases. Old databases created the models table directly without these columns.
// This runs BEFORE the migration engine so that the engine can safely assume
// the table schema is complete after migration 001 creates it.
func (m *sqliteManager) ensureLegacyColumns() error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// Skip if the models table doesn't exist yet (fresh database — migrations will create it)
	var exists int
	if err := m.db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='models'").Scan(&exists).Error; err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	columns := []struct {
		table  string
		col    string
		def    string
	}{
		{"models", "sub_type", "TEXT"},
		{"models", "engine_type", "TEXT DEFAULT 'vllm'"},
		{"models", "env_vars", "TEXT"},
		{"models", "command_args", "TEXT"},
		{"models", "input_token_cost", "REAL DEFAULT 0"},
		{"models", "output_token_cost", "REAL DEFAULT 0"},
		{"models", "capabilities", "TEXT"},
		{"models", "lite_llm_params", "TEXT"},
		{"models", "model_info", "TEXT"},
		{"models", "litellm_model_id", "TEXT"},
		{"models", "litellm_active_aliases", "TEXT"},
		{"models", "litellm_variant_ids", "TEXT"},
		{"models", "base_image_id", "TEXT DEFAULT ''"},
		{"models", "default", "BOOLEAN DEFAULT 0"},
	}
	for _, c := range columns {
		colRef := c.col
		if c.col == "default" {
			colRef = "`default`"
		}
		if err := m.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, colRef, c.def)).Error; err != nil {
			// Ignore "duplicate column" errors — column already exists
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("add column %s.%s: %w", c.table, c.col, err)
			}
		}
	}
	return nil
}

// MigrateTo migrates to a specific target schema version.
func (m *sqliteManager) MigrateTo(targetVersion int) error {
	if m.mg == nil {
		return fmt.Errorf("migration engine not initialized")
	}

	currentVersion, err := m.SchemaVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	maxVersion := m.mg.LatestVersion()
	if targetVersion > maxVersion {
		return fmt.Errorf("target version %d exceeds latest known migration %d", targetVersion, maxVersion)
	}

	// Get migrations we need to apply or roll back
	allMigrations := m.mg.Migrations()

	if targetVersion > currentVersion {
		// Up migration
		count := 0
		for _, mig := range allMigrations {
			if mig.Version > currentVersion && mig.Version <= targetVersion {
				if err := m.mg.RunUp(mig); err != nil {
					return fmt.Errorf("migration up to %d failed on %d: %w", targetVersion, mig.Version, err)
				}
				count++
			}
		}
		fmt.Printf("Applied %d up migrations to reach version %d\n", count, targetVersion)
	} else if targetVersion < currentVersion {
		// Down migration - reverse order, applied last first
		count := 0
		for i := len(allMigrations) - 1; i >= 0; i-- {
			mig := allMigrations[i]
			if mig.Version > targetVersion && mig.Version <= currentVersion {
				if err := m.mg.RunDown(mig); err != nil {
					return fmt.Errorf("down migration to %d failed on %d: %w", targetVersion, mig.Version, err)
				}
				count++
			}
		}
		fmt.Printf("Rolled back %d migrations to reach version %d\n", count, targetVersion)
	} else {
		fmt.Println("Already at target version")
	}

	return nil
}

// DB returns the underlying GORM database instance.
func (m *sqliteManager) DB() *gorm.DB {
	return m.db
}

// AutoMigrate runs the migration engine to ensure schema is up to date.
// Kept for backward compatibility with existing tests and code.
func (m *sqliteManager) AutoMigrate() error {
	if m.mg == nil {
		return fmt.Errorf("migration engine not initialized")
	}
	fmt.Println("Running migration engine (backward compatible)...")
	return m.ApplyPendingMigrations()
}

// SuppressRecordNotFound wraps a GORM query to suppress "record not found" log noise.
// Use this for existence checks where "not found" is expected behavior.
func SuppressRecordNotFound(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

// LogDB returns a GORM DB instance with verbose logging for debugging.
func LogDB(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Info),
	})
}

// ListModels returns all models in the database, sorted by slug.
func (m *sqliteManager) ListModels() ([]models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var models []models.Model
	if err := m.db.Order("slug ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	return models, nil
}

// ListModelsByTypeSubType returns models matching the given type and subType,
// ordered with Default models first, then alphabetically by slug.
func (m *sqliteManager) ListModelsByTypeSubType(modelType string, subType string) ([]models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var models []models.Model
	if err := m.db.Where("type = ? AND sub_type = ?", modelType, subType).Order("`default` DESC, slug ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to list models by type/subType: %w", err)
	}
	return models, nil
}

// GetModel returns a single model by its slug.
func (m *sqliteManager) GetModel(slug string) (*models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var model models.Model
	if err := m.db.Where("slug = ?", slug).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("model %s not found", slug)
		}
		return nil, fmt.Errorf("failed to get model %s: %w", slug, err)
	}
	return &model, nil
}

// CreateModel creates a new model in the database.
func (m *sqliteManager) CreateModel(model *models.Model) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(model).Error; err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}
	return nil
}

// UpdateModel updates a model by slug with the provided field updates.
func (m *sqliteManager) UpdateModel(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.Model{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update model %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("model %s not found", slug)
	}
	return nil
}

// DeleteModel removes a model from the database by slug.
func (m *sqliteManager) DeleteModel(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Where("slug = ?", slug).Delete(&models.Model{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete model %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("model %s not found", slug)
	}
	return nil
}

// ListModelsByEngineVersion returns models linked to the given engine version slug.
func (m *sqliteManager) ListModelsByEngineVersion(engineVersionSlug string) ([]models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var modelList []models.Model
	if err := m.db.Where("engine_version_slug = ?", engineVersionSlug).Find(&modelList).Error; err != nil {
		return nil, fmt.Errorf("failed to list models by engine version %s: %w", engineVersionSlug, err)
	}
	return modelList, nil
}

// ListContainers returns all containers in the database.
func (m *sqliteManager) ListContainers() ([]models.Container, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var containers []models.Container
	if err := m.db.Order("slug ASC").Find(&containers).Error; err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containers, nil
}

// GetContainerStatus returns the Docker status for a model's container by slug.
func (m *sqliteManager) GetContainerStatus(slug string) (string, error) {
	if m.db == nil {
		return "", fmt.Errorf("database not open")
	}
	var container models.Container
	if err := m.db.Where("slug = ?", slug).First(&container).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "unknown", nil
		}
		return "", fmt.Errorf("failed to get container status for %s: %w", slug, err)
	}
	return container.Status, nil
}

// UpdateContainerStatus updates the status of a container by slug.
func (m *sqliteManager) UpdateContainerStatus(slug string, status string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.Container{}).Where("slug = ?", slug).Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("failed to update container status for %s: %w", slug, result.Error)
	}
	return nil
}

// GetHotspot returns the active hotspot record.
func (m *sqliteManager) GetHotspot() (*models.Hotspot, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var hotspot models.Hotspot
	if err := m.db.Where("active = ?", true).First(&hotspot).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get hotspot: %w", err)
	}
	return &hotspot, nil
}

// SetHotspot sets the active hotspot model.
// It first clears any existing hotspot, then creates a new one.
func (m *sqliteManager) SetHotspot(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// Clear existing hotspot
	if err := m.db.Model(&models.Hotspot{}).Where("active = ?", true).Update("active", false).Error; err != nil {
		return fmt.Errorf("failed to clear existing hotspot: %w", err)
	}
	hotspot := models.Hotspot{
		ModelSlug: slug,
		Active:    true,
	}
	if err := m.db.Create(&hotspot).Error; err != nil {
		return fmt.Errorf("failed to set hotspot: %w", err)
	}
	return nil
}

// ClearHotspot removes the active hotspot record.
func (m *sqliteManager) ClearHotspot() error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Where("active = ?", true).Delete(&models.Hotspot{}).Error; err != nil {
		return fmt.Errorf("failed to clear hotspot: %w", err)
	}
	return nil
}

// GetConfig retrieves a config value by key.
// Returns nil, nil if the key is not found.
func (m *sqliteManager) GetConfig(key string) (*models.Config, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var config models.Config
	if err := m.db.Where("key = ?", key).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get config %s: %w", key, err)
	}
	return &config, nil
}

// SetConfig inserts or updates a config key-value pair.
// Uses UPSERT via ON CONFLICT to handle duplicates.
func (m *sqliteManager) SetConfig(key, value string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Exec(
		"INSERT INTO config (id, key, value) VALUES (1, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP",
		key, value,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to set config %s: %w", key, result.Error)
	}
	return nil
}

// UnsetConfig removes a config key from the database.
func (m *sqliteManager) UnsetConfig(key string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Where("key = ?", key).Delete(&models.Config{})
	if result.Error != nil {
		return fmt.Errorf("failed to unset config %s: %w", key, result.Error)
	}
	return nil
}

// ListConfig returns all config key-value pairs, sorted by key.
func (m *sqliteManager) ListConfig() ([]models.Config, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var configs []models.Config
	if err := m.db.Order("key ASC").Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to list config: %w", err)
	}
	return configs, nil
}

// =============================================================================
// BaseImage CRUD Operations
// =============================================================================

// ListBaseImages returns all base images sorted by slug.
func (m *sqliteManager) ListBaseImages() ([]models.BaseImage, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var baseimages []models.BaseImage
	if err := m.db.Order("slug ASC").Find(&baseimages).Error; err != nil {
		return nil, fmt.Errorf("failed to list base images: %w", err)
	}
	return baseimages, nil
}

// GetBaseImageBySlug returns a single base image by its slug.
func (m *sqliteManager) GetBaseImageBySlug(slug string) (*models.BaseImage, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var baseimage models.BaseImage
	if err := m.db.Where("slug = ?", slug).First(&baseimage).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("base image %s not found", slug)
		}
		return nil, fmt.Errorf("failed to get base image %s: %w", slug, err)
	}
	return &baseimage, nil
}

// GetBaseImageByID returns a single base image by its UUID.
func (m *sqliteManager) GetBaseImageByID(id string) (*models.BaseImage, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var baseimage models.BaseImage
	if err := m.db.Where("id = ?", id).First(&baseimage).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("base image %s not found", id)
		}
		return nil, fmt.Errorf("failed to get base image %s: %w", id, err)
	}
	return &baseimage, nil
}

// CreateBaseImage creates a new base image in the database.
func (m *sqliteManager) CreateBaseImage(baseimage *models.BaseImage) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(baseimage).Error; err != nil {
		return fmt.Errorf("failed to create base image: %w", err)
	}
	return nil
}

// UpdateBaseImage updates a base image by slug with the provided field updates.
func (m *sqliteManager) UpdateBaseImage(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.BaseImage{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update base image %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("base image %s not found", slug)
	}
	return nil
}

// DeleteBaseImage removes a base image from the database by slug.
func (m *sqliteManager) DeleteBaseImage(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	_, err := m.GetBaseImageBySlug(slug)
	if err != nil {
		return err
	}
	// Note: ComposedYmlFile cleanup is handled by the service layer,
	// not the persistence layer. Best-effort file removal is avoided
	// here because the path is environment-specific.
	result := m.db.Where("slug = ?", slug).Delete(&models.BaseImage{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete base image %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("base image %s not found", slug)
	}
	return nil
}

// =============================================================================
// EngineType CRUD Operations
// =============================================================================

// ListEngineTypes returns all engine types sorted by slug.
func (m *sqliteManager) ListEngineTypes() ([]models.EngineType, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineTypes []models.EngineType
	if err := m.db.Order("slug ASC").Find(&engineTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to list engine types: %w", err)
	}
	return engineTypes, nil
}

// GetEngineTypeBySlug returns a single engine type by its slug.
func (m *sqliteManager) GetEngineTypeBySlug(slug string) (*models.EngineType, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineType models.EngineType
	if err := m.db.Where("slug = ?", slug).First(&engineType).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine type %s not found", slug)
		}
		return nil, fmt.Errorf("failed to get engine type %s: %w", slug, err)
	}
	return &engineType, nil
}

// CreateEngineType creates a new engine type in the database.
func (m *sqliteManager) CreateEngineType(engineType *models.EngineType) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(engineType).Error; err != nil {
		return fmt.Errorf("failed to create engine type: %w", err)
	}
	return nil
}

// UpdateEngineType updates an engine type by slug with the provided field updates.
func (m *sqliteManager) UpdateEngineType(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.EngineType{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update engine type %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine type %s not found", slug)
	}
	return nil
}

// DeleteEngineType removes an engine type from the database by slug.
func (m *sqliteManager) DeleteEngineType(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	_, err := m.GetEngineTypeBySlug(slug)
	if err != nil {
		return err
	}
	// Check if any versions reference this type
	var count int64
	if err := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ?", slug).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check engine versions: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("engine type %s has %d version(s) — delete versions first", slug, count)
	}
	result := m.db.Where("slug = ?", slug).Delete(&models.EngineType{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete engine type %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine type %s not found", slug)
	}
	return nil
}

// EngineTypeExists checks whether an engine type with the given slug exists.
func (m *sqliteManager) EngineTypeExists(slug string) (bool, error) {
	if m.db == nil {
		return false, fmt.Errorf("database not open")
	}
	var count int64
	if err := m.db.Model(&models.EngineType{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check engine type %s: %w", slug, err)
	}
	return count > 0, nil
}

// =============================================================================
// EngineVersion CRUD Operations
// =============================================================================

// ListEngineVersions returns all engine versions sorted by created_at desc.
func (m *sqliteManager) ListEngineVersions() ([]models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersions []models.EngineVersion
	if err := m.db.Order("created_at DESC").Find(&engineVersions).Error; err != nil {
		return nil, fmt.Errorf("failed to list engine versions: %w", err)
	}
	return engineVersions, nil
}

// GetEngineVersionBySlugAndType returns an engine version by type slug and version slug.
func (m *sqliteManager) GetEngineVersionBySlugAndType(typeSlug, slug string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND slug = ?", typeSlug, slug).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine version %s/%s not found", typeSlug, slug)
		}
		return nil, fmt.Errorf("failed to get engine version %s/%s: %w", typeSlug, slug, err)
	}
	return &engineVersion, nil
}

// GetEngineVersionByID returns a single engine version by its UUID.
func (m *sqliteManager) GetEngineVersionByID(id string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("id = ?", id).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine version %s not found", id)
		}
		return nil, fmt.Errorf("failed to get engine version %s: %w", id, err)
	}
	return &engineVersion, nil
}

// GetEngineVersionByTypeAndVersion returns an engine version by type slug and version string.
func (m *sqliteManager) GetEngineVersionByTypeAndVersion(typeSlug, version string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND version = ?", typeSlug, version).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine version %s/%s not found", typeSlug, version)
		}
		return nil, fmt.Errorf("failed to get engine version %s/%s: %w", typeSlug, version, err)
	}
	return &engineVersion, nil
}

// CreateEngineVersion creates a new engine version in the database.
func (m *sqliteManager) CreateEngineVersion(engineVersion *models.EngineVersion) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(engineVersion).Error; err != nil {
		return fmt.Errorf("failed to create engine version: %w", err)
	}
	return nil
}

// UpdateEngineVersion updates an engine version by slug with the provided field updates.
func (m *sqliteManager) UpdateEngineVersion(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.EngineVersion{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update engine version %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine version %s not found", slug)
	}
	return nil
}

// DeleteEngineVersion removes an engine version from the database by slug.
func (m *sqliteManager) DeleteEngineVersion(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// Find the version by slug alone (unique within a type)
	var engineVersion models.EngineVersion
	result := m.db.Where("slug = ?", slug).First(&engineVersion)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return fmt.Errorf("engine version %s not found", slug)
		}
		return fmt.Errorf("failed to get engine version %s: %w", slug, result.Error)
	}
	result = m.db.Where("slug = ?", slug).Delete(&models.EngineVersion{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete engine version %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine version %s not found", slug)
	}
	return nil
}

// FindDefaultVersionByType finds an engine version with is_default=true for the given type.
// Returns nil (not error) when no default version exists.
func (m *sqliteManager) FindDefaultVersionByType(typeSlug string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND is_default = ?", typeSlug, true).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find default version for type %s: %w", typeSlug, err)
	}
	return &engineVersion, nil
}

// FindLatestVersionByType finds an engine version with is_latest=true for the given type,
// ordered by created_at descending (returns the most recent).
func (m *sqliteManager) FindLatestVersionByType(typeSlug string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND is_latest = ?", typeSlug, true).Order("created_at DESC").First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find latest version for type %s: %w", typeSlug, err)
	}
	return &engineVersion, nil
}

// ClearIsDefaultForType sets is_default=false for all versions of the given engine type.
func (m *sqliteManager) ClearIsDefaultForType(typeSlug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ? AND is_default = ?", typeSlug, true).Update("is_default", false)
	if result.Error != nil {
		return fmt.Errorf("failed to clear is_default for type %s: %w", typeSlug, result.Error)
	}
	return nil
}

// UpdateIsDefaultClearOthers sets is_default=true for the given version slug
// and clears is_default for all other versions of the same engine type.
func (m *sqliteManager) UpdateIsDefaultClearOthers(typeSlug, slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// First clear all defaults for this type
	if err := m.ClearIsDefaultForType(typeSlug); err != nil {
		return err
	}
	// Then set the target as default
	result := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ? AND slug = ?", typeSlug, slug).Update("is_default", true)
	if result.Error != nil {
		return fmt.Errorf("failed to set is_default for version %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine version %s not found", slug)
	}
	return nil
}

// EngineVersionExistsByTypeAndSlug checks whether an engine version with the
// given type slug and version slug exists in the database.
func (m *sqliteManager) EngineVersionExistsByTypeAndSlug(typeSlug, slug string) (bool, error) {
	if m.db == nil {
		return false, fmt.Errorf("database not open")
	}
	var count int64
	if err := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ? AND slug = ?", typeSlug, slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check engine version %s/%s: %w", typeSlug, slug, err)
	}
	return count > 0, nil
}
