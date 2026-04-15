package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/tensor-talks/results-crud-service/internal/metrics"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
	"github.com/tensor-talks/results-crud-service/internal/service"
	"go.uber.org/zap"
)

// PresetHandler handles HTTP requests for presets.
type PresetHandler struct {
	svc    *service.PresetService
	logger *zap.Logger
}

func NewPresetHandler(svc *service.PresetService, logger *zap.Logger) *PresetHandler {
	return &PresetHandler{svc: svc, logger: logger}
}

// RegisterRoutes registers preset routes on the given router.
func (h *PresetHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/presets", h.CreatePreset)
	router.GET("/presets/:preset_id", h.GetPreset)
	router.GET("/presets", h.GetPresets)
	router.DELETE("/presets/:preset_id", h.DeletePreset)
}

type createPresetRequest struct {
	UserID          uuid.UUID  `json:"user_id" binding:"required"`
	TargetMode      string     `json:"target_mode" binding:"required"`
	Topics          []string   `json:"topics,omitempty"`
	Materials       []string   `json:"materials,omitempty"`
	SourceSessionID *uuid.UUID `json:"source_session_id,omitempty"`
	ExpiresAt       *string    `json:"expires_at,omitempty"`
}

// CreatePreset creates a new preset.
func (h *PresetHandler) CreatePreset(c *gin.Context) {
	var req createPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("CreatePreset: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload: " + err.Error()})
		return
	}

	preset := &models.Preset{
		UserID:          req.UserID,
		TargetMode:      req.TargetMode,
		Topics:          pq.StringArray(req.Topics),
		Materials:       pq.StringArray(req.Materials),
		SourceSessionID: req.SourceSessionID,
	}

	if req.ExpiresAt != nil {
		t, err := parseTime(*req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires_at format, expected RFC3339"})
			return
		}
		preset.ExpiresAt = &t
	}

	if err := h.svc.CreatePreset(c.Request.Context(), preset); err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Message})
			return
		}

		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "create_preset", "error").Inc()
		h.logger.Error("CreatePreset failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "create_preset", "success").Inc()
	c.JSON(http.StatusCreated, gin.H{"preset": preset})
}

// GetPreset returns a preset by its id.
func (h *PresetHandler) GetPreset(c *gin.Context) {
	presetID, err := uuid.Parse(c.Param("preset_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset_id"})
		return
	}

	preset, err := h.svc.GetPresetByID(c.Request.Context(), presetID)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_preset", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		} else {
			metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_preset", "error").Inc()
			h.logger.Error("GetPreset failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_preset", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"preset": preset})
}

// DeletePreset removes a preset by its id.
func (h *PresetHandler) DeletePreset(c *gin.Context) {
	presetID, err := uuid.Parse(c.Param("preset_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset_id"})
		return
	}

	if err := h.svc.DeletePreset(c.Request.Context(), presetID); err != nil {
		if err == repository.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		} else {
			h.logger.Error("DeletePreset failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// GetPresets returns presets for a user (query: user_id).
func (h *PresetHandler) GetPresets(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id query parameter is required"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	presets, err := h.svc.GetPresetsByUser(c.Request.Context(), userID)
	if err != nil {
		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_presets", "error").Inc()
		h.logger.Error("GetPresets failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_presets", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"presets": presets})
}
