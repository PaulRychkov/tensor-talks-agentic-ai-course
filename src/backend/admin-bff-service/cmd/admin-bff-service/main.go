// admin-bff-service — Backend for the admin/operator frontend (§10.1).
//
// Routes all requests under /admin/api/ to downstream services after
// validating that the JWT token contains role=admin or role=content_editor.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tensor-talks/admin-bff-service/internal/config"
	"github.com/tensor-talks/admin-bff-service/internal/handler"
	"github.com/tensor-talks/admin-bff-service/internal/middleware"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	logger.Info("Starting admin-bff-service",
		zap.Int("port", cfg.Server.Port),
		zap.Strings("allowed_roles", cfg.AllowedRoles),
	)

	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "admin-bff-service"})
	})

	h := handler.New(cfg, logger)
	h.RegisterPublicRoutes(router)

	// All /admin/api/* routes require admin role authentication
	adminAuth := middleware.AdminAuthMiddleware(cfg.JWT.Secret, cfg.AllowedRoles, logger)
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/healthz" || c.Request.URL.Path == "/admin/login" {
			c.Next()
			return
		}
		adminAuth(c)
	})

	h.RegisterRoutes(router)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down admin-bff-service")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}
}
