package database

import (
	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// DatabaseManager defines the interface for database operations.
type DatabaseManager interface {
	Open() error
	Close() error

	// Schema version tracking and migrations
	SchemaVersion() (int, error)
	LatestVersion() (int, error)
	ApplyPendingMigrations() error
	MigrateTo(targetVersion int) error
	AutoMigrate() error

	DB() *gorm.DB

	// Model CRUD
	ListModels() ([]models.Model, error)
	ListModelsByTypeSubType(modelType string, subType string) ([]models.Model, error)
	GetModel(slug string) (*models.Model, error)
	CreateModel(model *models.Model) error
	UpdateModel(slug string, updates map[string]interface{}) error
	DeleteModel(slug string) error
	ListModelsByEngineVersion(engineVersionSlug string) ([]models.Model, error)

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

	// BaseImage CRUD
	ListBaseImages() ([]models.BaseImage, error)
	GetBaseImageBySlug(slug string) (*models.BaseImage, error)
	GetBaseImageByID(id string) (*models.BaseImage, error)
	CreateBaseImage(image *models.BaseImage) error
	UpdateBaseImage(slug string, updates map[string]interface{}) error
	DeleteBaseImage(slug string) error

	// EngineType CRUD
	ListEngineTypes() ([]models.EngineType, error)
	GetEngineTypeBySlug(slug string) (*models.EngineType, error)
	CreateEngineType(engineType *models.EngineType) error
	UpdateEngineType(slug string, updates map[string]interface{}) error
	DeleteEngineType(slug string) error
	EngineTypeExists(slug string) (bool, error)

	// EngineVersion CRUD
	ListEngineVersions() ([]models.EngineVersion, error)
	GetEngineVersionBySlugAndType(typeSlug, slug string) (*models.EngineVersion, error)
	GetEngineVersionByID(id string) (*models.EngineVersion, error)
	GetEngineVersionByTypeAndVersion(typeSlug, version string) (*models.EngineVersion, error)
	CreateEngineVersion(engineVersion *models.EngineVersion) error
	UpdateEngineVersion(slug string, updates map[string]interface{}) error
	DeleteEngineVersion(slug string) error
	FindDefaultVersionByType(typeSlug string) (*models.EngineVersion, error)
	FindLatestVersionByType(typeSlug string) (*models.EngineVersion, error)
	ClearIsDefaultForType(typeSlug string) error
	UpdateIsDefaultClearOthers(typeSlug, slug string) error
	EngineVersionExistsByTypeAndSlug(typeSlug, slug string) (bool, error)
}
