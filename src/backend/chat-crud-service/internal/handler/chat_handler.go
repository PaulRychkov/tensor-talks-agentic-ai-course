package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/chat-crud-service/internal/metrics"
	"github.com/tensor-talks/chat-crud-service/internal/models"
	"github.com/tensor-talks/chat-crud-service/internal/repository"
	"github.com/tensor-talks/chat-crud-service/internal/service"
	"go.uber.org/zap"
)

// ChatHandler обрабатывает HTTP-запросы для работы с чатами.
type ChatHandler struct {
	svc    *service.ChatService
	logger *zap.Logger
}

// NewChatHandler создаёт новый обработчик чатов.
func NewChatHandler(svc *service.ChatService, logger *zap.Logger) *ChatHandler {
	return &ChatHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с чатами.
func (h *ChatHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/messages", h.SaveMessage)
	router.GET("/messages/:session_id", h.GetMessages)
	router.PATCH("/messages/:message_id/mask", h.MaskMessage)
	router.GET("/chat-active/:session_id", h.GetActiveChatJSON)
	router.GET("/chat-dumps/:session_id", h.GetChatDump)
	router.POST("/chat-dumps/:session_id", h.CreateChatDump)
}

type saveMessageRequest struct {
	SessionID uuid.UUID          `json:"session_id" binding:"required"`
	Type      models.MessageType `json:"type" binding:"required"`
	Content   string             `json:"content" binding:"required"`
	Metadata  models.MessageMetadata `json:"metadata"`
}

// SaveMessage сохраняет сообщение.
func (h *ChatHandler) SaveMessage(c *gin.Context) {
	var req saveMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("SaveMessage: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	message, err := h.svc.SaveMessage(c.Request.Context(), req.SessionID, req.Type, req.Content, req.Metadata)
	if err != nil {
		metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "save_message", "error").Inc()
		h.logger.Error("SaveMessage failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "save_message", "success").Inc()
	c.JSON(http.StatusCreated, gin.H{"message": message})
}

// GetMessages возвращает все сообщения сессии.
func (h *ChatHandler) GetMessages(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("GetMessages: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	messages, err := h.svc.GetMessagesBySessionID(c.Request.Context(), sessionID)
	if err != nil {
		metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_messages", "error").Inc()
		h.logger.Error("GetMessages failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_messages", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// GetActiveChatJSON возвращает JSON структуру незавершенного чата.
func (h *ChatHandler) GetActiveChatJSON(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("GetActiveChatJSON: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	chatJSON, err := h.svc.GetActiveChatJSON(c.Request.Context(), sessionID)
	if err != nil {
		metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_active_chat_json", "error").Inc()
		h.logger.Error("GetActiveChatJSON failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_active_chat_json", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"messages": chatJSON.Messages})
}

// GetChatDump возвращает дамп чата.
func (h *ChatHandler) GetChatDump(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("GetChatDump: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	dump, err := h.svc.GetChatDump(c.Request.Context(), sessionID)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_dump", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "chat dump not found"})
		} else {
			metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_dump", "error").Inc()
			h.logger.Error("GetChatDump failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "get_dump", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"dump": dump})
}

// MaskMessage заменяет содержимое сообщения плейсхолдером (PII-маскирование).
// Принимает Kafka message_id (UUID) из пути — ищет по metadata->>'message_id'.
func (h *ChatHandler) MaskMessage(c *gin.Context) {
	kafkaMessageID := c.Param("message_id")
	if kafkaMessageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message_id required"})
		return
	}

	var req struct {
		Placeholder string `json:"placeholder" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("MaskMessage: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if err := h.svc.MaskMessageByKafkaID(c.Request.Context(), kafkaMessageID, req.Placeholder); err != nil {
		if err == repository.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		} else {
			h.logger.Error("MaskMessage failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "mask_message", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// CreateChatDump создаёт дамп чата из сообщений.
func (h *ChatHandler) CreateChatDump(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("CreateChatDump: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	if err := h.svc.CreateChatDump(c.Request.Context(), sessionID); err != nil {
		metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "create_dump", "error").Inc()
		h.logger.Error("CreateChatDump failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessChatOperationsTotal.WithLabelValues("chat-crud-service", "create_dump", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
