// Package middleware provides HTTP middleware for admin-bff-service (§10.1).
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// AdminAuthMiddleware validates the JWT token and enforces admin/content_editor role.
// Returns HTTP 401 for missing/invalid tokens and HTTP 403 for insufficient role.
func AdminAuthMiddleware(jwtSecret string, allowedRoles []string, logger *zap.Logger) gin.HandlerFunc {
	allowedSet := map[string]bool{}
	for _, r := range allowedRoles {
		allowedSet[r] = true
	}

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}

		tokenStr := parts[1]
		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			logger.Warn("Invalid JWT token", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// Check role claim
		role, _ := claims["role"].(string)
		if !allowedSet[role] {
			logger.Warn("Insufficient role for admin access",
				zap.String("role", role),
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
			return
		}

		// Inject operator context
		userID, _ := claims["sub"].(string)
		c.Set("operator_id", userID)
		c.Set("operator_role", role)
		c.Next()
	}
}
