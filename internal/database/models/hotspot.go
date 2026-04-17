package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Hotspot tracks the most recently used model for quick access.
type Hotspot struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ModelSlug string    `gorm:"uniqueIndex;size:128;not null"`
	Active    bool      `gorm:"not null;default:true"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName returns the database table name for Hotspot.
func (Hotspot) TableName() string { return "hotspots" }

// BeforeCreate generates a UUID for new Hotspot records.
func (h *Hotspot) BeforeCreate(tx *gorm.DB) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	return nil
}
