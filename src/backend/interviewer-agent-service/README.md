## interviewer-agent-service — AI-интервьюер (LangGraph)

`interviewer-agent-service` — Python FastAPI микросервис, реализующий AI-интервьюера на LangGraph. Получает контекст диалога через Kafka, прогоняет граф (классификация → оценка → решение → генерация), возвращает реплику и решение в `dialogue-aggregator`.

### Архитектура

```
Kafka (messages.full.data)
    ↓
Agent Service (Consumer)
    ↓
LangGraph State Machine
    ├── receive_message (входная точка)
    ├── check_pii (regex + sanitize; блокирует PII)
    ├── load_context (Redis, Session Service, KB CRUD)
    ├── check_off_topic (LLM классификация)
    ├── determine_question_index (LLM)
    ├── evaluate_answer (LLM)
    ├── make_decision (детерминированное)
    └── generate_response (LLM)
    ↓
Kafka (generated.phrases)
    ↓
Dialogue Aggregator → BFF → Frontend
```

### Поток данных

1. `dialogue-aggregator` публикует полный контекст диалога в `messages.full.data`.
2. Consumer парсит `MessageFullPayload` (chat_id, session_id, user_id, content, question_id, metadata).
3. Для `role=user` — обработка ответа; для `role=system` с `status=started` — генерация приветствия и первого вопроса.
4. LangGraph граф загружает контекст (история из Redis, программа из session-service, теория из KB CRUD).
5. Агент оценивает ответ и принимает решение:
   - `next_question` — переход к следующему вопросу (score ≥ 0.8);
   - `ask_clarification` — уточняющий вопрос (0.4 ≤ score < 0.8, макс. `MAX_CLARIFICATIONS`);
   - `off_topic_reminder` — возврат к теме интервью;
   - `thank_you` — завершение (все вопросы пройдены);
   - `hint` / `skip` — по запросу пользователя;
   - `blocked_pii` — сообщение заблокировано PII-фильтром (до `load_context`).
6. Ответ публикуется в `generated.phrases`, `dialogue-aggregator` доставляет его пользователю.

### Kafka

| Топик | Роль | Описание |
|-------|------|----------|
| `messages.full.data` | consumer | Входящий контекст диалога от dialogue-aggregator |
| `generated.phrases` | producer | Реплика + решение агента |
| `session.completed` | (config only) | Топик настроен, но публикация из dialogue-aggregator |

### Внешние зависимости

- `session-service` (`GET /sessions/:id/program`) — программа интервью
- `chat-crud-service` — история сообщений
- `knowledge-base-crud-service` — теория для вопросов
- Redis — кэш диалога (`dialogue:{chat_id}:state`, `dialogue:{chat_id}:messages`)

### LangGraph State

```python
AgentState = {
    "chat_id", "session_id", "user_id", "message_id",
    "user_message", "question_id", "session_mode",
    "dialogue_history", "interview_program",
    "current_question_index", "current_question", "current_theory",
    "answer_evaluation",   # {completeness_score, accuracy_score, overall_score, is_complete, missing_points}
    "agent_decision",      # next_question | ask_clarification | off_topic_reminder | thank_you | error | blocked_pii
    "generated_response",
    "evaluations",         # накопленные оценки по вопросам
    "attempts", "hints_given", "tool_calls",
}
```

### Конфигурация

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `KAFKA_BOOTSTRAP_SERVERS` | `localhost:9092` | Kafka брокеры |
| `KAFKA_TOPIC_MESSAGES_FULL` | `messages.full.data` | Топик входящих сообщений |
| `KAFKA_TOPIC_GENERATED` | `generated.phrases` | Топик исходящих реплик |
| `KAFKA_CONSUMER_GROUP` | `interviewer-agent-service-group` | Группа consumer |
| `LLM_PROVIDER` | `openai` | Провайдер LLM (openai, anthropic, local) |
| `LLM_BASE_URL` | — | URL LLM API (для прокси или локальной модели) |
| `LLM_API_KEY` | — | API ключ |
| `LLM_MODEL` | `gpt-5.4-mini` | Основная модель |
| `LLM_MODEL_NANO` | `gpt-5.4-nano` | Лёгкая модель (классификация) |
| `LLM_TEMPERATURE` | `0.7` | Температура |
| `LLM_MAX_TOKENS` | `2000` | Макс. токенов |
| `SESSION_SERVICE_URL` | `http://session-service:8083` | URL session-service |
| `CHAT_CRUD_SERVICE_URL` | `http://chat-crud-service:8087` | URL chat-crud |
| `KNOWLEDGE_BASE_CRUD_SERVICE_URL` | `http://knowledge-base-crud-service:8090` | URL KB CRUD |
| `REDIS_HOST` | `localhost` | Redis хост |
| `REDIS_PORT` | `6379` | Redis порт |
| `REST_API_PORT` | `8093` | Порт REST API (health, metrics) |
| `METRICS_PORT` | `9092` | Порт Prometheus метрик |
| `MAX_CLARIFICATIONS` | `3` | Макс. уточняющих вопросов подряд |

### Метрики

- `agent_messages_processed_total{status, decision}` — обработанные сообщения
- `agent_processing_duration_seconds` — время обработки
- `agent_current_question_index` — номер текущего вопроса
- `agent_error_count{error_type}` — ошибки
