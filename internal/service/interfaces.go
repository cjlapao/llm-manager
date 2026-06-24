// Package service provides business logic services that wrap database and config layers.
package service

import "github.com/user/llm-manager/internal/database/models"

// ModelServiceCore defines read/write model operations used across callers.
type ModelServiceCore interface {
	ListModels() ([]models.Model, error)
	GetModel(slug string) (*models.Model, error)
	CreateModel(model *models.Model) error
	UpdateModel(slug string, updates map[string]interface{}) error
	DeleteModel(slug string) error
}

var _ ModelServiceCore = (*ModelService)(nil)

// ModelSyncRunner handles syncing models from external sources.
type ModelSyncRunner interface {
	SyncModel(slug string) error
	SyncAll() error
}

var _ ModelSyncRunner = (*LiteLLMService)(nil)

// ContainerServiceCore defines container lifecycle & status operations.
type ContainerServiceCore interface {
	// Lifecycle controls
	StartModelBySlug(slug string) error
	StartModelWithHealthCheck(slug string, allowMultiple bool) error
	StopModelBySlug(slug string) error
	StopAllBySubType(modelType string, subType string) error
	StartComfyUI() error
	StopComfyUI() error

	// Status queries
	ListContainers() ([]models.Container, error)
	GetContainerStatus(slug string) (string, error)
	UpdateContainerStatus(slug, status string) error

	// Plugin activation
	ActivateFlux(string) error
	DeactivateFlux() error
	Activate3D(string) error
	Deactivate3D() error
}

var _ ContainerServiceCore = (*ContainerService)(nil)

// EngineResolver gives access to engine version resolution.
type EngineResolver interface {
	ResolveDefaultVersion(engineTypeSlug string) (*models.EngineVersion, error)
	ResolveLatestVersion(engineTypeSlug string) (*models.EngineVersion, error)
	ResolveVersionForModel(model models.Model) (*models.EngineVersion, error)
	SetAsDefault(engineTypeSlug, versionSlug string) error
}

var _ EngineResolver = (*EngineService)(nil)

// ServiceStateChannel represents a channel for tracking service health/state changes.
type ServiceStateChannel chan string
