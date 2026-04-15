package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"gorm.io/gorm"
)

// PresetRepository describes access to preset records.
type PresetRepository interface {
	Create(ctx context.Context, preset *models.Preset) error
	GetByID(ctx context.Context, presetID uuid.UUID) (*models.Preset, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.Preset, error)
	Delete(ctx context.Context, presetID uuid.UUID) error
}

// GormPresetRepository — GORM implementation of PresetRepository.
type GormPresetRepository struct {
	db *gorm.DB
}

func NewGormPresetRepository(db *gorm.DB) *GormPresetRepository {
	return &GormPresetRepository{db: db}
}

func (r *GormPresetRepository) Create(ctx context.Context, preset *models.Preset) error {
	return r.db.WithContext(ctx).Create(preset).Error
}

func (r *GormPresetRepository) GetByID(ctx context.Context, presetID uuid.UUID) (*models.Preset, error) {
	var preset models.Preset
	if err := r.db.WithContext(ctx).Where("preset_id = ?", presetID).First(&preset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &preset, nil
}

func (r *GormPresetRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.Preset, error) {
	var presets []models.Preset
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&presets).Error; err != nil {
		return nil, err
	}
	return presets, nil
}

func (r *GormPresetRepository) Delete(ctx context.Context, presetID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("preset_id = ?", presetID).Delete(&models.Preset{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
