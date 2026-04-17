package database

import (
	"gorm.io/gorm"
)

// DatabaseManager defines the interface for database operations.
type DatabaseManager interface {
	Open() error
	Close() error
	AutoMigrate() error
	DB() *gorm.DB
	MigrateFromJSON(path string) (int, error)
}
