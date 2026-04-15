package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
)

var validTargetModes = map[string]bool{
	"study":    true,
	"training": true,
}

// PresetService encapsulates business logic for presets.
type PresetService struct {
	repo repository.PresetRepository
}

func NewPresetService(repo repository.PresetRepository) *PresetService {
	return &PresetService{repo: repo}
}

// CreatePreset validates and persists a new preset.
func (s *PresetService) CreatePreset(ctx context.Context, preset *models.Preset) error {
	if preset.UserID == uuid.Nil {
		return &ValidationError{Message: "user_id is required"}
	}
	if !validTargetModes[preset.TargetMode] {
		return &ValidationError{Message: "target_mode must be 'study' or 'training'"}
	}
	return s.repo.Create(ctx, preset)
}

// GetPresetByID returns a preset by its id.
func (s *PresetService) GetPresetByID(ctx context.Context, presetID uuid.UUID) (*models.Preset, error) {
	return s.repo.GetByID(ctx, presetID)
}

// GetPresetsByUser returns all presets for a given user.
func (s *PresetService) GetPresetsByUser(ctx context.Context, userID uuid.UUID) ([]models.Preset, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// DeletePreset removes a preset by ID.
func (s *PresetService) DeletePreset(ctx context.Context, presetID uuid.UUID) error {
	return s.repo.Delete(ctx, presetID)
}
