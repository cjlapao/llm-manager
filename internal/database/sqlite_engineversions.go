package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// EngineVersion CRUD Operations
// =============================================================================

// Package sqlite_engineversions owns all persistence methods for the EngineVersion entity.
// Every method operates on a *sqliteManager receiver using the standard
// error-wrapping convention: fmt.Errorf("…: %w", err).

// ListEngineVersions returns all engine versions sorted by created_at desc.
func (m *sqliteManager) ListEngineVersions() ([]models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersions []models.EngineVersion
	if err := m.db.Order("created_at DESC").Find(&engineVersions).Error; err != nil {
		return nil, fmt.Errorf("failed to list engine versions: %w", err)
	}
	return engineVersions, nil
}

// GetEngineVersionBySlugAndType returns an engine version by type slug and version slug.
func (m *sqliteManager) GetEngineVersionBySlugAndType(typeSlug, slug string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND slug = ?", typeSlug, slug).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine version %s/%s not found", typeSlug, slug)
		}
		return nil, fmt.Errorf("failed to get engine version %s/%s: %w", typeSlug, slug, err)
	}
	return &engineVersion, nil
}

// GetEngineVersionByID returns a single engine version by its UUID.
func (m *sqliteManager) GetEngineVersionByID(id string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("id = ?", id).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine version %s not found", id)
		}
		return nil, fmt.Errorf("failed to get engine version %s: %w", id, err)
	}
	return &engineVersion, nil
}

// GetEngineVersionByTypeAndVersion returns an engine version by type slug and version string.
func (m *sqliteManager) GetEngineVersionByTypeAndVersion(typeSlug, version string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND version = ?", typeSlug, version).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("engine version %s/%s not found", typeSlug, version)
		}
		return nil, fmt.Errorf("failed to get engine version %s/%s: %w", typeSlug, version, err)
	}
	return &engineVersion, nil
}

// CreateEngineVersion creates a new engine version in the database.
func (m *sqliteManager) CreateEngineVersion(engineVersion *models.EngineVersion) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(engineVersion).Error; err != nil {
		return fmt.Errorf("failed to create engine version: %w", err)
	}
	return nil
}

// UpdateEngineVersion updates an engine version by slug with the provided field updates.
func (m *sqliteManager) UpdateEngineVersion(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.EngineVersion{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update engine version %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine version %s not found", slug)
	}
	return nil
}

// DeleteEngineVersion removes an engine version from the database by slug.
func (m *sqliteManager) DeleteEngineVersion(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// Find the version by slug alone (unique within a type)
	var engineVersion models.EngineVersion
	result := m.db.Where("slug = ?", slug).First(&engineVersion)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return fmt.Errorf("engine version %s not found", slug)
		}
		return fmt.Errorf("failed to get engine version %s: %w", slug, result.Error)
	}
	result = m.db.Where("slug = ?", slug).Delete(&models.EngineVersion{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete engine version %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine version %s not found", slug)
	}
	return nil
}

// FindDefaultVersionByType finds an engine version with is_default=true for the given type.
// Returns nil (not error) when no default version exists.
func (m *sqliteManager) FindDefaultVersionByType(typeSlug string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND is_default = ?", typeSlug, true).First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find default version for type %s: %w", typeSlug, err)
	}
	return &engineVersion, nil
}

// FindLatestVersionByType finds an engine version with is_latest=true for the given type,
// ordered by created_at descending (returns the most recent).
func (m *sqliteManager) FindLatestVersionByType(typeSlug string) (*models.EngineVersion, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var engineVersion models.EngineVersion
	if err := m.db.Where("engine_type_slug = ? AND is_latest = ?", typeSlug, true).Order("created_at DESC").First(&engineVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find latest version for type %s: %w", typeSlug, err)
	}
	return &engineVersion, nil
}

// ClearIsDefaultForType sets is_default=false for all versions of the given engine type.
func (m *sqliteManager) ClearIsDefaultForType(typeSlug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ? AND is_default = ?", typeSlug, true).Update("is_default", false)
	if result.Error != nil {
		return fmt.Errorf("failed to clear is_default for type %s: %w", typeSlug, result.Error)
	}
	return nil
}

// UpdateIsDefaultClearOthers sets is_default=true for the given version slug
// and clears is_default for all other versions of the same engine type.
func (m *sqliteManager) UpdateIsDefaultClearOthers(typeSlug, slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// First clear all defaults for this type
	if err := m.ClearIsDefaultForType(typeSlug); err != nil {
		return err
	}
	// Then set the target as default
	result := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ? AND slug = ?", typeSlug, slug).Update("is_default", true)
	if result.Error != nil {
		return fmt.Errorf("failed to set is_default for version %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("engine version %s not found", slug)
	}
	return nil
}

// EngineVersionExistsByTypeAndSlug checks whether an engine version with the
// given type slug and version slug exists in the database.
func (m *sqliteManager) EngineVersionExistsByTypeAndSlug(typeSlug, slug string) (bool, error) {
	if m.db == nil {
		return false, fmt.Errorf("database not open")
	}
	var count int64
	if err := m.db.Model(&models.EngineVersion{}).Where("engine_type_slug = ? AND slug = ?", typeSlug, slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check engine version %s/%s: %w", typeSlug, slug, err)
	}
	return count > 0, nil
}
