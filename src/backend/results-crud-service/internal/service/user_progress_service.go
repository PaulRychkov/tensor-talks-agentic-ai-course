package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
)

// UserProgressService encapsulates business logic for user topic progress.
type UserProgressService struct {
	repo repository.UserProgressRepository
}

func NewUserProgressService(repo repository.UserProgressRepository) *UserProgressService {
	return &UserProgressService{repo: repo}
}

// UpsertProgress creates or updates a user topic progress record.
func (s *UserProgressService) UpsertProgress(ctx context.Context, progress *models.UserTopicProgress) error {
	if progress.UserID == uuid.Nil {
		return &ValidationError{Message: "user_id is required"}
	}
	if progress.TopicID == "" {
		return &ValidationError{Message: "topic_id is required"}
	}
	return s.repo.Upsert(ctx, progress)
}

// GetProgressByUser returns all topic progress records for a user.
func (s *UserProgressService) GetProgressByUser(ctx context.Context, userID uuid.UUID) ([]models.UserTopicProgress, error) {
	return s.repo.GetByUserID(ctx, userID)
}
