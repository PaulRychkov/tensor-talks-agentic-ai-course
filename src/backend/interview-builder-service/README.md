## interview-builder-service — сервис создания программы интервью

`interview-builder-service` — Python FastAPI сервис для создания программы интервью из вопросов и знаний.

### Функциональность

- Слушает Kafka топик `interview.build.request`
- Получает параметры интервью (topics, level, type)
- Запрашивает вопросы из questions-crud-service по фильтрам
- Запрашивает знания из knowledge-base-crud-service для каждого вопроса
- Собирает программу интервью (5 вопросов по умолчанию)
- Упорядочивает вопросы по логике (связанные вопросы рядом)
- Отправляет программу в Kafka топик `interview.build.response`

### Маппинг параметров

**Уровни:**
- `junior` → complexity: 1
- `middle` → complexity: 2
- `senior` → complexity: 3

**Темы:**
- `classic_ml` → tags: ["machine_learning"]
- `nlp` → tags: ["nlp"]
- `llm` → tags: ["llm"]

### Конфигурация

- `KAFKA_BOOTSTRAP_SERVERS` — адреса Kafka брокеров (по умолчанию localhost:9092)
- `KAFKA_TOPIC_REQUEST` — топик для запросов (по умолчанию interview.build.request)
- `KAFKA_TOPIC_RESPONSE` — топик для ответов (по умолчанию interview.build.response)
- `KAFKA_CONSUMER_GROUP` — группа consumer (по умолчанию interview-builder-service-group)
- `QUESTIONS_CRUD_URL` — URL questions-crud-service (по умолчанию http://localhost:8091)
- `KNOWLEDGE_BASE_CRUD_URL` — URL knowledge-base-crud-service (по умолчанию http://localhost:8090)
- `QUESTIONS_PER_INTERVIEW` — количество вопросов в интервью (по умолчанию 5)
- `SERVER_HOST` — хост сервера (по умолчанию 0.0.0.0)
- `SERVER_PORT` — порт сервера (по умолчанию 8089)

### Формат программы интервью

```json
{
  "questions": [
    {
      "question": "Почему L2‑регуляризация уменьшает переобучение?",
      "theory": "L2‑регуляризация добавляет штраф за квадрат нормы весов...",
      "order": 1
    }
  ]
}
```

### Метрики

- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов

