package service

import (
	"context"

	"github.com/lib/pq"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/models"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/repository"
)

// KnowledgeService инкапсулирует бизнес-логику для базы знаний.
type KnowledgeService struct {
	repo repository.KnowledgeRepository
}

// NewKnowledgeService создаёт новый экземпляр сервиса знаний.
func NewKnowledgeService(repo repository.KnowledgeRepository) *KnowledgeService {
	return &KnowledgeService{repo: repo}
}

// CreateKnowledge создаёт новое знание.
func (s *KnowledgeService) CreateKnowledge(ctx context.Context, data models.KnowledgeJSONB) (*models.Knowledge, error) {
	knowledge := &models.Knowledge{
		ID:         data.ID,
		Concept:    data.Concept,
		Complexity: data.Complexity,
		ParentID:   data.ParentID,
		Tags:       pq.StringArray(data.Tags),
		Data:       data,
		Version:    data.Metadata.Version,
	}

	if err := s.repo.CreateKnowledge(ctx, knowledge); err != nil {
		return nil, err
	}

	return knowledge, nil
}

// GetKnowledgeByID возвращает знание по ID.
func (s *KnowledgeService) GetKnowledgeByID(ctx context.Context, id string) (*models.Knowledge, error) {
	return s.repo.GetKnowledgeByID(ctx, id)
}

// UpdateKnowledge обновляет знание.
func (s *KnowledgeService) UpdateKnowledge(ctx context.Context, id string, data models.KnowledgeJSONB) (*models.Knowledge, error) {
	existing, err := s.repo.GetKnowledgeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	existing.Concept = data.Concept
	existing.Complexity = data.Complexity
	existing.ParentID = data.ParentID
	existing.Tags = pq.StringArray(data.Tags)
	existing.Data = data
	existing.Version = data.Metadata.Version

	if err := s.repo.UpdateKnowledge(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

// DeleteKnowledge удаляет знание.
func (s *KnowledgeService) DeleteKnowledge(ctx context.Context, id string) error {
	return s.repo.DeleteKnowledge(ctx, id)
}

// GetKnowledgeByFilters возвращает знания по фильтрам.
func (s *KnowledgeService) GetKnowledgeByFilters(ctx context.Context, filters repository.KnowledgeFilters) ([]models.Knowledge, error) {
	return s.repo.GetKnowledgeByFilters(ctx, filters)
}
