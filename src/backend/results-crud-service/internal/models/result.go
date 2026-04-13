package models

import (
	"time"

	"github.com/google/uuid"
)

// Result представляет результат интервью.
type Result struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	SessionID       uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"session_id"`
	Score           int       `gorm:"not null" json:"score"`
	Feedback        string    `gorm:"type:text;not null" json:"feedback"`
	TerminatedEarly bool      `gorm:"default:false" json:"terminated_early"` // Флаг досрочного завершения
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName указывает имя таблицы в БД.
func (Result) TableName() string {
	return "results"
}
