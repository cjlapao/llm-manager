package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// Config CRUD Operations
// =============================================================================

// Package sqlite_configs owns all persistence methods for the Config entity.
// Every method here operates on a *sqliteManager receiver and uses the
// same error-wrapping convention: fmt.Errorf("…: %w", err).

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
