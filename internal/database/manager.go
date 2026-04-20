package database

import (
	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// DatabaseManager defines the interface for database operations.
type DatabaseManager interface {
	Open() error
	Close() error
	AutoMigrate() error
	DB() *gorm.DB
	MigrateFromJSON(path string) (int, error)

	// Model CRUD
	ListModels() ([]models.Model, error)
	GetModel(slug string) (*models.Model, error)
	CreateModel(model *models.Model) error
	UpdateModel(slug string, updates map[string]interface{}) error
	DeleteModel(slug string) error

	// Container operations
	ListContainers() ([]models.Container, error)
	GetContainerStatus(slug string) (string, error)
	UpdateContainerStatus(slug string, status string) error

	// Hotspot operations
	GetHotspot() (*models.Hotspot, error)
	SetHotspot(slug string) error
	ClearHotspot() error

	// Config CRUD
	GetConfig(key string) (*models.Config, error)
	SetConfig(key, value string) error
	UnsetConfig(key string) error
	ListConfig() ([]models.Config, error)
}
