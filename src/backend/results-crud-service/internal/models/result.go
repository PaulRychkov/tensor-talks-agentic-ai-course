package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Result представляет результат интервью.
type Result struct {
	ID                  uint             `gorm:"primaryKey" json:"id"`
	SessionID           uuid.UUID        `gorm:"type:uuid;uniqueIndex;not null" json:"session_id"`
	UserID              *uuid.UUID       `gorm:"type:uuid;index" json:"user_id,omitempty"` // §10.6 episodic memory
	Score               int              `gorm:"not null" json:"score"`
	Feedback            string           `gorm:"type:text;not null" json:"feedback"`
	TerminatedEarly     bool             `gorm:"default:false" json:"terminated_early"`
	ReportJSON          json.RawMessage  `gorm:"type:jsonb" json:"report_json,omitempty"`
	PresetTraining      json.RawMessage  `gorm:"type:jsonb" json:"preset_training,omitempty"`
	ResultFormatVersion int              `gorm:"default:1" json:"result_format_version"`
	Evaluations         json.RawMessage  `gorm:"type:jsonb" json:"evaluations,omitempty"`
	SessionKind         string           `gorm:"type:varchar(20);default:'interview'" json:"session_kind"`
	UserRating          *int             `gorm:"default:null" json:"user_rating,omitempty"` // 1-5 stars from user
	UserComment         string           `gorm:"type:text;default:''" json:"user_comment,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

// TableName указывает имя таблицы в БД.
func (Result) TableName() string {
	return "results"
}
