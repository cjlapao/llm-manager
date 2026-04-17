package database

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
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
	db, err := gorm.Open(sqlite.Open(m.dsn), &gorm.Config{})
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
	return m.db.AutoMigrate(
		&models.Model{},
		&models.Container{},
		&models.Hotspot{},
	)
}

// DB returns the underlying GORM database instance.
func (m *sqliteManager) DB() *gorm.DB {
	return m.db
}
