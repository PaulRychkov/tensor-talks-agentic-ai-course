# C4 Component Diagram

## Уровень 3: компоненты трёх LangGraph-агентов

В `docs/system-design.md` **этапы 2–4** реализуют три отдельных агента (контейнеры):


| Этап | Контейнер                       | Роль                                                        |
| ---- | ------------------------------- | ----------------------------------------------------------- |
| 2    | **Interview-builder-service**   | Агент-планировщик: программа интервью / тренировки / study  |
| 3    | **Interviewer-agent-service**   | Агент-интервьюер: диалог и оценка с confidence-aware routing (порт 8093, ранее `agent-service`) |
| 4    | **Analyst-agent-service**       | Агент-аналитик: отчёт, `validate_report`, `preset_training` (порт 8094) |


**Knowledge-producer-service** (этап 0) — **LLM-workflow**, не граф LangGraph; компоненты этого сервиса здесь не детализируются (см. `docs/specs/knowledge-producer-service.md`).

Ниже для каждого из трёх агентов: схема компонентов, state, узлы графа, **tools** и Kafka — в соответствии с `docs/system-design.md` и `docs/specs/`*.

---

## 1. Interview-builder-service (агент-планировщик, этап 2)

**Технологии**: FastAPI, LangChain, LangGraph. **Вход / выход Kafka**: consume `interview.build.request`, publish `interview.build.response`.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  Interview-builder-service                              │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ LangGraph State (черновик программы, session_type, ретраи)      │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Подграф Planner: parse_mode → search_* → form_draft →           │    │
│  │ validate_program ↔ check_topic_coverage (цикл при сбое)         │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Tool Layer                                                      │    │
│  │  search_questions(specialty?, grade?, topics?, difficulty?,     │    │
│  │    type?, related_to?)                                          │    │
│  │  check_topic_coverage(selected, required)                       │    │
│  │  validate_program(questions)                                    │    │
│  │  get_topic_relations(topic)                                     │    │
│  │  search_knowledge_base(query, topic)  — режимы B/C и теория     │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Guardrails: JSON Schema на program; лимиты tool calls / LLM     │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Kafka publish: interview.build.response                         │    │
│  │ HTTP-клиенты: Questions-crud, Knowledge-base-crud, Session-crud │    │
│  └─────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
```

**Режимы** (из system-design): **A** интервью (`specialty`, `grade`, `use_previous_results`), **B** тренировка (пресет или ручной `topic`/`level`), **C** study (рекомендации или ручной `topic`/`level`).

**Переходы (ориентир)**:

```
interview.build.request → parse_mode → tools → form_draft
→ validate_program → [FAIL] → adjust → validate_program
→ check_topic_coverage → [gaps] → supplement → validate_program
→ [OK] → interview.build.response → END
```

---

## 2. Interviewer-agent-service — Interviewer (этап 3)

**Технологии**: LangChain, LangGraph, Redis. **Kafka**: consume `messages.full.data` (от dialogue-aggregator), publish `generated.phrases` и по завершении всех n вопросов — `session.completed`.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  Interviewer-agent-service (Interviewer)                            │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ LangGraph State = InterviewState (system-design)                │    │
│  │  session_id, program, current_question_index, evaluations, …    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Подграф: formulate_question → evaluate_answer → decide_action   │    │
│  │  (полный / частичный / off-topic / «не знаю» / пропуск)         │    │
│  │  при частичном: retrieval; при необходимости web_search+fetch_url    │
│  │  summarize_dialogue при бюджете контекста                       │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Tool Layer                                                      │    │
│  │  evaluate_answer(question, answer, theory)                      │    │
│  │  search_knowledge_base(query, topic)                            │    │
│  │  search_questions(related_to=current_question)                  │    │
│  │  summarize_dialogue(messages)                                   │    │
│  │  web_search(query, num_results)  fetch_url(url)                 │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Guardrails: pre-call (PII 3-level: regex→LLM→sanitize,          │    │
│  │  injection strip), post-call (Pydantic, leakage detection)      │    │
│  │ Structured output: AnswerEvaluation с decision_confidence,      │    │
│  │  OffTopicClassification, PIICheckResult                         │    │
│  │ Episodic memory: get_user_history, previous_topic_scores        │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Kafka: generated.phrases; session.completed (финал)    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
```

**Поведение** (сжато из этапа 3): программа и теория из БД уже в state; подсказки генерирует LLM. В режиме тренировки — углублённое сопровождение (prompt policy, см. `docs/specs/interviewer-agent.md`).

---

## 3. Analyst-agent-service (этап 4)

**Технологии**: LangChain, LangGraph (порт 8094). **Kafka**: consume `session.completed`. **Персистентность**: код сервиса → **Results-crud-service** (отчёт + `presets` + `user_topic_progress`), не как LLM-tool. Маршрутизация по `session_kind`: interview → отчёт + presets, training → результаты, study → user_topic_progress.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  Analyst-agent-service                                  │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ LangGraph State: черновик отчёта, итерации validate_report, …   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Подграф: load → get_evaluations → (опц.) уточнение оценок (LLM) │    │
│  │  → group_errors_by_topic → retrieve_materials                   │    │
│  │  → generate_report_section* → validate_report                   │    │
│  │  → [VALIDATION_FAILED] → доработка → … (лимит итераций)         │    │
│  │  → persist Results-crud → chat.events.in                        │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Tool Layer                                                      │    │
│  │  get_evaluations(session_id)                                    │    │
│  │  group_errors_by_topic(evaluations)                             │    │
│  │  search_knowledge_base(query, topic)                            │    │
│  │  web_search   fetch_url                                         │    │
│  │  generate_report_section(section_type, data)                    │    │
│  │  validate_report(report_draft, evaluations)                     │    │
│  │  save_draft_material(…) — только черновик KB (HITL)             │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Guardrails: JSON Schema отчёта; лимиты tool calls               │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │ Results-crud (HTTP): report JSON, preset_training               │    │
│  │ Kafka: chat.events.in                                           │    │
│  └─────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
```

**Переходы (ориентир)**:

```
session.completed → load_state → get_evaluations
→ group_errors_by_topic → retrieve_materials (KB → web_search → fetch_url)
→ generate_report_section (Summary, Errors, Strengths, Plan, Materials)
→ validate_report → [FAIL] → доработка секций / материалов → validate_report
→ [OK] → Results-crud → chat.events.in → END
```

---

## Сводная таблица tools по агентам

Соответствует таблице «Описание tool/API-интеграций» в `docs/system-design.md`.


| Инструмент                 | Планировщик | Interviewer       | Analyst |
| -------------------------- | ----------- | ----------------- | ------- |
| `search_questions(…)`      | да          | да (`related_to`) | нет     |
| `check_topic_coverage`     | да          | нет               | нет     |
| `validate_program`         | да          | нет               | нет     |
| `get_topic_relations`      | да          | нет               | нет     |
| `search_knowledge_base`    | да          | да                | да      |
| `evaluate_answer`          | нет         | да                | нет     |
| `summarize_dialogue`       | нет         | да                | нет     |
| `get_evaluations`          | нет         | нет               | да      |
| `group_errors_by_topic`    | нет         | нет               | да      |
| `generate_report_section`  | нет         | нет               | да      |
| `validate_report`          | нет         | нет               | да      |
| `save_draft_material`      | нет         | нет               | да      |
| `web_search` / `fetch_url` | нет         | да                | да      |


**Вне системы LLM-tools**: запись отчёта и `preset_training` — клиент **Results-crud-service** в коде Analyst после успешной `validate_report`.

---

## Kafka (логические имена топиков)


| Сервис                  | Consume                       | Publish                                              |
| ----------------------- | ----------------------------- | ---------------------------------------------------- |
| Interview-builder       | `interview.build.request`     | `interview.build.response`                           |
| Interviewer-agent-service           | `messages.full.data`          | `generated.phrases`, `session.completed`             |
| Analyst-agent-service   | `session.completed`           | результаты → Results-crud (HTTP)                     |
| Dialogue-aggregator     | `chat.events.out`, `generated.phrases` | `messages.full.data`, `chat.events.in`, `session.completed` |


Префикс кластера (например `tensor-talks-`) — по реализации. Partition key обычно `session_id`.

---

## Guardrails и лимиты

Общие правила: `docs/system-design.md` (Pre-call, Post-call, max tool calls на шаг / на вопрос, бюджет токенов, таймауты). Детали по сервисам: `docs/specs/interview-builder-service.md`, `interviewer-agent.md`, `analyst-agent.md`.