# Interview-builder-service Specification (Planner Agent)

## Обзор

**Назначение**: Агент-планировщик (LangGraph): формирование **программы интервью**, **программы тренировки** или **учебного плана (study)** с покрытием тем, прогрессией и валидацией. Вход и выход — **Kafka**; персистентность программы — **Session-crud-service** после успешного ответа (см. `docs/system-design.md`).

**Технологии**: Python 3.11+, FastAPI (health/admin), LangChain, LangGraph, Kafka.

**Модель**: GPT-5.4-mini (основная для планировщика), при необходимости GPT-5.4-nano для лёгких шагов (конфиг).

**Workflow**, режимы и **tools** описаны в `docs/system-design.md` (этап 2, режимы A/B/C, таблица инструментов).

## Режимы

### Режим A: Интервью

Параметры от пользователя/сессии:
- `specialty`: NLP, LLM, Classic ML, Computer Vision, Data Science
- `grade`: junior / middle / senior или `auto`
- `use_previous_results`: учёт прошлых интервью только в части, релевантной текущей специальности и грейду

**Tools**: `search_questions(specialty, grade)`, `check_topic_coverage(selected_questions, required_topics)`, `validate_program(questions)`, `get_topic_relations(topic)`.

Результат: программа из **n** вопросов (в PoC часто 5): вопрос + **теория из БД**; подсказки в диалоге генерирует **интервьюер**, не БД.

### Режим B: Тренировка

- **Основной сценарий**: запрос из **пресета** после интервью (`preset_training`, слабые темы и материалы из **Results**); пользователь стартует тренировку с экрана результатов.
- **Опция**: ручной `topic` + `level` (junior/middle/senior) — отдельный запуск без опоры на последний пресет.

**Tools**: `search_questions(topics, difficulty, type="practice")`, `search_knowledge_base(query=topic, topic=topic)`, `check_topic_coverage(selected_questions, required_topics=[topic])`, `validate_program(questions)`.

### Режим C: Study session

- **Основной сценарий**: темы и уровень из **отчёта** / рекомендаций (связка с `preset_training` или секциями плана).
- **Опция**: ручной `topic` + `level` (beginner / intermediate / advanced).

**Tools**: `search_knowledge_base(query=topic, topic=topic)`, `get_topic_relations(topic)`, `search_questions(topics=[topic], difficulty=level, type="theory")`, `validate_program(questions)` (валидация учебного плана).

## Архитектура

**Компоненты**:
- LangGraph State — черновик программы, режим сессии, параметры запроса, счётчик ретраев валидации
- Planner Agent — выбор и корректировка программы
- Tool Layer — инструменты из `docs/system-design.md` (параметры — typed JSON Schema)

## Инструменты

| Инструмент | Применение |
|------------|------------|
| `search_questions(specialty?, grade?, topics?, difficulty?, type?, related_to?)` | Подбор вопросов под режим |
| `check_topic_coverage(selected, required)` | Покрытие тем |
| `validate_program(questions)` | Дубли, баланс, прогрессия |
| `get_topic_relations(topic)` | Связи тем из KB |
| `search_knowledge_base(query, topic)` | Теория для тренировки и study |

## Шаги (workflow)

1. Получить **`interview.build.request`** из Kafka (в payload: `session_id`, `session_type` / режим, поля режима A/B/C, при необходимости `preset_id` или ссылка на результаты).
2. Вызвать **tools** в соответствии с режимом; сформировать черновую программу (n вопросов или учебный план — по режиму).
3. **Валидация**: `validate_program`, `check_topic_coverage`; при сбое — скорректировать набор и повторить (лимит попыток).
4. Опубликовать **`interview.build.response`** в Kafka.
5. **Session-crud-service**: сохранить программу / учебный план (контракт реализации).

## Правила переходов (Agent)

```
[kafka request] → parse_mode → search_questions / search_knowledge_base / get_topic_relations
→ form_draft → validate_program → [fail] → adjust → validate_program
→ check_topic_coverage → [gaps] → supplement → validate_program
→ [ok] → publish interview.build.response → persist session-crud
```

## Stop Condition

- Программа прошла `validate_program` и `check_topic_coverage` (или зафиксирован fallback после лимита попыток)
- Таймаут SLA (см. `docs/system-design.md`, формирование программы)

## Retry / Fallback

| Сценарий | Retry | Fallback |
|----------|-------|----------|
| Questions-crud / KB-crud недоступны | 3 попытки | Кеш Redis, при отсутствии — урезанная программа + флаг |
| Недостаточно вопросов | — | Ослабить фильтры (level/topics), пометить `needs_review` |
| Валидация не проходит (> N попыток) | — | Черновик с `needs_review` |
| Kafka | экспоненциальный retry | Буферизация |
| LLM таймаут | 2 попытки | Детерминированная сборка из топ-N вопросов без LLM |

## Контракты Kafka

**Вход (`interview.build.request`)** :

```json
{
  "session_id": "uuid",
  "session_type": "interview | training | study",
  "specialty": "Classic ML",
  "grade": "middle",
  "use_previous_results": true,
  "topic": "Neural Networks",
  "level": "middle",
  "preset_id": "uuid-or-null",
  "source": "user_manual | post_interview_recommendation"
}
```

**Выход (`interview.build.response`)**:

```json
{
  "session_id": "uuid",
  "session_type": "interview | training | study",
  "program": {
    "questions": [
      {
        "id": "uuid",
        "topic": "Neural Networks",
        "difficulty": "junior",
        "type": "theory"
      }
    ],
    "total": 5,
    "validated": true,
    "coverage": {"Neural Networks": 1.0}
  }
}
```

## Лимиты

- Max retries валидации: 5 (конфигурируемо)
- Таймаут на построение программы: согласовать с системным дизайном (целевые p95)
- Max вопросов в программе: n

## Метрики

- Program build time (p95)
- Validation pass rate
- Распределение режимов A/B/C
- Agent fallback rate

## Serving / Config

**Запуск**: Docker (python-slim), Kubernetes, Helm, ArgoCD.

**Конфигурация**: `ENVIRONMENT`, `LOG_LEVEL`, `KAFKA_BOOTSTRAP_SERVERS`, `KAFKA_TOPIC_BUILD_REQUEST`, `KAFKA_TOPIC_BUILD_RESPONSE`, `LLM_MODEL`, URL клиентов к Session-crud, Questions-crud, Knowledge-base-crud.

**Секреты (Vault)**: `OPENAI_API_KEY`.

**Версии моделей**: GPT-5.4-mini (основная), GPT-5.4-nano (опционально).

**Лимиты**: как в разделе «Лимиты».

## Observability / Evals

**Метрики (Prometheus)**: `http_requests_total`, `kafka_messages_processed`, `validation_attempts_total`, `planner_mode_total` (по `session_type`).

**Логи (Loki)**: JSON, INFO.

**Трейсы (OpenTelemetry)**: Kafka, вызовы к CRUD, LLM.

**Evals**: покрытие тем, прогрессия сложности, валидность программы для каждого режима.
