# Dialogue-aggregator Specification

## Обзор

**Назначение**: Оркестратор диалога — маршрутизация Kafka-сообщений между BFF и AI-агентами. Сам бизнес-логику не принимает — все решения (next / hint / clarify / skip / complete) принимает agent-service.

**Технологии**: Go 1.21+, Kafka, Redis.

**Примечание**: Бывший `mock-model-service`, переименован в `dialogue-aggregator`.

## Функции

- Потребление `chat.events.out` (сообщения пользователя от BFF)
- Получение программы и метаданных сессии (`session_mode`, `topics`, `level`) у session-service
- Хранение состояния диалога в Redis (`dialogue:{chat_id}:state`)
- Отправка полного контекста агенту через `messages.full.data`
- Получение ответов агента через `generated.phrases`
- Публикация ответов агента в `chat.events.in` (для BFF)
- Сохранение сообщений в chat-crud-service
- Публикация `session.completed` (с `session_kind`, `topics`, `level`) для analyst-agent-service
- Сохранение placeholder-результата в results-crud-service (score=0, analyst-agent обновит)

## Kafka топики

| Направление | Топик | Описание |
|-------------|-------|----------|
| Consume | `chat.events.out` | Сообщения пользователя от BFF |
| Consume | `generated.phrases` | Ответы от agent-service |
| Publish | `messages.full.data` | Полный контекст диалога для agent-service |
| Publish | `chat.events.in` | Ответы агента для BFF |
| Publish | `session.completed` | Событие завершения сессии для analyst-agent-service |

## Порядок обработки событий

1. **`chat.started`**: получить программу у session-service, инициализировать Redis-state, отправить первый вопрос через AgentBridge
2. **`chat.user_message`**: сохранить в chat-crud, отправить контекст агенту (`messages.full.data`), получить ответ (`generated.phrases`), опубликовать как `chat.model_question`; при `complete` → placeholder в results-crud, `session.completed`, `chat.completed`
3. **`chat.terminate`**: досрочное завершение, `session.completed` с `terminated_early=true`
4. **`chat.resume`**: восстановление состояния из session-service и chat-crud

## Метрики

- Kafka throughput (messages processed / published)
- Session completion rate

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: `DIALOGUE_AGGREGATOR_SERVER_PORT` (по умолчанию 8084), `DIALOGUE_AGGREGATOR_KAFKA_BROKERS`, топики (`KAFKA_TOPIC_CHAT_OUT`, `KAFKA_TOPIC_CHAT_IN`, `KAFKA_TOPIC_SESSION_COMPLETED`, `KAFKA_TOPIC_MESSAGES_FULL_DATA`, `KAFKA_TOPIC_GENERATED_PHRASES`), URL клиентов (`SESSION_MANAGER_BASE_URL`, `CHAT_CRUD_BASE_URL`, `RESULTS_CRUD_BASE_URL`), Redis (`REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`).

**Секреты (Vault)**: Нет (общается только с внутренними сервисами).

**Версии моделей**: Не применимо (не вызывает LLM).

## Observability / Evals

**Метрики (Prometheus)**: `tensortalks_http_requests_total`, `tensortalks_kafka_producer_messages_total`, `tensortalks_kafka_consumer_messages_total`, `tensortalks_http_request_duration_seconds`.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: Kafka сообщения, HTTP запросы к session-service / chat-crud / results-crud.

**Evals**: Integration tests, Kafka flow tests.
