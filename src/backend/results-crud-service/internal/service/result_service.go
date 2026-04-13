package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
)

// ResultService инкапсулирует бизнес-логику для результатов.
type ResultService struct {
	repo repository.ResultRepository
}

// NewResultService создаёт новый экземпляр сервиса результатов.
func NewResultService(repo repository.ResultRepository) *ResultService {
	return &ResultService{repo: repo}
}

// CreateResult создаёт новый результат.
func (s *ResultService) CreateResult(ctx context.Context, sessionID uuid.UUID, score int, feedback string, terminatedEarly bool) (*models.Result, error) {
	result := &models.Result{
		SessionID:       sessionID,
		Score:           score,
		Feedback:        feedback,
		TerminatedEarly: terminatedEarly,
	}

	if err := s.repo.Create(ctx, result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetResult возвращает результат по session_id.
func (s *ResultService) GetResult(ctx context.Context, sessionID uuid.UUID) (*models.Result, error) {
	return s.repo.GetBySessionID(ctx, sessionID)
}

// GetResults возвращает результаты по списку session_id.
func (s *ResultService) GetResults(ctx context.Context, sessionIDs []uuid.UUID) ([]models.Result, error) {
	return s.repo.GetBySessionIDs(ctx, sessionIDs)
}
