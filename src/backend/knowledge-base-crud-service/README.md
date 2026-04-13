## knowledge-base-crud-service — CRUD сервис для базы знаний

`knowledge-base-crud-service` — микросервис для работы с базой знаний в PostgreSQL.

### Функциональность

- CRUD операции над знаниями (создание, чтение, обновление, удаление)
- Поиск знаний по фильтрам: complexity, concept, parent_id, tags
- Хранение структурированных знаний в JSONB формате

### Эндпоинты

- `POST /knowledge` — создание нового знания
- `GET /knowledge/:id` — получение знания по ID
- `PUT /knowledge/:id` — обновление знания
- `DELETE /knowledge/:id` — удаление знания
- `GET /knowledge?complexity=1&concept=...&parent_id=...&tags=...` — поиск знаний по фильтрам

### Конфигурация

- `KNOWLEDGE_BASE_CRUD_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `KNOWLEDGE_BASE_CRUD_SERVER_PORT` — порт сервера (по умолчанию 8090)
- `KNOWLEDGE_BASE_CRUD_DATABASE_HOST` — хост PostgreSQL
- `KNOWLEDGE_BASE_CRUD_DATABASE_PORT` — порт PostgreSQL
- `KNOWLEDGE_BASE_CRUD_DATABASE_USER` — пользователь БД
- `KNOWLEDGE_BASE_CRUD_DATABASE_PASSWORD` — пароль БД
- `KNOWLEDGE_BASE_CRUD_DATABASE_NAME` — имя БД

### Формат знания

```json
{
  "id": "theory_l2_regularization",
  "concept": "L2 регуляризация",
  "complexity": 2,
  "parent_id": "theory_regularization_common",
  "tags": ["machine_learning", "regularization"],
  "segments": [
    {
      "type": "definition",
      "content": "..."
    }
  ],
  "relations": [...],
  "metadata": {
    "version": "1.0",
    "language": "ru",
    "last_updated": "2025-01-27"
  }
}
```

### Метрики

- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов
- `tensortalks_business_knowledge_operations_total` — операции с базой знаний

