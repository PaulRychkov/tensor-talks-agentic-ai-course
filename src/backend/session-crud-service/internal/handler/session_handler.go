package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/session-crud-service/internal/metrics"
	"github.com/tensor-talks/session-crud-service/internal/models"
	"github.com/tensor-talks/session-crud-service/internal/repository"
	"github.com/tensor-talks/session-crud-service/internal/service"
	"go.uber.org/zap"
)

// SessionHandler обрабатывает HTTP-запросы для управления сессиями.
type SessionHandler struct {
	svc    *service.SessionService
	logger *zap.Logger
}

// NewSessionHandler создаёт новый обработчик сессий.
func NewSessionHandler(svc *service.SessionService, logger *zap.Logger) *SessionHandler {
	return &SessionHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с сессиями.
func (h *SessionHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/sessions", h.CreateSession)
	router.GET("/sessions/:id", h.GetSession)
	router.GET("/sessions/user/:user_id", h.GetSessionsByUserID)
	router.PUT("/sessions/:id/program", h.UpdateProgram)
	router.PUT("/sessions/:id/close", h.CloseSession)
	router.DELETE("/sessions/:id", h.DeleteSession)
}

type createSessionRequest struct {
	UserID uuid.UUID            `json:"user_id" binding:"required"`
	Params models.SessionParams `json:"params" binding:"required"`
}

// CreateSession создаёт новую сессию.
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("CreateSession: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	session, err := h.svc.CreateSession(c.Request.Context(), req.UserID, req.Params)
	if err != nil {
		metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "create", "error").Inc()
		h.logger.Error("CreateSession failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "create", "success").Inc()
	c.JSON(http.StatusCreated, gin.H{"session": session.ToPublic()})
}

// GetSession возвращает сессию по ID.
func (h *SessionHandler) GetSession(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.logger.Warn("GetSession: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	session, err := h.svc.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "get", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		} else {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "get", "error").Inc()
			h.logger.Error("GetSession failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "get", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"session": session.ToPublic()})
}

// GetSessionsByUserID возвращает все сессии пользователя.
func (h *SessionHandler) GetSessionsByUserID(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		h.logger.Warn("GetSessionsByUserID: invalid user id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	sessions, err := h.svc.GetSessionsByUserID(c.Request.Context(), userID)
	if err != nil {
		metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "get_by_user", "error").Inc()
		h.logger.Error("GetSessionsByUserID failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	result := make([]models.PublicSession, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, s.ToPublic())
	}

	metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "get_by_user", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"sessions": result})
}

type updateProgramRequest struct {
	Program *models.InterviewProgram `json:"program" binding:"required"`
}

// UpdateProgram обновляет программу интервью сессии.
func (h *SessionHandler) UpdateProgram(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.logger.Warn("UpdateProgram: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var req updateProgramRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("UpdateProgram: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if err := h.svc.UpdateProgram(c.Request.Context(), sessionID, req.Program); err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "update_program", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		} else {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "update_program", "error").Inc()
			h.logger.Error("UpdateProgram failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "update_program", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
		if err == repository.ErrNotFound {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "close", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		} else {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "close", "error").Inc()
			h.logger.Error("CloseSession failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "close", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeleteSession удаляет сессию.
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.logger.Warn("DeleteSession: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	if err := h.svc.DeleteSession(c.Request.Context(), sessionID); err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "delete", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		} else {
			metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "delete", "error").Inc()
			h.logger.Error("DeleteSession failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessSessionOperationsTotal.WithLabelValues("session-crud-service", "delete", "success").Inc()
	c.Status(http.StatusNoContent)
}
