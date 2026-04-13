package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// Question представляет запись вопроса в PostgreSQL.
type Question struct {
	ID           string        `gorm:"primaryKey;type:varchar(255)"`
	TheoryID     *string       `gorm:"type:varchar(255);index"`
	QuestionType string        `gorm:"type:varchar(50);not null;index"`
	Complexity   int           `gorm:"type:integer;not null;index"`
	Data         QuestionJSONB `gorm:"type:jsonb;not null"`
	Version      string        `gorm:"type:varchar(50);default:'1.0'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName указывает имя таблицы в БД.
func (Question) TableName() string {
	return "questions"
}

// QuestionJSONB представляет JSON структуру вопроса.
type QuestionJSONB struct {
	ID             string              `json:"id"`
	TheoryID       *string             `json:"theory_id,omitempty"`
	LinkedSegments []string            `json:"linked_segments,omitempty"`
	QuestionType   string              `json:"question_type"`
	Complexity     int                 `json:"complexity"`
	Content        QuestionContent     `json:"content"`
	IdealAnswer    QuestionIdealAnswer `json:"ideal_answer"`
	Metadata       QuestionMetadata    `json:"metadata"`
}

// QuestionContent представляет содержимое вопроса.
type QuestionContent struct {
	Question       string               `json:"question"`
	ExpectedPoints []string             `json:"expected_points,omitempty"`
	LinksToTheory  []QuestionTheoryLink `json:"links_to_theory,omitempty"`
}

// QuestionTheoryLink представляет связь вопроса с теорией.
type QuestionTheoryLink struct {
	TheoryID    string `json:"theory_id"`
	SegmentType string `json:"segment_type"`
}

// QuestionIdealAnswer представляет идеальный ответ.
type QuestionIdealAnswer struct {
	Text   string   `json:"text"`
	Covers []string `json:"covers,omitempty"`
}

// QuestionMetadata представляет метаданные вопроса.
type QuestionMetadata struct {
	Version     string `json:"version,omitempty"`
	Language    string `json:"language"`
	CreatedBy   string `json:"created_by"`
	LastUpdated string `json:"last_updated"`
}

// Value реализует driver.Valuer для сохранения в JSONB.
func (qj QuestionJSONB) Value() (driver.Value, error) {
	return json.Marshal(qj)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (qj *QuestionJSONB) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, qj)
}
