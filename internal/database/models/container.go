package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Container represents a running container for an LLM model.
type Container struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug      string    `gorm:"uniqueIndex;size:128;not null"`
	Name      string    `gorm:"size:256"`
	Status    string    `gorm:"size:32;not null;default:'stopped'"`
	Port      int
	GPUUsed   bool
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName returns the database table name for Container.
func (Container) TableName() string { return "containers" }

// BeforeCreate generates a UUID for new Container records.
func (c *Container) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
