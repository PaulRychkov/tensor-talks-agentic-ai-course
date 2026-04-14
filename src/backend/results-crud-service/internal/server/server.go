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
	"github.com/tensor-talks/results-crud-service/internal/config"
	"github.com/tensor-talks/results-crud-service/internal/handler"
	"github.com/tensor-talks/results-crud-service/internal/metrics"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
	"github.com/tensor-talks/results-crud-service/internal/service"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Server инкапсулирует HTTP-сервер results-crud-service.
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
	resultRepo repository.ResultRepository // для фонового обновления метрик
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
	if err := db.AutoMigrate(
		&models.Result{},
		&models.UserTopicProgress{},
		&models.Preset{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	logger.Info("Database migrations completed")

	// Result
	resultRepo := repository.NewGormResultRepository(db)
	resultSvc := service.NewResultService(resultRepo)
	resultHandler := handler.NewResultHandler(resultSvc, logger)

	// User topic progress
	progressRepo := repository.NewGormUserProgressRepository(db)
	progressSvc := service.NewUserProgressService(progressRepo)
	progressHandler := handler.NewUserProgressHandler(progressSvc, logger)

	// Presets
	presetRepo := repository.NewGormPresetRepository(db)
	presetSvc := service.NewPresetService(presetRepo)
	presetHandler := handler.NewPresetHandler(presetSvc, logger)

	router := gin.Default()

	router.Use(loggingMiddleware(logger))
	router.Use(metricsMiddleware("results-crud-service"))

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/metrics", metricsHandler())

	resultHandler.RegisterRoutes(router)
	progressHandler.RegisterRoutes(router)
	presetHandler.RegisterRoutes(router)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Server{httpServer: httpServer, logger: logger, resultRepo: resultRepo}, nil
}

// Run запускает HTTP-сервер и фоновый воркер продуктовых метрик.
func (s *Server) Run(ctx context.Context) error {
	// Запускаем фоновый воркер обновления продуктовых метрик (каждые 30 с)
	go s.runProductMetricsWorker(ctx)

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

// runProductMetricsWorker периодически вычисляет продуктовые метрики из БД
// и обновляет соответствующие Prometheus gauges.
func (s *Server) runProductMetricsWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Первый запуск сразу при старте
	s.updateProductMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateProductMetrics(ctx)
		}
	}
}

func (s *Server) updateProductMetrics(ctx context.Context) {
	pm, err := s.resultRepo.GetProductMetrics(ctx)
	if err != nil {
		s.logger.Warn("Failed to update product metrics", zap.Error(err))
		return
	}

	metrics.ProductTotalSessions.Set(float64(pm.TotalSessions))
	metrics.ProductCompletionRate.Set(pm.CompletionRate / 100.0) // храним как 0-1
	metrics.ProductAvgScore.Set(pm.AvgScore)
	metrics.ProductAvgRating.Set(pm.AvgRating)
	metrics.ProductRatedSessions.Set(float64(pm.RatedSessions))

	for kind, count := range pm.ByKind {
		metrics.ProductSessionsByKind.WithLabelValues(kind).Set(float64(count))
	}

	s.logger.Debug("Product metrics updated",
		zap.Int64("total_sessions", pm.TotalSessions),
		zap.Float64("completion_rate", pm.CompletionRate),
		zap.Float64("avg_score", pm.AvgScore),
	)
}

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

func metricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
