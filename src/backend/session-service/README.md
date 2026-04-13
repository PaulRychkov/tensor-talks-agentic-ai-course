## session-service — микросервис управления сессиями (Session Manager)

`session-service` — микросервис для управления жизненным циклом сессий интервью. Координирует создание сессий, кэширует активные сессии в Redis, интегрируется с interview-builder-service через Kafka и сохраняет данные в session-crud-service.

### Функциональность

- **POST /sessions** — создание новой сессии с параметрами интервью
  - Request: `{ "user_id": "uuid", "params": { "topics": [...], "level": "...", "type": "..." } }`
  - Response: `{ "session_id": "uuid", "ready": true }`
  - Проверяет лимит активных сессий в Redis
  - Создаёт сессию в session-crud-service
  - Отправляет запрос в Kafka на создание программы интервью
  - Ожидает готовую программу (таймаут 30 секунд)
  - Сохраняет программу в CRUD и Redis

- **GET /sessions/:id/program** — получение программы интервью
  - Response: `{ "program": { "questions": [...] } }`
  - Сначала проверяет Redis кэш, если нет — загружает из session-crud-service

- **PUT /sessions/:id/close** — закрытие сессии
  - Удаляет сессию из Redis кэша
  - Обновляет время окончания в session-crud-service

### Архитектура

```
BFF → Session Manager → Session CRUD (БД)
                  ↓
            Redis (кэш активных сессий)
                  ↓
            Kafka Producer (interview.build.request)
                  ↓
            Interview Builder Service
                  ↓
            Kafka Consumer (interview.build.response)
                  ↓
            Session Manager (сохранение программы)
```

### Redis

Используется для кэширования активных сессий:
- Ключ: `session:{session_id}`
- Значение: JSON с программой интервью и метаданными
- TTL: настраиваемый (по умолчанию 24 часа)

### Kafka

Работает с двумя топиками:
- `interview.build.request` — запрос на создание программы интервью
- `interview.build.response` — ответ с готовой программой

### Конфигурация

- `SESSION_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `SESSION_SERVER_PORT` — порт сервера (по умолчанию 8083)
- `SESSION_REDIS_ADDR` — адрес Redis (по умолчанию "localhost:6379")
- `SESSION_REDIS_PASSWORD` — пароль Redis
- `SESSION_REDIS_DB` — номер БД Redis (по умолчанию 0)
- `SESSION_REDIS_TTL_HOURS` — TTL для сессий в часах (по умолчанию 24)
- `SESSION_SESSION_CRUD_BASE_URL` — URL session-crud-service
- `SESSION_SESSION_CRUD_TIMEOUT_SECONDS` — таймаут запросов к CRUD
- `SESSION_KAFKA_BROKERS` — адреса Kafka брокеров
- `SESSION_KAFKA_TOPIC_REQUEST` — топик для запросов
- `SESSION_KAFKA_TOPIC_RESPONSE` — топик для ответов
- `SESSION_KAFKA_CONSUMER_GROUP` — группа consumer
- `SESSION_SESSION_MANAGER_MAX_ACTIVE_SESSIONS` — максимальное количество активных сессий (по умолчанию 100)

### Метрики

- `tensortalks_business_sessions_created_total` — количество созданных сессий
- `tensortalks_business_sessions_operations_total` — операции с сессиями
- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов
