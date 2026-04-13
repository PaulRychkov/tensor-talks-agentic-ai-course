package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/session-crud-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrNotFound обозначает, что сессия не найдена.
	ErrNotFound = errors.New("session not found")
)

// SessionRepository описывает интерфейс доступа к сессиям.
type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.Session, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.Session, error)
	GetActiveSessionByUserID(ctx context.Context, userID uuid.UUID) (*models.Session, error)
	Update(ctx context.Context, session *models.Session) error
	UpdateProgram(ctx context.Context, sessionID uuid.UUID, program *models.InterviewProgram) error
	UpdateEndTime(ctx context.Context, sessionID uuid.UUID, endTime *time.Time) error
	Delete(ctx context.Context, sessionID uuid.UUID) error
}

// GormSessionRepository — реализация SessionRepository на основе GORM.
type GormSessionRepository struct {
	db *gorm.DB
}

// NewGormSessionRepository создаёт новый экземпляр репозитория сессий.
func NewGormSessionRepository(db *gorm.DB) *GormSessionRepository {
	return &GormSessionRepository{db: db}
}

// Create создаёт новую сессию.
func (r *GormSessionRepository) Create(ctx context.Context, session *models.Session) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return err
	}
	return nil
}

// GetBySessionID возвращает сессию по ID.
func (r *GormSessionRepository) GetBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.Session, error) {
	var session models.Session
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &session, nil
}

// GetByUserID возвращает все сессии пользователя.
func (r *GormSessionRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.Session, error) {
	var sessions []models.Session
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("start_time DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveSessionByUserID возвращает активную сессию пользователя (где end_time IS NULL).
func (r *GormSessionRepository) GetActiveSessionByUserID(ctx context.Context, userID uuid.UUID) (*models.Session, error) {
	var session models.Session
	if err := r.db.WithContext(ctx).Where("user_id = ? AND end_time IS NULL", userID).Order("start_time DESC").First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &session, nil
}

// Update обновляет сессию.
func (r *GormSessionRepository) Update(ctx context.Context, session *models.Session) error {
	result := r.db.WithContext(ctx).Model(&models.Session{}).Where("session_id = ?", session.SessionID).Updates(session)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateProgram обновляет только программу интервью сессии.
func (r *GormSessionRepository) UpdateProgram(ctx context.Context, sessionID uuid.UUID, program *models.InterviewProgram) error {
	result := r.db.WithContext(ctx).Model(&models.Session{}).Where("session_id = ?", sessionID).Update("interview_program", program)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateEndTime обновляет только время окончания сессии.
func (r *GormSessionRepository) UpdateEndTime(ctx context.Context, sessionID uuid.UUID, endTime *time.Time) error {
	result := r.db.WithContext(ctx).Model(&models.Session{}).Where("session_id = ?", sessionID).Update("end_time", endTime)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete удаляет сессию.
func (r *GormSessionRepository) Delete(ctx context.Context, sessionID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&models.Session{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
