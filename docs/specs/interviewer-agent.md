# Interviewer-agent-service Specification

## Обзор

**Назначение**: Ведение диалога (интервью / тренировка / сопровождение сценария по программе от планировщика), оценка ответов, принятие решений в реальном времени. Обмен с пользователем — через **Kafka** (потребляет `messages.full.data` от dialogue-aggregator, публикует `generated.phrases`). После последнего вопроса публикуется событие завершения сессии (`session.completed`) для **analyst-agent-service**.

**Сервис**: `interviewer-agent-service` (отдельный деплой, порт 8093; ранее `agent-service`). Реализован как **LangGraph ReAct-агент**. Аналитик выделен в отдельный сервис `analyst-agent-service` (порт 8094).

**Технологии**: Python 3.11+, LangChain, LangGraph, Kafka, Redis.

**Модель**: GPT-5.4-mini (основная), GPT-5.4-nano (суммаризация и лёгкие шаги — по конфигу).

**Workflow**  в `docs/system-design.md`, этап 3. Событие завершения сессии для аналитика и топики Kafka — этап 4 и соответствующий раздел того же документа.

## Вход и интеграции

- **Вход**: `messages.full.data` — сообщения пользователя (от dialogue-aggregator); программа сессии уже сформирована планировщиком (этап 2) и доступна в state.
- **Выход**: `generated.phrases` — реплики агента (в dialogue-aggregator → `chat.events.in`); по завершении всех n вопросов — **Kafka** `session.completed` с `session_id` для **analyst-agent-service**.
- Персистентность сообщений обеспечивают **BFF** и **Chat-crud-service** (см. `docs/system-design.md`), не через публичный **tool** у LLM.

## Архитектура

**Компоненты**:
- LangGraph State — программа, текущий вопрос, оценки, история, бюджет контекста
- Tool Layer — см. ниже
- **Structured output** (Pydantic-модели в `src/models/llm_outputs.py`): `AnswerEvaluation` (с `decision_confidence`), `OffTopicClassification`, `PIICheckResult` — LLM возвращает валидированные объекты, а не «сырой» JSON
- **Episodic memory**: перед стартом сессии и на релевантных шагах агент подтягивает `previous_topic_scores` из results-crud через tool `get_user_history` для адаптации сложности и подсказок
- Guardrails — pre-call (PII 3-уровневая фильтрация: regex → LLM → sanitize; strip prompt-injection; truncation) и post-call (Pydantic-валидация, range checks, leakage detection, tone)

## Инструменты

| Инструмент | Назначение |
|------------|------------|
| `evaluate_answer(question, answer, theory)` | Предварительная оценка (Pydantic `AnswerEvaluation` с `decision_confidence`) |
| `get_user_history(user_id, topic?)` | Эпизодическая память: прошлые оценки по темам из results-crud |
| `search_knowledge_base(query, topic)` | Дополнительная теория при частичном ответе |
| `search_questions(related_to=current_question)` | Подвопросы / уточнения |
| `web_search(query, num_results)` | Свежие фреймворки, если кандидат явно сослался |
| `fetch_url(url)` | Загрузка содержимого страницы после `web_search` при необходимости |
| `summarize_dialogue(messages)` | Укладка контекста при приближении к лимиту бюджета |

## Шаги (Workflow)

1. **Kafka**: прочитать `messages.full.data` (от dialogue-aggregator), обновить state.
2. **LangGraph State**: загрузить контекст сессии (программа уже в state).
3. **Агент-интервьюер**:
   - текущий вопрос и теория из программы (БД уже учтена планировщиком)
   - если первое сообщение сессии диалога — сформулировать вопрос
   - если ответ пользователя:
     - `evaluate_answer(question, answer, theory)` → Pydantic `AnswerEvaluation(score, decision_confidence, ...)`
     - решение по **score + decision_confidence**:
       - score ≥ 0.8 и `decision_confidence ≥ 0.7` → следующий вопрос
       - `decision_confidence < 0.5` → self-reflection (перепроверка оценки с доп. контекстом)
       - 0.4 ≤ score < 0.8 или `decision_confidence < 0.7` → подсказка: `search_knowledge_base`, `search_questions(related_to=...)`, уточняющий подвопрос
       - off-topic (Pydantic `OffTopicClassification`) → вернуть к теме
       - «Не знаю» → подсказка из уже загруженной теории
       - пропуск → зафиксировать в оценке
       - упоминание свежего фреймворка → `web_search` + при необходимости `fetch_url`
   - сгенерировать реплику
4. **Kafka**: `generated.phrases` с ответом (dialogue-aggregator доставит пользователю через `chat.events.in`).
5. Повторять до завершения всех **n** вопросов.
6. После последнего вопроса: **Kafka** `session.completed` (`session_id`, при необходимости метаданные сессии).

При **тренировочном** режиме (программа от планировщика с `type=practice`): более глубокое сопровождение — пояснение теории на шагах, адаптивные подсказки через **prompt policy** (отдельный список **tools** в `docs/system-design.md` не требуется).

## Правила переходов

```
[сообщение пользователя] → load_program
→ formulate_question | evaluate_answer
evaluate_answer → decide_action
decide_action → next_question | hint_branch | redirect | skip | web_search_branch
→ generate_reply → publish generated.phrases
→ [все вопросы заданы и обработаны] → publish session.completed → END
```

## Stop Condition

- Все n вопросов пройдены (индекс и статус в state)
- Запрос завершения от пользователя (если поддерживается продуктом)
- Таймаут сессии (например > 2 ч, TTL Redis)
- Превышен бюджет токенов (> 8000, см. `docs/system-design.md`)

## Retry / Fallback

| Сценарий | Retry | Fallback |
|----------|-------|----------|
| LLM таймаут (>30s) | 2 попытки | Шаблонная реплика + переход к следующему вопросу при повторе сбоя |
| Невалидный JSON оценки | 1 попытка | Оценка по умолчанию (например score 0.5) |
| Зацикливание (>5 tool calls на шаг) | — | Принудительное действие по политике графа |
| web_search нерелевантный | до 3 запросов | Только внутренние материалы |
| Отказ Kafka | экспоненциальный retry | Буферизация в памяти |

## Состояние (LangGraph State)

См. `docs/system-design.md`: `session_id`, `user_id`, `program`, `current_question_index`, `attempts`, `hints_given`, `evaluations`, `dialogue_history`, `dialogue_summary`, `context_budget`, `interview_status`.

**Context budget**: при ~70% расхода — `summarize_dialogue`.

## Лимиты

- Max tool calls на шаг: 3
- Max tool calls на вопрос: 10
- Таймаут tool call: 10 с
- Бюджет токенов: 8000 на сессию (`docs/system-design.md`)

## Метрики

- Доля корректных ветвлений (evaluate → действие)
- LLM response time (p95)
- Согласованность оценок с экспертом (eval)
- Loop detection rate

## Serving / Config

**Запуск**: Docker (python-slim), Kubernetes, Helm, ArgoCD.

**Конфигурация**: `ENVIRONMENT`, `LOG_LEVEL`, `KAFKA_BOOTSTRAP_SERVERS`, `KAFKA_TOPIC_MESSAGES_FULL_DATA`, `KAFKA_TOPIC_GENERATED_PHRASES`, `KAFKA_TOPIC_SESSION_COMPLETED`, `REDIS_HOST`, `LLM_MODEL`, `MAX_CONTEXT_BUDGET`.

**Секреты (Vault)**: `OPENAI_API_KEY`, ключи поиска при использовании `web_search`.

**Версии моделей**: GPT-5.4-mini, GPT-5.4-nano.

**Лимиты**: как в разделе «Лимиты».

## Observability / Evals

**Метрики (Prometheus)**: `tool_calls_total`, `llm_tokens_used`, `llm_request_duration`, `agent_errors`, `session_completed_total`.

**Логи (Loki)**: JSON, INFO; без сырого текста пользователя при политике приватности; PII маскируется.

**Трейсы (OpenTelemetry)**: LLM, tools, Kafka.

**Evals**: «не знаю», подсказка, пропуск, off-topic, полный/частичный ответ; для тренировки — наличие пояснений и траектории обучения.
