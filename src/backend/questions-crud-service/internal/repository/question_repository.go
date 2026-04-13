package repository

import (
	"context"
	"errors"

	"github.com/tensor-talks/questions-crud-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrNotFound обозначает, что запись не найдена.
	ErrNotFound = errors.New("not found")
)

// QuestionRepository описывает интерфейс доступа к базе вопросов.
type QuestionRepository interface {
	CreateQuestion(ctx context.Context, question *models.Question) error
	GetQuestionByID(ctx context.Context, id string) (*models.Question, error)
	UpdateQuestion(ctx context.Context, question *models.Question) error
	DeleteQuestion(ctx context.Context, id string) error
	GetQuestionsByFilters(ctx context.Context, filters QuestionFilters) ([]models.Question, error)
}

// QuestionFilters представляет фильтры для поиска вопросов.
type QuestionFilters struct {
	Complexity   *int
	TheoryID     *string
	QuestionType *string
	Tags         []string
}

// GormQuestionRepository — реализация QuestionRepository на основе GORM.
type GormQuestionRepository struct {
	db *gorm.DB
}

// NewGormQuestionRepository создаёт новый экземпляр репозитория вопросов.
func NewGormQuestionRepository(db *gorm.DB) *GormQuestionRepository {
	return &GormQuestionRepository{db: db}
}

// CreateQuestion создаёт новый вопрос.
func (r *GormQuestionRepository) CreateQuestion(ctx context.Context, question *models.Question) error {
	if err := r.db.WithContext(ctx).Create(question).Error; err != nil {
		return err
	}
	return nil
}

// GetQuestionByID возвращает вопрос по ID.
func (r *GormQuestionRepository) GetQuestionByID(ctx context.Context, id string) (*models.Question, error) {
	var question models.Question
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&question).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &question, nil
}

// UpdateQuestion обновляет вопрос.
func (r *GormQuestionRepository) UpdateQuestion(ctx context.Context, question *models.Question) error {
	result := r.db.WithContext(ctx).Model(&models.Question{}).Where("id = ?", question.ID).Updates(question)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteQuestion удаляет вопрос.
func (r *GormQuestionRepository) DeleteQuestion(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&models.Question{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetQuestionsByFilters возвращает вопросы по фильтрам.
func (r *GormQuestionRepository) GetQuestionsByFilters(ctx context.Context, filters QuestionFilters) ([]models.Question, error) {
	var questions []models.Question
	query := r.db.WithContext(ctx).Model(&models.Question{})

	if filters.Complexity != nil {
		query = query.Where("complexity = ?", *filters.Complexity)
	}
	if filters.TheoryID != nil {
		query = query.Where("theory_id = ?", *filters.TheoryID)
	}
	if filters.QuestionType != nil {
		query = query.Where("question_type = ?", *filters.QuestionType)
	}

	if err := query.Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}
