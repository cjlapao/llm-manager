// Package models defines the GORM data models for the application.
package models

import (
	"time"

	"gorm.io/gorm"
)

// Config represents a persistent configuration key-value pair.
// Each key has its own row; id is always 1 as a marker column.
type Config struct {
	ID        int       `gorm:"column:id;not null;default:1"`
	Key       string    `gorm:"primaryKey;size:128;not null"`
	Value     string    `gorm:"type:text;not null;default:''"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName returns the database table name for Config.
func (Config) TableName() string { return "config" }

// BeforeCreate ensures the config row always has id=1.
func (c *Config) BeforeCreate(tx *gorm.DB) error {
	c.ID = 1
	return nil
}

// BeforeUpdate ensures the config row always has id=1.
func (c *Config) BeforeUpdate(tx *gorm.DB) error {
	c.ID = 1
	return nil
}
