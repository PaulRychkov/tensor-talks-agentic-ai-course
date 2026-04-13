package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MessageType представляет тип сообщения.
type MessageType string

const (
	MessageTypeSystem MessageType = "system"
	MessageTypeUser   MessageType = "user"
)

// Message представляет сообщение в чате.
type Message struct {
	ID        uint        `gorm:"primaryKey"`
	SessionID uuid.UUID   `gorm:"type:uuid;not null;index"`
	Type      MessageType `gorm:"type:varchar(20);not null"`
	Content   string      `gorm:"type:text;not null"`
	Metadata  MessageMetadata `gorm:"type:jsonb"`
	CreatedAt time.Time
}

// TableName указывает имя таблицы в БД.
func (Message) TableName() string {
	return "messages"
}

// ChatDump представляет дамп завершенного чата.
type ChatDump struct {
	ID        uint      `gorm:"primaryKey"`
	SessionID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
	Chat      ChatJSONB `gorm:"type:jsonb;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName указывает имя таблицы в БД.
func (ChatDump) TableName() string {
	return "chat_dumps"
}

// ChatJSONB представляет JSON структуру дампа чата.
type ChatJSONB struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage представляет сообщение в дампе чата.
type ChatMessage struct {
	Type      string          `json:"type"`
	Content   string          `json:"content"`
	Metadata  MessageMetadata `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// MessageMetadata представляет метаданные сообщения в JSONB.
type MessageMetadata map[string]any

// Value реализует driver.Valuer для сохранения в JSONB.
func (mm MessageMetadata) Value() (driver.Value, error) {
	return json.Marshal(mm)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (mm *MessageMetadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, mm)
}

// Value реализует driver.Valuer для сохранения в JSONB.
func (cj ChatJSONB) Value() (driver.Value, error) {
	return json.Marshal(cj)
}

// Scan реализует sql.Scanner для чтения из JSONB.
func (cj *ChatJSONB) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, cj)
}
