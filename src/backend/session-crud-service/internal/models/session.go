package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InterviewProgram представляет программу интервью с упорядоченным списком вопросов и теории.
type InterviewProgram struct {
	Questions []QuestionItem `json:"questions"`
}

// QuestionItem представляет один пункт программы (вопрос + теория).
type QuestionItem struct {
	Question string `json:"question"`
	Theory   string `json:"theory"`
	Order    int    `json:"order"`
}

// Value реализует driver.Valuer для сохранения в JSONB.
func (ip InterviewProgram) Value() (driver.Value, error) {
	return json.Marshal(ip)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (ip *InterviewProgram) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, ip)
}

// SessionParams представляет параметры интервьюируемого (темы, уровень, тип).
type SessionParams struct {
	Topics []string `json:"topics"`
	Level  string   `json:"level"` // junior, middle, senior
	Type   string   `json:"type"`  // interview, training
}

// Value реализует driver.Valuer для сохранения в JSONB.
func (sp SessionParams) Value() (driver.Value, error) {
	return json.Marshal(sp)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (sp *SessionParams) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, sp)
}

// Session описывает сессию интервью в PostgreSQL.
type Session struct {
	SessionID        uuid.UUID         `gorm:"type:uuid;primaryKey"`
	UserID           uuid.UUID         `gorm:"type:uuid;not null;index"`
	StartTime        time.Time         `gorm:"not null"`
	EndTime          *time.Time        `gorm:"default:null"`
	Params           SessionParams     `gorm:"type:jsonb;not null"`
	InterviewProgram *InterviewProgram `gorm:"type:jsonb;default:null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TableName указывает имя таблицы в БД.
func (Session) TableName() string {
	return "sessions"
}

// BeforeCreate заполняет SessionID перед вставкой записи, если он не задан.
func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.SessionID == uuid.Nil {
		s.SessionID = uuid.New()
	}
	return nil
}

// PublicSession — представление сессии для JSON-ответа.
type PublicSession struct {
	SessionID        uuid.UUID         `json:"session_id"`
	UserID           uuid.UUID         `json:"user_id"`
	StartTime        time.Time         `json:"start_time"`
	EndTime          *time.Time        `json:"end_time,omitempty"`
	Params           SessionParams     `json:"params"`
	InterviewProgram *InterviewProgram `json:"interview_program,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// ToPublic конвертирует ORM-модель в структуру для JSON-ответа.
func (s Session) ToPublic() PublicSession {
	return PublicSession{
		SessionID:        s.SessionID,
		UserID:           s.UserID,
		StartTime:        s.StartTime,
		EndTime:          s.EndTime,
		Params:           s.Params,
		InterviewProgram: s.InterviewProgram,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}
