package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/tensor-talks/bff-service/internal/config"
)

/*
Пакет middleware содержит общие Gin-мидлвары для BFF.

Сейчас реализован CORS-мидлвар, который конфигурируется из YAML/окружения.
*/

// NewCORS создаёт CORS-мидлвару Gin на основе настроек CORSConfig.
// Особенности:
//   - если список origin пуст, включается режим AllowAllOrigins (удобно для разработки);
//   - если среди origin есть "*", также включается AllowAllOrigins;
//   - при пустом списке заголовков по умолчанию разрешаются `Content-Type` и `Authorization`.
func NewCORS(cfg config.CORSConfig) gin.HandlerFunc {
	corsCfg := cors.Config{
		AllowOrigins: cfg.AllowOrigins,
		AllowHeaders: cfg.AllowHeaders,
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		MaxAge:       12 * time.Hour,
	}
	if len(corsCfg.AllowOrigins) == 0 {
		corsCfg.AllowAllOrigins = true
	} else {
		for _, origin := range corsCfg.AllowOrigins {
			if origin == "*" {
				corsCfg.AllowAllOrigins = true
				corsCfg.AllowOrigins = nil
				break
			}
		}
	}
	if len(corsCfg.AllowHeaders) == 0 {
		corsCfg.AllowHeaders = []string{"Content-Type", "Authorization"}
	}
	return cors.New(corsCfg)
}
