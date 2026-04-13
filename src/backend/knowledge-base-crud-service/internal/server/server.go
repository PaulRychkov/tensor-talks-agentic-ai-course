package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/config"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/handler"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/models"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/repository"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/service"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Server инкапсулирует HTTP-сервер knowledge-base-crud-service.
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

// New создаёт новый экземпляр Server.
func New(cfg config.Config, logger *zap.Logger) (*Server, error) {
	logger.Info("Connecting to database",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("database", cfg.Database.Name),
	)

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	logger.Info("Running database migrations")
	if err := db.AutoMigrate(&models.Knowledge{}); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	logger.Info("Database migrations completed")

	repo := repository.NewGormKnowledgeRepository(db)
	svc := service.NewKnowledgeService(repo)
	handler := handler.NewKnowledgeHandler(svc, logger)

	router := gin.Default()

	// Middleware для логирования
	router.Use(loggingMiddleware(logger))

	// Middleware для метрик
	router.Use(metricsMiddleware("knowledge-base-crud-service"))

	// Health check
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Metrics endpoint
	router.GET("/metrics", metricsHandler())

	handler.RegisterRoutes(router)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Server{httpServer: httpServer, logger: logger}, nil
}

// Run запускает HTTP-сервер.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// loggingMiddleware создаёт middleware для логирования HTTP-запросов
func loggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)

		logger.Info("HTTP request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
		)
	}
}

// HTTP метрики
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"service", "method", "endpoint", "status_code"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tensortalks_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "endpoint"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

// metricsMiddleware создаёт middleware для сбора HTTP-метрик
func metricsMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		c.Next()

		duration := time.Since(start).Seconds()
		statusCode := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(
			serviceName,
			c.Request.Method,
			path,
			statusCode,
		).Inc()

		httpRequestDuration.WithLabelValues(
			serviceName,
			c.Request.Method,
			path,
		).Observe(duration)
	}
}

// metricsHandler возвращает handler для /metrics endpoint
func metricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
