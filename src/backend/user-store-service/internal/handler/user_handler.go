package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/user-store-service/internal/metrics"
	"github.com/tensor-talks/user-store-service/internal/models"
	"github.com/tensor-talks/user-store-service/internal/repository"
	"github.com/tensor-talks/user-store-service/internal/service"
	"go.uber.org/zap"
)

// UserHandler связывает HTTP-эндпоинты с сервисом пользователей.
type UserHandler struct {
	svc    *service.UserService
	logger *zap.Logger
}

// NewUserHandler создаёт новый экземпляр HTTP-обработчика пользователей.
func NewUserHandler(svc *service.UserService, logger *zap.Logger) *UserHandler {
	return &UserHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты пользовательского API на переданном роутере.
//
// ВНИМАНИЕ: эндпоинты /users и /users/by-login предназначены для внутреннего
// использования другими микросервисами (например, auth-service) и не должны
// проксироваться во внешний мир через BFF.
func (h *UserHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/users", h.CreateUser)
	router.GET("/users/:id", h.GetUserByID)
	router.GET("/users/by-login/:login", h.GetUserByLogin)
	router.PUT("/users/:id", h.UpdateUser)
	router.DELETE("/users/:id", h.DeleteUser)

	// Отладочное API для выборки всех пользователей с фильтрами/пагинацией.
	// НЕ ДОЛЖНО использоваться на проде и не должно быть доступно из BFF.
	router.GET("/debug/users", h.ListUsersDebug)
}

type createUserRequest struct {
	Login        string `json:"login" binding:"required"`
	PasswordHash string `json:"password_hash" binding:"required"`
}

type updateUserRequest struct {
	Login        *string `json:"login"`
	PasswordHash *string `json:"password_hash"`
}

// listUsersQuery описывает параметры строки запроса для отладочного списка пользователей.
type listUsersQuery struct {
	Login  string `form:"login"`
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

// CreateUser обрабатывает POST /users и создаёт нового пользователя.
// Детали:
//   - ожидает JSON `{ "login": "...", "password_hash": "..." }`;
//   - не принимает сырой пароль, только уже захешированную строку (например, bcrypt);
//   - нормализует и валидирует логин/хеш в слое `UserService`;
//   - в случае конфликта логина возвращает 409, при ошибках валидации — 400,
//     при прочих ошибках — 500;
//   - при успехе возвращает 201 с публичным представлением пользователя.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("CreateUser: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	h.logger.Info("CreateUser request", zap.String("login", req.Login))
	user, err := h.svc.CreateUser(c.Request.Context(), req.Login, req.PasswordHash)
	if err != nil {
		switch err {
		case service.ErrInvalidInput:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "create", "error").Inc()
			h.logger.Warn("CreateUser failed: invalid input", zap.String("login", req.Login), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case repository.ErrDuplicateLogin:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "create", "error").Inc()
			h.logger.Warn("CreateUser failed: duplicate login", zap.String("login", req.Login))
			c.JSON(http.StatusConflict, gin.H{"error": "login already exists"})
		default:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "create", "error").Inc()
			h.logger.Error("CreateUser failed: internal error", zap.Error(err), zap.String("login", req.Login))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "create", "success").Inc()
	h.logger.Info("CreateUser successful", zap.String("user_id", user.ExternalID.String()), zap.String("login", user.Login))
	c.JSON(http.StatusCreated, gin.H{"user": user.ToPublic()})
}

// GetUserByID обрабатывает GET /users/:id и возвращает пользователя по GUID.
// Используется другими микросервисами (например, `auth-service`) для получения данных
// по внешнему идентификатору `external_id`. При отсутствии пользователя возвращает 404.
func (h *UserHandler) GetUserByID(c *gin.Context) {
	externalID, err := parseUUIDParam(c.Param("id"))
	if err != nil {
		h.logger.Warn("GetUserByID: invalid user id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	h.logger.Info("GetUserByID request", zap.String("user_id", externalID.String()))
	user, err := h.svc.GetByExternalID(c.Request.Context(), externalID)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "get_by_id", "not_found").Inc()
			h.logger.Warn("GetUserByID: user not found", zap.String("user_id", externalID.String()))
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "get_by_id", "error").Inc()
			h.logger.Error("GetUserByID failed: internal error", zap.Error(err), zap.String("user_id", externalID.String()))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "get_by_id", "success").Inc()
	h.logger.Info("GetUserByID successful", zap.String("user_id", externalID.String()))
	c.JSON(http.StatusOK, gin.H{"user": user.ToPublic()})
}

// GetUserByLogin обрабатывает GET /users/by-login/:login и возвращает пользователя по логину.
// Логин в URL должен соответствовать нормализованному значению (lowercase). Эндпоинт
// используется, в частности, `auth-service` для поиска учётной записи при логине.
func (h *UserHandler) GetUserByLogin(c *gin.Context) {
	login := c.Param("login")
	h.logger.Info("GetUserByLogin request", zap.String("login", login))
	user, err := h.svc.GetByLogin(c.Request.Context(), login)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "get_by_login", "not_found").Inc()
			h.logger.Warn("GetUserByLogin: user not found", zap.String("login", login))
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "get_by_login", "error").Inc()
			h.logger.Error("GetUserByLogin failed: internal error", zap.Error(err), zap.String("login", login))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}
	metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "get_by_login", "success").Inc()
	h.logger.Info("GetUserByLogin successful", zap.String("login", login))
	c.JSON(http.StatusOK, gin.H{"user": user.ToPublic()})
}

// UpdateUser обрабатывает PUT /users/:id и обновляет логин и/или хеш пароля пользователя.
// Поддерживает частичное обновление: можно передать только одно из полей. Все проверки
// корректности значений выполняются в `UserService`, а конфликты логинов/отсутствие
// пользователя маппятся на соответствующие HTTP-коды.
func (h *UserHandler) UpdateUser(c *gin.Context) {
	externalID, err := parseUUIDParam(c.Param("id"))
	if err != nil {
		h.logger.Warn("UpdateUser: invalid user id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("UpdateUser: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if req.Login == nil && req.PasswordHash == nil {
		h.logger.Warn("UpdateUser: no fields provided")
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields provided"})
		return
	}

	h.logger.Info("UpdateUser request", zap.String("user_id", externalID.String()))
	user, err := h.svc.UpdateUser(c.Request.Context(), externalID, req.Login, req.PasswordHash)
	if err != nil {
		switch err {
		case service.ErrInvalidInput:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "update", "error").Inc()
			h.logger.Warn("UpdateUser failed: invalid input", zap.String("user_id", externalID.String()), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case repository.ErrDuplicateLogin:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "update", "error").Inc()
			h.logger.Warn("UpdateUser failed: duplicate login", zap.String("user_id", externalID.String()))
			c.JSON(http.StatusConflict, gin.H{"error": "login already exists"})
		case repository.ErrNotFound:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "update", "not_found").Inc()
			h.logger.Warn("UpdateUser: user not found", zap.String("user_id", externalID.String()))
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		default:
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "update", "error").Inc()
			h.logger.Error("UpdateUser failed: internal error", zap.Error(err), zap.String("user_id", externalID.String()))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "update", "success").Inc()
	h.logger.Info("UpdateUser successful", zap.String("user_id", externalID.String()))
	c.JSON(http.StatusOK, gin.H{"user": user.ToPublic()})
}

// DeleteUser обрабатывает DELETE /users/:id и удаляет пользователя по GUID.
// В случае отсутствия записи возвращает 404, при успехе — 204 без тела.
func (h *UserHandler) DeleteUser(c *gin.Context) {
	externalID, err := parseUUIDParam(c.Param("id"))
	if err != nil {
		h.logger.Warn("DeleteUser: invalid user id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	h.logger.Info("DeleteUser request", zap.String("user_id", externalID.String()))
	if err := h.svc.DeleteUser(c.Request.Context(), externalID); err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "delete", "not_found").Inc()
			h.logger.Warn("DeleteUser: user not found", zap.String("user_id", externalID.String()))
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "delete", "error").Inc()
			h.logger.Error("DeleteUser failed: internal error", zap.Error(err), zap.String("user_id", externalID.String()))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessUserOperationsTotal.WithLabelValues("user-store-service", "delete", "success").Inc()
	h.logger.Info("DeleteUser successful", zap.String("user_id", externalID.String()))
	c.Status(http.StatusNoContent)
}

// ListUsersDebug обрабатывает GET /debug/users и возвращает список пользователей
// с возможностью фильтрации по логину и пагинации через limit/offset.
// Особенности:
//   - query-параметр `login` позволяет выполнить case-insensitive поиск по подстроке;
//   - `limit` и `offset` нормализуются и ограничиваются в слое `UserService`;
//   - используется только в отладочных сценариях и не должен быть доступен извне.
func (h *UserHandler) ListUsersDebug(c *gin.Context) {
	query := listUsersQuery{}

	// Используем ручной парсинг, чтобы явно контролировать значения limit/offset.
	if rawLimit := c.Query("limit"); rawLimit != "" {
		if v, err := strconv.Atoi(rawLimit); err == nil {
			query.Limit = v
		}
	}
	if rawOffset := c.Query("offset"); rawOffset != "" {
		if v, err := strconv.Atoi(rawOffset); err == nil {
			query.Offset = v
		}
	}
	query.Login = c.Query("login")

	var loginFilter *string
	if query.Login != "" {
		loginCopy := query.Login
		loginFilter = &loginCopy
	}

	users, err := h.svc.ListUsers(c.Request.Context(), service.ListFilters{
		Login:  loginFilter,
		Limit:  query.Limit,
		Offset: query.Offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	result := make([]models.PublicUser, 0, len(users))
	for _, u := range users {
		result = append(result, u.ToPublic())
	}

	c.JSON(http.StatusOK, gin.H{
		"users":  result,
		"limit":  query.Limit,
		"offset": query.Offset,
	})
}

func parseUUIDParam(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}
