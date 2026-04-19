// Package models defines the GORM data models for the application.
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Model represents an LLM model in the registry.
type Model struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug            string    `gorm:"uniqueIndex;size:128;not null"`
	Type            string    `gorm:"size:32;not null;index"`
	Name            string    `gorm:"size:256;not null"`
	HFRepo          string    `gorm:"size:512"`
	YML             string    `gorm:"type:text"`
	Container       string    `gorm:"size:256"`
	Port            int       `gorm:"not null"`
	EngineType      string    `gorm:"size:16;default:'vllm'"`
	EnvVars         string    `gorm:"type:text"`
	CommandArgs     string    `gorm:"type:text"`
	InputTokenCost  float64   `gorm:"default:0"`
	OutputTokenCost float64   `gorm:"default:0"`
	Capabilities    string    `gorm:"type:text"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

// TableName returns the database table name for Model.
func (Model) TableName() string { return "models" }

// BeforeCreate generates a UUID for new Model records.
func (m *Model) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
