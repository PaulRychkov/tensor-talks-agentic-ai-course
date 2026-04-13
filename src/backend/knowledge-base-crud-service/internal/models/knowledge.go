package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// Knowledge представляет запись базы знаний в PostgreSQL.
type Knowledge struct {
	ID         string         `gorm:"primaryKey;type:varchar(255)"`
	Concept    string         `gorm:"type:varchar(255);not null;index"`
	Complexity int            `gorm:"type:integer;not null;index"`
	ParentID   *string        `gorm:"type:varchar(255);index"`
	Tags       pq.StringArray `gorm:"type:text[]"`
	Data       KnowledgeJSONB `gorm:"type:jsonb;not null"`
	Version    string         `gorm:"type:varchar(50);default:'1.0'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName указывает имя таблицы в БД.
func (Knowledge) TableName() string {
	return "knowledge"
}

// KnowledgeJSONB представляет JSON структуру базы знаний.
type KnowledgeJSONB struct {
	ID         string              `json:"id"`
	Concept    string              `json:"concept"`
	Complexity int                 `json:"complexity"`
	ParentID   *string             `json:"parent_id,omitempty"`
	Tags       []string            `json:"tags"`
	Segments   []KnowledgeSegment  `json:"segments"`
	Relations  []KnowledgeRelation `json:"relations"`
	Metadata   KnowledgeMetadata   `json:"metadata"`
}

// KnowledgeSegment представляет сегмент знания.
type KnowledgeSegment struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// KnowledgeRelation представляет связь между знаниями.
type KnowledgeRelation struct {
	TargetID     string `json:"target_id"`
	RelationType string `json:"relation_type"`
}

// KnowledgeMetadata представляет метаданные знания.
type KnowledgeMetadata struct {
	Version     string `json:"version"`
	Language    string `json:"language"`
	LastUpdated string `json:"last_updated"`
}

// Value реализует driver.Valuer для сохранения в JSONB.
func (kj KnowledgeJSONB) Value() (driver.Value, error) {
	return json.Marshal(kj)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (kj *KnowledgeJSONB) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, kj)
}
