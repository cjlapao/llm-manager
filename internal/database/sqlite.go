package database

import (
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqliteManager implements DatabaseManager using SQLite.
type sqliteManager struct {
	dsn string
	db  *gorm.DB
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

// AutoMigrate runs database migrations.
func (m *sqliteManager) AutoMigrate() error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.AutoMigrate(
		&models.Model{},
		&models.Container{},
		&models.Hotspot{},
		&models.Config{},
	); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	// SQLite doesn't support ALTER TABLE ADD COLUMN in AutoMigrate for all cases.
	// We need to explicitly add any new columns that GORM might miss.
	return m.ensureModelColumns()
}

// ensureModelColumns adds any missing columns to the models table.
// This is needed because GORM's AutoMigrate in SQLite doesn't always add columns to existing tables.
func (m *sqliteManager) ensureModelColumns() error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}

	// Check which columns already exist
	var existingColumns []string
	if err := m.db.Raw(`
		SELECT name FROM pragma_table_info('models')
		WHERE name IN ('engine_type', 'env_vars', 'command_args', 'input_token_cost', 'output_token_cost', 'capabilities', 'lite_llm_params', 'model_info')
	`).Scan(&existingColumns).Error; err != nil {
		return fmt.Errorf("failed to check model columns: %w", err)
	}

	existingSet := make(map[string]bool)
	for _, c := range existingColumns {
		existingSet[c] = true
	}

	// Define columns to add with their types and defaults
	type columnDef struct {
		name         string
		sqlType      string
		defaultValue string
	}
	columns := []columnDef{
		{"engine_type", "TEXT", "'vllm'"},
		{"env_vars", "TEXT", "NULL"},
		{"command_args", "TEXT", "NULL"},
		{"input_token_cost", "REAL", "0"},
		{"output_token_cost", "REAL", "0"},
		{"capabilities", "TEXT", "NULL"},
		{"lite_llm_params", "TEXT", "NULL"},
		{"model_info", "TEXT", "NULL"},
	}

	for _, col := range columns {
		if existingSet[col.name] {
			continue // already exists, skip
		}
		sql := fmt.Sprintf("ALTER TABLE models ADD COLUMN %s %s DEFAULT %s", col.name, col.sqlType, col.defaultValue)
		if err := m.db.Exec(sql).Error; err != nil {
			// Column might already exist (race condition or partial migration)
			if !strings.Contains(err.Error(), "duplicate column") && !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to add column %s: %w", col.name, err)
			}
		}
	}

	return nil
}

// DB returns the underlying GORM database instance.
func (m *sqliteManager) DB() *gorm.DB {
	return m.db
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
