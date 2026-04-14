## session-crud-service — CRUD микросервис для сессий

`session-crud-service` — микросервис для работы с сессиями интервью в базе данных PostgreSQL.

### Функциональность

- **POST /sessions** — создание новой сессии
  - Request: `{ "user_id": "uuid", "params": { "topics": [...], "level": "junior|middle|senior", "type": "ml|nlp|llm|cv|ds", "mode": "interview|training|study" } }`
  - Response: `{ "session": {...} }`

- **GET /sessions/:id** — получение сессии по ID

- **GET /sessions/user/:user_id** — получение всех сессий пользователя

- **PUT /sessions/:id/program** — обновление программы интервью
  - Request: `{ "program": { "questions": [...] } }`

- **PUT /sessions/:id/close** — закрытие сессии (установка end_time)

- **DELETE /sessions/:id** — удаление сессии

### База данных

Таблица `sessions`:
- `session_id` (UUID, primary key)
- `user_id` (UUID, indexed)
- `start_time` (timestamp)
- `end_time` (timestamp, nullable)
- `params` (JSONB) — параметры интервьюируемого (`topics`, `level`, `type`, `mode`)
- `interview_program` (JSONB) — программа интервью
- `created_at`, `updated_at` (timestamps)

### Конфигурация

- `SESSION_CRUD_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `SESSION_CRUD_SERVER_PORT` — порт сервера (по умолчанию 8085)
- `SESSION_CRUD_DATABASE_HOST` — хост БД
- `SESSION_CRUD_DATABASE_PORT` — порт БД
- `SESSION_CRUD_DATABASE_USER` — пользователь БД
- `SESSION_CRUD_DATABASE_PASSWORD` — пароль БД
- `SESSION_CRUD_DATABASE_NAME` — имя БД
- `SESSION_CRUD_DATABASE_SSL_MODE` — режим SSL

