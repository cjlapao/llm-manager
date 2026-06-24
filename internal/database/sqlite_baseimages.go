package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// BaseImage CRUD Operations
// =============================================================================

// Package sqlite_baseimages owns all persistence methods for the BaseImage entity.
// Every method operates on a *sqliteManager receiver using the standard
// error-wrapping convention: fmt.Errorf("…: %w", err).

// ListBaseImages returns all base images sorted by slug.
func (m *sqliteManager) ListBaseImages() ([]models.BaseImage, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var baseimages []models.BaseImage
	if err := m.db.Order("slug ASC").Find(&baseimages).Error; err != nil {
		return nil, fmt.Errorf("failed to list base images: %w", err)
	}
	return baseimages, nil
}

// GetBaseImageBySlug returns a single base image by its slug.
func (m *sqliteManager) GetBaseImageBySlug(slug string) (*models.BaseImage, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var baseimage models.BaseImage
	if err := m.db.Where("slug = ?", slug).First(&baseimage).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("base image %s not found", slug)
		}
		return nil, fmt.Errorf("failed to get base image %s: %w", slug, err)
	}
	return &baseimage, nil
}

// GetBaseImageByID returns a single base image by its UUID.
func (m *sqliteManager) GetBaseImageByID(id string) (*models.BaseImage, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var baseimage models.BaseImage
	if err := m.db.Where("id = ?", id).First(&baseimage).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("base image %s not found", id)
		}
		return nil, fmt.Errorf("failed to get base image %s: %w", id, err)
	}
	return &baseimage, nil
}

// CreateBaseImage creates a new base image in the database.
func (m *sqliteManager) CreateBaseImage(baseimage *models.BaseImage) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(baseimage).Error; err != nil {
		return fmt.Errorf("failed to create base image: %w", err)
	}
	return nil
}

// UpdateBaseImage updates a base image by slug with the provided field updates.
func (m *sqliteManager) UpdateBaseImage(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.BaseImage{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update base image %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("base image %s not found", slug)
	}
	return nil
}

// DeleteBaseImage removes a base image from the database by slug.
func (m *sqliteManager) DeleteBaseImage(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	_, err := m.GetBaseImageBySlug(slug)
	if err != nil {
		return err
	}
	// Note: ComposedYmlFile cleanup is handled by the service layer,
	// not the persistence layer. Best-effort file removal is avoided
	// here because the path is environment-specific.
	result := m.db.Where("slug = ?", slug).Delete(&models.BaseImage{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete base image %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("base image %s not found", slug)
	}
	return nil
}
