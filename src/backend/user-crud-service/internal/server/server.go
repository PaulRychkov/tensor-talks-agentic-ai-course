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
	"github.com/tensor-talks/user-crud-service/internal/config"
	"github.com/tensor-talks/user-crud-service/internal/handler"
	"github.com/tensor-talks/user-crud-service/internal/models"
	"github.com/tensor-talks/user-crud-service/internal/repository"
	"github.com/tensor-talks/user-crud-service/internal/service"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

/*
Пакет server собирает зависимости user-crud-service и управляет жизненным циклом HTTP-сервера.

Здесь:
  - инициализируется подключение к PostgreSQL через GORM;
  - выполняется автоматическая миграция схемы (модель User);
  - создаются репозиторий, сервис и HTTP-обработчик;
  - поднимается Gin-сервер с health-check и CRUD/отладочными маршрутами.
*/

// Server инкапсулирует HTTP-сервер user-crud-service.
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

// New создаёт новый экземпляр Server, настраивая подключение к БД и HTTP-маршруты.
// Включает:
//   - установку соединения с PostgreSQL через GORM и логирование SQL-запросов;
//   - AutoMigrate для модели User (создание/обновление схемы таблицы);
//   - создание репозитория, сервисного слоя и HTTP-обработчика;
//   - инициализацию Gin-роутера с health-check и CRUD/отладочными маршрутами.
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
		&models.User{},
		&models.LoginAdjective{},
		&models.LoginNoun{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	logger.Info("Database migrations completed")

	repo := repository.NewGormUserRepository(db)
	svc := service.NewUserService(repo)

	// Засеваем словарь логинов при первом запуске (§10.14/3)
	if err := seedLoginWords(db, logger); err != nil {
		logger.Warn("Failed to seed login words", zap.Error(err))
	}

	handler := handler.NewUserHandler(svc, logger)

	router := gin.Default()

	// Middleware для логирования
	router.Use(loggingMiddleware(logger))

	// Middleware для метрик
	router.Use(metricsMiddleware("user-crud-service"))

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

// Run запускает HTTP-сервер и ожидает завершения по контексту или ошибке.
// При остановке по контексту выполняет корректное завершение с таймаутом, что
// позволяет завершить текущие HTTP-запросы.
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

// seedLoginWords засевает словарь прилагательных и существительных для генерации логинов
// при первом запуске сервиса, если таблицы пусты (§10.14/3).
func seedLoginWords(db *gorm.DB, logger *zap.Logger) error {
	adjectives := []string{
		"Bright", "Swift", "Bold", "Clever", "Steady", "Keen", "Calm", "Sharp",
		"Noble", "Brave", "Vivid", "Quick", "Smart", "Warm", "Cool", "Pure",
		"Grand", "Fair", "True", "Deep", "Free", "High", "Wide", "Open",
		"Strong", "Light", "Clear", "Fresh", "Great", "Fine", "Solid", "Prime",
		"Rapid", "Agile", "Super", "Mega", "Ultra", "Hyper", "Turbo", "Alpha",
		"Elite", "Epic", "Lucky", "Happy", "Witty", "Exact", "Lucid", "Fluent",
		"Daring", "Cosmic",
	}

	nouns := []string{
		"Neural", "Tensor", "Compiler", "Kernel", "Pipeline", "Gradient", "Cluster",
		"Socket", "Lambda", "Vector", "Matrix", "Neuron", "Sigmoid", "Softmax",
		"Encoder", "Decoder", "Transformer", "Attention", "Embedding", "Dropout",
		"Backprop", "Perceptron", "Convolution", "Pooling", "Batch", "Epoch",
		"Optimizer", "Scheduler", "Tokenizer", "Parser", "Lexer", "Runtime",
		"Container", "Daemon", "Thread", "Process", "Mutex", "Semaphore",
		"Buffer", "Cache", "Queue", "Stack", "Heap", "Graph", "Tree", "Node",
		"Edge", "Vertex", "Pointer", "Iterator", "Hash", "Bloom", "Trie",
		"Subnet", "Gateway", "Proxy", "Router", "Firewall", "Protocol",
		"Payload", "Header", "Schema", "Index", "Cursor", "Shard", "Replica",
		"Broker", "Consumer", "Producer", "Offset", "Partition", "Topic",
		"Checkpoint", "Snapshot", "Rollback", "Migration", "Sandbox", "Beacon",
		"Circuit", "Register", "Signal", "Interrupt", "Module",
		"Package", "Library", "Framework", "Pattern", "Factory", "Adapter",
		"Observer", "Strategy", "Singleton", "Builder", "Bridge", "Facade",
		"Mediator", "Prototype", "Functor", "Monad", "Coroutine", "Fiber",
	}

	var adjCount, nounCount int64
	db.Model(&models.LoginAdjective{}).Count(&adjCount)
	db.Model(&models.LoginNoun{}).Count(&nounCount)

	if adjCount > 0 && nounCount > 0 {
		return nil // уже засеяно
	}

	if adjCount == 0 {
		for _, word := range adjectives {
			if err := db.FirstOrCreate(&models.LoginAdjective{}, models.LoginAdjective{Word: word}).Error; err != nil {
				logger.Warn("Failed to seed adjective", zap.String("word", word), zap.Error(err))
			}
		}
		logger.Info("Login adjectives seeded", zap.Int("count", len(adjectives)))
	}

	if nounCount == 0 {
		for _, word := range nouns {
			if err := db.FirstOrCreate(&models.LoginNoun{}, models.LoginNoun{Word: word}).Error; err != nil {
				logger.Warn("Failed to seed noun", zap.String("word", word), zap.Error(err))
			}
		}
		logger.Info("Login nouns seeded", zap.Int("count", len(nouns)))
	}

	return nil
}


