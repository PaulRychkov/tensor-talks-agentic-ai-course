# Системный дизайн TensorTalks

## Обзор

Этот документ описывает архитектуру PoC-версии агентной системы TensorTalks - платформы для проведения и оценки технических интервью по машинному обучению. Документ фиксирует состав модулей, их взаимодействие, execution flow, контракты, ограничения и контрольные точки.

## Ключевые архитектурные решения

### 1. Гибридная архитектура: детерминированные сервисы + агенты

Система разделена на две части:

**Детерминированные сервисы (Go + Python)** - надёжные, предсказуемые компоненты:
- Управление пользователями и аутентификация
- CRUD-операции с данными
- Оркестрация сессий
- Маршрутизация событий
- Валидация схем и лимиты

**Агентная система (Python + LangGraph)** — автономные агенты с инструментами (планировщик, интервьюер, аналитик). Отдельно: **knowledge-producer-service** — фиксированный LLM-workflow наполнения базы (этап 0), без графа принятия решений как у агента.

- Агент-планировщик (interview-builder-service)
- Агент-интервьюер (interviewer-agent-service, порт 8093)
- Агент-аналитик (analyst-agent-service, порт 8094)
- LLM-workflow наполнения базы (knowledge-producer-service) — только этап 0

**Обоснование**: агенты потребляют в ~4 раза больше токенов и менее предсказуемы. Критичная инфраструктура (аутентификация, хранение данных) должна быть стабильной. Агентное поведение добавляем только там, где нужна автономность и работа с неструктурированными данными.

### 2. Асинхронное взаимодействие через Kafka

Агенты не вызывают CRUD-сервисы напрямую. Взаимодействие через Kafka-топики:
- `chat.events.out` — сообщения от пользователя (BFF → Dialogue-aggregator)
- `chat.events.in` — ответы агента (Dialogue-aggregator → BFF)
- `messages.full.data` — полные сообщения для агента (Dialogue-aggregator → Interviewer-agent-service)
- `generated.phrases` — реплики агента (Interviewer-agent-service → Dialogue-aggregator)
- `interview.build.request` / `interview.build.response` — формирование программы
- `session.completed` — сигнал завершения сессии для запуска агента-аналитика (Dialogue-aggregator → Analyst-agent-service)

**Обоснование**: буферизация при пиковых нагрузках, устойчивость к временной недоступности LLM, возможность масштабировать агентов независимо.

### 3. State & Memory через Redis + LangGraph

Состояние сессии хранится в Redis (кеширование активных интервью) и передаётся в LangGraph State для агентов.

**Обоснование**: быстрый доступ к контексту диалога, персистентность между шагами агента, возможность восстановления после сбоев.

### 4. Инструменты с typed schemas

Все инструменты агентов имеют строгие JSON Schema:
- Типизированные параметры (enums вместо строк)
- Валидация до вызова
- Информативные ошибки
- Лимиты на вызовы

**Обоснование**: снижение галлюцинаций, предсказуемость вызовов, безопасность.

### 5. Human-in-the-loop для write-операций

Агент не может напрямую изменять базу знаний. Write-инструмент `save_draft_material` создаёт черновик, который проверяет человек.

**Обоснование**: контроль качества базы знаний, защита от галлюцинаций и некорректных данных.

## Список модулей и их роли

**AI сервисы:**
| Модуль | Технология | Роль |
|--------|------------|------|
| Interviewer-agent-service | Python (LangGraph, ReAct) | Агент-интервьюер: ведение диалога, принятие решений, confidence-aware routing (порт 8093) |
| Analyst-agent-service | Python (LangGraph, ReAct) | Агент-аналитик: отчёт, пресеты и рекомендации (порт 8094) |
| Interview-builder-service | Python (FastAPI + LangGraph) | Агент-планировщик: формирование программы интервью и тренировок |
| Knowledge-producer-service | Python (FastAPI + LLM) | LLM-workflow (не агент): наполнение базы, этап 0 |

**Детерминированные сервисы:**
| Модуль | Технология | Роль |
|--------|------------|------|
| Frontend | React + TypeScript | Пользовательский интерфейс (интервью, тренировки, study, результаты) |
| Admin-frontend | React + TypeScript | Панель администратора (база знаний, черновики, метрики) |
| BFF | Go (Gin) | Backend-for-frontend, агрегация запросов |
| Admin-BFF | Go (Gin) | Backend-for-frontend для admin-панели |
| Auth-service | Go | Аутентификация, JWT-токены |
| User-crud-service | Go + PostgreSQL | Хранение данных пользователей |
| Session-service | Go + Redis | Управление сессиями, кеширование |
| Session-crud-service | Go + PostgreSQL | Персистентное хранение сессий |
| Knowledge-base-crud-service | Go + PostgreSQL | CRUD базы знаний |
| Questions-crud-service | Go + PostgreSQL | CRUD базы вопросов |
| Chat-crud-service | Go + PostgreSQL | Хранение сообщений чата |
| Results-crud-service | Go + PostgreSQL | Хранение результатов |
| Dialogue-aggregator | Go | Оркестрация Kafka-сообщений между BFF и агентами (порт 8088) |
| Kafka | Confluent (Zookeeper в docker-compose, KRaft в K8s) | Брокер сообщений |

## Основной workflow выполнения запроса

**Роли этапов**: этап 0 — только LLM-workflow (knowledge-producer-service), не многошаговый LangGraph-агент. Этап 1 — детерминированная оркестрация (BFF, Session-service, Redis, Kafka), без LLM. Этапы 2–4 — LangGraph-агенты; обмен с пользователем и оркестратором в основном через Kafka (`interview.build.*`, `chat.events.*`) и персистентность через CRUD-сервисы.

### Этап 0: Наполнение базы знаний (LLM-workflow, фоновый процесс)

```
1. Knowledge-producer-service получает тему и уровень сложности (вход workflow)
2. web_search(query, num_results) — поиск по ML-ресурсам (документация, статьи, учебники)
3. fetch_url(url) — загрузка страниц и извлечение текста для последующей обработки
4. Ручной источник (вне тула агента): контент, загруженный оператором через UI/API, попадает в тот же шаг обработки, что и результат fetch_url
5. LLM обрабатывает собранное:
   - Извлекает ключевые концепции
   - Формулирует вопросы (теоретические, практические)
   - Структурирует теорию (описание, формулы, примеры)
   - Приводит к формату базы знаний
6. Проверка на дубликаты:
   - search_knowledge_base(query, topic) — поиск близких концепций в Knowledge-base-crud-service
   - search_questions(topics, difficulty, type) или поиск по текстовой близости формулировки — поиск близких вопросов в Questions-crud-service
7. save_draft_material(title, content, topic, type) — черновик (human-in-the-loop до публикации)
8. Оператор проверяет и подтверждает → публикация в базу через admin UI и CRUD, не автономной записью write-агента
```

### Этап 1: Инициализация сессии

```
1. Пользователь выбирает параметры (specialty, grade, use_previous_results)
2. Frontend → BFF: POST /api/sessions
3. BFF → Session-service: создать сессию
4. Session-service → Redis: кешировать сессию
5. Session-service → Kafka: interview.build.request
6. Session-service → Session-crud-service: сохранить сессию (метаданные, статус ожидания программы; сама программа фиксируется после этапа 2)
7. Session-service → BFF: сессия создана, клиент ожидает interview.build.response
```

### Этап 2: Формирование программы (агент-планировщик)

**Режим A: Интервью (оценка с грейдом/специальностью)**
```
1. Пользователь выбирает:
   - specialty: NLP, LLM, Classic ML, Computer Vision, Data Science
   - grade: junior/middle/senior или "auto" (система определит)
   - use_previous_results: true/false
2. Interview-builder-service получает параметры из Kafka (interview.build.request)
3. Формирование программы (агент-планировщик с инструментами):
   - search_questions(specialty, grade) — выборка из Questions-crud-service
   - check_topic_coverage(selected_questions, required_topics) — проверка покрытия
   - validate_program(questions) — валидация (дубли, баланс, прогрессия)
   - get_topic_relations(topic) — связи тем из Knowledge-base-crud-service
4. Формирование программы из n вопросов (вопрос + теория из БД; подсказки при диалоге генерирует интервьюер)
5. Interview-builder-service → Kafka: interview.build.response
6. Session-crud-service: сохранить программу
```

**Режим B: Тренировка**

Основной сценарий: программа тренировки опирается на пресеты, которые агент-аналитик создаёт после интервью (слабые темы, материалы). Пользователь запускает тренировку из экрана результатов, без повторного обращения к планировщику «с нуля».

Дополнительная опция: пользователь вручную задаёт тему и уровень — тогда планировщик строит программу как отдельный запуск (не по пресету последнего интервью).

```
1. Источник запроса (одно из):
   - пресет после интервью (topic/level/вопросы выводятся из preset_training и связанных данных);
   - ручной выбор: topic (например, "Neural Networks"), level (junior/middle/senior)
2. Interview-builder-service получает параметры из Kafka (interview.build.request)
3. Формирование программы тренировки (агент-планировщик с инструментами):
   - search_questions(topics=[topic], difficulty=level, type="practice") — выборка из Questions-crud-service
   - search_knowledge_base(query=topic, topic=topic) — загрузка теории из Knowledge-base-crud-service
   - check_topic_coverage(selected_questions, required_topics=[topic]) — проверка покрытия темы
   - validate_program(questions) — валидация (дубли, баланс, прогрессия)
4. Формирование программы из n вопросов (вопрос + теория из БД)
5. Interview-builder-service → Kafka: interview.build.response
6. Session-crud-service: сохранить программу
```

**Режим C: Study session (изучение темы)**

Основной сценарий: план изучения предлагается из отчёта после интервью (пробелы по темам, рекомендованные материалы). Пользователь открывает учебную сессию из рекомендаций.

Дополнительная опция: пользователь сам задаёт topic и level — план строится без привязки к только что завершённому интервью.

```
1. Источник запроса (одно из):
   - рекомендации после интервью (темы и уровень из отчёта / связки с preset_training);
   - ручной выбор: topic (например, "Backpropagation"), level (beginner/intermediate/advanced)
2. Interview-builder-service получает параметры из Kafka (interview.build.request)
3. Формирование учебного плана (агент-планировщик с инструментами):
   - search_knowledge_base(query=topic, topic=topic) — загрузка материалов из Knowledge-base-crud-service
   - get_topic_relations(topic) — связанные темы из Knowledge-base-crud-service
   - search_questions(topics=[topic], difficulty=level, type="theory") — контрольные вопросы из Questions-crud-service
   - validate_program(questions) — валидация учебного плана
4. Формирование учебного плана (теория + примеры + контрольные вопросы из БД)
5. Interview-builder-service → Kafka: interview.build.response
6. Session-crud-service: сохранить учебный план
```

### Этап 3: Проведение интервью (агент-интервьюер)

**Примечание**: Агент уже получил от планировщика программу (n вопросов + теория из БД). Реализован как ReAct-агент на LangGraph — LLM самостоятельно выбирает инструменты через tool-calling, а не по фиксированной цепочке. Терминальный инструмент `emit_response` завершает ReAct-цикл.

**LangGraph flow**: `receive_message → check_pii → load_context → check_off_topic → determine_question_index → agent_init → [call_agent_llm ↔ execute_tools] → finalize_response → publish_response`

```
1. BFF → Kafka: chat.events.out (сообщение пользователя)
2. Dialogue-aggregator → Kafka: messages.full.data (полное сообщение для агента)
3. Interviewer-agent-service читает messages.full.data
4. Pre-processing nodes:
   a. check_pii — PII-фильтрация (Level 1: regex — email, телефон, ИНН, СНИЛС, паспорт, компании из blacklist; Level 2: LLM-классификация неявных данных; Level 3: санитизация prompt injection). При обнаружении PII → блокировка, маскирование в chat-crud, объяснение пользователю
   b. load_context — загрузка из Redis (dialogue_history, per-question counters: attempts, hints_given, clarifications), программы из session-service, episodic memory (previous_topic_scores из results-crud)
   c. check_off_topic — LLM-классификация (on_topic / off_topic / comment) через Pydantic OffTopicClassification
   d. determine_question_index — определение текущего вопроса в истории
5. ReAct agent loop (agent_init → call_agent_llm ↔ execute_tools):
   - LLM видит 7 tool definitions и сам решает какой вызвать
   - evaluate_answer(question, answer, theory) → AnswerEvaluation (Pydantic: completeness_score, accuracy_score, overall_score, decision, decision_confidence, reasoning, missing_points, strong_points)
   - Confidence-aware routing: score ≥ 0.85 → next; score 0.75-0.85 + confidence ≥ 0.7 → next; score 0.75-0.85 + confidence < 0.7 → hint; confidence < 0.5 → self-reflection (переоценка)
   - search_knowledge_base, search_questions, web_search, fetch_url, summarize_dialogue — по необходимости
   - emit_response(action, message, rationale, score) — терминальный tool, завершает цикл
   - Лимит: max 6 итераций ReAct на один ход
6. Per-question counters: max 2 clarifications (interview), 3 (training), 1 (study); max 2 hints; max 3 attempts
7. finalize_response — sanity checks, advance question index
8. publish_response → Kafka: generated.phrases + Redis: persist state
9. Dialogue-aggregator → Kafka: chat.events.in
10. BFF → Frontend: доставить; BFF → Chat-crud-service: сохранить
11. Повторять до завершения всех n вопросов
```

### Этап 4: Формирование отчёта и создание пресетов (агент-аналитик)

**Примечание**: Завершение сессии объявляется через Kafka; Analyst-agent-service подписан на `session.completed` и запускает граф. Цикл не линейный: при `VALIDATION_FAILED` аналитик снова вызывает тулзы материалов и/или `generate_report_section`, затем `validate_report`, пока отчёт не станет валидным или не исчерпан лимит итераций (конфиг). Маршрутизация по `session_kind`: interview → полный отчёт + presets, training → результаты, study → user_topic_progress.

```
1. Dialogue-aggregator → Kafka: session.completed (session_id, session_kind, topics, level)
2. Analyst-agent-service читает Kafka, стартует LangGraph
3. LangGraph State: подтянуть session_id, параметры сессии, историю и оценки из Session-service / Chat-crud-service / Results-crud-service
4. Агент-аналитик (маршрутизация по session_kind):
   a. get_evaluations(session_id)
   b. При необходимости уточнить оценки по ответам (LLM, порядок «по ответам / целиком по сессии» не фиксирован)
   c. group_errors_by_topic(evaluations) — темы для секции Errors и для пресетов
   d. При необходимости: search_knowledge_base(query, topic); если мало внутренних материалов — web_search(query, num_results), затем fetch_url(url) для выбранных ссылок
   e. Новый контент в KB только через save_draft_material и подтверждение оператором (без автозаписи)
   f. generate_report_section(section_type, data) — Summary, Errors, Strengths, Plan, Materials (по секциям или пакетом, в рамках лимитов tool calls)
   g. validate_report(report_draft, evaluations) — JSON Schema, полнота секций, согласованность оценок и текста с group_errors_by_topic
   h. Если VALIDATION_FAILED — повторить (d)–(g) с доработкой проблемных секций; максимум итераций — конфиг
5. Сборка финального report JSON после успешной validate_report
6. Analyst-agent-service → Results-crud-service: сохранить отчёт и presets (weak_topics, recommended_materials; вопросы тренировки подбираются при старте этапа 2) и/или user_topic_progress
7. BFF → Frontend: перенаправление на /results (рекомендации тренировок и study)
```

**Примечание**: Тренировки и study session — отдельные сессии, запускаются через Этап 1 → Этап 2 (с session_type: training или study).

## Описание state / memory / context handling

### LangGraph State Structure

**Агент-интервьюер (AgentState)**:
```python
class AgentState(TypedDict):
    # Входные данные
    chat_id: str
    session_id: str
    user_id: str
    user_message: str
    session_mode: str                         # interview | training | study

    # Программа и текущий вопрос
    interview_program: List[Dict]             # n вопросов с теорией
    current_question_index: int
    current_question: Optional[str]
    current_theory: Optional[str]

    # Per-question counters (persisted в Redis)
    attempts: int                             # 0..MAX_ATTEMPTS (3)
    hints_given: int                          # 0..MAX_HINTS (2)
    clarifications_in_question: int           # 0..MAX по режиму (1/2/3)

    # Накопленные оценки (Pydantic QuestionEvaluation)
    evaluations: List[QuestionEvaluation]

    # Episodic memory
    previous_topic_scores: Dict[str, float]   # средние scores по темам из прошлых сессий

    # ReAct transients
    _agent_messages: List[Dict]
    _agent_iterations: int                    # max 6
    _skip_agent_loop: bool
    _last_eval_score: Optional[float]

    # Контроль
    dialogue_history: List[Dict]
    error: Optional[str]
    pii_masked_content: Optional[str]
```

**Агент-аналитик (AnalystState)**:
```python
class AnalystState(TypedDict):
    session_id: str
    session_kind: str                         # interview | training | study
    user_id: str
    chat_id: str
    topics: List[str]
    level: str

    # Загруженные данные
    chat_messages: List[Dict]                 # Транскрипт чата
    program: Optional[Dict]                   # Программа сессии
    evaluations: List[Dict]                   # Per-question оценки

    # Промежуточный анализ
    errors_by_topic: Dict[str, List[str]]

    # Отчёт и валидация
    report: Optional[Dict]                    # AnalystReport (Pydantic)
    validation_result: Optional[Dict]
    validation_attempts: int                  # max 2

    # Финальные выходы
    presets: Optional[List[Dict]]             # TrainingPreset (для interview)
    topic_progress: Optional[List[Dict]]      # Для study

    # ReAct transients
    _agent_messages: List[Dict]
    _agent_iterations: int                    # max 10
    _skip_agent_loop: bool
```

**Агент-планировщик (PlannerState)**:
```python
class PlannerState(TypedDict):
    session_id: str
    mode: str                                 # interview | training | study
    topics: List[str]
    level: str
    weak_topics: List[str]
    n_questions: int

    messages: List[Dict]                      # LLM conversation (OpenAI format)
    candidate_questions: List[Dict]
    coverage_report: Optional[Dict]
    validation_report: Optional[Dict]
    knowledge_snippets: List[Dict]

    final_program: Optional[List[Dict]]
    program_meta: Optional[Dict]              # ProgramMeta (Pydantic)

    iteration: int                            # max 8
    error: Optional[str]
```

### Memory Policy

**Краткосрочная память (Redis)**:
- Активные сессии (TTL: 2 часа)
- Dialogue history и per-question counters (attempts, hints_given, clarifications_in_question)
- Текущий вопрос и метрики

**Долгосрочная память (PostgreSQL)**:
- Полная история интервью (chat-crud-service)
- Результаты и оценки (results-crud-service)
- История прогресса пользователя (user_topic_progress)
- Пресеты тренировок и study (presets)

**Episodic memory (персонализация)**:
- Интервьюер загружает `previous_topic_scores` из results-crud — средние оценки по темам из прошлых сессий пользователя, для учёта прогресса при выборе стратегии
- Планировщик вызывает `get_user_history(user_id, topics)` — слабые/сильные темы для адаптации программы
- Аналитик использует `ProgressDelta` (Pydantic: topic, previous_score, current_score, delta, assessment) — сравнение динамики между сессиями

**Context Budget**:
- Максимум 8000 токенов на сессию
- При превышении → summarize_dialogue
- Суммаризация вызывается агентом автономно при достижении 70% бюджета

### Retrieval-контура

**Источники**:
1. Внутренняя база знаний (PostgreSQL, ~500 концепций ML)
2. База вопросов (~100 вопросов, 5 уровней, 8 тем)
3. Внешний поиск (web_search через API)

**Механика**:
- Агент формулирует query на основе контекста вопроса
- Retrieval возвращает топ-3 релевантных элемента
- Агент использует как контекст для оценки/подсказки
- Для внешних материалов → фильтрация по доверенным источникам

## Structured output (Pydantic-модели)

Все ответы LLM парсятся в строго типизированные Pydantic-модели. JSON Schema из модели инжектируется в системный промпт. Парсинг через `model_validate_json` с fallback при невалидном JSON.

**Интервьюер:**
```python
class AnswerEvaluation(BaseModel):
    completeness_score: float      # [0.0, 1.0]
    accuracy_score: float          # [0.0, 1.0]
    overall_score: float           # [0.0, 1.0]
    decision: Literal["next", "hint", "clarify", "redirect", "skip"]
    missing_points: List[str]
    strong_points: List[str]
    feedback: str
    decision_confidence: float     # [0.0, 1.0] — уверенность в решении
    reasoning: str                 # обоснование уверенности

class OffTopicClassification(BaseModel):
    classification: Literal["on_topic", "off_topic", "comment"]
    reason: str

class PIICheckResult(BaseModel):
    contains_pii: bool
    reason: str
    masked_text: str
```

**Аналитик:**
```python
class AnalystReport(BaseModel):
    summary: str                   # min_length=10, 2-4 предложения
    score: int                     # [0, 100], auto-clamped
    errors_by_topic: Dict[str, List[Union[ErrorEntry, str]]]
    strengths: List[str]
    preparation_plan: List[str]    # 3-5 действий
    materials: List[str]           # 3-5 материалов

class TrainingPreset(BaseModel):
    target_mode: Literal["study", "training"]
    topic: str
    weak_topics: List[str]
    recommended_materials: List[str]
    priority: int                  # [1, 3]

class ProgressDelta(BaseModel):
    topic: str
    previous_score: float
    current_score: float
    delta: float
    assessment: Literal["improved", "same", "declined"]
```

**Планировщик:**
```python
class ProgramMeta(BaseModel):
    validation_passed: bool
    coverage: Dict[str, int]       # {topic: count}
    fallback_reason: Optional[str]
    generator_version: str
```

## Описание tool/API-интеграций

Ниже перечислены те же инструменты, что используются в workflow выше. Параметры — ориентир для JSON Schema; часть полей опциональна в зависимости от сценария.

### Внутренние инструменты (Read-only)

| Инструмент | Параметры | Возвращает | Ошибки | Где используется |
|------------|-----------|------------|--------|------------------|
| `search_questions(specialty?, grade?, topics?, difficulty?, type?, related_to?)` | отбор по специальности/грейду или по темам/сложности; `related_to` — привязка к текущему вопросу (интервьюер) | List[Question] | NO_QUESTIONS_FOUND | Планировщик, knowledge-producer (дедуп), интервьюер |
| `check_topic_coverage(selected, required)` | выбранные вопросы, требуемые темы | CoverageReport | — | Планировщик |
| `validate_program(questions)` | программа / список вопросов | ValidationReport | INVALID_PROGRAM | Планировщик |
| `get_topic_relations(topic)` | тема | List[RelatedTopic] | TOPIC_NOT_FOUND | Планировщик |
| `evaluate_answer(question, answer, theory)` | вопрос, ответ, теория | EvaluationJSON | EVALUATION_FAILED | Интервьюер |
| `search_knowledge_base(query, topic)` | запрос, тема | List[KnowledgeItem] | NO_MATERIALS_FOUND | Планировщик, интервьюер, аналитик, knowledge-producer |
| `get_evaluations(session_id)` | ID сессии | List[Evaluation] | SESSION_NOT_FOUND | Аналитик |
| `summarize_dialogue(messages)` | история | str (summary) | SUMMARIZATION_FAILED | Интервьюер |
| `group_errors_by_topic(evaluations)` | список оценок/ошибок | Map[topic, errors] | — | Аналитик |
| `generate_report_section(section_type, data)` | тип секции (Summary, Errors, Strengths, Plan, Materials), данные | SectionJSON | SECTION_GENERATION_FAILED | Аналитик |
| `validate_report(report_draft, evaluations)` | черновик отчёта, эталонные оценки | ValidationResult (ok / issues[]) | VALIDATION_FAILED | Аналитик |
| `get_user_history(user_id, topics?, limit?)` | user_id, темы, лимит | Dict (weak/strong topics, scores) | — | Планировщик (episodic memory) |

### Терминальные инструменты

| Инструмент | Параметры | Действие | Где используется |
|------------|-----------|----------|------------------|
| `emit_response(action, message, rationale, score)` | действие (next_question, ask_clarification, give_hint, thank_you, off_topic_reminder, skip), текст, обоснование, score | Завершает ReAct-цикл, устанавливает agent_decision и generated_response | Интервьюер |
| `emit_report(report)` | собранный отчёт (Dict) | Завершает ReAct-цикл, передаёт report в state | Аналитик |

### Внешние инструменты (Read-only)

| Инструмент | Параметры | Возвращает | Лимиты | Где используется |
|------------|-----------|------------|--------|------------------|
| `web_search(query, num_results=5)` | запрос, число результатов | List[SearchResult] | лимиты на сессию (интервьюер / аналитик / producer по конфигу) | Knowledge-producer, интервьюер, аналитик |
| `fetch_url(url)` | URL | str (parsed content) | лимиты на сессию | Knowledge-producer, интервьюер, аналитик |

### Write-инструменты

| Инструмент | Параметры | Требует подтверждения | Где используется |
|------------|-----------|------------------------|------------------|
| `save_draft_material(title, content, topic, type)` | заголовок, контент, тема, тип | Да (human-in-the-loop) | Knowledge-producer, аналитик (только предложение черновика, не прямая запись в KB) |

Запись отчёта и `preset_training` в PostgreSQL выполняется сервисом аналитика через контракт Results-crud-service (не как публичный tool LLM), после успешной `validate_report`.

### Контракты API (BFF ↔ Services)

```yaml
POST /api/chat/start:
  request: { topics: string[], level: string, type: string, mode: string }
  response: { session_id: string }

POST /api/chat/message:
  request: { session_id: string, content: string }
  response: { status: string }

GET /api/chat/history/{session_id}:
  response: { messages: Message[] }

POST /api/chat/terminate:
  request: { session_id: string }
  response: { status: string }

POST /api/chat/resume:
  request: { session_id: string }
  response: { status: string }

GET /api/results/sessions/{session_id}:
  response: { score: number, evaluations: Evaluation[], report_json: ReportJSON }
```

## Основные failure modes, fallback и guardrails

### Failure Modes

| Сценарий | Детект | Fallback |
|----------|--------|----------|
| LLM таймаут (>30s) | TimeoutException | "Извините, возникла техническая задержка. Продолжаем..." + retry (max 2) |
| LLM невалидный JSON | JSONDecodeError | Шаблонный ответ: "Давайте продолжим..." + переход к следующему вопросу |
| Зацикливание (N > 5 tool calls на шаг) | Счётчик вызовов | Принудительный переход к следующему действию |
| web_search нерелевантный | Оценка релевантности агентом (<0.5) | Попытка с другим query (max 3), затем отказ |
| Нет материалов в KB | NO_MATERIALS_FOUND | Только web_search + пометка "внешний источник" |
| Перерасход токенов (>бюджет) | Context budget check | Суммаризация + предупреждение пользователю |
| Отказ Kafka | ConnectionError | Буферизация в памяти, retry с экспоненциальной задержкой |

### Guardrails

**Pre-call (PII-фильтрация, 152-ФЗ)**:
- **Level 1 (regex, hard block)**: email, телефон, банковская карта, ИНН, СНИЛС, паспорт, самоидентификация ("меня зовут..."), компании из blacklist (Сбер, Яндекс, Google и др. из company_blacklist.json). При обнаружении → `agent_decision = "blocked_pii"`, маскирование в chat-crud-service
- **Level 2 (LLM, soft block)**: классификация неявных PII (непрямые упоминания работодателя, даты). Temp=0, structured output PIICheckResult (Pydantic)
- **Level 3 (sanitize, non-blocking)**: truncation (max 4000 chars), strip prompt injection markers (`<|...|>`, `[INST]`, `<<SYS>>`), residual PII masking

**Post-call**:
- Валидация формата через Pydantic-модели (`AnswerEvaluation`, `AnalystReport`, `OffTopicClassification`, `ProgramMeta`)
- Range checks: score в [0.0, 1.0], decision_confidence в [0.0, 1.0]
- Leakage detection: проверка на раскрытие системного промпта в ответе
- Tone sanitization: замена агрессивных формулировок на нейтральные
- Fallback при невалидном JSON: retry с structured prompt → deterministic fallback

**Лимиты**:
- Max 5 tool calls на шаг агента (интервьюер), max 6 итераций ReAct
- Max 10 итераций ReAct (аналитик), max 8 итераций (планировщик)
- Таймаут обработки: 120s (интервьюер), 180s (аналитик)
- URL fetch: только trusted domains (arxiv.org, pytorch.org, huggingface.co и др.), max 5000 chars

## Технические и операционные ограничения

### Latency

| Метрика | Target | P95 | Максимум |
|---------|--------|-----|----------|
| Время ответа агента | <5s | <8s | <10s |
| Формирование программы | <3s | <5s | <10s |
| Суммаризация диалога | <2s | <3s | <5s |
| Генерация отчёта | <10s | <15s | <20s |

### Cost

| Компонент | Токены на интервью | Стоимость (GPT-5.4-mini) |
|-----------|-------------------|-------------------------|
| Агент-интервьюер | ~3000 input + ~1500 output | ~$0.015 |
| Агент-аналитик | ~2000 input + ~1000 output | ~$0.010 |
| Суммаризация | ~1000 input + ~300 output | ~$0.003 |
| **Итого** | **~6000 токенов** | **~$0.03** |

### Reliability

- Доступность: 99% (целевое)
- Успешное завершение сессии: >90%
- Сохранность данных при сбоях: 100% (персистентное хранилище)

### Scalability

- Горизонтальное масштабирование агентов: до 10 реплик
- Пропускная способность Kafka: до 1000 сообщений/сек
- Одновременных интервью: до 100 (при 1 реплике агента)

## Диаграммы

Диаграммы расположены в `docs/diagrams/`:

1. **C4 Context** — система, пользователь, внешние сервисы и границы
2. **C4 Container** — frontend/backend, orchestrator, retriever, tool layer, storage, observability
3. **C4 Component** — внутреннее устройство interviewer-agent-service
4. **Workflow Diagram** — пошаговое выполнение запроса, включая ветки ошибок
5. **Data Flow Diagram** — как данные проходят через систему, что хранится, что логируется
