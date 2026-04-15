## interview-builder-service — сервис создания программы интервью

`interview-builder-service` — Python FastAPI сервис-планировщик, реализованный как **LangGraph ReAct-агент** с tool-calling. Создаёт программу интервью/тренировки/study с осмысленной траекторией и прогрессией сложности, с fallback на детерминированный pipeline при сбое агента.

**Ключевые особенности**:
- **Tools**: `search_questions`, `check_topic_coverage`, `validate_program`, `get_topic_relations`, `search_knowledge_base`, `get_user_history`.
- **Structured output (Pydantic)**: `ProgramMeta` (`validation_passed`, `coverage`, `fallback_reason`).
- **Episodic memory**: при `use_previous_results=true` вызывается `get_user_history(user_id, topics)` — слабые/сильные темы и scores из предыдущих сессий учитываются при подборе вопросов и прогрессии.
- **Fallback**: при сбое агента — детерминированный pipeline `filter → dedup → coverage → balance → mode_profile → sort → enrich`.

### Функциональность

- Слушает Kafka топик `interview.build.request`.
- Получает параметры (`topics`, `level`, `type`, `mode` ∈ `interview` | `training` | `study`).
- Запрашивает вопросы из `questions-crud-service` и теорию из `knowledge-base-crud-service`.
- Прогоняет программу через пайплайн `filter → dedup → coverage → balance → mode_profile → sort → enrich`.
- Возвращает программу + `program_meta` (`validation_passed`, `coverage`, `fallback_reason`, `generator_version`) в `interview.build.response`.

### Режимы (`mode`)

- `interview` — стандартное собеседование, баланс по сложности и темам.
- `training` — режим тренировки, расширенный набор вопросов одной темы.
- `study` — обучающий режим, акцент на теории и постепенном усложнении.

Профиль режима применяется на шаге `mode_profile` (количество вопросов, распределение complexity, минимальное покрытие тем).

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
  "program": {
    "questions": [
      {
        "id": "q-001",
        "question": "Почему L2‑регуляризация уменьшает переобучение?",
        "theory": "L2‑регуляризация добавляет штраф за квадрат нормы весов...",
        "order": 1
      }
    ]
  },
  "program_meta": {
    "validation_passed": true,
    "coverage": {"nlp": 3, "llm": 2},
    "fallback_reason": null,
    "generator_version": "1.2.0"
  }
}
```

### Метрики

- `tensortalks_http_requests_total` — HTTP запросы
- `tensortalks_http_request_duration_seconds` — длительность HTTP запросов

