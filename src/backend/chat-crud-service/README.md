## chat-crud-service — CRUD микросервис для чатов

`chat-crud-service` — микросервис для работы с чатами и сообщениями в базе данных PostgreSQL.

### Функциональность

- **POST /messages** — сохранение нового сообщения
  - Request: `{ "session_id": "uuid", "type": "system"|"user", "content": "..." }`
  - Response: `{ "message": {...} }`

- **GET /messages/:session_id** — получение всех сообщений сессии

- **GET /chat-dumps/:session_id** — получение дампа завершенного чата

- **POST /chat-dumps/:session_id** — создание дампа чата из сообщений

### База данных

**Таблица `messages`:**
- id (PK)
- session_id (UUID, indexed)
- type (system/user)
- content (text)
- created_at (timestamp)

**Таблица `chat_dumps`:**
- id (PK)
- session_id (UUID, unique indexed)
- chat (JSONB) — структурированный дамп чата
- created_at, updated_at (timestamps)

### Конфигурация

- `CHAT_CRUD_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `CHAT_CRUD_SERVER_PORT` — порт сервера (по умолчанию 8087)
- `CHAT_CRUD_DATABASE_HOST` — хост БД
- `CHAT_CRUD_DATABASE_PORT` — порт БД
- `CHAT_CRUD_DATABASE_USER` — пользователь БД
- `CHAT_CRUD_DATABASE_PASSWORD` — пароль БД
- `CHAT_CRUD_DATABASE_NAME` — имя БД
- `CHAT_CRUD_DATABASE_SSL_MODE` — режим SSL

