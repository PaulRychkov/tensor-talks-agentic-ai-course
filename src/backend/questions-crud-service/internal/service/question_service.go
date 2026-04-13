package service

import (
	"context"

	"github.com/tensor-talks/questions-crud-service/internal/models"
	"github.com/tensor-talks/questions-crud-service/internal/repository"
)

// QuestionService инкапсулирует бизнес-логику для базы вопросов.
type QuestionService struct {
	repo repository.QuestionRepository
}

// NewQuestionService создаёт новый экземпляр сервиса вопросов.
func NewQuestionService(repo repository.QuestionRepository) *QuestionService {
	return &QuestionService{repo: repo}
}

// CreateQuestion создаёт новый вопрос.
func (s *QuestionService) CreateQuestion(ctx context.Context, data models.QuestionJSONB) (*models.Question, error) {
	version := data.Metadata.Version
	if version == "" {
		version = "1.0"
	}
	question := &models.Question{
		ID:           data.ID,
		TheoryID:     data.TheoryID,
		QuestionType: data.QuestionType,
		Complexity:   data.Complexity,
		Data:         data,
		Version:      version,
	}

	if err := s.repo.CreateQuestion(ctx, question); err != nil {
		return nil, err
	}

	return question, nil
}

// GetQuestionByID возвращает вопрос по ID.
func (s *QuestionService) GetQuestionByID(ctx context.Context, id string) (*models.Question, error) {
	return s.repo.GetQuestionByID(ctx, id)
}

// UpdateQuestion обновляет вопрос.
func (s *QuestionService) UpdateQuestion(ctx context.Context, id string, data models.QuestionJSONB) (*models.Question, error) {
	existing, err := s.repo.GetQuestionByID(ctx, id)
	if err != nil {
		return nil, err
	}

	version := data.Metadata.Version
	if version == "" {
		version = "1.0"
	}
	existing.TheoryID = data.TheoryID
	existing.QuestionType = data.QuestionType
	existing.Complexity = data.Complexity
	existing.Data = data
	existing.Version = version

	if err := s.repo.UpdateQuestion(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

// DeleteQuestion удаляет вопрос.
func (s *QuestionService) DeleteQuestion(ctx context.Context, id string) error {
	return s.repo.DeleteQuestion(ctx, id)
}

// GetQuestionsByFilters возвращает вопросы по фильтрам.
func (s *QuestionService) GetQuestionsByFilters(ctx context.Context, filters repository.QuestionFilters) ([]models.Question, error) {
	return s.repo.GetQuestionsByFilters(ctx, filters)
}
