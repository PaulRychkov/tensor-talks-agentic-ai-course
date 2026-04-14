package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
)


var requiredReportSections = []string{"summary", "errors_by_topic", "preparation_plan", "materials"}

// ValidationError represents a client-side validation failure (400).
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// ResultService инкапсулирует бизнес-логику для результатов.
type ResultService struct {
	repo repository.ResultRepository
}

// NewResultService создаёт новый экземпляр сервиса результатов.
func NewResultService(repo repository.ResultRepository) *ResultService {
	return &ResultService{repo: repo}
}

// CreateResult создаёт новый результат с валидацией report_json.
func (s *ResultService) CreateResult(ctx context.Context, result *models.Result) error {
	if len(result.ReportJSON) > 0 {
		if err := validateReportJSON(result.ReportJSON); err != nil {
			return err
		}
	}

	if result.SessionKind == "" {
		result.SessionKind = "interview"
	}
	if result.ResultFormatVersion == 0 {
		result.ResultFormatVersion = 1
	}

	return s.repo.Create(ctx, result)
}

// GetResult возвращает результат по session_id.
func (s *ResultService) GetResult(ctx context.Context, sessionID uuid.UUID) (*models.Result, error) {
	return s.repo.GetBySessionID(ctx, sessionID)
}

// UpdateRating сохраняет пользовательскую оценку (1-5) и комментарий.
func (s *ResultService) UpdateRating(ctx context.Context, sessionID uuid.UUID, rating int, comment string) error {
	if rating < 1 || rating > 5 {
		return &ValidationError{Message: "rating must be between 1 and 5"}
	}
	return s.repo.UpdateRating(ctx, sessionID, rating, comment)
}

// GetResults возвращает результаты по списку session_id.
func (s *ResultService) GetResults(ctx context.Context, sessionIDs []uuid.UUID) ([]models.Result, error) {
	return s.repo.GetBySessionIDs(ctx, sessionIDs)
}

// GetResultsByUserID возвращает последние N результатов для пользователя (§10.6).
func (s *ResultService) GetResultsByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]models.Result, error) {
	return s.repo.GetByUserID(ctx, userID, limit)
}

// GetProductMetrics возвращает агрегированные продуктовые метрики.
func (s *ResultService) GetProductMetrics(ctx context.Context) (*repository.ProductMetrics, error) {
	return s.repo.GetProductMetrics(ctx)
}

func validateReportJSON(raw json.RawMessage) error {
	var report map[string]json.RawMessage
	if err := json.Unmarshal(raw, &report); err != nil {
		return &ValidationError{Message: "report_json is not a valid JSON object"}
	}

	for _, section := range requiredReportSections {
		if _, ok := report[section]; !ok {
			return &ValidationError{Message: fmt.Sprintf("report_json missing required section: %s", section)}
		}
	}

	return nil
}
