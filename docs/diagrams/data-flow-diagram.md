# Data Flow Diagram

## Потоки данных через систему

**Этап 0** (наполнение базы, **Knowledge-producer-service**, LLM-workflow): `web_search` → `fetch_url` → LLM →  KB/Questions → `save_draft_material` → HITL; на общей схеме ниже не раскрыт.

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         Data Flow Overview                               │
└──────────────────────────────────────────────────────────────────────────┘

┌─────────────┐
│ Пользователь│
└──────┬──────┘
       │ 1. Выбор параметров интервью
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Frontend (React)                                                        │
│ - Сохранение состояния выбора                                           │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 2. POST /api/sessions {specialty, grade, use_previous_results, …}
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ BFF (Go + Gin)                                                          │
│ - JWT валидация                                                         │
│ - Маршрутизация                                                         │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 3. Создать сессию
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Session-service (Go + Redis)                                            │
│ - Генерация session_id                                                  │
│ - Кеширование в Redis (TTL: 2h)                                         │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ Redis Key: session:{session_id}                                     │ │
│ │ Value: { user_id, specialty, grade, session_type,                   │ │
│ │             status: "pending_program" }                             │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 4. Publish: interview.build.request
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Kafka Topic: interview.build.request                                    │
│ Partition key: session_id                                               │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 5. Consume
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Interview-builder-service (FastAPI + LangGraph)                         │
│ - Параметры из Kafka; tools к CRUD (см. specs/interview-builder)        │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 6. HTTP GET: /api/questions?topic=X&level=Y
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Questions-crud-service (Go + PostgreSQL)                                │
│ - Фильтрация вопросов                                                   │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ PostgreSQL: questions_crud_db                                       │ │
│ │ Table: questions                                                    │ │
│ │ Columns: id, topic, difficulty, type, content, expected_answer,     │ │
│ │          criteria, version, created_at                              │ │
│ │ Format: JSONB для content, expected_answer, criteria                │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 7. Возврат вопросов
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Interview-builder-service                                               │
│ - Обогащение контекстом из базы знаний                                  │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 8. HTTP GET: /api/knowledge?concept=X
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Knowledge-base-crud-service (Go + PostgreSQL)                           │
│ - Поиск релевантных концепций                                           │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ PostgreSQL: knowledge_base_crud_db                                  │ │
│ │ Table: knowledge_items                                              │ │
│ │ Columns: id, concept, difficulty, description, formulas, examples,  │ │
│ │          related_concepts, tags, version                            │ │
│ │ Format: JSONB для description, formulas, examples                   │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 9. Возврат концепций
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Interview-builder-service                                               │
│ - Формирование программы (5 вопросов)                                   │
│ - Обогащение теорией для каждого вопроса                                │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 10. Publish: interview.build.response
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Kafka Topic: interview.build.response                                   │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 11. Consume
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Session-service                                                         │
│ - Обновление Redis: status = "ready", program = [...]                   │
│ - Сохранение программы в Session-crud (после build.response)            │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 12. Запись программы (внутренний вызов к Session-crud)
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Session-crud-service (Go + PostgreSQL)                                  │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ PostgreSQL: session_crud_db                                         │ │
│ │ Table: sessions                                                     │ │
│ │ Columns: id, user_id, specialty, grade, session_type, program (JSONB),│
│ │          status, created_at, updated_at                             │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
└──────┬──────────────────────────────────────────────────────────────────┘
       │ 13. Подтверждение сохранения
       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Frontend → Пользователь: Интервью готово, показ первого вопроса         │
└─────────────────────────────────────────────────────────────────────────┘


┌─────────────────────────────────────────────────────────────────────────┐
│                    Поток сообщений чата (Interview)                     │
└─────────────────────────────────────────────────────────────────────────┘

Пользователь → Frontend: Ввод ответа
       │
       ▼
Frontend → BFF: POST /api/sessions/{id}/message {content}
       │
       ▼
BFF → Kafka: Publish chat.events.out
       │         ┌──────────────────────────────────────────────────────┐
       │         │ Topic: chat.events.out                               │
       │         │ Partition: session_id                                │
       │         │ Message: {event_type, session_id, content,           │
       │         │           timestamp, user_id}                        │
       │         └──────────────────────────────────────────────────────┘
       │
       ▼
Dialogue-aggregator: Consume chat.events.out
       │
       ├─→ Chat-crud-service: Сохранить сообщение пользователя
       │         ┌──────────────────────────────────────────────────────┐
       │         │ PostgreSQL: chat_crud_db.messages                    │
       │         │ Columns: id, session_id, role, content, timestamp    │
       │         └──────────────────────────────────────────────────────┘
       │
       ├─→ Kafka: Publish messages.full.data (полный контекст диалога)
       │
       ▼
Interviewer-agent-service: Consume messages.full.data
       │
       ▼
LangGraph State: Загрузка контекста сессии
       │
       ▼
Агент-интервьюер: Анализ ответа
       │
       ├─→ Tool: evaluate_answer()
       │         ├─→ LLM: Оценка ответа (GPT-5.4-mini)
       │         └─→ Возврат: Pydantic AnswerEvaluation (score, decision_confidence, errors, missing_points, strong_points)
       │         └─→ confidence-aware routing: ≥0.7 next, <0.7 hint, <0.5 self-reflection
       │
       ├─→ Tool: search_knowledge_base(query, topic) (подсказка)
       │         └─→ Knowledge-base-crud: Поиск теории
       │
       ├─→ Tool: search_questions(related_to=...) (подвопрос)
       │
       ├─→ Tool: web_search() + fetch_url() (при необходимости)
       │         └─→ Внешнее API: результаты и текст страницы
       │
       ▼
Агент: Принятие решения (следующий вопрос / подсказка / уточнение)
       │
       ▼
Агент → LLM: Генерация реплики
       │
       ▼
Interviewer-agent-service → Kafka: Publish generated.phrases
       │         ┌──────────────────────────────────────────────────────┐
       │         │ Topic: generated.phrases                             │
       │         │ Message: {event_type, session_id, content,           │
       │         │           decision, metadata}                        │
       │         └──────────────────────────────────────────────────────┘
       │
       ▼
Dialogue-aggregator: Consume generated.phrases
       │
       ├─→ Chat-crud-service: Сохранить ответ агента
       │
       ├─→ Kafka: Publish chat.events.in
       │         ┌──────────────────────────────────────────────────────┐
       │         │ Topic: chat.events.in                                │
       │         │ Message: {event_type, session_id, content,           │
       │         │           evaluation?, metadata}                     │
       │         └──────────────────────────────────────────────────────┘
       │
       ├─→ BFF: Consume chat.events.in
       │         │
       │         └─→ Frontend: Доставить ответ пользователю
       │
       └─→ (при complete) Results-crud-service: placeholder (score=0)
                 ┌──────────────────────────────────────────────────────┐
                 │ PostgreSQL: results_crud_db.evaluations              │
                 │ Columns: id, session_id, question_index, score,      │
                 │          errors (JSONB), feedback (JSONB)            │
                 └──────────────────────────────────────────────────────┘


┌─────────────────────────────────────────────────────────────────────────┐
│                    Поток формирования отчёта (Report)                   │
└─────────────────────────────────────────────────────────────────────────┘

Kafka: session.completed (session_id, session_kind, topics, level) от Dialogue-aggregator
       │
       ▼
Analyst-agent-service: get_evaluations(session_id); маршрутизация по session_kind
       │
       ▼
group_errors_by_topic(evaluations)
       │
       ├─→ search_knowledge_base(query, topic)
       │         └─→ Knowledge-base-crud
       │
       ├─→ При необходимости: web_search, fetch_url
       │         └─→ Внешнее API
       │
       ├─→ При необходимости: save_draft_material (черновик KB, HITL)
       │
       ▼
generate_report_section(...) по секциям: Summary, Errors, Strengths, Plan, Materials
       │
       ▼
validate_report(report_draft, evaluations) → при FAIL: доработка (лимит итераций)
       │
       ▼
Results-crud-service: отчёт + presets / user_topic_progress (по session_kind)
                 ┌────────────────────────────────────────────────────────┐
                 │ PostgreSQL: results_crud_db                            │
                 │ Таблицы: reports, presets, user_topic_progress         │
                 └────────────────────────────────────────────────────────┘
       │
       ▼
BFF → Frontend: /results (рекомендации тренировок / study)
```

## Что хранится (Data Storage)


| Хранилище                           | Данные                          | Схема                                   | Срок хранения                         |
| ----------------------------------- | ------------------------------- | --------------------------------------- | ------------------------------------- |
| Redis                               | Активные сессии                 | Key-value: session:{id} → {state}       | 2 часа (TTL)                          |
| PostgreSQL (user_crud_db)           | Пользователи, хеши паролей, recovery keys | Реляционная + индексы         | Бессрочно (пока пользователь активен) |
| PostgreSQL (session_crud_db)        | Сессии, программы интервью      | JSONB для program                       | 1 год                                 |
| PostgreSQL (chat_crud_db)           | Сообщения чатов                 | JSONB для content                       | 1 год                                 |
| PostgreSQL (results_crud_db)        | Оценки, отчёты, preset_training | JSONB для evaluations, report, пресетов | Бессрочно (история пользователя)      |
| PostgreSQL (questions_crud_db)      | База вопросов                   | JSONB для content, criteria             | Бессрочно (версионируется)            |
| PostgreSQL (knowledge_base_crud_db) | База знаний                     | JSONB для description, formulas         | Бессрочно (версионируется)            |


## Что логируется (Logging)


| Уровень           | Что логируется                                         | Куда                           | PII                          |
| ----------------- | ------------------------------------------------------ | ------------------------------ | ---------------------------- |
| Frontend          | Действия пользователя (клики, переходы)                | Browser console → Grafana Loki | Нет                          |
| BFF               | HTTP запросы (метод, путь, статус, длительность)       | Structured JSON logs → Loki    | Нет (session_id, не user_id) |
| Services          | Бизнес-события (создана сессия, сохранены результаты)  | Structured JSON logs → Loki    | Нет                          |
| Interviewer-agent-service     | Tool calls (название, параметры без PII, длительность) | Structured JSON logs → Loki    | Нет                          |
| Interviewer-agent-service     | LLM вызовы (модель, токены input/output, стоимость)    | Metrics → Prometheus           | Нет                          |
| **Не логируется** | Текст сообщений пользователя и агента в прод-логах     | -                              | -                            |


## Метрики (Observability)


| Метрика                        | Тип       | Агрегация                        | Дашборд          |
| ------------------------------ | --------- | -------------------------------- | ---------------- |
| http_requests_total            | Counter   | По сервисам, методам, статусам   | BFF Overview     |
| http_request_duration_seconds  | Histogram | P50, P95, P99                    | Performance      |
| kafka_messages_processed_total | Counter   | По топикам, агентам              | Agent Metrics    |
| llm_tokens_used_total          | Counter   | По моделям, типам (input/output) | Cost Tracking    |
| llm_request_duration_seconds   | Histogram | P50, P95                         | LLM Performance  |
| tool_calls_total               | Counter   | По инструментам, успех/ошибка    | Agent Tools      |
| interview_completion_rate      | Gauge     | Доля завершённых сессий          | Business Metrics |
| active_sessions                | Gauge     | Текущие активные интервью        | Real-time        |


