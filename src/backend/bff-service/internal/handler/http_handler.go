package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tensor-talks/bff-service/internal/client"
	"github.com/tensor-talks/bff-service/internal/middleware"
	"github.com/tensor-talks/bff-service/internal/service"
	"go.uber.org/zap"
)

var messageFeedbackTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "tensortalks_message_feedback_total",
		Help: "Per-message user ratings submitted in chat",
	},
	[]string{"rating"},
)

func init() {
	prometheus.MustRegister(messageFeedbackTotal)
}

// ── PII guardrail (Level 1 — BFF, before Kafka publish) ──────────────────────
// Быстрая regex-проверка на персональные данные (152-ФЗ).
// Если обнаружены PII — сообщение отклоняется до публикации в Kafka и сохранения в chat-crud.
var (
	piiEmail    = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	piiPhone    = regexp.MustCompile(`(?:\+7|8)[\s\-]?\(?\d{3}\)?[\s\-]?\d{3}[\s\-]?\d{2}[\s\-]?\d{2}`)
	piiPassport = regexp.MustCompile(`\b\d{4}\s?\d{6}\b`)
	piiINN      = regexp.MustCompile(`(?i)\bИНН\s*:?\s*\d{10,12}\b`)
	piiSNILS    = regexp.MustCompile(`\b\d{3}-\d{3}-\d{3}\s?\d{2}\b`)
	piiCard     = regexp.MustCompile(`\b(?:\d{4}[\s\-]){3}\d{4}\b`)
)

// detectPII возвращает категорию PII если найдена, или "" если чисто.
func detectPII(text string) string {
	switch {
	case piiEmail.MatchString(text):
		return "email"
	case piiPhone.MatchString(text):
		return "телефон"
	case piiCard.MatchString(text):
		return "банковская карта"
	case piiSNILS.MatchString(text):
		return "СНИЛС"
	case piiINN.MatchString(text):
		return "ИНН"
	case piiPassport.MatchString(text):
		return "паспортные данные"
	}
	return ""
}

/*
Пакет handler реализует HTTP-слой BFF (backend-for-frontend) сервиса.

Основная задача:
  - предоставить фронтенду стабильное и упрощённое API;
  - проксировать запросы аутентификации в auth-service, не раскрывая внутреннюю топологию микросервисов.

Важно: BFF не имеет прямого доступа ни к user-crud-service, ни к базе данных.
*/

// Handler инкапсулирует HTTP-эндпоинты, которые вызываются фронтендом.
type Handler struct {
	auth          *service.AuthService
	chat          *service.ChatService
	userCrudURL   string // base URL для user-crud-service (для /generate-random-login)
	knowledgeCRUD *client.KnowledgeCRUDClient
	logger        *zap.Logger
}

// New создаёт новый обработчик HTTP-запросов BFF.
func New(auth *service.AuthService, chat *service.ChatService, logger *zap.Logger) *Handler {
	return &Handler{auth: auth, chat: chat, logger: logger}
}

// NewWithUserCrud создаёт обработчик с userCrudURL для проксирования запросов генерации логина.
func NewWithUserCrud(auth *service.AuthService, chat *service.ChatService, userCrudURL string, logger *zap.Logger) *Handler {
	return &Handler{auth: auth, chat: chat, userCrudURL: userCrudURL, logger: logger}
}

// SetKnowledgeCRUD sets the knowledge-base-crud client for subtopics endpoint.
func (h *Handler) SetKnowledgeCRUD(c *client.KnowledgeCRUDClient) {
	h.knowledgeCRUD = c
}

// RegisterRoutes регистрирует маршруты BFF под префиксом /api.
// Все маршруты здесь являются внешним API для фронтенда.
func (h *Handler) RegisterRoutes(router gin.IRouter) {
	api := router.Group("/api")
	{
		auth := api.Group("/auth")
		auth.POST("/register", h.register)
		auth.POST("/login", h.login)
		auth.POST("/refresh", h.refresh)
		auth.GET("/me", h.me)

		chat := api.Group("/chat")
		chat.Use(middleware.AuthMiddleware(h.auth, h.logger))
		chat.POST("/start", h.startChat)
		chat.POST("/message", h.sendMessage)
		chat.POST("/:session_id/resume", h.resumeChat)
		chat.POST("/:session_id/terminate", h.terminateChat)
		chat.GET("/:session_id/question", h.getNextQuestion)
		chat.GET("/:session_id/results", h.getResults)
		chat.POST("/:session_id/message-feedback", h.submitMessageFeedback)

		interviews := api.Group("/interviews")
		interviews.Use(middleware.AuthMiddleware(h.auth, h.logger))
		interviews.Use(func(c *gin.Context) {
			h.logger.Info("Interviews group request",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.String("query", c.Request.URL.RawQuery),
			)
		})
		// Важно: сначала регистрируем конкретные маршруты, потом параметризованные
		interviews.GET("", h.getInterviews)
		interviews.GET("/:session_id/chat", h.getInterviewChat)
		interviews.GET("/:session_id/result", h.getInterviewResult)
		interviews.POST("/:session_id/rating", h.submitInterviewRating)

		// Dashboard endpoints (§10.2) — proxy to results-crud and session-crud services
		dashboard := api.Group("/dashboard")
		dashboard.Use(middleware.AuthMiddleware(h.auth, h.logger))
		dashboard.GET("/summary", h.getDashboardSummary)
		dashboard.GET("/activity", h.getDashboardActivity)
		dashboard.GET("/topic-progress", h.getDashboardTopicProgress)
		dashboard.GET("/recommendations", h.getDashboardRecommendations)

		// Knowledge base (public, no auth required)
		api.GET("/subtopics", h.getSubtopics)

		// Presets (study/training follow-ups created by analyst)
		presets := api.Group("/presets")
		presets.Use(middleware.AuthMiddleware(h.auth, h.logger))
		presets.GET("", h.getPresets)
		presets.DELETE("/:preset_id", h.deletePreset)

		// Public auth endpoints (no token required)
		auth.POST("/recover", h.recover)

		// Auth management endpoints (require Bearer token)
		authManage := api.Group("/auth")
		authManage.Use(middleware.AuthMiddleware(h.auth, h.logger))
		authManage.POST("/logout", h.logout)
		authManage.POST("/change-password", h.changePassword)
		authManage.POST("/regenerate-recovery-key", h.regenerateRecoveryKey)
		authManage.DELETE("/account", h.deleteAccount)

		// User utility endpoints
		users := api.Group("/users")
		users.GET("/generate-random-login", h.generateRandomLogin)
		users.GET("/login-words/adjectives", h.getLoginAdjectives)
		users.GET("/login-words/nouns", h.getLoginNouns)
		users.POST("/login-words/check-availability", h.checkLoginAvailability)
	}
}

type credentialsRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// register обрабатывает POST /api/auth/register.
// Детали:
// deletePreset удаляет использованный пресет.
func (h *Handler) deletePreset(c *gin.Context) {
	presetID, err := uuid.Parse(c.Param("preset_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid preset_id"})
		return
	}

	if err := h.chat.DeletePreset(c.Request.Context(), presetID); err != nil {
		h.logger.Error("Delete preset failed", zap.Error(err), zap.String("preset_id", presetID.String()))
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete preset"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// getPresets возвращает пресеты (study/training follow-ups) пользователя.
func (h *Handler) getPresets(c *gin.Context) {
	userIDValue, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	userIDStr, ok := userIDValue.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	presets, err := h.chat.GetPresetsByUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Get presets failed", zap.Error(err), zap.String("user_id", userID.String()))
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to get presets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"presets": presets})
}

// getSubtopics returns available subtopics from the knowledge base.
func (h *Handler) getSubtopics(c *gin.Context) {
	if h.knowledgeCRUD == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge service not configured"})
		return
	}
	subtopics, err := h.knowledgeCRUD.GetSubtopics(c.Request.Context())
	if err != nil {
		h.logger.Error("getSubtopics failed", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "knowledge service unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subtopics": subtopics})
}

//   - принимает JSON `{ "login": "...", "password": "..." }` от фронтенда;
//   - делегирует регистрацию в `AuthService`, который вызывает `auth-service`;
//   - маппит доменные ошибки BFF (конфликт, некорректный ввод) на HTTP-коды 409/400;
//   - при успехе возвращает 201 с пользователем и токенами, не модифицируя формат ответа.
func (h *Handler) register(c *gin.Context) {
	var req credentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Register: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	h.logger.Info("Register request", zap.String("login", req.Login))
	resp, err := h.auth.Register(c.Request.Context(), req.Login, req.Password)
	if err != nil {
		switch {
		case service.IsError(err, service.ErrConflict):
			h.logger.Warn("Register failed: conflict", zap.String("login", req.Login))
			c.JSON(http.StatusConflict, gin.H{"error": "login already exists"})
		case service.IsError(err, service.ErrBadRequest):
			h.logger.Warn("Register failed: bad request", zap.String("login", req.Login), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrorMessage(err)})
		default:
			h.logger.Error("Register failed: internal error", zap.Error(err), zap.String("login", req.Login))
			// Пытаемся получить детальное сообщение об ошибке, если оно доступно
			errorMsg := service.ErrorMessage(err)
			if errorMsg == err.Error() {
				// Если сообщение не извлечено, используем общее
				errorMsg = "internal error"
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": errorMsg})
		}
		return
	}

	h.logger.Info("Register successful", zap.String("login", req.Login))
	c.JSON(http.StatusCreated, resp)
}

// login обрабатывает POST /api/auth/login.
// Детали:
//   - принимает логин и пароль;
//   - передаёт их в `AuthService`, который проксирует запрос в `auth-service`;
//   - при неверных учётных данных возвращает 401 с единым сообщением `invalid credentials`,
//     не раскрывая, существует ли указанный логин;
//   - при успехе возвращает 200 с пользователем и парой токенов.
func (h *Handler) login(c *gin.Context) {
	var req credentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Login: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	h.logger.Info("Login request", zap.String("login", req.Login))
	resp, err := h.auth.Login(c.Request.Context(), req.Login, req.Password)
	if err != nil {
		switch {
		case service.IsError(err, service.ErrInvalidCredentials):
			h.logger.Warn("Login failed: invalid credentials", zap.String("login", req.Login))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		case service.IsError(err, service.ErrBadRequest):
			h.logger.Warn("Login failed: bad request", zap.String("login", req.Login), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrorMessage(err)})
		default:
			h.logger.Error("Login failed: internal error", zap.Error(err), zap.String("login", req.Login))
			// Пытаемся получить детальное сообщение об ошибке, если оно доступно
			errorMsg := service.ErrorMessage(err)
			if errorMsg == err.Error() {
				// Если сообщение не извлечено, используем общее
				errorMsg = "internal error"
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": errorMsg})
		}
		return
	}

	h.logger.Info("Login successful", zap.String("login", req.Login))
	c.JSON(http.StatusOK, resp)
}

// refresh обрабатывает POST /api/auth/refresh.
// Принимает JSON `{ "refresh_token": "..." }`, передаёт его в `AuthService` и
// при успехе возвращает новую пару токенов. Валидация и проверка срока действия
// токена происходят в `auth-service`, BFF только маппит ошибки на HTTP-ответы.
func (h *Handler) refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Refresh: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	h.logger.Info("Refresh token request")
	resp, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		switch {
		case service.IsError(err, service.ErrInvalidCredentials):
			h.logger.Warn("Refresh failed: invalid token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		default:
			h.logger.Error("Refresh failed: internal error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	h.logger.Info("Refresh successful")
	c.JSON(http.StatusOK, resp)
}

// me обрабатывает GET /api/auth/me.
// Достаёт access-токен из заголовка `Authorization: Bearer <token>`, проксирует
// запрос в `auth-service /auth/me` через `AuthService` и возвращает данные пользователя.
// При отсутствии или некорректности токена возвращает 401.
func (h *Handler) me(c *gin.Context) {
	raw := c.GetHeader("Authorization")
	token := extractBearer(raw)
	if token == "" {
		h.logger.Warn("Me: missing token")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}

	h.logger.Info("Get current user request")
	user, err := h.auth.CurrentUser(c.Request.Context(), token)
	if err != nil {
		if service.IsError(err, service.ErrInvalidCredentials) {
			h.logger.Warn("Me: invalid token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		h.logger.Error("Me: internal error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	h.logger.Info("Get current user successful")
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// extractBearer извлекает значение Bearer-токена из заголовка Authorization.
func extractBearer(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

type startChatRequest struct {
	Params client.SessionParams `json:"params" binding:"required"`
}

type startChatResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Ready     bool      `json:"ready"`
}

// startChat обрабатывает POST /api/chat/start.
func (h *Handler) startChat(c *gin.Context) {
	startTime := time.Now()
	logDuration := func(status string) {
		h.logger.Info("StartChat completed",
			zap.String("status", status),
			zap.Duration("duration", time.Since(startTime)),
		)
	}
	// Получаем user_id из контекста (установлен middleware)
	userIDValue, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.logger.Warn("StartChat: user ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		logDuration("unauthorized")
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		h.logger.Warn("StartChat: invalid user ID type in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		logDuration("invalid_user_type")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Warn("StartChat: invalid user ID format", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		logDuration("invalid_user_id")
		return
	}

	var req startChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("StartChat: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		logDuration("invalid_payload")
		return
	}
	if len(req.Params.Topics) == 0 || req.Params.Mode == "" ||
		(req.Params.Level == "" && req.Params.Type == "") {
		h.logger.Warn("StartChat: missing params fields")
		c.JSON(http.StatusBadRequest, gin.H{"error": "params must include topics, mode, and level or type"})
		logDuration("invalid_params")
		return
	}

	h.logger.Info("Start chat request", zap.String("user_id", userID.String()))
	// Create a context with timeout for session creation (allows time for interview program building)
	// Session service waits 30s for interview program, so we need at least 35s+ context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	sessionID, err := h.chat.StartChat(ctx, userID, req.Params)
	if err != nil {
		if err.Error() == "max active sessions reached" {
			h.logger.Warn("Start chat failed: max active sessions reached", zap.String("user_id", userID.String()))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "max active sessions reached"})
			logDuration("max_active_sessions")
			return
		}
		h.logger.Error("Start chat failed", zap.Error(err), zap.String("user_id", userID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start chat"})
		logDuration("start_failed")
		return
	}

	h.logger.Info("Chat started successfully", zap.String("session_id", sessionID.String()))
	c.JSON(http.StatusCreated, startChatResponse{
		SessionID: sessionID,
		Ready:     true,
	})
	logDuration("success")
}

type sendMessageRequest struct {
	SessionID uuid.UUID `json:"session_id" binding:"required"`
	Content   string    `json:"content" binding:"required"`
}

// sendMessage обрабатывает POST /api/chat/message.
func (h *Handler) sendMessage(c *gin.Context) {
	startTime := time.Now()
	logDuration := func(status string) {
		h.logger.Info("SendMessage completed",
			zap.String("status", status),
			zap.Duration("duration", time.Since(startTime)),
		)
	}
	// Получаем user_id из контекста (установлен middleware)
	userIDValue, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.logger.Warn("SendMessage: user ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		logDuration("unauthorized")
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		h.logger.Warn("SendMessage: invalid user ID type in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		logDuration("invalid_user_type")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Warn("SendMessage: invalid user ID format", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		logDuration("invalid_user_id")
		return
	}

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("SendMessage: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		logDuration("invalid_payload")
		return
	}

	h.logger.Info("Send message request",
		zap.String("session_id", req.SessionID.String()),
		zap.String("user_id", userID.String()),
	)

	// PII guardrail (Level 1 — BFF, §10.11): блокируем до Kafka
	if category := detectPII(req.Content); category != "" {
		h.logger.Info("PII detected in BFF, blocking message",
			zap.String("session_id", req.SessionID.String()),
			zap.String("category", category),
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":        "Пожалуйста, не указывайте персональные данные (" + category + ") — требование 152-ФЗ. Перефразируйте ответ без личной информации.",
			"pii_detected": true,
			"category":     category,
		})
		logDuration("pii_blocked")
		return
	}

	if err := h.chat.SendMessage(c.Request.Context(), req.SessionID, userID, req.Content); err != nil {
		h.logger.Error("Send message failed",
			zap.Error(err),
			zap.String("session_id", req.SessionID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
		logDuration("send_failed")
		return
	}

	h.logger.Info("Message sent successfully", zap.String("session_id", req.SessionID.String()))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
	logDuration("success")
}

// resumeChat обрабатывает POST /api/chat/:session_id/resume (восстановление активной сессии чата).
func (h *Handler) resumeChat(c *gin.Context) {
	// Получаем user_id из контекста (установлен middleware)
	userIDValue, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.logger.Warn("ResumeChat: user ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		h.logger.Warn("ResumeChat: invalid user ID type in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Warn("ResumeChat: invalid user ID format", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	sessionIDStr := c.Param("session_id")
	if sessionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	h.logger.Info("Resume chat request",
		zap.String("session_id", sessionID.String()),
		zap.String("user_id", userID.String()),
	)

	if err := h.chat.ResumeChat(c.Request.Context(), sessionID, userID); err != nil {
		if err.Error() == "session already completed" {
			h.logger.Warn("Resume chat failed: session already completed",
				zap.String("session_id", sessionID.String()),
			)
			c.JSON(http.StatusBadRequest, gin.H{"error": "session already completed"})
			return
		}
		h.logger.Error("Resume chat failed",
			zap.Error(err),
			zap.String("session_id", sessionID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resume chat"})
		return
	}

	h.logger.Info("Chat resumed successfully", zap.String("session_id", sessionID.String()))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// terminateChat обрабатывает POST /api/chat/:session_id/terminate (досрочное завершение чата).
func (h *Handler) terminateChat(c *gin.Context) {
	// Получаем user_id из контекста (установлен middleware)
	userIDValue, exists := c.Get(middleware.UserIDKey)
	if !exists {
		h.logger.Warn("TerminateChat: user ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		h.logger.Warn("TerminateChat: invalid user ID type in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Warn("TerminateChat: invalid user ID format", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	sessionIDStr := c.Param("session_id")
	if sessionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	h.logger.Info("Terminate chat request",
		zap.String("session_id", sessionID.String()),
		zap.String("user_id", userID.String()),
	)

	if err := h.chat.TerminateChat(c.Request.Context(), sessionID, userID); err != nil {
		h.logger.Error("Terminate chat failed",
			zap.Error(err),
			zap.String("session_id", sessionID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to terminate chat"})
		return
	}

	h.logger.Info("Chat terminated successfully", zap.String("session_id", sessionID.String()))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type questionResponse struct {
	Question         string `json:"question"`
	QuestionID       string `json:"question_id"`
	Timestamp        string `json:"timestamp"`
	QuestionNumber   int    `json:"question_number"`
	TotalQuestions   int    `json:"total_questions"`
	PIIMaskedContent string `json:"pii_masked_content,omitempty"`
}

// getNextQuestion обрабатывает GET /api/chat/:session_id/question (polling для получения вопросов).
func (h *Handler) getNextQuestion(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	h.logger.Info("GetNextQuestion called",
		zap.String("session_id", sessionID),
		zap.String("path", c.Request.URL.Path),
	)

	question, ok := h.chat.GetNextQuestion(sessionID)
	if !ok {
		// Возвращаем 200 с пустым ответом вместо 404 для polling (чтобы не засорять консоль)
		h.logger.Info("No question available for session",
			zap.String("session_id", sessionID),
		)
		processingStep := h.chat.GetProcessingStep(sessionID)
		c.JSON(http.StatusOK, gin.H{"question": nil, "available": false, "processing_step": processingStep})
		return
	}

	h.logger.Info("Question retrieved",
		zap.String("session_id", sessionID),
		zap.String("question_id", question.QuestionID),
	)

	c.JSON(http.StatusOK, questionResponse{
		Question:         question.Question,
		QuestionID:       question.QuestionID,
		Timestamp:        question.Timestamp.Format(time.RFC3339),
		QuestionNumber:   question.QuestionNumber,
		TotalQuestions:   question.TotalQuestions,
		PIIMaskedContent: question.PIIMaskedContent,
	})
}

type resultsResponse struct {
	Score           int      `json:"score"`
	Feedback        string   `json:"feedback"`
	Recommendations []string `json:"recommendations"`
	CompletedAt     string   `json:"completed_at"`
}

// interviewResultLegacy — ответ для результатов без report_json (обратная совместимость с UI).
type interviewResultLegacy struct {
	ID              uint      `json:"id"`
	SessionID       uuid.UUID `json:"session_id"`
	Score           int       `json:"score"`
	Feedback        string    `json:"feedback"`
	TerminatedEarly bool      `json:"terminated_early"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// interviewResultWithReport — полный ответ, когда в CRUD есть отчёт.
type interviewResultWithReport struct {
	ID                  uint            `json:"id"`
	SessionID           uuid.UUID       `json:"session_id"`
	Score               int             `json:"score"`
	Feedback            string          `json:"feedback"`
	TerminatedEarly     bool            `json:"terminated_early"`
	ReportJSON          json.RawMessage `json:"report_json"`
	PresetTraining      json.RawMessage `json:"preset_training,omitempty"`
	Evaluations         json.RawMessage `json:"evaluations,omitempty"`
	ResultFormatVersion int             `json:"result_format_version"`
	SessionKind         string          `json:"session_kind,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

func reportJSONPresent(raw json.RawMessage) bool {
	b := bytes.TrimSpace(raw)
	if len(b) == 0 {
		return false
	}
	return string(b) != "null"
}

// getResults обрабатывает GET /api/chat/:session_id/results (получение результатов чата).
func (h *Handler) getResults(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	results, ok := h.chat.GetResults(sessionID)
	if !ok {
		// Возвращаем 200 с пустым ответом вместо 404 для polling (чтобы не засорять консоль)
		c.JSON(http.StatusOK, gin.H{"results": nil, "available": false, "completed": false})
		return
	}

	if results.Pending {
		// Чат завершён, аналитик ещё формирует отчёт
		c.JSON(http.StatusOK, gin.H{"results": nil, "available": false, "completed": true, "pending": true})
		return
	}

	h.logger.Info("Results retrieved",
		zap.String("session_id", sessionID),
		zap.Int("score", results.Score),
	)

	c.JSON(http.StatusOK, gin.H{
		"score":           results.Score,
		"feedback":        results.Feedback,
		"recommendations": results.Recommendations,
		"completed_at":    results.CompletedAt.Format(time.RFC3339),
		"available":       true,
		"completed":       true,
	})
}

// getInterviews обрабатывает GET /api/interviews (список всех интервью пользователя).
func (h *Handler) getInterviews(c *gin.Context) {
	userIDValue, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	userIDStr, ok := userIDValue.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	interviews, err := h.chat.GetInterviews(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Get interviews failed",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get interviews"})
		return
	}

	h.logger.Info("Interviews retrieved",
		zap.String("user_id", userID.String()),
		zap.Int("count", len(interviews)),
	)

	c.JSON(http.StatusOK, gin.H{"interviews": interviews})
}

// getInterviewChat обрабатывает GET /api/interviews/:session_id/chat (история чата).
func (h *Handler) getInterviewChat(c *gin.Context) {
	h.logger.Info("GetInterviewChat called", zap.String("path", c.Request.URL.Path))
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("Invalid session_id in GetInterviewChat", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}
	h.logger.Info("Getting chat history", zap.String("session_id", sessionID.String()))

	messages, err := h.chat.GetChatHistory(c.Request.Context(), sessionID)
	if err != nil {
		errStr := err.Error()
		if errStr == "chat dump not found" || errStr == "chat crud service error: chat dump not found" ||
			errStr == "get chat dump: chat dump not found" {
			h.logger.Warn("Chat dump not found", zap.String("session_id", sessionID.String()))
			c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
			return
		}
		h.logger.Error("Get chat history failed",
			zap.Error(err),
			zap.String("session_id", sessionID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get chat history"})
		return
	}

	h.logger.Info("Chat history retrieved",
		zap.String("session_id", sessionID.String()),
		zap.Int("messages_count", len(messages)),
	)

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// getInterviewResult обрабатывает GET /api/interviews/:session_id/result (результат интервью).
func (h *Handler) getInterviewResult(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	result, err := h.chat.GetChatResult(c.Request.Context(), sessionID)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "result not found") {
			h.logger.Warn("Result not found", zap.String("session_id", sessionID.String()))
			c.JSON(http.StatusNotFound, gin.H{
				"error":      "Result not found",
				"error_code": "RESULT_NOT_FOUND",
			})
			return
		}
		h.logger.Error("Get chat result failed",
			zap.Error(err),
			zap.String("session_id", sessionID.String()),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":      "Unable to load interview result. Please try again later.",
			"error_code": "RESULT_UNAVAILABLE",
		})
		return
	}

	h.logger.Info("Chat result retrieved",
		zap.String("session_id", sessionID.String()),
		zap.Int("score", result.Score),
	)

	if reportJSONPresent(result.ReportJSON) {
		c.JSON(http.StatusOK, gin.H{"result": interviewResultWithReport{
			ID:                  result.ID,
			SessionID:           result.SessionID,
			Score:               result.Score,
			Feedback:            result.Feedback,
			TerminatedEarly:     result.TerminatedEarly,
			ReportJSON:          result.ReportJSON,
			PresetTraining:      result.PresetTraining,
			Evaluations:         result.Evaluations,
			ResultFormatVersion: result.ResultFormatVersion,
			SessionKind:         result.SessionKind,
			CreatedAt:           result.CreatedAt,
			UpdatedAt:           result.UpdatedAt,
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"result": interviewResultLegacy{
		ID:              result.ID,
		SessionID:       result.SessionID,
		Score:           result.Score,
		Feedback:        result.Feedback,
		TerminatedEarly: result.TerminatedEarly,
		CreatedAt:       result.CreatedAt,
		UpdatedAt:       result.UpdatedAt,
	}})
}

// submitInterviewRating обрабатывает POST /api/interviews/:session_id/rating.
// Принимает {"rating": 1-5, "comment": "..."} и сохраняет в results-crud.
func (h *Handler) submitInterviewRating(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	var req struct {
		Rating  int    `json:"rating" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be between 1 and 5"})
		return
	}

	if err := h.chat.SubmitSessionRating(c.Request.Context(), sessionID, req.Rating, req.Comment); err != nil {
		h.logger.Warn("SubmitSessionRating failed",
			zap.String("session_id", sessionID.String()),
			zap.Error(err),
		)
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to save rating"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Dashboard endpoints (§10.2) ─────────────────────────────────────────────

// getDashboardSummary возвращает сводную статистику пользователя для дашборда.
// Проксирует агрегированные данные из results-crud и session-crud сервисов.
// При отсутствии данных возвращает пустое состояние (не захардкоженные значения).
func (h *Handler) getDashboardSummary(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	summary, err := h.chat.GetDashboardSummary(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get dashboard summary",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, gin.H{
			"total_sessions":            0,
			"completed_sessions":        0,
			"avg_score":                 0.0,
			"streak_days":               0,
			"current_level":             "",
			"training_unlocked_topics":  []string{},
			"last_session_date":         nil,
		})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// getDashboardActivity возвращает календарь активности пользователя.
func (h *Handler) getDashboardActivity(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	from := c.Query("from")
	to := c.Query("to")

	activity, err := h.chat.GetDashboardActivity(c.Request.Context(), userID, from, to)
	if err != nil {
		h.logger.Error("Failed to get dashboard activity",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	c.JSON(http.StatusOK, activity)
}

// getDashboardTopicProgress возвращает прогресс пользователя по темам.
func (h *Handler) getDashboardTopicProgress(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	progress, err := h.chat.GetDashboardTopicProgress(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get topic progress",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	c.JSON(http.StatusOK, progress)
}

// getDashboardRecommendations возвращает рекомендации из последнего отчёта аналитика.
func (h *Handler) getDashboardRecommendations(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	recommendations, err := h.chat.GetDashboardRecommendations(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get dashboard recommendations",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	c.JSON(http.StatusOK, recommendations)
}

// logout инвалидирует текущую сессию пользователя.
func (h *Handler) logout(c *gin.Context) {
	token := extractBearer(c.GetHeader("Authorization"))
	if token != "" {
		// Инвалидируем Redis-сессию в auth-service (ошибка не блокирует logout)
		if err := h.auth.Logout(c.Request.Context(), token); err != nil {
			h.logger.Warn("Logout: auth-service error (ignored)", zap.Error(err))
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// recover обрабатывает POST /api/auth/recover — сброс пароля по ключу восстановления.
func (h *Handler) recover(c *gin.Context) {
	var req struct {
		Login       string `json:"login" binding:"required"`
		RecoveryKey string `json:"recovery_key" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := h.auth.Recover(c.Request.Context(), req.Login, req.RecoveryKey, req.NewPassword); err != nil {
		switch {
		case service.IsError(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid login or recovery key"})
		case service.IsError(err, service.ErrBadRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrorMessage(err)})
		default:
			h.logger.Error("Recover failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// changePassword обрабатывает POST /api/auth/change-password.
func (h *Handler) changePassword(c *gin.Context) {
	token := extractBearer(c.GetHeader("Authorization"))
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), token, req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case service.IsError(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect current password"})
		case service.IsError(err, service.ErrBadRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrorMessage(err)})
		default:
			h.logger.Error("ChangePassword failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// regenerateRecoveryKey обрабатывает POST /api/auth/regenerate-recovery-key.
func (h *Handler) regenerateRecoveryKey(c *gin.Context) {
	token := extractBearer(c.GetHeader("Authorization"))
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	rawKey, err := h.auth.RegenerateRecoveryKey(c.Request.Context(), token, req.Password)
	if err != nil {
		switch {
		case service.IsError(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		case service.IsError(err, service.ErrBadRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrorMessage(err)})
		default:
			h.logger.Error("RegenerateRecoveryKey failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"recovery_key": rawKey})
}

// deleteAccount обрабатывает DELETE /api/auth/account.
func (h *Handler) deleteAccount(c *gin.Context) {
	token := extractBearer(c.GetHeader("Authorization"))
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := h.auth.DeleteAccount(c.Request.Context(), token, req.Password); err != nil {
		switch {
		case service.IsError(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		case service.IsError(err, service.ErrBadRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrorMessage(err)})
		default:
			h.logger.Error("DeleteAccount failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// generateRandomLogin проксирует GET /api/users/generate-random-login → user-crud-service.
func (h *Handler) generateRandomLogin(c *gin.Context) {
	userCrudURL := h.userCrudURL
	if userCrudURL == "" {
		userCrudURL = "http://user-crud-service:8082"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, userCrudURL+"/login-words/generate-random", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.logger.Warn("generateRandomLogin: user-crud-service unreachable", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "login generation unavailable"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.JSON(resp.StatusCode, gin.H{"error": "login generation failed"})
		return
	}

	var result struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Login == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"login": result.Login})
}

// getLoginAdjectives проксирует GET /api/users/login-words/adjectives → user-crud-service.
func (h *Handler) getLoginAdjectives(c *gin.Context) {
	h.proxyUserCrudGet(c, "/login-words/adjectives")
}

// getLoginNouns проксирует GET /api/users/login-words/nouns → user-crud-service.
func (h *Handler) getLoginNouns(c *gin.Context) {
	h.proxyUserCrudGet(c, "/login-words/nouns")
}

// checkLoginAvailability проксирует POST /api/users/login-words/check-availability → user-crud-service.
func (h *Handler) checkLoginAvailability(c *gin.Context) {
	userCrudURL := h.userCrudURL
	if userCrudURL == "" {
		userCrudURL = "http://user-crud-service:8082"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, userCrudURL+"/login-words/check-availability", c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "unavailable"})
		return
	}
	defer resp.Body.Close()

	c.Status(resp.StatusCode)
	c.Header("Content-Type", "application/json")
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		h.logger.Warn("checkLoginAvailability: copy error", zap.Error(err))
	}
}

// proxyUserCrudGet выполняет GET-прокси к user-crud-service и отдаёт ответ напрямую.
// submitMessageFeedback обрабатывает POST /api/chat/:session_id/message-feedback.
// Сохраняет оценку (1-5 звёзд) для конкретного сообщения агента.
func (h *Handler) submitMessageFeedback(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	var req struct {
		QuestionID string `json:"question_id" binding:"required"`
		Rating     int    `json:"rating" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be 1-5"})
		return
	}

	h.logger.Info("Message feedback received",
		zap.String("session_id", sessionID),
		zap.String("question_id", req.QuestionID),
		zap.Int("rating", req.Rating),
	)

	// Записываем в Prometheus для агрегации (хранение отдельных оценок — будущая задача)
	messageFeedbackTotal.WithLabelValues(fmt.Sprintf("%d", req.Rating)).Inc()

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) proxyUserCrudGet(c *gin.Context, path string) {
	userCrudURL := h.userCrudURL
	if userCrudURL == "" {
		userCrudURL = "http://user-crud-service:8082"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userCrudURL+path, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "unavailable"})
		return
	}
	defer resp.Body.Close()

	c.Status(resp.StatusCode)
	c.Header("Content-Type", "application/json")
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		h.logger.Warn("proxyUserCrudGet: copy error", zap.Error(err))
	}
}
