package repository

import (
	"context"
	"errors"

	"github.com/lib/pq"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrNotFound обозначает, что запись не найдена.
	ErrNotFound = errors.New("not found")
)

// KnowledgeRepository описывает интерфейс доступа к базе знаний.
type KnowledgeRepository interface {
	CreateKnowledge(ctx context.Context, knowledge *models.Knowledge) error
	GetKnowledgeByID(ctx context.Context, id string) (*models.Knowledge, error)
	UpdateKnowledge(ctx context.Context, knowledge *models.Knowledge) error
	DeleteKnowledge(ctx context.Context, id string) error
	GetKnowledgeByFilters(ctx context.Context, filters KnowledgeFilters) ([]models.Knowledge, error)
}

// KnowledgeFilters представляет фильтры для поиска знаний.
type KnowledgeFilters struct {
	Complexity *int
	Concept    *string
	ParentID   *string
	Tags       []string
}

// GormKnowledgeRepository — реализация KnowledgeRepository на основе GORM.
type GormKnowledgeRepository struct {
	db *gorm.DB
}

// NewGormKnowledgeRepository создаёт новый экземпляр репозитория знаний.
func NewGormKnowledgeRepository(db *gorm.DB) *GormKnowledgeRepository {
	return &GormKnowledgeRepository{db: db}
}

// CreateKnowledge создаёт новое знание.
func (r *GormKnowledgeRepository) CreateKnowledge(ctx context.Context, knowledge *models.Knowledge) error {
	if err := r.db.WithContext(ctx).Create(knowledge).Error; err != nil {
		return err
	}
	return nil
}

// GetKnowledgeByID возвращает знание по ID.
func (r *GormKnowledgeRepository) GetKnowledgeByID(ctx context.Context, id string) (*models.Knowledge, error) {
	var knowledge models.Knowledge
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&knowledge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &knowledge, nil
}

// UpdateKnowledge обновляет знание.
func (r *GormKnowledgeRepository) UpdateKnowledge(ctx context.Context, knowledge *models.Knowledge) error {
	result := r.db.WithContext(ctx).Model(&models.Knowledge{}).Where("id = ?", knowledge.ID).Updates(knowledge)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteKnowledge удаляет знание.
func (r *GormKnowledgeRepository) DeleteKnowledge(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&models.Knowledge{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetKnowledgeByFilters возвращает знания по фильтрам.
func (r *GormKnowledgeRepository) GetKnowledgeByFilters(ctx context.Context, filters KnowledgeFilters) ([]models.Knowledge, error) {
	var knowledge []models.Knowledge
	query := r.db.WithContext(ctx).Model(&models.Knowledge{})

	if filters.Complexity != nil {
		query = query.Where("complexity = ?", *filters.Complexity)
	}
	if filters.Concept != nil {
		query = query.Where("concept ILIKE ?", "%"+*filters.Concept+"%")
	}
	if filters.ParentID != nil {
		query = query.Where("parent_id = ?", *filters.ParentID)
	}
	if len(filters.Tags) > 0 {
		query = query.Where("tags && ?", pq.Array(filters.Tags))
	}

	if err := query.Find(&knowledge).Error; err != nil {
		return nil, err
	}
	return knowledge, nil
}
