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
	"github.com/tensor-talks/session-service/internal/client"
	"github.com/tensor-talks/session-service/internal/config"
	"github.com/tensor-talks/session-service/internal/handler"
	"github.com/tensor-talks/session-service/internal/kafka"
	"github.com/tensor-talks/session-service/internal/redis"
	"github.com/tensor-talks/session-service/internal/service"
	"go.uber.org/zap"
)

// Server инкапсулирует HTTP-сервер session-service.
type Server struct {
	httpServer    *http.Server
	logger        *zap.Logger
	kafkaConsumer *kafka.Consumer
	kafkaProducer *kafka.Producer
	redisCache    *redis.Cache
}

// New создаёт новый экземпляр Server.
func New(cfg config.Config, logger *zap.Logger) (*Server, error) {
	// Инициализация Redis кэша
	redisCache := redis.NewCache(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.TTLHours,
		logger,
	)

	// Проверка подключения к Redis
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisCache.Ping(ctx); err != nil {
		logger.Warn("Failed to connect to Redis, continuing anyway", zap.Error(err))
	} else {
		logger.Info("Connected to Redis", zap.String("addr", cfg.Redis.Addr))
	}

	// Инициализация клиента к session-crud-service
	crudClient := client.NewSessionCRUDClient(
		cfg.SessionCRUD.BaseURL,
		cfg.SessionCRUD.TimeoutSeconds,
	)

	// Инициализация Kafka producer
	kafkaProducer, err := kafka.NewProducer(
		cfg.Kafka.Brokers,
		cfg.Kafka.TopicRequest,
		"session-manager-service",
		"1.0.0",
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("init kafka producer: %w", err)
	}

	// Инициализация Kafka consumer
	kafkaConsumer, err := kafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.TopicResponse,
		cfg.Kafka.ConsumerGroup,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("init kafka consumer: %w", err)
	}

	// Создаём сервис управления сессиями
	sessionManagerService := service.NewSessionManagerService(
		crudClient,
		redisCache,
		kafkaProducer,
		cfg.SessionManager.MaxActiveSessions,
		cfg.SessionManager.ProgramTimeoutSeconds,
		logger,
	)

	// Устанавливаем обработчик событий для consumer
	kafkaConsumer.SetEventHandler(sessionManagerService)

	sessionHandler := handler.NewSessionHandler(sessionManagerService, logger)

	router := gin.Default()

	// Middleware для логирования
	router.Use(loggingMiddleware(logger))

	// Middleware для метрик
	router.Use(metricsMiddleware("session-service"))

	// Health check
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Metrics endpoint
	router.GET("/metrics", metricsHandler())

	sessionHandler.RegisterRoutes(router)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Server{
		httpServer:    httpServer,
		logger:        logger,
		kafkaConsumer: kafkaConsumer,
		kafkaProducer: kafkaProducer,
		redisCache:    redisCache,
	}, nil
}

// Run запускает HTTP-сервер и Kafka consumer.
func (s *Server) Run(ctx context.Context) error {
	// Запускаем Kafka consumer
	if err := s.kafkaConsumer.Start(ctx); err != nil {
		return fmt.Errorf("start kafka consumer: %w", err)
	}
	defer s.kafkaConsumer.Close()

	errCh := make(chan error, 1)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down server")
		s.kafkaProducer.Close()
		s.redisCache.Close()
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
