## questions-crud-service — CRUD сервис для базы вопросов

`questions-crud-service` — микросервис для работы с базой вопросов в PostgreSQL.

### Функциональность

- CRUD операции над вопросами (создание, чтение, обновление, удаление)
- Поиск вопросов по фильтрам: complexity, theory_id, question_type
- Хранение структурированных вопросов в JSONB формате

### Эндпоинты

- `POST /questions` — создание нового вопроса
- `GET /questions/:id` — получение вопроса по ID
- `PUT /questions/:id` — обновление вопроса
- `DELETE /questions/:id` — удаление вопроса
- `GET /questions?complexity=1&theory_id=...&question_type=...` — поиск вопросов по фильтрам

### Конфигурация

- `QUESTIONS_CRUD_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `QUESTIONS_CRUD_SERVER_PORT` — порт сервера (по умолчанию 8091)
- `QUESTIONS_CRUD_DATABASE_HOST` — хост PostgreSQL
- `QUESTIONS_CRUD_DATABASE_PORT` — порт PostgreSQL
- `QUESTIONS_CRUD_DATABASE_USER` — пользователь БД
- `QUESTIONS_CRUD_DATABASE_PASSWORD` — пароль БД
- `QUESTIONS_CRUD_DATABASE_NAME` — имя БД

### Формат вопроса

```json
{
  "id": "question_l2_01",
  "theory_id": "theory_l2_regularization",
  "linked_segments": ["definition", "formula"],
  "question_type": "conceptual",
  "complexity": 2,
  "content": {
    "question": "...",
    "expected_points": [...],
    "links_to_theory": [...]
  },
  "ideal_answer": {
    "text": "...",
    "covers": [...]
  },
  "metadata": {
    "language": "ru",
    "created_by": "auto-generator",
    "last_updated": "2025-01-27"
  }
}
```

### Метрики

- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов
- `tensortalks_business_question_operations_total` — операции с базой вопросов

