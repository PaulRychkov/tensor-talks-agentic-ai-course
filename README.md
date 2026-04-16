# TensorTalks

## Проект

TensorTalks — AI-симулятор технических ML-собеседований. Платформа моделирует реальные интервью с AI-интервьюером, анализирует ответы и выдаёт персонализированный разбор: сильные стороны, слабые места и рекомендации по подготовке.

Основная аудитория MVP — ML-инженеры уровня junior-middle, которые готовятся к интервью в продуктовые компании. Проблема: специалисты месяцами готовятся, но не понимают, где именно проваливаются. Даже если есть отказ, обратная связь обычно формальная и не превращается в план действий. TensorTalks решает это — проводит интервью по программе, оценивает каждый ответ и формирует конкретный итог с ошибками и материалами.

На следующем этапе платформа также может использоваться компаниями для стандартизации оценки кандидатов и вузами как практический тренажёр.

Проект ВКР, работает на стыке AI-EdTech и HR-Tech.

**доступен на сайте https://www.tensor-talks.ru/** (пока только с нероссийским ip)

## Демонстрация

### Регистрация

![Регистрация](docs/demo/registration.gif)

### Изучение выбранной темы

![Изучение выбранной темы](docs/demo/study-selected.gif)

### Отчет по изученной теме

![Отчет по изученной теме](docs/demo/study-report.gif)

### Интервью с отчетом и изучением слабой темы

![Интервью](docs/demo/interview.gif)

## Запуск

```bash
docker-compose up -d
```

**Сервисы:**

| Сервис | URL | Описание |
|--------|-----|----------|
| Frontend | http://localhost:5173 | React SPA |
| Admin Frontend | http://localhost:8097 | Панель администратора |
| BFF API | http://localhost:8080 | Backend API |
| Grafana | http://localhost:3000 | Метрики и логи |
| Prometheus | http://localhost:9090 | Метрики |
| Kafdrop | http://localhost:9000 | Kafka UI |
| pgAdmin | http://localhost:5050 | PostgreSQL UI |

## 4 этапа применения агентов

| Этап | Было | Что добавлено |
|------|------|---------------|
| 1. Наполнение базы знаний и вопросов | Полностью вручную | LLM-workflow (knowledge-producer-service): ingestion из файлов и URL, LLM-структурирование, дедупликация, web search (arxiv, semantic scholar). Human-in-the-loop через admin-панель |
| 2. Формирование интервью | Детерминированная сборка по фильтрам | Агент-планировщик (interview-builder-service, LangGraph): 6 инструментов (`search_questions`, `check_topic_coverage`, `validate_program`, `get_topic_relations`, `search_knowledge_base`, `get_user_history`). Три режима: interview, training, study. Fallback на детерминированный pipeline |
| 3. Процесс интервью | LLM-workflow (фиксированная цепочка) | Агент-интервьюер (interviewer-agent-service, LangGraph ReAct): 7 инструментов, автономное ведение диалога, confidence-aware решения, episodic memory, PII-фильтрация (regex + LLM), per-question state persistence в Redis |
| 4. Отчёт и материалы | Score + простой текстовый фидбек | Агент-аналитик (analyst-agent-service, LangGraph ReAct): 9 инструментов, структурированный отчёт (Pydantic AnalystReport), пресеты для тренировок и study, web search (arxiv), fallback на линейный pipeline |

## Архитектура

- 3 агента на LangGraph (планировщик, интервьюер, аналитик) — ReAct с tool-calling
- LLM-workflow для наполнения базы (knowledge-producer) — 4-шаговый pipeline
- State & Memory: Redis (краткосрочная, per-question counters) + PostgreSQL (долгосрочная, episodic memory)
- Оркестрация: Kafka (7 топиков, асинхронное взаимодействие)
- Guardrails: pre-call (PII regex + LLM, prompt injection detection, token limits) и post-call (JSON Schema валидация, leakage detection, tone sanitization)
- Structured output: все ответы LLM парсятся в Pydantic-модели (`AnswerEvaluation`, `AnalystReport`, `ProgramMeta`, `OffTopicClassification`, `PIICheckResult`, `TrainingPreset`)
- Observability: Prometheus метрики (LLM latency, confidence distribution, guardrail triggers), structlog (JSON)

**Инструменты агентов**:
- Read-only: `search_questions`, `evaluate_answer`, `search_knowledge_base`, `get_evaluations`, `summarize_dialogue`, `check_topic_coverage`, `validate_program`, `get_topic_relations`, `group_errors_by_topic`, `generate_report_section`, `validate_report`, `get_user_history`
- Терминальные: `emit_response` (интервьюер), `emit_report` (аналитик)
- Внешние: `web_search` (arxiv, trusted domains), `fetch_url` (allowlist: arxiv.org, pytorch.org, huggingface.co и др.)
- Write (human-in-the-loop): `save_draft_material`

**Confidence scores**: `AnswerEvaluation` содержит `decision_confidence` (0.0–1.0). При score в пограничной зоне (0.75–0.85): confidence ≥ 0.7 → next, < 0.7 → hint. При confidence < 0.5 — self-reflection (переоценка). Метрики: `agent_decision_confidence` (Histogram), `agent_low_confidence_decisions_total` (Counter).

**Episodic memory**: интервьюер загружает `previous_topic_scores` из results-crud для персонализации. Планировщик использует `get_user_history(user_id, topics)` для учёта слабых/сильных тем. Аналитик сравнивает динамику через `ProgressDelta`.

## Доработки не связанные с AI

**Регистрация и приватность (152-ФЗ).** Автогенерация логина (`{Прилагательное}{Существительное}{Число}`) — без email и персональных данных. Recovery key для восстановления. PII-фильтрация: regex (email, телефон, ИНН, СНИЛС, паспорт, компании из blacklist) + LLM-классификация неявных данных.

**Три режима сессий.** Интервью (оценка с грейдом), тренировка (70% практических вопросов, из пресетов аналитика), изучение темы (иерархия subtopic → теоретические блоки → контрольные вопросы, разблокирует тренировку).

**Admin-панель.** Управление базой знаний и вопросами (CRUD), очередь черновиков (HITL review), метрики (технические, AI, продуктовые).

**Дашборд.** Рекомендации после интервью (кнопки «Изучить тему» и «Тренировка» из пресетов), прогресс по темам.

**Результаты.** Структурированный отчёт: Summary, Errors by topic (с correction), Strengths, Preparation plan, Materials, scores по темам.

**Cloudflared.** Доступ к платформе через https://www.tensor-talks.ru/ без статического IP.
