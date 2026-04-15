## analyst-agent-service — AI-аналитик результатов сессий

`analyst-agent-service` — Python FastAPI микросервис, формирующий финальный отчёт после завершения любой сессии (interview, training, study). Реализован как **LangGraph ReAct-агент** с tool-calling. Подписан на Kafka топик `session.completed`, по `session_kind` маршрутизирует обработку.

**Ключевые особенности**:
- **Structured output (Pydantic)**: `AnalystReport` (summary, errors_by_topic, strengths, preparation_plan, materials), `TrainingPreset`, `ProgressDelta` — валидация через `model_validate_json` + fallback на линейный pipeline при невалидации.
- **Episodic memory**: `ProgressDelta` для сравнения динамики между сессиями (текущая vs предыдущие по темам).
- **Tools**: `get_evaluations`, `group_errors_by_topic`, `search_knowledge_base`, `web_search` (arxiv), `fetch_url` (trusted domains), `generate_report_section`, `validate_report`, `save_draft_material` (HITL), `emit_report`.
- **Validation loop**: `validate_report` → при FAIL доработка секций (лимит итераций); после max итераций — fallback на линейный pipeline.

### Архитектура

```
Kafka (session.completed)
    ↓
Analyst Agent Service (Consumer)
    ↓
LangGraph → AnalystService
    ├── Загрузка данных (results-crud, session-service, chat-crud)
    ├── Анализ evaluations
    ├── Генерация report_json (LLM)
    └── Сохранение результата
    ↓
Results CRUD Service (PATCH /results/:session_id)
```

### Маршрутизация по session_kind

| session_kind | Действие |
|-------------|----------|
| `interview` | Формирует `report_json` + `evaluations`, обновляет `results`, создаёт `presets` (слабые темы, рекомендованные материалы) |
| `training` | Формирует `report_json` + `evaluations`, обновляет `results` итоговой оценкой. Пресеты **не** создаются |
| `study` | Формирует `report_json` + `evaluations`, обновляет `user_topic_progress` (`theory_completed` по теме) |

### Kafka

| Топик | Роль | Описание |
|-------|------|----------|
| `session.completed` | consumer | Событие завершения сессии от dialogue-aggregator |

**Payload `session.completed`:**
```json
{
  "session_id": "uuid",
  "session_kind": "interview|training|study",
  "user_id": "uuid",
  "chat_id": "uuid",
  "topics": ["nlp", "llm"],
  "level": "middle",
  "terminated_early": false,
  "answered_questions": 8,
  "total_questions": 10,
  "completed_at": "2026-04-01T12:05:00Z"
}
```

### Внешние зависимости

- `results-crud-service` (`PATCH /results/:session_id`, `POST /presets`, `POST /user-topic-progress`) — сохранение отчёта, пресетов, прогресса
- `session-service` (`GET /sessions/:id/program`) — программа и параметры сессии
- `chat-crud-service` (`GET /messages/:session_id`) — история диалога для анализа

### Структура отчёта (report_json)

```json
{
  "summary": "Общая оценка кандидата...",
  "sections": {
    "errors": [...],
    "strengths": [...],
    "plan": [...],
    "materials": [...]
  },
  "evaluations": [
    {"question_id": "q1", "score": 0.8, "feedback": "..."}
  ],
  "preset_training": {
    "weak_topics": ["nlp"],
    "recommended_materials": [...]
  }
}
```

### Конфигурация

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `KAFKA_BOOTSTRAP_SERVERS` | `localhost:9092` | Kafka брокеры |
| `KAFKA_TOPIC_SESSION_COMPLETED` | `session.completed` | Топик входящих событий |
| `KAFKA_CONSUMER_GROUP` | `analyst-agent-service-group` | Группа consumer |
| `LLM_PROVIDER` | `openai` | Провайдер LLM |
| `LLM_BASE_URL` | — | URL LLM API |
| `LLM_API_KEY` | — | API ключ |
| `LLM_MODEL` | `gpt-5.4` | Основная модель (отчёты) |
| `LLM_MODEL_MINI` | `gpt-5.4-mini` | Средняя модель |
| `LLM_MODEL_NANO` | `gpt-5.4-nano` | Лёгкая модель |
| `LLM_TEMPERATURE` | `0.4` | Температура |
| `LLM_MAX_TOKENS` | `4000` | Макс. токенов |
| `RESULTS_CRUD_SERVICE_URL` | `http://results-crud-service:8095` | URL results-crud |
| `SESSION_SERVICE_URL` | `http://session-service:8083` | URL session-service |
| `CHAT_CRUD_SERVICE_URL` | `http://chat-crud-service:8087` | URL chat-crud |
| `REST_API_PORT` | `8094` | Порт REST API (health, metrics) |
| `METRICS_PORT` | `9094` | Порт Prometheus метрик |

### Метрики

- `analyst_sessions_analyzed_total{session_kind, status}` — обработанные сессии
- `analyst_error_count{error_type}` — ошибки обработки
- `analyst_report_score` (Histogram) — распределение итоговых score
- `analyst_validation_attempts` (Histogram) — число итераций `validate_report`
- `agent_llm_call_duration_seconds`, `guardrail_triggered_total` — общие agent-метрики
