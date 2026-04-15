## dialogue-aggregator (Go, бывший mock-model-service) — оркестратор интервью и диалогов

`dialogue-aggregator` (ранее `mock-model-service`) — оркестратор диалога: читает события из Kafka (`chat.events.out`), хранит активный диалог в Redis и `chat-crud-service`, общается с `interviewer-agent-service` через AgentBridge (`messages.full.data` / `generated.phrases`) и публикует результаты обратно в `chat.events.in`. **Сам бизнес-логику не принимает** — все решения (`next` / `hint` / `clarify` / `skip` / `complete`) принимает `interviewer-agent-service`. Дополнительно публикует `session.completed` в Kafka для `analyst-interviewer-agent-service`.

## Логика работы

### Порядок обработки событий

1. **Событие `chat.started`:**
   - Парсит `session_id`, вызывает `GET /sessions/{id}/program` у `session-service` и получает программу + метаданные сессии (`session_mode`, `topics`, `level`).
   - Сохраняет программу и метаданные в локальный `SessionManager` (`SetProgram` + `SetMeta`).
   - Инициализирует Redis-состояние диалога (`dialogue:{chat_id}:state`).
   - Отправляет первый вопрос через AgentBridge (агенту) и отдаёт его пользователю как `chat.model_question`.

2. **Событие `chat.user_message`:**
   - Сохраняет сообщение пользователя в `chat-crud-service`.
   - Отправляет полный контекст диалога агенту через `messages.full.data`.
   - Получает от агента решение (`generated.phrases`) и публикует его как `chat.model_question`.
   - Если агент решил `complete` → сохраняет placeholder-результат в `results-crud-service` (score=0, аналитик обновит), закрывает сессию в `session-service`, публикует `session.completed` (с реальными `session_kind`/`topics`/`level`) и `chat.completed`.

3. **Событие `chat.terminate` (досрочное завершение):**
   - Считает фактически отвеченные вопросы из Redis-состояния.
   - Сохраняет placeholder-результат с `terminated_early=true`, закрывает сессию, публикует `session.completed` (`terminated_early=true`) и `chat.completed`.

4. **Событие `chat.resume`:**
   - Перечитывает программу + метаданные у `session-service`, восстанавливает историю из `chat-crud-service` в Redis.

### Особенности

- **Программа интервью**: получается у `session-service` (без статичных вопросов).
- **Метаданные сессии** (`session_kind`, `topics`, `level`) хранятся в `SessionState` и пробрасываются в `session.completed` — analyst-agent корректно роутит интервью / training / study.
- **Решения принимает только interviewer-agent-service**: dialogue-aggregator не модифицирует payload агента и не вычисляет score — placeholder со score=0 нужен лишь для того, чтобы UI получил заглушку, пока analyst-agent не положит финальный `report_json`.
- **Сохранение сообщений**: каждое сообщение (system/user) сохраняется в `chat-crud-service`, затем уходит в Kafka.

## Конфигурация

- `DIALOGUE_AGGREGATOR_SERVER_HOST` — хост сервера (по умолчанию `0.0.0.0`)
- `DIALOGUE_AGGREGATOR_SERVER_PORT` — порт сервера (по умолчанию `8084`)
- `DIALOGUE_AGGREGATOR_KAFKA_BROKERS` — адреса Kafka брокеров
- `DIALOGUE_AGGREGATOR_KAFKA_TOPIC_CHAT_OUT` — топик для чтения (`chat.events.out`)
- `DIALOGUE_AGGREGATOR_KAFKA_TOPIC_CHAT_IN` — топик для записи (`chat.events.in`)
- `DIALOGUE_AGGREGATOR_KAFKA_TOPIC_SESSION_COMPLETED` — топик `session.completed` (для analyst-agent-service)
- `DIALOGUE_AGGREGATOR_KAFKA_TOPIC_MESSAGES_FULL_DATA` — топик для отправки контекста агенту
- `DIALOGUE_AGGREGATOR_KAFKA_TOPIC_GENERATED_PHRASES` — топик для приёма ответов агента
- `DIALOGUE_AGGREGATOR_KAFKA_CONSUMER_GROUP` — группа consumer
- `DIALOGUE_AGGREGATOR_MODEL_QUESTION_DELAY_SECONDS` — задержка перед отправкой вопроса (по умолчанию 2)
- `DIALOGUE_AGGREGATOR_SESSION_MANAGER_BASE_URL` — URL `session-service` (по умолчанию `http://session-service:8083`)
- `DIALOGUE_AGGREGATOR_SESSION_MANAGER_TIMEOUT_SECONDS` — таймаут запросов к session-service
- `DIALOGUE_AGGREGATOR_CHAT_CRUD_BASE_URL` — URL `chat-crud-service`
- `DIALOGUE_AGGREGATOR_CHAT_CRUD_TIMEOUT_SECONDS` — таймаут запросов к chat-crud
- `DIALOGUE_AGGREGATOR_RESULTS_CRUD_BASE_URL` — URL `results-crud-service`
- `DIALOGUE_AGGREGATOR_RESULTS_CRUD_TIMEOUT_SECONDS` — таймаут запросов к results-crud
- `DIALOGUE_AGGREGATOR_REDIS_ADDR` / `_PASSWORD` / `_DB` — Redis для dialogue state

## Результаты сессии

`dialogue-aggregator` **не вычисляет** score / feedback / recommendations — это делает `analyst-interviewer-agent-service` после `session.completed`. На момент завершения сессии dialogue-aggregator кладёт в `results-crud-service` только placeholder (`score=0`, `report_json=null`), который analyst-agent позже обновляет финальным отчётом. BFF `GetResults` возвращает результаты только когда `report_json` присутствует — до этого фронт показывает состояние «отчёт формируется».

## Метрики

- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов
- `tensortalks_kafka_producer_messages_total` — сообщения, опубликованные в Kafka
- `tensortalks_kafka_consumer_messages_total` — сообщения, обработанные из Kafka

## Логирование

Все ключевые операции логируются: получение программы и метаданных, сохранение сообщений, отправка вопросов, публикация `session.completed`, завершение чатов и ошибки обработки.
