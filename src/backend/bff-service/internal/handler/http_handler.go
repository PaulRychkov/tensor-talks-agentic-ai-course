package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/bff-service/internal/client"
	"github.com/tensor-talks/bff-service/internal/middleware"
	"github.com/tensor-talks/bff-service/internal/service"
	"go.uber.org/zap"
)

/*
Пакет handler реализует HTTP-слой BFF (backend-for-frontend) сервиса.

Основная задача:
  - предоставить фронтенду стабильное и упрощённое API;
  - проксировать запросы аутентификации в auth-service, не раскрывая внутреннюю топологию микросервисов.

Важно: BFF не имеет прямого доступа ни к user-store-service, ни к базе данных.
*/

// Handler инкапсулирует HTTP-эндпоинты, которые вызываются фронтендом.
type Handler struct {
	auth   *service.AuthService
	chat   *service.ChatService
	logger *zap.Logger
}

// New создаёт новый обработчик HTTP-запросов BFF.
func New(auth *service.AuthService, chat *service.ChatService, logger *zap.Logger) *Handler {
	return &Handler{auth: auth, chat: chat, logger: logger}
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
	Question   string `json:"question"`
	QuestionID string `json:"question_id"`
	Timestamp  string `json:"timestamp"`
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
		c.JSON(http.StatusOK, gin.H{"question": nil, "available": false})
		return
	}

	h.logger.Info("Question retrieved",
		zap.String("session_id", sessionID),
		zap.String("question_id", question.QuestionID),
	)

	c.JSON(http.StatusOK, questionResponse{
		Question:   question.Question,
		QuestionID: question.QuestionID,
		Timestamp:  question.Timestamp.Format(time.RFC3339),
	})
}

type resultsResponse struct {
	Score           int      `json:"score"`
	Feedback        string   `json:"feedback"`
	Recommendations []string `json:"recommendations"`
	CompletedAt     string   `json:"completed_at"`
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
		c.JSON(http.StatusOK, gin.H{"results": nil, "available": false})
		return
	}

	h.logger.Info("Results retrieved",
		zap.String("session_id", sessionID),
		zap.Int("score", results.Score),
	)

	c.JSON(http.StatusOK, resultsResponse{
		Score:           results.Score,
		Feedback:        results.Feedback,
		Recommendations: results.Recommendations,
		CompletedAt:     results.CompletedAt.Format(time.RFC3339),
	})
}

// getInterviews обрабатывает GET /api/interviews (список всех интервью пользователя).
func (h *Handler) getInterviews(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id query parameter required"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
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
		if err.Error() == "result not found" || err.Error() == "results crud service error: result not found" {
			h.logger.Warn("Result not found", zap.String("session_id", sessionID.String()))
			c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
			return
		}
		h.logger.Error("Get chat result failed",
			zap.Error(err),
			zap.String("session_id", sessionID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get result"})
		return
	}

	h.logger.Info("Chat result retrieved",
		zap.String("session_id", sessionID.String()),
		zap.Int("score", result.Score),
	)

	c.JSON(http.StatusOK, gin.H{"result": result})
}
