# Analyst-agent-service Specification

## Обзор

**Назначение**: Формирование структурированного отчёта по итогам сессии, подбор материалов, запись результатов и **preset_training** (рекомендации тренировок и study). LangGraph-агент с циклом валидации отчёта.

**Сервис**: `analyst-agent-service` (отдельный деплой, порт 8094). Выделен из agent-service в самостоятельный сервис.

**Технологии**: Python 3.11+, LangChain, LangGraph, Kafka.

**Модель**: GPT-5.4 (тяжёлый разбор при необходимости), GPT-5.4-mini (основной режим отчёта и секций), GPT-5.4-nano (вспомогательные шаги).

**Workflow**, контракты и топики Kafka описаны в `docs/system-design.md` (этап 4, таблица инструментов).

## Вход и интеграции

**Триггер**: сообщение в Kafka `session.completed` с `session_id`, `session_kind`, `topics`, `level` от dialogue-aggregator.

**Потребление данных**: LangGraph State, при необходимости **Chat-crud-service**, **Session-service** и **Results-crud-service** (история, промежуточные оценки, параметры сессии).

**Маршрутизация по session_kind**:
- `interview` → полный отчёт + `presets` (рекомендации тренировок)
- `training` → результаты тренировки
- `study` → прогресс по теме (`user_topic_progress`)

**Выход**:
- Персистентность: **Results-crud-service** — итоговый отчёт, **presets** (`weak_topics`, `recommended_materials`; вопросы тренировки не фиксируются, подбираются на этапе 2) и/или **user_topic_progress**.
- Пользователю: **Kafka** `chat.events.in` (текст/метаданные отчёта).

Запись в PostgreSQL выполняется кодом сервиса аналитика по контракту CRUD, не как публичный LLM-tool.

## Архитектура

**Компоненты**:
- LangGraph State — черновик отчёта, счётчик итераций валидации, ссылки на `session_id`
- Tool Layer — см. раздел «Инструменты»
- Guardrails — JSON Schema для секций и отчёта целиком, post-call проверки

## Инструменты (typed JSON Schema)

| Инструмент | Назначение |
|------------|------------|
| `get_evaluations(session_id)` | Промежуточные оценки по сессии |
| `group_errors_by_topic(evaluations)` | Группировка для секции Errors и для пресетов |
| `search_knowledge_base(query, topic)` | Внутренние материалы |
| `web_search(query, num_results)` | Внешние источники при нехватке KB |
| `fetch_url(url)` | Извлечение текста по выбранным ссылкам после поиска |
| `generate_report_section(section_type, data)` | Секции Summary, Errors, Strengths, Plan, Materials |
| `validate_report(report_draft, evaluations)` | Схема, полнота, согласованность с оценками и с `group_errors_by_topic` |
| `save_draft_material(title, content, topic, type)` | Только черновик в KB; публикация после HITL оператором |

## Шаги (Workflow)

1. **Kafka**: получить событие завершения сессии → старт графа.
2. **State**: подтянуть `session_id`, программу, историю, оценки (Redis/state, при необходимости CRUD).
3. **Агент**:
   - `get_evaluations(session_id)`
   - при необходимости уточнить оценки по ответам (LLM; порядок «по ответам / целиком» не фиксирован)
   - `group_errors_by_topic(evaluations)`
   - при необходимости: `search_knowledge_base`; при слабом покрытии — `web_search`, затем `fetch_url` для выбранных URL
   - предложить новый материал в KB: только `save_draft_material` + подтверждение оператором
   - `generate_report_section` для нужных секций (по одной или пакетом, в лимитах **tool calls** на шаг)
   - `validate_report(report_draft, evaluations)`
   - при **VALIDATION_FAILED**: повторить доработку материалов и/или секций и снова `validate_report` (максимум итераций — конфиг)
4. Сборка финального **report JSON** после успешной `validate_report`.
5. **Results-crud-service**: сохранить отчёт и `preset_training`.
6. **Kafka** `chat.events.in`: доставка отчёта на сторону клиента (BFF → Frontend).

## Выход (JSON-отчёт, ориентир)

```json
{
  "overall_score": 0.72,
  "summary": "...",
  "errors_by_topic": {"Deep Learning": ["ошибка 1", "ошибка 2"]},
  "strengths": ["сильная сторона 1", "сильная сторона 2"],
  "preparation_plan": [
    {"priority": "high", "topic": "Deep Learning", "action": "...", "estimated_hours": 4}
  ],
  "materials": [
    {"title": "...", "type": "internal", "url": "/knowledge/123", "relevance": "..."},
    {"title": "...", "type": "external", "url": "https://...", "relevance": "..."}
  ]
}
```

## Правила переходов (ориентир для графа)

```
[session.completed] → load_state → get_evaluations
→ (optional) refine_evaluations_llm
→ group_errors_by_topic
→ retrieve_materials (KB → при необходимости web_search → fetch_url)
→ generate_sections (итерации по секциям)
→ validate_report
→ [VALIDATION_FAILED] → retrieve_materials / generate_sections (частично) → validate_report
→ [OK] → persist_results_crud → publish_chat_events_in → END
```

## Retry / Fallback

| Сценарий | Retry | Fallback |
|----------|-------|----------|
| LLM таймаут (>30s) | 2 попытки | Шаблонный отчёт с базовой структурой и флагом `requires_manual_review` |
| Невалидный JSON секции | 1–2 попытки | Повтор `generate_report_section` для проблемной секции |
| `validate_report` не проходит после max итераций | — | Сохранить последний черновик с `validation_issues`, алерт |
| web_search нерелевантный | до 3 запросов | Только внутренние материалы + пометка |
| Нет материалов в KB | — | Внешний поиск + пометка источника |

## Лимиты

- Лимиты `web_search` / `fetch_url` на одну сессию отчёта — по конфигу (согласовать с `system-design.md`: счётчики на сессию аналитика)
- Таймаут одного tool call: 10 с
- Max tool calls на шаг агента: 3 (как общий лимит системы)
- Целевое время генерации отчёта: см. `docs/system-design.md` (p95 до ~20 с)

## Метрики

- Report generation time (p95)
- Доля отчётов с успешной `validate_report` с первой попытки
- Число итераций validate → regenerate
- Materials relevance (LLM-as-judge), при наличии

## Serving / Config

**Запуск**: Docker (python-slim), Kubernetes, Helm, ArgoCD.

**Конфигурация**: `ENVIRONMENT`, `LOG_LEVEL`, `KAFKA_BOOTSTRAP_SERVERS`, `KAFKA_TOPIC_SESSION_COMPLETED`, `KAFKA_TOPIC_CHAT_IN`, `LLM_MODEL`, `LLM_MODEL_MINI`, `MAX_REPORT_VALIDATION_ITERATIONS`, URL/таймауты клиентов к Results-crud и Chat-crud.

**Секреты (Vault)**: `OPENAI_API_KEY`, ключи внешнего поиска при использовании.

**Версии моделей**: GPT-5.4 / GPT-5.4-mini / GPT-5.4-nano — назначение и сравнение по точности, задержке и стоимости (см. `docs/system-design.md`, подраздел Cost).

**Лимиты**: как в разделе «Лимиты» выше.

## Observability / Evals

**Метрики (Prometheus)**: `tool_calls_total`, `llm_tokens_used`, `llm_request_duration`, `report_generated_total`, `report_validation_failed_total`, `report_validation_iterations`.

**Логи (Loki)**: JSON, уровень INFO, PII маскируется.

**Трейсы (OpenTelemetry)**: LLM-вызовы, tool calls, публикация в Kafka, вызовы к CRUD.

**Evals**: полнота секций, согласованность отчёта с `evaluations`, релевантность материалов, сценарии с принудительным `VALIDATION_FAILED` и проверкой восстановления.
