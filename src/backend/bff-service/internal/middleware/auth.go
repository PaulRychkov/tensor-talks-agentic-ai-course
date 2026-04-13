package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tensor-talks/bff-service/internal/service"
	"go.uber.org/zap"
)

/*
Middleware для проверки JWT токенов в BFF.

Middleware извлекает access-токен из заголовка Authorization: Bearer <token>
и валидирует его через auth-service. При успешной валидации сохраняет
userID в контекст для использования в обработчиках.
*/

const (
	// UserIDKey ключ для сохранения userID в gin.Context после успешной аутентификации.
	UserIDKey = "user_id"
	// UserKey ключ для сохранения данных пользователя в gin.Context.
	UserKey = "user"
)

// AuthMiddleware создаёт middleware для проверки JWT токенов.
// Токен валидируется через auth-service, userID сохраняется в контексте.
func AuthMiddleware(authService *service.AuthService, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		token := extractBearer(raw)
		if token == "" {
			logger.Warn("Auth middleware: missing token",
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		// Валидируем токен через auth-service
		user, err := authService.CurrentUser(c.Request.Context(), token)
		if err != nil {
			logger.Warn("Auth middleware: invalid token",
				zap.String("path", c.Request.URL.Path),
				zap.Error(err),
			)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Сохраняем userID и user в контексте для использования в обработчиках
		c.Set(UserIDKey, user.ID)
		c.Set(UserKey, user)

		c.Next()
	}
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

// GetUserID извлекает userID из контекста Gin (устанавливается AuthMiddleware).
// Возвращает пустую строку, если userID не найден в контексте.
func GetUserID(c *gin.Context) string {
	if userID, exists := c.Get(UserIDKey); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return ""
}
