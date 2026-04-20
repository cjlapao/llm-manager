// Package service provides business logic services that wrap the database layer.
package service

import (
	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

// ConfigService handles business logic for persistent configuration operations.
type ConfigService struct {
	db database.DatabaseManager
}

// NewConfigService creates a new ConfigService.
func NewConfigService(db database.DatabaseManager) *ConfigService {
	return &ConfigService{db: db}
}

// Set stores a config key-value pair in the database.
// Validates the key before storing.
func (s *ConfigService) Set(key, value string) error {
	if _, err := config.NormalizeKey(key); err != nil {
		return err
	}
	return s.db.SetConfig(key, value)
}

// Unset removes a config key from the database.
// Validates the key before removing.
func (s *ConfigService) Unset(key string) error {
	if _, err := config.NormalizeKey(key); err != nil {
		return err
	}
	return s.db.UnsetConfig(key)
}

// Get retrieves a config value from the database.
// Returns nil if the key is not found.
func (s *ConfigService) Get(key string) (*models.Config, error) {
	if _, err := config.NormalizeKey(key); err != nil {
		return nil, err
	}
	return s.db.GetConfig(key)
}

// List returns all config key-value pairs from the database.
func (s *ConfigService) List() ([]models.Config, error) {
	return s.db.ListConfig()
}
