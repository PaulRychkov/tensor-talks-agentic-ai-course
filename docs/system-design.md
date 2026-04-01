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
- Агент-интервьюер (Agent-service, Interviewer)
- Агент-аналитик (Agent-service, Analyst)
- LLM-workflow наполнения базы (knowledge-producer-service) — только этап 0

**Обоснование**: агенты потребляют в ~4 раза больше токенов и менее предсказуемы. Критичная инфраструктура (аутентификация, хранение данных) должна быть стабильной. Агентное поведение добавляем только там, где нужна автономность и работа с неструктурированными данными.

### 2. Асинхронное взаимодействие через Kafka

Агенты не вызывают CRUD-сервисы напрямую. Взаимодействие через Kafka-топики:
- `chat.events.out` — сообщения от пользователя
- `chat.events.in` — ответы агента (в том числе финальный отчёт)
- `interview.build.request` / `interview.build.response` — формирование программы
- `interview.session.completed` (или эквивалент в контракте) — сигнал завершения интервью для запуска агента-аналитика

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
| Agent-service (Interviewer) | Python (LangChain + LangGraph) | Агент-интервьюер: ведение диалога, принятие решений |
| Agent-service (Analyst) | Python (LangChain + LangGraph) | Агент-аналитик: отчёт и рекомендации |
| Interview-builder-service | Python (FastAPI + LangGraph) | Агент-планировщик: формирование программы интервью и тренировок |
| Knowledge-producer-service | Python (FastAPI + LLM) | LLM-workflow (не агент): наполнение базы, этап 0 |

**Детерминированные сервисы:**
| Модуль | Технология | Роль |
|--------|------------|------|
| Frontend | React + TypeScript | Пользовательский интерфейс |
| BFF | Go (Gin) | Backend-for-frontend, агрегация запросов |
| Auth-service | Go | Аутентификация, JWT-токены |
| User-store-service | Go + PostgreSQL | Хранение данных пользователей |
| Session-service | Go + Redis | Управление сессиями, кеширование |
| Session-crud-service | Go + PostgreSQL | Персистентное хранение сессий |
| Knowledge-base-crud-service | Go + PostgreSQL | CRUD базы знаний |
| Questions-crud-service | Go + PostgreSQL | CRUD базы вопросов |
| Chat-crud-service | Go + PostgreSQL | Хранение сообщений чата |
| Results-crud-service | Go + PostgreSQL | Хранение результатов |
| Kafka | Strimzi (KRaft) | Брокер сообщений |

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

**Примечание**: Агент уже получил от планировщика программу (5 вопросов + теория из БД). Инструменты используются только при необходимости.

```
1. BFF → Kafka: chat.events.out (сообщение пользователя)
2. Agent-service читает топик
3. LangGraph State: загрузить контекст сессии (программа уже в state)
4. Агент-интервьюер:
   a. Получить текущий вопрос из state (вопрос + теория уже загружены)
   b. Если первое сообщение — сформулировать вопрос
   c. Если ответ пользователя:
      - evaluate_answer(question, answer, theory) — предварительная оценка для решения
      - Принять решение:
        * score ≥ 0.8 → следующий вопрос
        * 0.4 ≤ score < 0.8 (только при необходимости):
          · search_knowledge_base(query, topic) — найти дополнительную теорию
          · search_questions(related_to=current_question) — найти подвопросы
          · Сформулировать уточняющий подвопрос или подсказку
        * Off-topic → вернуть к теме
        * "Не знаю" → подсказка на основе загруженной теории
        * Пропуск → записать в оценку
        * Свежий фреймворк (только если кандидат упомянул) → web_search(query) + fetch_url(url)
   d. Сгенерировать реплику
5. Agent-service → Kafka: chat.events.in
6. BFF → Frontend: доставить
7. BFF → Chat-crud-service: сохранить
8. Повторять до завершения всех n вопросов
```

### Этап 4: Формирование отчёта и создание пресетов (агент-аналитик)

**Примечание**: Завершение сессии объявляется через Kafka; Agent-service (Analyst) подписан на событие и запускает граф. Цикл не линейный: при `VALIDATION_FAILED` аналитик снова вызывает тулзы материалов и/или `generate_report_section`, затем `validate_report`, пока отчёт не станет валидным или не исчерпан лимит итераций (конфиг).

```
1. После последнего вопроса: Agent-service (Interviewer) → Kafka: событие завершения интервью (например `interview.session.completed` с session_id; точное имя топика — контракт реализации, в одном кластере с `chat.events.*`)
2. Agent-service (Analyst) читает Kafka, стартует LangGraph
3. LangGraph State: подтянуть session_id, программу, историю и оценки из Redis/state и при необходимости из Chat-crud-service / Results-crud-service
4. Агент-аналитик:
   a. get_evaluations(session_id)
   b. При необходимости уточнить оценки по ответам (LLM, порядок «по ответам / целиком по сессии» не фиксирован)
   c. group_errors_by_topic(evaluations) — темы для секции Errors и для пресетов
   d. При необходимости: search_knowledge_base(query, topic); если мало внутренних материалов — web_search(query, num_results), затем fetch_url(url) для выбранных ссылок
   e. Новый контент в KB только через save_draft_material и подтверждение оператором (без автозаписи)
   f. generate_report_section(section_type, data) — Summary, Errors, Strengths, Plan, Materials (по секциям или пакетом, в рамках лимитов tool calls)
   g. validate_report(report_draft, evaluations) — JSON Schema, полнота секций, согласованность оценок и текста с group_errors_by_topic
   h. Если VALIDATION_FAILED — повторить (d)–(g) с доработкой проблемных секций; максимум итераций — конфиг
5. Сборка финального report JSON после успешной validate_report
6. Agent-service (Analyst) → Results-crud-service: сохранить отчёт и preset_training (weak_topics, recommended_materials; вопросы тренировки подбираются при старте этапа 2)
7. Agent-service (Analyst) → Kafka: chat.events.in (отчёт пользователю)
8. BFF → Frontend: перенаправление на /results (рекомендации тренировок и study)
```

**Примечание**: Тренировки и study session — отдельные сессии, запускаются через Этап 1 → Этап 2 (с session_type: training или study).

## Описание state / memory / context handling

### LangGraph State Structure

```python
class InterviewState(TypedDict):
    session_id: str
    user_id: str
    program: InterviewProgram  # n вопросов с метаданными
    current_question_index: int
    attempts: int
    hints_given: int
    evaluations: List[AnswerEvaluation]  # JSON-оценки по вопросам
    dialogue_history: List[Message]  # полная история
    dialogue_summary: str  # сжатая история (если > N токенов)
    context_budget: int  # оставшиеся токены контекста
    interview_status: str  # active, completed, failed
```

### Memory Policy

**Краткосрочная память (Redis)**:
- Активные сессии (TTL: 2 часа)
- Последние 10 сообщений диалога
- Текущий вопрос и метрики (попытки, подсказки)

**Долгосрочная память (PostgreSQL)**:
- Полная история интервью (chat-crud-service)
- Результаты и оценки (results-crud-service)
- История прогресса пользователя
- preset_training (тренировки по результатам интервью)

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
POST /api/sessions:
  request: { specialty: string, grade: string, use_previous_results: boolean }
  response: { session_id: string, program: InterviewProgram }

GET /api/sessions/{id}:
  response: { status: string, current_question: Question, history: Message[] }

POST /api/sessions/{id}/message:
  request: { content: string }
  response: { agent_message: string, evaluation?: Evaluation, status: string }

GET /api/sessions/{id}/results:
  response: { score: number, evaluations: Evaluation[], report: ReportJSON }
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

**Pre-call**:
- Маскирование PII (email, имена, телефоны) перед отправкой в LLM — по умолчанию пользователь не должен это писать, только если сам ввёл зачем-то
- Детект prompt injection (ключевые паттерны: "ignore previous", "system prompt")
- Лимит длины входа (max 4000 токенов)

**Post-call**:
- Валидация формата (JSON Schema для оценок)
- Запрет раскрытия промпта (детект паттернов в ответе)
- Fallback при невалидном ответе

**Лимиты**:
- Max 3 tool calls на шаг агента
- Max 10 tool calls на вопрос
- Таймаут на каждый tool call: 10s
- Бюджет токенов на сессию: 8000

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
3. **C4 Component** — внутреннее устройство agent-service
4. **Workflow Diagram** — пошаговое выполнение запроса, включая ветки ошибок
5. **Data Flow Diagram** — как данные проходят через систему, что хранится, что логируется
