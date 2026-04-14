package service

import (
	"context"
	"fmt"

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

// SubtopicEntry represents a subtopic derived from a knowledge item.
type SubtopicEntry struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Topics []string `json:"topics"`
}

// GetSubtopics returns all knowledge items as subtopics grouped by top-level topic.
func (s *KnowledgeService) GetSubtopics(ctx context.Context) ([]SubtopicEntry, error) {
	all, err := s.repo.GetKnowledgeByFilters(ctx, repository.KnowledgeFilters{})
	if err != nil {
		return nil, err
	}

	topLevel := map[string]bool{
		"llm": true, "nlp": true, "machine_learning": true,
		"deep_learning": true, "neural_networks": true,
		"cv": true, "data_science": true,
	}

	result := make([]SubtopicEntry, 0, len(all))
	for _, k := range all {
		id := k.Data.ID
		if id == "" {
			id = k.ID
		}
		label := k.Data.Concept
		if label == "" {
			label = id
		}

		var topics []string
		for _, tag := range k.Tags {
			if topLevel[tag] {
				topics = append(topics, tag)
			}
		}
		// Map to frontend topic names
		for i, t := range topics {
			switch t {
			case "machine_learning", "deep_learning", "neural_networks":
				topics[i] = "classic_ml"
			}
		}
		// Deduplicate after mapping
		seen := map[string]bool{}
		deduped := topics[:0]
		for _, t := range topics {
			if !seen[t] {
				seen[t] = true
				deduped = append(deduped, t)
			}
		}
		topics = deduped
		if len(topics) == 0 {
			topics = []string{"classic_ml"}
		}

		result = append(result, SubtopicEntry{
			ID:     id,
			Label:  label,
			Topics: topics,
		})
	}
	return result, nil
}

// SemanticSearchResult represents a single result from semantic search (§10.3).
type SemanticSearchResult struct {
	ID         string  `json:"segment_id"`
	Content    string  `json:"content"`
	Topic      string  `json:"topic"`
	Similarity float64 `json:"similarity"`
}

// SearchSemantic performs cosine similarity search using pgvector (§10.3).
// Currently returns a stub response; requires pgvector migration and embedding client.
// Full implementation: query → embedding API → pgvector ORDER BY cosine distance.
func (s *KnowledgeService) SearchSemantic(
	ctx context.Context,
	query, topic string,
	limit int,
	threshold float64,
) ([]SemanticSearchResult, error) {
	// TODO (§10.3): Call embedding API to get query vector, then:
	// SELECT id, content, topic, (1 - (embedding <=> $1)) AS similarity
	// FROM knowledge_segments
	// WHERE status = 'published'
	//   AND ($2 = '' OR topic = $2)
	//   AND (1 - (embedding <=> $1)) >= $3
	// ORDER BY embedding <=> $1
	// LIMIT $4
	//
	// For now, return empty list to indicate semantic search is configured
	// but the embedding pipeline hasn't been set up yet.
	return []SemanticSearchResult{}, fmt.Errorf(
		"semantic search requires pgvector migration and embedding API configuration (§10.3)",
	)
}
