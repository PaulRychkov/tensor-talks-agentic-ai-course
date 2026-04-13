package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/tensor-talks/user-store-service/internal/config"
	"github.com/tensor-talks/user-store-service/internal/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		zap.L().Fatal("Failed to load config", zap.Error(err))
	}

	// Инициализируем логгер
	logger := initLogger("user-store-service", "1.0.0")
	defer logger.Sync()

	srv, err := server.New(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize server", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	logger.Info("Service starting",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
	)

	if err := srv.Run(ctx); err != nil {
		logger.Fatal("Server stopped with error", zap.Error(err))
	}

	logger.Info("Service stopped gracefully")
}

// initLogger создаёт логгер с единым форматом для микросервиса
func initLogger(serviceName, version string) *zap.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stdout"}
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.CallerKey = "caller"

	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		panic(err)
	}

	return logger.With(
		zap.String("service", serviceName),
		zap.String("version", version),
		zap.String("environment", "production"),
	)
}
