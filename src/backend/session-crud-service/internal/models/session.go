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
	ID       string `json:"id,omitempty"`
	Question string `json:"question"`
	Theory   string `json:"theory"`
	Order    int    `json:"order"`
	Topic    string `json:"topic,omitempty"`
	// Study-mode hierarchy fields
	Subtopic        string `json:"subtopic,omitempty"`
	PointID         string `json:"point_id,omitempty"`
	PointTitle      string `json:"point_title,omitempty"`
	PointTheory     string `json:"point_theory,omitempty"`
	QuestionInPoint int    `json:"question_in_point,omitempty"`
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

// SessionParams представляет параметры сессии (темы, уровень, режим).
type SessionParams struct {
	Topics     []string `json:"topics"`
	Level      string   `json:"level"`                // junior, middle, senior
	Type       string   `json:"type,omitempty"`       // specialty: ml, nlp, llm, cv, ds
	Mode       string   `json:"mode"`                 // interview, training, study
	Subtopics  []string `json:"subtopics,omitempty"`   // selected subtopics (study/training)
	FocusPoints []string `json:"focus_points,omitempty"` // specific points for follow-up study
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

// ProgramMeta — техническая метаинформация о качестве сборки программы.
type ProgramMeta struct {
	ValidationPassed bool           `json:"validation_passed"`
	Coverage         map[string]int `json:"coverage,omitempty"`
	FallbackReason   *string        `json:"fallback_reason"`
	GeneratorVersion string         `json:"generator_version"`
}

// Value реализует driver.Valuer для сохранения в JSONB.
func (pm ProgramMeta) Value() (driver.Value, error) {
	return json.Marshal(pm)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (pm *ProgramMeta) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, pm)
}

// Session описывает сессию интервью в PostgreSQL.
type Session struct {
	SessionID        uuid.UUID         `gorm:"type:uuid;primaryKey"`
	UserID           uuid.UUID         `gorm:"type:uuid;not null;index"`
	StartTime        time.Time         `gorm:"not null"`
	EndTime          *time.Time        `gorm:"default:null"`
	Params           SessionParams     `gorm:"type:jsonb;not null"`
	InterviewProgram *InterviewProgram `gorm:"type:jsonb;default:null"`
	ProgramStatus    string            `gorm:"type:varchar(32);default:null" json:"program_status,omitempty"`
	ProgramMeta      *ProgramMeta      `gorm:"type:jsonb;default:null" json:"program_meta,omitempty"`
	ProgramVersion   string            `gorm:"type:varchar(64);default:null" json:"program_version,omitempty"`
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
	ProgramStatus    string            `json:"program_status,omitempty"`
	ProgramMeta      *ProgramMeta      `json:"program_meta,omitempty"`
	ProgramVersion   string            `json:"program_version,omitempty"`
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
		ProgramStatus:    s.ProgramStatus,
		ProgramMeta:      s.ProgramMeta,
		ProgramVersion:   s.ProgramVersion,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}
