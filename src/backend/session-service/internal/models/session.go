package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SessionParams представляет параметры интервьюируемого.
type SessionParams struct {
	Topics             []string `json:"topics"`
	Level              string   `json:"level"`                          // junior, middle, senior
	Type               string   `json:"type,omitempty"`                 // specialty: ml, nlp, llm, cv, ds
	Mode               string   `json:"mode"`                           // interview, training, study
	Source             string   `json:"source,omitempty"`               // manual, preset
	PresetID           string   `json:"preset_id,omitempty"`
	Subtopics          []string `json:"subtopics,omitempty"`            // selected subtopics (study/training)
	UsePreviousResults bool     `json:"use_previous_results,omitempty"` // enable episodic memory
	NumQuestions       *int     `json:"num_questions,omitempty"`        // interview duration override (5/10/15)
	FocusPoints        []string `json:"focus_points,omitempty"`         // specific points to focus on (from preset follow-up)
}

// InterviewProgram представляет программу интервью.
type InterviewProgram struct {
	Questions []QuestionItem `json:"questions"`
}

// QuestionItem представляет один пункт программы (вопрос + теория).
// Topic — нормализованный тег подтемы (например, "theory_rag"), используется
// в study-режиме для группировки вопросов по подтемам в плане.
type QuestionItem struct {
	ID       string `json:"id"`
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

// ProgramMeta содержит техническую метаинформацию о качестве сборки программы.
type ProgramMeta struct {
	ValidationPassed bool           `json:"validation_passed"`
	Coverage         map[string]int `json:"coverage,omitempty"`
	FallbackReason   *string        `json:"fallback_reason"`
	GeneratorVersion string         `json:"generator_version"`
}

// CachedSession представляет кэшированную сессию в Redis.
type CachedSession struct {
	SessionID        uuid.UUID        `json:"session_id"`
	UserID           uuid.UUID        `json:"user_id"`
	Params           SessionParams    `json:"params"`
	InterviewProgram InterviewProgram `json:"interview_program"`
	ProgramStatus    string           `json:"program_status,omitempty"` // ready, failed, pending
	ProgramMeta      *ProgramMeta     `json:"program_meta,omitempty"`
	ProgramVersion   string           `json:"program_version,omitempty"`
	CachedAt         time.Time        `json:"cached_at"`
}

// ProgramMetaFromMap parses a map[string]interface{} into ProgramMeta.
func ProgramMetaFromMap(raw map[string]interface{}) *ProgramMeta {
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var meta ProgramMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}
