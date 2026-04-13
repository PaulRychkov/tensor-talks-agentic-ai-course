package models

import (
	"time"

	"github.com/google/uuid"
)

// SessionParams представляет параметры интервьюируемого.
type SessionParams struct {
	Topics []string `json:"topics"`
	Level  string   `json:"level"` // junior, middle, senior
	Type   string   `json:"type"`  // interview, training
}

// InterviewProgram представляет программу интервью.
type InterviewProgram struct {
	Questions []QuestionItem `json:"questions"`
}

// QuestionItem представляет один пункт программы (вопрос + теория).
type QuestionItem struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Theory   string `json:"theory"`
	Order    int    `json:"order"`
}

// CachedSession представляет кэшированную сессию в Redis.
type CachedSession struct {
	SessionID        uuid.UUID        `json:"session_id"`
	UserID           uuid.UUID        `json:"user_id"`
	Params           SessionParams    `json:"params"`
	InterviewProgram InterviewProgram `json:"interview_program"`
	CachedAt         time.Time        `json:"cached_at"`
}
