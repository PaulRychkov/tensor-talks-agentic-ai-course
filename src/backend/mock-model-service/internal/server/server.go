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
	"github.com/tensor-talks/mock-model-service/internal/client"
	"github.com/tensor-talks/mock-model-service/internal/config"
	"github.com/tensor-talks/mock-model-service/internal/kafka"
	"github.com/tensor-talks/mock-model-service/internal/redisclient"
	"github.com/tensor-talks/mock-model-service/internal/service"
	"go.uber.org/zap"
)

// Server инкапсулирует HTTP-сервер и Kafka consumer/producer.
type Server struct {
	httpServer    *http.Server
	logger        *zap.Logger
	kafkaConsumer *kafka.Consumer
	kafkaProducer *kafka.Producer
	agentBridge   *kafka.AgentBridge
}

// New создаёт новый экземпляр Server.
func New(cfg config.Config, logger *zap.Logger) (*Server, error) {
	// Инициализация Kafka producer
	kafkaProducer, err := kafka.NewProducer(
		cfg.Kafka.Brokers,
		cfg.Kafka.TopicChatIn,
		"dialogue-aggregator",
		"1.0.0",
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("init kafka producer: %w", err)
	}

	// Инициализация Kafka consumer
	kafkaConsumer, err := kafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.TopicChatOut,
		cfg.Kafka.ConsumerGroup,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("init kafka consumer: %w", err)
	}

	// Инициализируем bridge к agent-service
	agentBridge, err := kafka.NewAgentBridge(kafka.AgentBridgeConfig{
		Brokers:        cfg.Kafka.Brokers,
		TopicMessages:  cfg.Kafka.TopicMessages,
		TopicGenerated: cfg.Kafka.TopicGenerated,
		GroupID:        cfg.Kafka.AgentGroupID,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("init agent bridge: %w", err)
	}

	// Инициализация клиентов
	sessionManagerClient := client.NewSessionManagerClient(
		cfg.SessionManager.BaseURL,
		cfg.SessionManager.TimeoutSeconds,
	)
	chatCRUDClient := client.NewChatCRUDClient(
		cfg.ChatCRUD.BaseURL,
		cfg.ChatCRUD.TimeoutSeconds,
	)
	resultsCRUDClient := client.NewResultsCRUDClient(
		cfg.ResultsCRUD.BaseURL,
		cfg.ResultsCRUD.TimeoutSeconds,
	)

	// Инициализируем Redis клиент
	redisClient := redisclient.New(redisclient.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Создаём сервис модели (будущий marking-service)
	modelService := service.NewModelService(
		kafkaProducer,
		sessionManagerClient,
		chatCRUDClient,
		resultsCRUDClient,
		redisClient,
		agentBridge,
		cfg.Model.QuestionDelaySeconds,
		logger,
	)

	// Устанавливаем обработчик событий для consumer
	kafkaConsumer.SetEventHandler(modelService)

	router := gin.Default()

	// Middleware для логирования
	router.Use(loggingMiddleware(logger))

	// Middleware для метрик
	router.Use(metricsMiddleware("dialogue-aggregator"))

	// Health check
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Metrics endpoint
	router.GET("/metrics", metricsHandler())

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
		agentBridge:   agentBridge,
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
