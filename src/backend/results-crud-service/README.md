## results-crud-service — CRUD микросервис для результатов, пресетов и прогресса по темам

`results-crud-service` хранит итоги сессий любого типа (`interview` / `training` / `study`), рекомендованные пресеты программ и прогресс пользователя по темам в PostgreSQL.

### Функциональность

#### Результаты сессий

- **POST /results** — создание/upsert результата
  - Request: `{ "session_id": "uuid", "user_id": "uuid", "session_kind": "interview", "score": 0, "feedback": "...", "terminated_early": false }`
  - Используется dialogue-aggregator для записи placeholder-а сразу после `chat.completed`.

- **PATCH /results/:session_id** — обновление финального отчёта
  - Используется `analyst-agent-service` для записи `report_json`, `evaluations`, итогового `score`, `result_format_version`.

- **GET /results/:session_id** — получение результата по сессии.
- **GET /results?session_ids=uuid1,uuid2,...** — батч-получение.

#### Пресеты (рекомендованные программы)

- **POST /presets** — создание пресета (вызывается `analyst-agent-service` после interview-сессии).
- **GET /presets?user_id=...** — список пресетов пользователя для Dashboard.

#### Прогресс по темам

- **POST /user-topic-progress** / **PATCH** — обновляется `analyst-agent-service` после study-сессии.
- **GET /user-topic-progress?user_id=...** — текущий прогресс пользователя по темам.

### База данных

**Таблица `results`:**
- `id` (PK)
- `session_id` (UUID, unique indexed)
- `user_id` (UUID, indexed)
- `session_kind` (varchar) — `interview` / `training` / `study`
- `score` (int) — 0 у placeholder-а, обновляется analyst-agent
- `feedback` (text)
- `terminated_early` (bool)
- `report_json` (jsonb, nullable) — финальный отчёт от analyst-agent
- `evaluations` (jsonb) — покомпонентные оценки по вопросам
- `result_format_version` (varchar)
- `created_at`, `updated_at`

**Таблица `presets`:**
- `id` (PK), `user_id` (indexed), `source_session_id`, `payload` (jsonb), `created_at`.

**Таблица `user_topic_progress`:**
- `id` (PK), `user_id` (indexed), `topic`, `level`, `mastery_score`, `last_session_id`, `updated_at`.

Схема создаётся через GORM AutoMigrate при старте сервиса.

### Конфигурация

- `RESULTS_CRUD_SERVER_HOST` / `RESULTS_CRUD_SERVER_PORT`
- `RESULTS_CRUD_DATABASE_HOST` / `_PORT` / `_USER` / `_PASSWORD` / `_NAME` / `_SSL_MODE`
