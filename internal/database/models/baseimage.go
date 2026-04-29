package models

import "time"

type BaseImage struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	Slug            string    `gorm:"uniqueIndex;size:128;not null"`
	Name            string    `gorm:"size:255"`
	EngineType      string    `gorm:"size:50"`
	DockerImage     string    `gorm:"size:500"`
	Entrypoint      string    `gorm:"type:text"`
	EnvironmentJSON string    `gorm:"type:text"`
	VolumesJSON     string    `gorm:"type:text"`
	ComposedYmlFile string    `gorm:"size:255"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

func (BaseImage) TableName() string { return "base_images" }
