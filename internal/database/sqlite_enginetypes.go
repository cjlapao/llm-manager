package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// EngineType CRUD Operations
// =============================================================================

// Package sqlite_enginetypes owns all persistence methods for the EngineType entity.
// Every method operates on a *sqliteManager receiver using the standard
// error-wrapping convention: fmt.Errorf("…: %w", err).

// ListEngineTypes returns all engine types sorted by slug.
func (m *sqliteManager) ListEngineTypes() ([]models.EngineType, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineTypes []models.EngineType
	if err := m.db.Order("slug ASC").Find(&engineTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to list engine types: %w", err)
	}
	return engineTypes, nil
}

// GetEngineTypeBySlug returns a single engine type by its slug.
func (m *sqliteManager) GetEngineTypeBySlug(slug string) (*models.EngineType, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineType models.EngineType
	if err := m.db.Where("slug = ?", slug).First(&engineType).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine type %s not found", slug)
		}
		return nil, fmt.Errorf("failed to get engine type %s: %w", slug, err)
	}
	return &engineType, nil
}

// CreateEngineType creates a new engine type in the database.
func (m *sqliteManager) CreateEngineType(engineType *models.EngineType) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(engineType).Error; err != nil {
		return fmt.Errorf("failed to create engine type: %w", err)
	}
	return nil
}

// UpdateEngineType updates an engine type by slug with the provided field updates.
func (m *sqliteManager) UpdateEngineType(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.EngineType{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update engine type %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine type %s not found", slug)
	}
	return nil
}

// DeleteEngineType removes an engine type from the database by slug.
func (m *sqliteManager) DeleteEngineType(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	_, err := m.GetEngineTypeBySlug(slug)
	if err != nil {
		return err
	}
	// Check if any versions reference this type
	var count int64
	if err := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ?", slug).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check engine versions: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("engine type %s has %d version(s) — delete versions first", slug, count)
	}
	result := m.db.Where("slug = ?", slug).Delete(&models.EngineType{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete engine type %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine type %s not found", slug)
	}
	return nil
}

// EngineTypeExists checks whether an engine type with the given slug exists.
func (m *sqliteManager) EngineTypeExists(slug string) (bool, error) {
	if m.db == nil {
		return false, fmt.Errorf("database not open")
	}
	var count int64
	if err := m.db.Model(&models.EngineType{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check engine type %s: %w", slug, err)
	}
	return count > 0, nil
}
