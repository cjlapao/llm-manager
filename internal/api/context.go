// Package api provides the HTTP API server for llm-manager.
package api

import (
	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/service"
)

// APIContext holds the shared dependencies injected into all API handlers.
type APIContext struct {
	DB               database.DatabaseManager
	Config           *config.Config
	ModelService     *service.ModelService
	ContainerService *service.ContainerService
}

// NewAPIContext creates an APIContext from a database manager and config.
func NewAPIContext(db database.DatabaseManager, cfg *config.Config) *APIContext {
	return &APIContext{
		DB:               db,
		Config:           cfg,
		ModelService:     service.NewModelService(db, cfg),
		ContainerService: service.NewContainerService(db, cfg),
	}
}
