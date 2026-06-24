package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// Containers CRUD Operations
// =============================================================================

// Package sqlite_containers owns all persistence methods for the Container entity.
// Every method here operates on a *sqliteManager receiver and uses the
// same error-wrapping convention: fmt.Errorf("…: %w", err).

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
