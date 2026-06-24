package database

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// =============================================================================
// Models CRUD Operations
// =============================================================================

// Package sqlite_models owns all persistence methods for the Model entity.
// Every method here operates on a *sqliteManager receiver and uses the
// same error-wrapping convention: fmt.Errorf("…: %w", err).

// ListModels returns all models in the database, sorted by slug.
func (m *sqliteManager) ListModels() ([]models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var modelList []models.Model
	if err := m.db.Order("slug ASC").Find(&modelList).Error; err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	return modelList, nil
}

// ListModelsByTypeSubType returns models matching the given type and subType,
// ordered with Default models first, then alphabetically by slug.
func (m *sqliteManager) ListModelsByTypeSubType(modelType string, subType string) ([]models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var modelList []models.Model
	if err := m.db.Where("type = ? AND sub_type = ?", modelType, subType).Order("`default` DESC, slug ASC").Find(&modelList).Error; err != nil {
		return nil, fmt.Errorf("failed to list models by type/subType: %w", err)
	}
	return modelList, nil
}

// GetModel returns a single model by its slug.
func (m *sqliteManager) GetModel(slug string) (*models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var model models.Model
	if err := m.db.Where("slug = ?", slug).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("model %s not found", slug)
		}
		return nil, fmt.Errorf("failed to get model %s: %w", slug, err)
	}
	return &model, nil
}

// CreateModel creates a new model in the database.
func (m *sqliteManager) CreateModel(model *models.Model) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	if err := m.db.Create(model).Error; err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}
	return nil
}

// UpdateModel updates a model by slug with the provided field updates.
func (m *sqliteManager) UpdateModel(slug string, updates map[string]interface{}) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Model(&models.Model{}).Where("slug = ?", slug).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update model %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("model %s not found", slug)
	}
	return nil
}

// DeleteModel removes a model from the database by slug.
func (m *sqliteManager) DeleteModel(slug string) error {
	if m.db == nil {
		return fmt.Errorf("database not open")
	}
	result := m.db.Where("slug = ?", slug).Delete(&models.Model{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete model %s: %w", slug, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("model %s not found", slug)
	}
	return nil
}

// ListModelsByEngineVersion returns models linked to the given engine version slug.
func (m *sqliteManager) ListModelsByEngineVersion(engineVersionSlug string) ([]models.Model, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}
	var modelList []models.Model
	if err := m.db.Where("engine_version_slug = ?", engineVersionSlug).Find(&modelList).Error; err != nil {
		return nil, fmt.Errorf("failed to list models by engine version %s: %w", engineVersionSlug, err)
	}
	return modelList, nil
}
