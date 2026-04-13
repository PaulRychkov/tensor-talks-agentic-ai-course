# Логирование в микросервисах TensorTalks

## Обзор

Все микросервисы TensorTalks используют единый формат логирования для централизованного сбора и анализа через **Grafana Loki**. Логи выводятся в **stdout** в JSON-формате, что позволяет легко собирать их через Docker/Kubernetes и отправлять в Loki.

## Архитектура

```
┌─────────────────┐
│  Микросервисы    │
│  (Go/Python)     │
│  stdout (JSON)   │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Promtail        │
│  (сбор логов)    │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Grafana Loki    │
│  (хранилище)     │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Grafana         │
│  (визуализация)  │
└─────────────────┘
```

## Единый формат логов

Все логи должны соответствовать следующему JSON-формату:

```json
{
  "timestamp": "2025-01-15T10:30:00.123Z",
  "level": "info",
  "message": "Service started successfully",
  "service": "auth-service",
  "version": "1.0.0",
  "environment": "production",
  "caller": "server/server.go:45",
  "request_id": "req-abc123",
  "user_id": "user-xyz789",
  "duration_ms": 150,
  "status_code": 200,
  "error": "connection timeout"
}
```

### Обязательные поля

- `timestamp` — ISO 8601 UTC время события
- `level` — уровень логирования: `debug`, `info`, `warn`, `error`
- `message` — текстовое описание события
- `service` — имя микросервиса (например, `auth-service`)
- `version` — версия сервиса
- `environment` — окружение: `development`, `staging`, `production`

### Опциональные поля

- `caller` — файл и строка кода (формат: `path/to/file.go:123`)
- `request_id` — уникальный ID запроса для трейсинга
- `user_id` — ID пользователя (если применимо)
- `duration_ms` — длительность операции в миллисекундах
- `status_code` — HTTP статус код
- `error` — текст ошибки
- `stacktrace` — стектрейс для ошибок

## Использование в Go-микросервисах

### Установка

Используйте общий пакет `github.com/tensor-talks/common/logger`:

```go
import "github.com/tensor-talks/common/logger"
```

### Базовое использование

```go
package main

import (
    "github.com/tensor-talks/common/logger"
)

func main() {
    // Создание логгера
    log := logger.New("auth-service", "1.0.0")
    defer log.Sync()

    log.Info("Service starting",
        zap.String("port", "8081"),
        zap.String("host", "0.0.0.0"),
    )
}
```

### Логирование с контекстом запроса

```go
func (h *AuthHandler) Login(c *gin.Context) {
    // Генерируем request_id
    requestID := uuid.New().String()
    log := logger.New("auth-service", "1.0.0").WithRequestID(requestID)
    
    log.Info("Login attempt",
        zap.String("login", req.Login),
    )
    
    // ... обработка запроса ...
    
    if err != nil {
        log.Error("Login failed",
            zap.Error(err),
            zap.String("reason", "invalid credentials"),
        )
        return
    }
    
    log.Info("Login successful",
        zap.String("user_id", user.ID.String()),
        zap.Duration("duration", time.Since(start)),
    )
}
```

### Логирование ошибок

```go
if err != nil {
    log.Error("Failed to connect to database",
        zap.Error(err),
        zap.String("host", dbHost),
        zap.Int("port", dbPort),
        zap.String("database", dbName),
    )
}
```

### Уровни логирования

```go
log.Debug("Detailed debug information")  // Только в development
log.Info("General information")          // Важные события
log.Warn("Warning message")             // Предупреждения
log.Error("Error occurred", zap.Error(err)) // Ошибки
```

## Использование в Python-микросервисах

### Установка

Скопируйте `common/logger/python/logger.py` в ваш микросервис или добавьте как зависимость.

### Базовое использование

```python
from logger import get_logger

logger = get_logger("my-service", "1.0.0", level="INFO")

logger.info("Service started", extra={"port": 8080})
logger.warning("Deprecated endpoint used")
logger.error("Failed to process request", exc_info=True)
```

### Логирование с контекстом запроса

```python
from logger import get_logger, with_request_id, with_user_id

logger = get_logger("auth-service", "1.0.0")

# С request_id
request_logger = with_request_id(logger, "req-123")
request_logger.info("Processing login request", extra={"login": "user@example.com"})

# С user_id
user_logger = with_user_id(logger, "user-456")
user_logger.info("User action performed")
```

## Рекомендации по логированию

### Что логировать

✅ **Обязательно логировать:**
- Запуск и остановку сервиса
- Ошибки всех уровней (с контекстом)
- Критические операции (регистрация, логин, платежи)
- Внешние вызовы (HTTP-запросы к другим сервисам)
- Медленные запросы (>1 секунды)
- Изменения конфигурации

✅ **Рекомендуется логировать:**
- Входные параметры важных операций (без секретов!)
- Результаты операций (успех/неудача)
- Метрики производительности (длительность операций)
- Бизнес-события (создание заказа, отправка сообщения)

❌ **Не логировать:**
- Пароли, токены, секретные ключи
- Полные тела запросов с чувствительными данными
- Избыточная информация на каждом шаге (используйте debug уровень)

### Уровни логирования

- **DEBUG** — детальная информация для разработки (только в development)
- **INFO** — общая информация о работе сервиса
- **WARN** — предупреждения о потенциальных проблемах
- **ERROR** — ошибки, требующие внимания

### Контекст и трейсинг

Всегда добавляйте `request_id` для трейсинга запросов через несколько микросервисов:

```go
// В middleware или начале обработчика
requestID := c.GetHeader("X-Request-ID")
if requestID == "" {
    requestID = uuid.New().String()
}
c.Set("request_id", requestID)

log := logger.New("service", "1.0.0").WithRequestID(requestID)
```

## Просмотр логов в Grafana

### Доступ к Grafana

1. Откройте Grafana: `http://localhost:3000`
2. Логин: `admin`, пароль: `admin` (измените в проде!)

### Запросы в Loki

#### Все логи сервиса

```logql
{service="auth-service"}
```

#### Ошибки за последний час

```logql
{service="auth-service"} |= "error" | level="error"
```

#### Логи конкретного запроса

```logql
{service="auth-service"} | json | request_id="req-abc123"
```

#### Медленные запросы

```logql
{service="auth-service"} | json | duration_ms > 1000
```

#### Логи по пользователю

```logql
{service="auth-service"} | json | user_id="user-xyz789"
```

### Создание дашборда

1. В Grafana: **Explore** → выберите **Loki** как источник данных
2. Введите LogQL запрос
3. Нажмите **Add to dashboard** для сохранения

### Полезные запросы

**Топ ошибок:**
```logql
sum by (error) (count_over_time({service="auth-service"} | json | level="error" [5m]))
```

**Средняя длительность запросов:**
```logql
avg_over_time({service="auth-service"} | json | duration_ms [5m])
```

**Количество запросов по статус-кодам:**
```logql
sum by (status_code) (count_over_time({service="auth-service"} | json | status_code [5m]))
```

## Конфигурация Promtail

Promtail собирает логи из контейнеров Docker и отправляет в Loki. Конфигурация находится в `docker-compose.yml`:

```yaml
promtail:
  image: grafana/promtail:latest
  volumes:
    - /var/lib/docker/containers:/var/lib/docker/containers:ro
    - ./promtail-config.yml:/etc/promtail/config.yml
  command: -config.file=/etc/promtail/config.yml
```

## Troubleshooting

### Логи не появляются в Grafana

1. Проверьте, что Promtail запущен: `docker ps | grep promtail`
2. Проверьте логи Promtail: `docker logs promtail`
3. Убедитесь, что формат логов соответствует JSON
4. Проверьте подключение Promtail к Loki

### Логи в неправильном формате

Убедитесь, что используете общий пакет `common/logger` и выводите логи в stdout, а не в stderr или файлы.

### Слишком много логов

Используйте уровни логирования правильно:
- В production: `INFO` и выше
- В development: можно `DEBUG`
- Настройте фильтры в Promtail для исключения debug-логов в проде

## Дополнительные ресурсы

- [Grafana Loki документация](https://grafana.com/docs/loki/latest/)
- [LogQL язык запросов](https://grafana.com/docs/loki/latest/logql/)
- [Zap logger документация](https://pkg.go.dev/go.uber.org/zap)
- [Python logging документация](https://docs.python.org/3/library/logging.html)
