package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/session-crud-service/internal/models"
	"github.com/tensor-talks/session-crud-service/internal/repository"
)

// SessionService инкапсулирует бизнес-логику для сессий.
type SessionService struct {
	repo repository.SessionRepository
}

// NewSessionService создаёт новый экземпляр сервиса сессий.
func NewSessionService(repo repository.SessionRepository) *SessionService {
	return &SessionService{repo: repo}
}

// CreateSession создаёт новую сессию.
func (s *SessionService) CreateSession(ctx context.Context, userID uuid.UUID, params models.SessionParams) (*models.Session, error) {
	session := &models.Session{
		UserID:    userID,
		StartTime: time.Now(),
		Params:    params,
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// GetSession возвращает сессию по ID.
func (s *SessionService) GetSession(ctx context.Context, sessionID uuid.UUID) (*models.Session, error) {
	return s.repo.GetBySessionID(ctx, sessionID)
}

// GetSessionsByUserID возвращает все сессии пользователя.
func (s *SessionService) GetSessionsByUserID(ctx context.Context, userID uuid.UUID) ([]models.Session, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// GetActiveSessionByUserID возвращает активную сессию пользователя (где end_time IS NULL).
func (s *SessionService) GetActiveSessionByUserID(ctx context.Context, userID uuid.UUID) (*models.Session, error) {
	return s.repo.GetActiveSessionByUserID(ctx, userID)
}

// UpdateProgram обновляет программу интервью сессии.
func (s *SessionService) UpdateProgram(ctx context.Context, sessionID uuid.UUID, program *models.InterviewProgram) error {
	return s.repo.UpdateProgram(ctx, sessionID, program)
}

// CloseSession закрывает сессию, устанавливая end_time.
func (s *SessionService) CloseSession(ctx context.Context, sessionID uuid.UUID) error {
	endTime := time.Now()
	return s.repo.UpdateEndTime(ctx, sessionID, &endTime)
}

// DeleteSession удаляет сессию.
func (s *SessionService) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	return s.repo.Delete(ctx, sessionID)
}
