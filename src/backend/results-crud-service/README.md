## results-crud-service — CRUD микросервис для результатов

`results-crud-service` — микросервис для работы с результатами интервью в базе данных PostgreSQL.

### Функциональность

- **POST /results** — создание нового результата
  - Request: `{ "session_id": "uuid", "score": 85, "feedback": "..." }`
  - Response: `{ "result": {...} }`

- **GET /results/:session_id** — получение результата по session_id

- **GET /results?session_ids=uuid1,uuid2,...** — получение результатов по списку session_id

### База данных

**Таблица `results`:**
- id (PK)
- session_id (UUID, unique indexed)
- score (int)
- feedback (text)
- created_at, updated_at (timestamps)

### Конфигурация

- `RESULTS_CRUD_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `RESULTS_CRUD_SERVER_PORT` — порт сервера (по умолчанию 8088)
- `RESULTS_CRUD_DATABASE_HOST` — хост БД
- `RESULTS_CRUD_DATABASE_PORT` — порт БД
- `RESULTS_CRUD_DATABASE_USER` — пользователь БД
- `RESULTS_CRUD_DATABASE_PASSWORD` — пароль БД
- `RESULTS_CRUD_DATABASE_NAME` — имя БД
- `RESULTS_CRUD_DATABASE_SSL_MODE` — режим SSL

