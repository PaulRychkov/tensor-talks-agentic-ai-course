package model

import (
	"sync"
	"time"

	"github.com/tensor-talks/dialogue-aggregator/internal/models"
)

// SessionState хранит состояние сессии чата.
type SessionState struct {
	SessionID      string
	UserID         string
	Program        *models.InterviewProgram // Программа интервью
	SessionKind    string                   // interview / training / study
	Topics         []string                 // Темы сессии (из params)
	Level          string                   // junior / middle / senior
	CurrentIndex   int                      // Текущий индекс вопроса в программе
	QuestionsAsked int
	LastQuestionAt time.Time
	StartedAt      time.Time
	mu             sync.RWMutex
}

// SessionManager управляет состоянием сессий.
type SessionManager struct {
	sessions sync.Map // map[string]*SessionState
}

// NewSessionManager создаёт новый менеджер сессий.
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// GetOrCreate получает или создаёт состояние сессии.
func (sm *SessionManager) GetOrCreate(sessionID, userID string) *SessionState {
	if state, ok := sm.sessions.Load(sessionID); ok {
		return state.(*SessionState)
	}

	newState := &SessionState{
		SessionID:      sessionID,
		UserID:         userID,
		Program:        nil,
		CurrentIndex:   0,
		QuestionsAsked: 0,
		StartedAt:      time.Now(),
		LastQuestionAt: time.Time{},
	}

	sm.sessions.Store(sessionID, newState)
	return newState
}

// SetProgram устанавливает программу интервью для сессии.
func (sm *SessionManager) SetProgram(sessionID string, program *models.InterviewProgram) {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.Lock()
		// Если программа уже установлена и есть текущий индекс, не сбрасываем его
		if s.Program == nil {
			s.CurrentIndex = 0
		}
		s.Program = program
		s.mu.Unlock()
	}
}

// SetMeta устанавливает метаданные сессии (kind/topics/level).
func (sm *SessionManager) SetMeta(sessionID, kind string, topics []string, level string) {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.Lock()
		s.SessionKind = kind
		s.Topics = topics
		s.Level = level
		s.mu.Unlock()
	}
}

// GetMeta возвращает метаданные сессии.
func (sm *SessionManager) GetMeta(sessionID string) (kind string, topics []string, level string) {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.SessionKind, s.Topics, s.Level
	}
	return "", nil, ""
}

// RestoreStateFromChatHistory восстанавливает состояние сессии на основе истории чата.
// Определяет, сколько вопросов уже задано, и устанавливает соответствующий CurrentIndex.
func (sm *SessionManager) RestoreStateFromChatHistory(sessionID string, program *models.InterviewProgram, systemMessagesCount int) {
	state := sm.GetOrCreate(sessionID, "")
	state.mu.Lock()
	defer state.mu.Unlock()

	state.Program = program
	// systemMessagesCount - количество сообщений типа "system", каждое соответствует вопросу
	// Устанавливаем CurrentIndex на следующий вопрос (уже задано systemMessagesCount вопросов)
	state.CurrentIndex = systemMessagesCount
	state.QuestionsAsked = systemMessagesCount
}

// GetNextQuestion возвращает следующий вопрос из программы и увеличивает индекс.
func (sm *SessionManager) GetNextQuestion(sessionID string) (models.QuestionItem, bool) {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.Program == nil || s.CurrentIndex >= len(s.Program.Questions) {
			return models.QuestionItem{}, false
		}

		question := s.Program.Questions[s.CurrentIndex]
		s.CurrentIndex++
		return question, true
	}
	return models.QuestionItem{}, false
}

// GetQuestionByIndex returns the question at a specific index without advancing CurrentIndex.
func (sm *SessionManager) GetQuestionByIndex(sessionID string, idx int) (models.QuestionItem, bool) {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.Program == nil || idx < 0 || idx >= len(s.Program.Questions) {
			return models.QuestionItem{}, false
		}
		return s.Program.Questions[idx], true
	}
	return models.QuestionItem{}, false
}

// GetProgram returns the interview program for a session.
func (sm *SessionManager) GetProgram(sessionID string) *models.InterviewProgram {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.Program
	}
	return nil
}

// HasMoreQuestions проверяет, есть ли ещё вопросы в программе.
func (sm *SessionManager) HasMoreQuestions(sessionID string) bool {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.RLock()
		defer s.mu.RUnlock()

		if s.Program == nil {
			return false
		}
		return s.CurrentIndex < len(s.Program.Questions)
	}
	return false
}

// IncrementQuestion увеличивает счётчик вопросов.
func (sm *SessionManager) IncrementQuestion(sessionID string) {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.Lock()
		s.QuestionsAsked++
		s.LastQuestionAt = time.Now()
		s.mu.Unlock()
	}
}

// GetQuestionCount возвращает количество заданных вопросов.
func (sm *SessionManager) GetQuestionCount(sessionID string) int {
	if state, ok := sm.sessions.Load(sessionID); ok {
		s := state.(*SessionState)
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.QuestionsAsked
	}
	return 0
}

// Delete удаляет сессию.
func (sm *SessionManager) Delete(sessionID string) {
	sm.sessions.Delete(sessionID)
}

// ShouldComplete проверяет, нужно ли завершить чат (все вопросы заданы).
func (sm *SessionManager) ShouldComplete(sessionID string) bool {
	return !sm.HasMoreQuestions(sessionID)
}
