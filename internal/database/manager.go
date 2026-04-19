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

	// New CRUD methods
	ListModels() ([]models.Model, error)
	GetModel(slug string) (*models.Model, error)
	CreateModel(model *models.Model) error
	UpdateModel(slug string, updates map[string]interface{}) error
	DeleteModel(slug string) error
	ListContainers() ([]models.Container, error)
	GetContainerStatus(slug string) (string, error)
	UpdateContainerStatus(slug string, status string) error
	GetHotspot() (*models.Hotspot, error)
	SetHotspot(slug string) error
	ClearHotspot() error
}
