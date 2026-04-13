package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/metrics"
	"github.com/tensor-talks/results-crud-service/internal/repository"
	"github.com/tensor-talks/results-crud-service/internal/service"
	"go.uber.org/zap"
)

// ResultHandler обрабатывает HTTP-запросы для работы с результатами.
type ResultHandler struct {
	svc    *service.ResultService
	logger *zap.Logger
}

// NewResultHandler создаёт новый обработчик результатов.
func NewResultHandler(svc *service.ResultService, logger *zap.Logger) *ResultHandler {
	return &ResultHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с результатами.
func (h *ResultHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/results", h.CreateResult)
	router.GET("/results/:session_id", h.GetResult)
	router.GET("/results", h.GetResults)
}

type createResultRequest struct {
	SessionID       uuid.UUID `json:"session_id" binding:"required"`
	Score           int       `json:"score" binding:"required"`
	Feedback        string    `json:"feedback" binding:"required"`
	TerminatedEarly bool      `json:"terminated_early,omitempty"`
}

// CreateResult создаёт новый результат.
func (h *ResultHandler) CreateResult(c *gin.Context) {
	var req createResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("CreateResult: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	result, err := h.svc.CreateResult(c.Request.Context(), req.SessionID, req.Score, req.Feedback, req.TerminatedEarly)
	if err != nil {
		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "create", "error").Inc()
		h.logger.Error("CreateResult failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "create", "success").Inc()
	c.JSON(http.StatusCreated, gin.H{"result": result})
}

// GetResult возвращает результат по session_id.
func (h *ResultHandler) GetResult(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("GetResult: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	result, err := h.svc.GetResult(c.Request.Context(), sessionID)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
		} else {
			metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get", "error").Inc()
			h.logger.Error("GetResult failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// GetResults возвращает результаты по списку session_id (query параметр session_ids, разделённые запятыми).
func (h *ResultHandler) GetResults(c *gin.Context) {
	sessionIDsStr := c.Query("session_ids")
	if sessionIDsStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_ids parameter is required"})
		return
	}

	sessionIDStrs := strings.Split(sessionIDsStr, ",")
	sessionIDs := make([]uuid.UUID, 0, len(sessionIDStrs))
	for _, idStr := range sessionIDStrs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			h.logger.Warn("GetResults: invalid session id", zap.String("id", idStr), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id: " + idStr})
			return
		}
		sessionIDs = append(sessionIDs, id)
	}

	results, err := h.svc.GetResults(c.Request.Context(), sessionIDs)
	if err != nil {
		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_many", "error").Inc()
		h.logger.Error("GetResults failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_many", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"results": results})
}
