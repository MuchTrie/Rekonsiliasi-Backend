package models

import (
	"time"
	"gorm.io/gorm"
)

// Setting represents a feature toggle setting
type Setting struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	FeatureName string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"feature_name"`
	IsEnabled   bool           `gorm:"default:true" json:"is_enabled"`
	Description string         `gorm:"type:varchar(500)" json:"description"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName overrides the table name
func (Setting) TableName() string {
	return "settings"
}
