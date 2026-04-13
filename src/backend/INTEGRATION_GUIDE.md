# Руководство по интеграции логирования и метрик

Это руководство описывает, как интегрировать логирование и метрики в новые или существующие микросервисы.

## Быстрый старт

### 1. Добавьте зависимости в `go.mod`

```go
require (
    github.com/prometheus/client_golang v1.20.5
    go.uber.org/zap v1.27.0
)
```

### 2. Инициализируйте логгер в `main.go`

```go
package main

import (
    "context"
    "os/signal"
    "syscall"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

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

func main() {
    logger := initLogger("my-service", "1.0.0")
    defer logger.Sync()
    
    // ... остальной код
}
```

### 3. Добавьте middleware в `server.go`

Скопируйте функции `loggingMiddleware`, `metricsMiddleware` и `metricsHandler` из `auth-service/internal/server/server.go`.

### 4. Используйте логгер в handlers

```go
type MyHandler struct {
    logger *zap.Logger
}

func (h *MyHandler) MyEndpoint(c *gin.Context) {
    h.logger.Info("Processing request", zap.String("endpoint", "/my-endpoint"))
    // ... обработка
}
```

### 5. Добавьте бизнес-метрики

Создайте файл `internal/metrics/metrics.go` с метриками для вашего сервиса:

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    MyBusinessMetric = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "tensortalks_business_my_metric_total",
            Help: "Description of metric",
        },
        []string{"service", "status"},
    )
)

func init() {
    prometheus.MustRegister(MyBusinessMetric)
}
```

### 6. Обновите Prometheus конфигурацию

Добавьте ваш сервис в `prometheus.yml`:

```yaml
- job_name: 'my-service'
  static_configs:
    - targets: ['my-service:8083']
      labels:
        service: 'my-service'
        environment: 'development'
```

## Пример: auth-service

Полный пример интеграции можно посмотреть в `auth-service`:
- `cmd/auth-service/main.go` — инициализация логгера
- `internal/server/server.go` — middleware для логирования и метрик
- `internal/handler/auth_handler.go` — использование логгера в handlers
- `internal/metrics/metrics.go` — бизнес-метрики

## Проверка

После интеграции проверьте:

1. **Логи**: `docker logs <service-name>` — должны быть в JSON формате
2. **Метрики**: `curl http://localhost:<port>/metrics` — должны быть доступны
3. **Grafana**: Откройте `http://localhost:3000` и проверьте логи и метрики
4. **Kafka** (если используется): Откройте `http://localhost:9000` (Kafdrop) для просмотра сообщений

## Следующие шаги

- Добавьте логирование в критические операции
- Добавьте бизнес-метрики для важных событий
- Создайте дашборды в Grafana для вашего сервиса
