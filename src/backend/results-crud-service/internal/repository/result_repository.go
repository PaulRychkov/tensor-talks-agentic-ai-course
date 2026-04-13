package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrNotFound обозначает, что результат не найден.
	ErrNotFound = errors.New("result not found")
)

// ResultRepository описывает интерфейс доступа к результатам.
type ResultRepository interface {
	Create(ctx context.Context, result *models.Result) error
	GetBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.Result, error)
	GetBySessionIDs(ctx context.Context, sessionIDs []uuid.UUID) ([]models.Result, error)
}

// GormResultRepository — реализация ResultRepository на основе GORM.
type GormResultRepository struct {
	db *gorm.DB
}

// NewGormResultRepository создаёт новый экземпляр репозитория результатов.
func NewGormResultRepository(db *gorm.DB) *GormResultRepository {
	return &GormResultRepository{db: db}
}

// Create создаёт новый результат.
func (r *GormResultRepository) Create(ctx context.Context, result *models.Result) error {
	if err := r.db.WithContext(ctx).Create(result).Error; err != nil {
		return err
	}
	return nil
}

// GetBySessionID возвращает результат по session_id.
func (r *GormResultRepository) GetBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.Result, error) {
	var result models.Result
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&result).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// GetBySessionIDs возвращает результаты по списку session_id.
func (r *GormResultRepository) GetBySessionIDs(ctx context.Context, sessionIDs []uuid.UUID) ([]models.Result, error) {
	var results []models.Result
	if err := r.db.WithContext(ctx).Where("session_id IN ?", sessionIDs).Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}
