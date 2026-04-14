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
	"github.com/tensor-talks/bff-service/internal/client"
	"github.com/tensor-talks/bff-service/internal/config"
	"github.com/tensor-talks/bff-service/internal/handler"
	"github.com/tensor-talks/bff-service/internal/kafka"
	"github.com/tensor-talks/bff-service/internal/middleware"
	"github.com/tensor-talks/bff-service/internal/service"
	"go.uber.org/zap"
)

/*
Пакет server отвечает за сборку зависимостей и запуск HTTP-сервера BFF.

Здесь создаются:
  - HTTP-клиент к auth-service;
  - сервис аутентификации BFF;
  - HTTP-обработчики и middleware (CORS);
  - Gin-роутер с внешним API /api и health-check /healthz.
*/

// Server инкапсулирует HTTP-сервер BFF.
type Server struct {
	httpServer    *http.Server
	logger        *zap.Logger
	kafkaProducer *kafka.Producer
	kafkaConsumer *kafka.Consumer
}

// New конструирует HTTP-сервер BFF, инициализируя все зависимости.
// На этом этапе:
//   - создаётся HTTP-клиент к auth-service;
//   - инициализируется сервис аутентификации и HTTP-обработчики;
//   - навешивается CORS-мидлвара;
//   - регистрируется health-check и маршруты /api.
func New(cfg config.Config, logger *zap.Logger) (*Server, error) {
	authClient, err := client.NewAuthClient(cfg.AuthService.BaseURL, cfg.AuthService.TimeoutSeconds)
	if err != nil {
		return nil, fmt.Errorf("init auth client: %w", err)
	}

	sessionClient := client.NewSessionClient(cfg.SessionService.BaseURL, cfg.SessionService.TimeoutSeconds)
	sessionCRUDClient := client.NewSessionCRUDClient(cfg.SessionCRUD.BaseURL, cfg.SessionCRUD.TimeoutSeconds)
	chatCRUDClient := client.NewChatCRUDClient(cfg.ChatCRUD.BaseURL, cfg.ChatCRUD.TimeoutSeconds)
	resultsCRUDClient := client.NewResultsCRUDClient(cfg.ResultsCRUD.BaseURL, cfg.ResultsCRUD.TimeoutSeconds)

	// Инициализация Kafka producer
	brokers := cfg.Kafka.Brokers
	if len(brokers) == 0 {
		brokers = []string{"kafka:9092"} // fallback
	}

	var kafkaProducer *kafka.Producer
	for attempt := 1; attempt <= 10; attempt++ {
		kafkaProducer, err = kafka.NewProducer(brokers, cfg.Kafka.TopicChatOut, "bff-service", "1.0.0", logger)
		if err == nil {
			break
		}
		logger.Warn("Kafka producer not ready, retrying", zap.Int("attempt", attempt), zap.Error(err))
		time.Sleep(time.Duration(attempt*3) * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("init kafka producer: %w", err)
	}

	var kafkaConsumer *kafka.Consumer
	for attempt := 1; attempt <= 10; attempt++ {
		kafkaConsumer, err = kafka.NewConsumer(brokers, cfg.Kafka.TopicChatIn, cfg.Kafka.ConsumerGroup, logger)
		if err == nil {
			break
		}
		logger.Warn("Kafka consumer not ready, retrying", zap.Int("attempt", attempt), zap.Error(err))
		time.Sleep(time.Duration(attempt*3) * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("init kafka consumer: %w", err)
	}

	// Redis step client — читает текущий шаг агента для polling endpoint
	redisAddr := cfg.Redis.Addr
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	redisStepClient := client.NewRedisStepClient(redisAddr, cfg.Redis.Password, cfg.Redis.DB, logger)

	authService := service.NewAuthService(authClient)
	chatService := service.NewChatService(
		sessionClient,
		sessionCRUDClient,
		chatCRUDClient,
		resultsCRUDClient,
		kafkaProducer,
		redisStepClient,
		logger,
	)

	// Устанавливаем обработчик событий для consumer
	kafkaConsumer.SetEventHandler(chatService)

	knowledgeCRUDClient := client.NewKnowledgeCRUDClient(cfg.KnowledgeCRUD.BaseURL, cfg.KnowledgeCRUD.TimeoutSeconds)

	httpHandler := handler.NewWithUserCrud(authService, chatService, cfg.UserCRUD.BaseURL, logger)
	httpHandler.SetKnowledgeCRUD(knowledgeCRUDClient)

	engine := gin.Default()

	// Middleware для логирования
	engine.Use(loggingMiddleware(logger))

	// Middleware для метрик
	engine.Use(metricsMiddleware("bff-service"))

	// CORS middleware
	engine.Use(middleware.NewCORS(cfg.CORS))

	// Health check
	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	engine.GET("/healthz", healthHandler)
	engine.GET("/health", healthHandler)

	// Metrics endpoint
	engine.GET("/metrics", metricsHandler())

	httpHandler.RegisterRoutes(engine)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &Server{
		httpServer:    httpServer,
		logger:        logger,
		kafkaProducer: kafkaProducer,
		kafkaConsumer: kafkaConsumer,
	}, nil
}

// Run запускает HTTP-сервер и ожидает завершения по контексту или ошибке.
// При завершении контекста выполняется корректное завершение с таймаутом.
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
