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
	"github.com/tensor-talks/auth-service/internal/client"
	"github.com/tensor-talks/auth-service/internal/config"
	"github.com/tensor-talks/auth-service/internal/handler"
	"github.com/tensor-talks/auth-service/internal/redis"
	"github.com/tensor-talks/auth-service/internal/service"
	"github.com/tensor-talks/auth-service/internal/tokens"
	"go.uber.org/zap"
)

/*
Пакет server отвечает за "сборку" всех зависимостей auth-service и управление жизненным
циклом HTTP-сервера (запуск, graceful shutdown).

Внутри:
  - инициализируется конфигурация, HTTP-клиент user-store, менеджер токенов и сервис аутентификации;
  - конфигурируется Gin-роутер и health-check;
  - запускается HTTP-сервер и обрабатывается завершение по контексту.
*/

// Server инкапсулирует HTTP-сервер и его жизненный цикл.
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

// New создаёт новый экземпляр Server, собирая все зависимости.
// На этом этапе:
//   - создаётся HTTP-клиент к user-store-service;
//   - инициализируется менеджер токенов и сервис аутентификации;
//   - настраивается Gin-роутер, health-check и HTTP-сервер с таймаутом заголовков.
func New(cfg config.Config, logger *zap.Logger) (*Server, error) {
	userStoreClient, err := client.NewUserStoreClient(cfg.UserStore)
	if err != nil {
		return nil, fmt.Errorf("init user store client: %w", err)
	}

	tokenManager := tokens.NewManager(cfg.JWT)

	// Инициализируем Redis для управления логин-сессиями
	var sessionStore service.SessionStore
	if cfg.Redis.Addr != "" {
		sessionStore = redis.NewSessionStore(
			cfg.Redis.Addr,
			cfg.Redis.Password,
			cfg.Redis.DB,
			cfg.JWT.AccessTokenTTL, // TTL сессии равен TTL access токена
			logger,
		)
		// Проверяем подключение к Redis
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sessionStore.Ping(ctx); err != nil {
			logger.Warn("Failed to connect to Redis, session management disabled", zap.Error(err))
			sessionStore = nil
		} else {
			logger.Info("Redis connection established for session management")
		}
	}

	authService := service.NewAuthService(userStoreClient, tokenManager, sessionStore)
	authHandler := handler.NewAuthHandler(authService, logger)

	engine := gin.Default()

	// Middleware для логирования
	engine.Use(loggingMiddleware(logger))

	// Middleware для метрик
	engine.Use(metricsMiddleware("auth-service"))

	// Health check
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Metrics endpoint
	engine.GET("/metrics", metricsHandler())

	authHandler.RegisterRoutes(engine)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Server{httpServer: httpServer, logger: logger}, nil
}

// Run запускает HTTP-сервер и блокируется до остановки по контексту или ошибке.
// При завершении контекста инициирует корректное завершение (`Shutdown`) с таймаутом,
// чтобы дать активным запросам завершиться.
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
			zap.Int("size", c.Writer.Size()),
		)

		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				logger.Error("Request error",
					zap.String("method", c.Request.Method),
					zap.String("path", path),
					zap.Error(err),
				)
			}
		}
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
