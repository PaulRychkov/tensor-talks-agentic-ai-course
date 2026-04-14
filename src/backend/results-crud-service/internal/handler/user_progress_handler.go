package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/metrics"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/service"
	"go.uber.org/zap"
)

// UserProgressHandler handles HTTP requests for user topic progress.
type UserProgressHandler struct {
	svc    *service.UserProgressService
	logger *zap.Logger
}

func NewUserProgressHandler(svc *service.UserProgressService, logger *zap.Logger) *UserProgressHandler {
	return &UserProgressHandler{svc: svc, logger: logger}
}

// RegisterRoutes registers user-progress routes on the given router.
func (h *UserProgressHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/user-progress", h.UpsertProgress)
	router.GET("/user-progress", h.GetProgress)
}

type upsertProgressRequest struct {
	UserID            uuid.UUID  `json:"user_id" binding:"required"`
	TopicID           string     `json:"topic_id" binding:"required"`
	TheoryCompletedAt *string    `json:"theory_completed_at,omitempty"`
	SourceSessionID   *uuid.UUID `json:"source_session_id,omitempty"`
}

// UpsertProgress creates or updates a user topic progress record.
func (h *UserProgressHandler) UpsertProgress(c *gin.Context) {
	var req upsertProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("UpsertProgress: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload: " + err.Error()})
		return
	}

	progress := &models.UserTopicProgress{
		UserID:          req.UserID,
		TopicID:         req.TopicID,
		SourceSessionID: req.SourceSessionID,
	}

	if req.TheoryCompletedAt != nil {
		t, err := parseTime(*req.TheoryCompletedAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid theory_completed_at format, expected RFC3339"})
			return
		}
		progress.TheoryCompletedAt = &t
	}

	if err := h.svc.UpsertProgress(c.Request.Context(), progress); err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Message})
			return
		}

		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "upsert_progress", "error").Inc()
		h.logger.Error("UpsertProgress failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "upsert_progress", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"progress": progress})
}

// GetProgress returns all topic progress for a user (query: user_id).
func (h *UserProgressHandler) GetProgress(c *gin.Context) {
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

	records, err := h.svc.GetProgressByUser(c.Request.Context(), userID)
	if err != nil {
		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_progress", "error").Inc()
		h.logger.Error("GetProgress failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_progress", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"progress": records})
}
