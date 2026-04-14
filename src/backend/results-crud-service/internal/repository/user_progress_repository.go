package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserProgressRepository describes access to user topic progress records.
type UserProgressRepository interface {
	Upsert(ctx context.Context, progress *models.UserTopicProgress) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.UserTopicProgress, error)
}

// GormUserProgressRepository — GORM implementation of UserProgressRepository.
type GormUserProgressRepository struct {
	db *gorm.DB
}

func NewGormUserProgressRepository(db *gorm.DB) *GormUserProgressRepository {
	return &GormUserProgressRepository{db: db}
}

// Upsert creates or updates a user topic progress record (unique on user_id + topic_id).
func (r *GormUserProgressRepository) Upsert(ctx context.Context, progress *models.UserTopicProgress) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "topic_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"theory_completed_at", "source_session_id"}),
		}).
		Create(progress).Error
}

// GetByUserID returns all topic progress records for a given user.
func (r *GormUserProgressRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.UserTopicProgress, error) {
	var records []models.UserTopicProgress
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&records).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return records, nil
}
