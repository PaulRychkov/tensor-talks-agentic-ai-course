package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

/*
Пакет logger предоставляет единый интерфейс логирования для всех Go-микросервисов.

Все логи выводятся в stdout в JSON-формате для централизованного сбора через Grafana Loki.
Формат логов соответствует единому стандарту TensorTalks для удобного парсинга и анализа.

Использование:
	logger := logger.New("auth-service", "1.0.0")
	logger.Info("service started", zap.String("port", "8081"))
	logger.Error("failed to connect", zap.Error(err))
*/

// Logger обёртка над zap.Logger с единым форматом для всех микросервисов.
type Logger struct {
	*zap.Logger
	serviceName string
	version     string
}

// Config конфигурация логгера.
type Config struct {
	ServiceName string // Имя сервиса (например, "auth-service")
	Version     string // Версия сервиса (например, "1.0.0")
	Level       string // Уровень логирования: "debug", "info", "warn", "error" (по умолчанию "info")
	Environment string // Окружение: "development", "production" (по умолчанию "production")
}

// New создаёт новый экземпляр логгера с единым форматом для микросервисов.
// Все логи выводятся в stdout в JSON-формате для сбора через Loki.
func New(serviceName, version string) *Logger {
	return NewWithConfig(Config{
		ServiceName: serviceName,
		Version:     version,
		Level:       "info",
		Environment: "production",
	})
}

// NewWithConfig создаёт логгер с кастомной конфигурацией.
func NewWithConfig(cfg Config) *Logger {
	// Определяем уровень логирования
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Настройка encoder config для JSON-формата
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// В development режиме используем более читаемый формат
	if cfg.Environment == "development" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// Создаём core для вывода в stdout
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		zap.NewAtomicLevelAt(level),
	)

	// Создаём logger с полями сервиса
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// Добавляем стандартные поля для всех логов
	logger = logger.With(
		zap.String("service", cfg.ServiceName),
		zap.String("version", cfg.Version),
		zap.String("environment", cfg.Environment),
	)

	return &Logger{
		Logger:      logger,
		serviceName: cfg.ServiceName,
		version:     cfg.Version,
	}
}

// WithRequestID добавляет request_id к логгеру для трейсинга запросов.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{
		Logger:      l.Logger.With(zap.String("request_id", requestID)),
		serviceName: l.serviceName,
		version:     l.version,
	}
}

// WithUserID добавляет user_id к логгеру.
func (l *Logger) WithUserID(userID string) *Logger {
	return &Logger{
		Logger:      l.Logger.With(zap.String("user_id", userID)),
		serviceName: l.serviceName,
		version:     l.version,
	}
}

// WithFields добавляет произвольные поля к логгеру.
func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	return &Logger{
		Logger:      l.Logger.With(fields...),
		serviceName: l.serviceName,
		version:     l.version,
	}
}
