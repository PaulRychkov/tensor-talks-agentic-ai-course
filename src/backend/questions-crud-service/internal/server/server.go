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
	"github.com/tensor-talks/questions-crud-service/internal/config"
	"github.com/tensor-talks/questions-crud-service/internal/handler"
	"github.com/tensor-talks/questions-crud-service/internal/models"
	"github.com/tensor-talks/questions-crud-service/internal/repository"
	"github.com/tensor-talks/questions-crud-service/internal/service"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Server инкапсулирует HTTP-сервер questions-crud-service.
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
	if err := db.AutoMigrate(&models.Question{}); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	logger.Info("Database migrations completed")

	repo := repository.NewGormQuestionRepository(db)
	svc := service.NewQuestionService(repo)

	// Засеваем вопросы по Python при первом запуске (§10.14/7)
	if err := seedPythonQuestions(db, logger); err != nil {
		logger.Warn("Failed to seed Python questions", zap.Error(err))
	}

	handler := handler.NewQuestionHandler(svc, logger)

	router := gin.Default()

	// Middleware для логирования
	router.Use(loggingMiddleware(logger))

	// Middleware для метрик
	router.Use(metricsMiddleware("questions-crud-service"))

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

// seedPythonQuestions засевает базу вопросами по Python если они ещё не созданы (§10.14/7).
func seedPythonQuestions(db *gorm.DB, logger *zap.Logger) error {
	theoryID := "python"
	var count int64
	if err := db.Model(&models.Question{}).Where("theory_id = ?", theoryID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil // уже засеяно
	}

	ptr := func(s string) *string { return &s }

	seeds := []models.Question{
		{
			ID: "python-q-001", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 1,
			Data: models.QuestionJSONB{
				ID: "python-q-001", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 1,
				Content: models.QuestionContent{
					Question:       "Что такое list comprehension в Python и чем он отличается от обычного цикла for?",
					ExpectedPoints: []string{"Синтаксис [expr for item in iterable if condition]", "Компактнее цикла for + append", "Быстрее за счёт оптимизаций интерпретатора", "Создаёт список целиком в памяти"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "List comprehension — синтаксический сахар для создания списков. [x*2 for x in range(5) if x>1] компактнее и немного быстрее цикла с append. В отличие от генератора — создаёт весь список сразу.", Covers: []string{"синтаксис", "производительность", "память"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-002", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
			Data: models.QuestionJSONB{
				ID: "python-q-002", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
				Content: models.QuestionContent{
					Question:       "Объясните разницу между генератором (generator) и обычной функцией. Когда стоит использовать генераторы?",
					ExpectedPoints: []string{"yield вместо return — функция возобновляется с того же места", "Ленивые вычисления (lazy evaluation)", "Экономия памяти для больших последовательностей", "Генератор — объект-итератор, поддерживает next()"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "Функция-генератор содержит yield: при каждом next() она продолжает с последнего yield. Позволяет создавать бесконечные последовательности и обрабатывать большие данные без загрузки всего в память.", Covers: []string{"yield", "lazy evaluation", "память"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-003", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
			Data: models.QuestionJSONB{
				ID: "python-q-003", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
				Content: models.QuestionContent{
					Question:       "Что такое декоратор в Python? Напишите простой декоратор для измерения времени выполнения функции.",
					ExpectedPoints: []string{"Функция, принимающая функцию и возвращающая новую функцию", "Синтаксис @decorator", "functools.wraps для сохранения метаданных", "Применение: логирование, авторизация, кэширование"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "Декоратор — обёртка над функцией. def timer(func): @functools.wraps(func) def wrapper(*a,**kw): t=time.time(); r=func(*a,**kw); print(time.time()-t); return r; return wrapper. @functools.wraps сохраняет __name__ и __doc__.", Covers: []string{"синтаксис", "functools.wraps", "применение"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-004", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 3,
			Data: models.QuestionJSONB{
				ID: "python-q-004", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 3,
				Content: models.QuestionContent{
					Question:       "Что такое GIL (Global Interpreter Lock) в CPython? Как он влияет на многопоточность?",
					ExpectedPoints: []string{"GIL — мьютекс, допускающий один поток в байткоде одновременно", "Нет выигрыша от потоков для CPU-bound задач", "Для I/O-bound потоки эффективны — GIL освобождается при ожидании I/O", "Для CPU-параллелизма — multiprocessing или C-расширения"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "GIL — глобальная блокировка CPython: только один поток исполняет байткод одновременно. Потоки не дают параллелизма для CPU-bound задач. При ожидании I/O GIL освобождается — threading.Thread подходит для I/O-bound. Для CPU-bound — multiprocessing.", Covers: []string{"определение", "CPU-bound vs I/O-bound", "альтернативы"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-005", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 3,
			Data: models.QuestionJSONB{
				ID: "python-q-005", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 3,
				Content: models.QuestionContent{
					Question:       "Объясните принципы asyncio: event loop, корутины, await. Чем asyncio отличается от threading?",
					ExpectedPoints: []string{"async def — корутина, await приостанавливает выполнение", "Event loop планирует выполнение корутин", "Кооперативная многозадачность vs вытесняющая (threading)", "asyncio не создаёт новых OS-потоков", "Подходит для I/O-bound, не для CPU-bound"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "asyncio — кооперативная многозадачность: корутина сама говорит 'я жду' через await, передавая управление event loop. В отличие от threading не создаёт OS-потоков — накладные расходы ниже. CPU-bound код заблокирует весь loop.", Covers: []string{"event loop", "await", "кооперативность", "отличие от threading"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-006", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
			Data: models.QuestionJSONB{
				ID: "python-q-006", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
				Content: models.QuestionContent{
					Question:       "Что означают *args и **kwargs? Приведите пример функции с произвольным числом аргументов.",
					ExpectedPoints: []string{"*args собирает позиционные аргументы в кортеж", "**kwargs собирает именованные аргументы в словарь", "Порядок: обычные, *args, keyword-only, **kwargs", "Используются в wrapper-функциях и при распаковке"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "*args — кортеж позиционных аргументов, **kwargs — словарь именованных. Пример: def log(level, *msgs, sep=' ', **meta). При вызове: log('INFO','a','b',sep='-',user='x'). Звёздочки работают и при вызове для распаковки: func(*lst, **dct).", Covers: []string{"*args", "**kwargs", "порядок", "распаковка"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-007", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
			Data: models.QuestionJSONB{
				ID: "python-q-007", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
				Content: models.QuestionContent{
					Question:       "Что такое контекстный менеджер? Как реализовать свой с помощью класса и contextlib?",
					ExpectedPoints: []string{"with ... as ..., методы __enter__ и __exit__", "__exit__ получает информацию об исключении и может подавить его", "Гарантирует выполнение завершающего кода", "@contextlib.contextmanager — генераторный способ"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "Контекстный менеджер реализует __enter__ и __exit__. __exit__ вызывается при выходе, даже при исключении, и может его подавить. Через contextlib: @contextmanager def cm(): setup(); yield val; teardown(). Код до yield — __enter__, после — __exit__.", Covers: []string{"__enter__/__exit__", "исключение", "contextlib"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-008", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 1,
			Data: models.QuestionJSONB{
				ID: "python-q-008", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 1,
				Content: models.QuestionContent{
					Question:       "Объясните duck typing в Python: преимущества и потенциальные проблемы.",
					ExpectedPoints: []string{"Python проверяет наличие методов, а не типы", "Гибкость: полиморфизм без наследования", "Ошибки типов обнаруживаются только в runtime", "Для ранней проверки — type hints и mypy"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "Duck typing: Python не проверяет тип объекта — он проверяет наличие нужного метода. Если объект умеет __len__, он 'ведёт себя как последовательность'. Плюс: гибкость и полиморфизм. Минус: ошибки типов только в runtime. Решение — type hints + mypy.", Covers: []string{"концепция", "полиморфизм", "риски", "type hints"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-009", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 3,
			Data: models.QuestionJSONB{
				ID: "python-q-009", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 3,
				Content: models.QuestionContent{
					Question:       "Что такое дескрипторы в Python? Как работают property, classmethod и staticmethod под капотом?",
					ExpectedPoints: []string{"Дескриптор — объект с __get__, __set__ или __delete__", "property — дескриптор данных для управления атрибутом", "classmethod передаёт класс как первый аргумент", "staticmethod — обычная функция без cls и self"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "Дескриптор управляет доступом к атрибуту через __get__/__set__/__delete__. property — дескриптор данных: getter/setter вызываются при чтении/записи. classmethod.__get__ возвращает метод с классом как первым аргументом. staticmethod.__get__ — просто функция.", Covers: []string{"__get__/__set__", "property", "classmethod", "staticmethod"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
		{
			ID: "python-q-010", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
			Data: models.QuestionJSONB{
				ID: "python-q-010", TheoryID: ptr(theoryID), QuestionType: "open", Complexity: 2,
				Content: models.QuestionContent{
					Question:       "Чем отличаются изменяемые (mutable) и неизменяемые (immutable) объекты? Как это влияет на передачу аргументов?",
					ExpectedPoints: []string{"Immutable: int, str, tuple — нельзя изменить", "Mutable: list, dict, set — изменяемые на месте", "Python передаёт объекты по ссылке (pass by object reference)", "Баг: изменяемый дефолтный аргумент def f(x=[])"},
				},
				IdealAnswer: models.QuestionIdealAnswer{Text: "Immutable-объекты нельзя изменить — операции создают новый объект. Mutable изменяются на месте. Python передаёт ссылку: изменение mutable-аргумента внутри функции видно снаружи. Классический баг: def f(x=[]) — список создаётся один раз. Правильно: def f(x=None): if x is None: x=[].", Covers: []string{"immutable vs mutable", "передача по ссылке", "дефолтный список"}},
				Metadata: models.QuestionMetadata{Language: "ru", CreatedBy: "seed", LastUpdated: "2025-01-01"},
			},
		},
	}

	for _, q := range seeds {
		q.Version = "1.0"
		if err := db.Create(&q).Error; err != nil {
			logger.Warn("Failed to seed question", zap.String("id", q.ID), zap.Error(err))
		}
	}

	logger.Info("Python questions seeded", zap.Int("count", len(seeds)))
	return nil
}
