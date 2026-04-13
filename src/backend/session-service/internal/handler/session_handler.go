package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/session-service/internal/metrics"
	"github.com/tensor-talks/session-service/internal/models"
	"github.com/tensor-talks/session-service/internal/service"
	"go.uber.org/zap"
)

// SessionHandler обрабатывает HTTP-запросы для управления сессиями.
type SessionHandler struct {
	svc    *service.SessionManagerService
	logger *zap.Logger
}

// NewSessionHandler создаёт новый обработчик сессий.
func NewSessionHandler(svc *service.SessionManagerService, logger *zap.Logger) *SessionHandler {
	return &SessionHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с сессиями.
func (h *SessionHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/sessions", h.CreateSession)
	router.GET("/sessions/:id/program", h.GetInterviewProgram)
	router.PUT("/sessions/:id/close", h.CloseSession)
}

type createSessionRequest struct {
	UserID uuid.UUID            `json:"user_id" binding:"required"`
	Params models.SessionParams `json:"params" binding:"required"`
}

// CreateSession создаёт новую сессию с параметрами интервью.
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("CreateSession: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	resp, err := h.svc.CreateSession(c.Request.Context(), req.UserID, req.Params)
	if err != nil {
		metrics.BusinessSessionsCreatedTotal.WithLabelValues("session-service", "error").Inc()
		h.logger.Error("CreateSession failed", zap.Error(err))

		errMsg := err.Error()
		if errMsg == "user already has an active session" || errMsg == "max active sessions reached" {
			c.JSON(http.StatusConflict, gin.H{"error": errMsg})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessSessionsCreatedTotal.WithLabelValues("session-service", "success").Inc()
	c.JSON(http.StatusCreated, resp)
}

// GetInterviewProgram возвращает программу интервью для сессии.
func (h *SessionHandler) GetInterviewProgram(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.logger.Warn("GetInterviewProgram: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	program, err := h.svc.GetInterviewProgram(c.Request.Context(), sessionID)
	if err != nil {
		metrics.BusinessSessionsOperationsTotal.WithLabelValues("session-service", "get_program", "error").Inc()
		h.logger.Error("GetInterviewProgram failed", zap.Error(err))

		if err.Error() == "interview program not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "interview program not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessSessionsOperationsTotal.WithLabelValues("session-service", "get_program", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"program": program})
}

// CloseSession закрывает сессию.
func (h *SessionHandler) CloseSession(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.logger.Warn("CloseSession: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	if err := h.svc.CloseSession(c.Request.Context(), sessionID); err != nil {
		metrics.BusinessSessionsOperationsTotal.WithLabelValues("session-service", "close", "error").Inc()
		h.logger.Error("CloseSession failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessSessionsOperationsTotal.WithLabelValues("session-service", "close", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
