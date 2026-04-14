package models

import (
	"time"

	"github.com/google/uuid"
)

// UserTopicProgress tracks whether a user completed theory for a topic,
// unlocking training mode for that topic.
type UserTopicProgress struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_user_topic" json:"user_id"`
	TopicID           string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_user_topic" json:"topic_id"`
	TheoryCompletedAt *time.Time `json:"theory_completed_at,omitempty"`
	SourceSessionID   *uuid.UUID `gorm:"type:uuid" json:"source_session_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (UserTopicProgress) TableName() string {
	return "user_topic_progress"
}
