package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// Hotspot Operations
// =============================================================================

// Package sqlite_hotspot owns all persistence methods for the Hotspot entity.
// Every method here operates on a *sqliteManager receiver and uses the
// same error-wrapping convention: fmt.Errorf("…: %w", err).

// GetHotspot returns the active hotspot record.
func (m *sqliteManager) GetHotspot() (*models.Hotspot, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var hotspot models.Hotspot
	if err := m.db.Where("active = ?", true).First(&hotspot).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get hotspot: %w", err)
	}
	return &hotspot, nil
}

// SetHotspot sets the active hotspot model.
// It first clears any existing hotspot, then creates a new one.
func (m *sqliteManager) SetHotspot(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	// Clear existing hotspot
	if err := m.db.Model(&models.Hotspot{}).Where("active = ?", true).Update("active", false).Error; err != nil {
		return fmt.Errorf("failed to clear existing hotspot: %w", err)
	}
	hotspot := models.Hotspot{
		ModelSlug: slug,
		Active:    true,
	}
	if err := m.db.Create(&hotspot).Error; err != nil {
		return fmt.Errorf("failed to set hotspot: %w", err)
	}
	return nil
}

// ClearHotspot removes the active hotspot record.
func (m *sqliteManager) ClearHotspot() error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Where("active = ?", true).Delete(&models.Hotspot{}).Error; err != nil {
		return fmt.Errorf("failed to clear hotspot: %w", err)
	}
	return nil
}
