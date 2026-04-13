## dialogue-aggregator (Go, бывший mock-model-service) — оркестратор интервью и диалогов

`dialogue-aggregator` (ранее `mock-model-service`) — это сервис-агрегатор, который оркестрирует интервью: читает события из Kafka (`chat.events.out`), хранит активный диалог в Redis и `chat-crud-service`, обменивается сообщениями с `agent-service` через Kafka и отправляет вопросы/результаты обратно в Kafka (`chat.events.in`).

## Логика работы

### Порядок обработки событий

1. **Событие `chat.started`:**
   - Получает программу интервью от `session-manager-service` по session_id
   - Сохраняет программу в локальное состояние
   - Отправляет первый вопрос через `chat.model_question`
   - Сохраняет системное сообщение в `chat-crud-service`
   - Увеличивает счётчик вопросов

2. **Событие `chat.user_message`:**
   - Сохраняет сообщение пользователя в `chat-crud-service`
   - Проверяет программу интервью — есть ли ещё вопросы
   - Если все вопросы заданы → завершает чат:
     - Сохраняет финальное системное сообщение в `chat-crud-service`
     - Сигнализирует `chat-crud-service` о завершении (создаёт дамп чата)
     - Сохраняет результаты в `results-crud-service`
     - Закрывает сессию в `session-manager-service`
     - Отправляет `chat.completed` с результатами в Kafka
   - Если есть ещё вопросы → отправляет следующий вопрос:
     - Сохраняет системное сообщение в `chat-crud-service`
     - Отправляет вопрос в Kafka
     - Увеличивает счётчик вопросов

### Особенности

- **Программа интервью:** Получает программу от `session-manager-service`, не использует статичные вопросы
- **Сохранение сообщений:** Каждое сообщение (system/user) сначала сохраняется в `chat-crud-service`, затем отправляется в Kafka
- **Результаты:** Сохраняет результаты интервью в `results-crud-service`
- **Управление сессиями:** Закрывает сессии через `session-manager-service`

## Конфигурация

- `MOCK_MODEL_SERVER_HOST` — хост сервера (по умолчанию "0.0.0.0")
- `MOCK_MODEL_SERVER_PORT` — порт сервера (по умолчанию 8084)
- `MOCK_MODEL_KAFKA_BROKERS` — адреса Kafka брокеров
- `MOCK_MODEL_KAFKA_TOPIC_CHAT_OUT` — топик для чтения (chat.events.out)
- `MOCK_MODEL_KAFKA_TOPIC_CHAT_IN` — топик для записи (chat.events.in)
- `MOCK_MODEL_KAFKA_CONSUMER_GROUP` — группа consumer
- `MOCK_MODEL_MODEL_QUESTION_DELAY_SECONDS` — задержка перед отправкой вопроса (по умолчанию 2)
- `MOCK_MODEL_SESSION_MANAGER_BASE_URL` — URL session-manager-service (по умолчанию "http://session-service:8083")
- `MOCK_MODEL_SESSION_MANAGER_TIMEOUT_SECONDS` — таймаут запросов к session-manager (по умолчанию 5)
- `MOCK_MODEL_CHAT_CRUD_BASE_URL` — URL chat-crud-service (по умолчанию "http://chat-crud-service:8087")
- `MOCK_MODEL_CHAT_CRUD_TIMEOUT_SECONDS` — таймаут запросов к chat-crud (по умолчанию 5)
- `MOCK_MODEL_RESULTS_CRUD_BASE_URL` — URL results-crud-service (по умолчанию "http://results-crud-service:8088")
- `MOCK_MODEL_RESULTS_CRUD_TIMEOUT_SECONDS` — таймаут запросов к results-crud (по умолчанию 5)

## Генерация результатов

### Оценка (Score)
- Базовая оценка: `60 + (questions_asked * 5)`
- Случайный фактор: ±10
- Диапазон: 0-100

### Обратная связь (Feedback)
Зависит от оценки:
- ≥90: "Отличное понимание основ машинного обучения..."
- ≥75: "Хорошее понимание основ ML..."
- ≥60: "Базовое понимание машинного обучения..."
- <60: "Требуется дополнительное изучение..."

### Рекомендации
Зависят от оценки:
- <70: Базовые рекомендации (линейная регрессия, метрики)
- 70-85: Продвинутые рекомендации (регуляризация, feature engineering)
- ≥85: Экспертные рекомендации (нейронные сети, ML System Design)

## Метрики

- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов

## Логирование

Все операции логируются:
- Получение программы интервью
- Сохранение сообщений в chat-crud
- Отправка вопросов
- Сохранение результатов
- Завершение чатов
- Ошибки обработки