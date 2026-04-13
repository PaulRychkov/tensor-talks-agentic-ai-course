# Python Logger

Единый модуль логирования для Python-микросервисов TensorTalks.

## Установка

Скопируйте файл `logger.py` в ваш микросервис или добавьте как зависимость.

## Использование

```python
from logger import get_logger, with_request_id, with_user_id

# Создание логгера
logger = get_logger("my-service", "1.0.0", level="INFO")

# Базовое логирование
logger.info("Service started", extra={"port": 8080})
logger.warning("Deprecated endpoint used")
logger.error("Failed to process request", exc_info=True)

# Логирование с request_id для трейсинга
request_logger = with_request_id(logger, "req-123")
request_logger.info("Processing request")

# Логирование с user_id
user_logger = with_user_id(logger, "user-456")
user_logger.info("User action performed")
```

## Формат логов

Все логи выводятся в stdout в JSON-формате:

```json
{
  "timestamp": "2025-01-15T10:30:00.123Z",
  "level": "info",
  "message": "Service started",
  "service": "my-service",
  "version": "1.0.0",
  "environment": "production",
  "caller": "main.py:42",
  "port": 8080
}
```
