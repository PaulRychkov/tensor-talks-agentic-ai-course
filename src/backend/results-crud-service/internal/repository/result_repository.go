package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrNotFound обозначает, что результат не найден.
	ErrNotFound = errors.New("result not found")
)

// ProductMetrics содержит агрегированные продуктовые метрики.
type ProductMetrics struct {
	TotalSessions      int64              `json:"total_sessions"`
	CompletedNaturally int64              `json:"completed_naturally"`
	TerminatedEarly    int64              `json:"terminated_early"`
	CompletionRate     float64            `json:"completion_rate"`
	AvgScore           float64            `json:"avg_score"`
	AvgRating          float64            `json:"avg_rating"`
	RatedSessions      int64              `json:"rated_sessions"`
	ByKind             map[string]int64   `json:"by_kind"`
}

// ResultRepository описывает интерфейс доступа к результатам.
type ResultRepository interface {
	Create(ctx context.Context, result *models.Result) error
	GetBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.Result, error)
	GetBySessionIDs(ctx context.Context, sessionIDs []uuid.UUID) ([]models.Result, error)
	GetByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]models.Result, error) // §10.6
	UpdateRating(ctx context.Context, sessionID uuid.UUID, rating int, comment string) error
	GetProductMetrics(ctx context.Context) (*ProductMetrics, error)
}

// GormResultRepository — реализация ResultRepository на основе GORM.
type GormResultRepository struct {
	db *gorm.DB
}

// NewGormResultRepository создаёт новый экземпляр репозитория результатов.
func NewGormResultRepository(db *gorm.DB) *GormResultRepository {
	return &GormResultRepository{db: db}
}

// Create создаёт или обновляет результат (upsert по session_id).
// Если результат для данной сессии уже существует, обновляет все поля.
func (r *GormResultRepository) Create(ctx context.Context, result *models.Result) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "session_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"score", "feedback", "terminated_early",
				"report_json", "preset_training", "result_format_version",
				"evaluations", "session_kind", "updated_at",
			}),
		}).
		Create(result).Error
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

// UpdateRating сохраняет оценку пользователя (1-5) и комментарий для сессии.
func (r *GormResultRepository) UpdateRating(ctx context.Context, sessionID uuid.UUID, rating int, comment string) error {
	res := r.db.WithContext(ctx).
		Model(&models.Result{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"user_rating":  rating,
			"user_comment": comment,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByUserID возвращает последние N результатов пользователя (§10.6).
func (r *GormResultRepository) GetByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]models.Result, error) {
	var results []models.Result
	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

// GetProductMetrics возвращает агрегированные продуктовые метрики (для admin-дашборда).
func (r *GormResultRepository) GetProductMetrics(ctx context.Context) (*ProductMetrics, error) {
	db := r.db.WithContext(ctx).Table("results")

	// Aggregate totals
	type aggRow struct {
		Total              int64   `gorm:"column:total"`
		CompletedNaturally int64   `gorm:"column:completed_naturally"`
		TerminatedEarly    int64   `gorm:"column:terminated_early"`
		AvgScore           float64 `gorm:"column:avg_score"`
		AvgRating          float64 `gorm:"column:avg_rating"`
		RatedSessions      int64   `gorm:"column:rated_sessions"`
	}
	var agg aggRow
	err := db.Select(
		"COUNT(*) AS total",
		"SUM(CASE WHEN terminated_early = false THEN 1 ELSE 0 END) AS completed_naturally",
		"SUM(CASE WHEN terminated_early = true THEN 1 ELSE 0 END) AS terminated_early",
		"COALESCE(AVG(score), 0) AS avg_score",
		"COALESCE(AVG(CASE WHEN user_rating IS NOT NULL THEN user_rating END), 0) AS avg_rating",
		"COUNT(user_rating) AS rated_sessions",
	).Scan(&agg).Error
	if err != nil {
		return nil, err
	}

	// By kind
	type kindRow struct {
		SessionKind string `gorm:"column:session_kind"`
		Count       int64  `gorm:"column:count"`
	}
	var kindRows []kindRow
	if err := r.db.WithContext(ctx).Table("results").
		Select("session_kind, COUNT(*) AS count").
		Group("session_kind").
		Scan(&kindRows).Error; err != nil {
		return nil, err
	}
	byKind := make(map[string]int64, len(kindRows))
	for _, kr := range kindRows {
		byKind[kr.SessionKind] = kr.Count
	}

	completionRate := 0.0
	if agg.Total > 0 {
		completionRate = float64(agg.CompletedNaturally) / float64(agg.Total) * 100
	}

	return &ProductMetrics{
		TotalSessions:      agg.Total,
		CompletedNaturally: agg.CompletedNaturally,
		TerminatedEarly:    agg.TerminatedEarly,
		CompletionRate:     completionRate,
		AvgScore:           agg.AvgScore,
		AvgRating:          agg.AvgRating,
		RatedSessions:      agg.RatedSessions,
		ByKind:             byKind,
	}, nil
}
