package database

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/user/llm-manager/internal/database/models"
)

// modelsJSON represents the structure of models.json.
type modelsJSON struct {
	Version     string               `json:"version"`
	HFCacheDir  string               `json:"hf_cache_dir"`
	ModelGroups map[string]modelJSON `json:"models"`
}

// modelJSON represents a single model entry from models.json.
type modelJSON struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	HFRepo    string `json:"hf_repo"`
	YML       string `json:"yml"`
	Container string `json:"container"`
	Port      int    `json:"port"`
}

// MigrateFromJSON reads models.json and imports records into SQLite.
func (m *sqliteManager) MigrateFromJSON(path string) (int, error) {
	if m.db == nil {
		return 0, fmt.Errorf("database not open")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to read models file: %w", err)
	}

	var mj modelsJSON
	if err := json.Unmarshal(data, &mj); err != nil {
		return 0, fmt.Errorf("failed to parse models file: %w", err)
	}

	// Check if models table already has records
	var count int64
	m.db.Model(&models.Model{}).Count(&count)
	if count > 0 {
		return 0, nil
	}

	// Insert each model from JSON
	for slug, mjModel := range mj.ModelGroups {
		var existing models.Model
		result := m.db.Where("slug = ?", slug).First(&existing)
		if result.Error == nil {
			continue
		}

		model := models.Model{
			Slug:      slug,
			Type:      mjModel.Type,
			Name:      mjModel.Name,
			HFRepo:    mjModel.HFRepo,
			YML:       mjModel.YML,
			Container: mjModel.Container,
			Port:      mjModel.Port,
		}

		if err := m.db.Create(&model).Error; err != nil {
			return 0, fmt.Errorf("failed to insert model %s: %w", slug, err)
		}
	}

	// Count total inserted
	m.db.Model(&models.Model{}).Count(&count)
	return int(count), nil
}
