package database

import (
	"fmt"

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

	// Check which columns already exist by querying the sqlite_master table
	var columnCount int
	if err := m.db.Raw(`
		SELECT COUNT(*) FROM pragma_table_info('models')
		WHERE name IN ('engine_type', 'env_vars', 'command_args', 'input_token_cost', 'output_token_cost', 'capabilities')
	`).Scan(&columnCount).Error; err != nil {
		return fmt.Errorf("failed to check model columns: %w", err)
	}

	// If we found 0 matching columns, we need to add all of them
	if columnCount == 0 {
		alterSQLs := []string{
			"ALTER TABLE models ADD COLUMN engine_type TEXT DEFAULT 'vllm'",
			"ALTER TABLE models ADD COLUMN env_vars TEXT",
			"ALTER TABLE models ADD COLUMN command_args TEXT",
			"ALTER TABLE models ADD COLUMN input_token_cost REAL DEFAULT 0",
			"ALTER TABLE models ADD COLUMN output_token_cost REAL DEFAULT 0",
			"ALTER TABLE models ADD COLUMN capabilities TEXT",
		}

		for _, sql := range alterSQLs {
			if err := m.db.Exec(sql).Error; err != nil {
				// Column might already exist (SQLite allows multiple ADD COLUMN in some versions)
				// Check if it's a "duplicate column" error
				if !strings.Contains(err.Error(), "duplicate column") && !strings.Contains(err.Error(), "already exists") {
					return fmt.Errorf("failed to add column: %w", err)
				}
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
