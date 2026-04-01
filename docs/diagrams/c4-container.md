# C4 Container Diagram

## Уровень 2: Контейнеры системы

```
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│                         TensorTalks Platform                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌──────────────┐     ┌──────────────┐     ┌──────────────────────┐    │
│   │ Frontend     │───▶│ BFF           │───▶│ Session-service      │    │
│   │ (React SPA)  │◀───│ (Go + Gin)    │◀───│ (Go + Redis)         │    │
│   └──────────────┘     └──────────────┘     └──────────────────────┘    │
│          ▲                    │                       │                 │
│          │                    │                       │                 │
│          │                    ▼                       ▼                 │
│          │            ┌──────────────┐     ┌──────────────────────┐     │
│          │            │ Auth-service │     │ Session-crud-service │     │
│          │            │ (Go + JWT)   │     │ (Go + PostgreSQL)    │     │
│          │            └──────────────┘     └──────────────────────┘     │
│          │                    │                       │                 │
│          │                    ▼                       │                 │
│          │            ┌──────────────┐                │                 │
│          │            │ User-store   │◀──────────────┘                 │
│          │            │ (Go + PG)    │                                  │
│          │            └──────────────┘                                  │
│          │                                                              │
│          │     ┌─────────────────────────────────────────────────┐      │
│          │     │ Kafka                                           │      │
│          │     │ chat.events.* | interview.build.*               │      │
│          │     │ interview.session.completed                     │      │
│          │     └─────────────────────────────────────────────────┘      │
│          │                    ▲                   ▲                     │
│          │                    │                   │                     │
│          ▼                                        ▼                     │
│   ┌──────────────────────────────┐   ┌──────────────────────────────┐   │
│   │ Interview-builder            │   │ Agent-service                │   │
│   │ (FastAPI + LangGraph)        │   │ (LangChain + LangGraph)      │   │
│   │ Планировщик; interview /     │   │ Интервьюер + аналитик        │   │
│   │ training / study             │   │ (позже — на 2 два сервиса)   │   │
│   └──────────────────────────────┘   └──────────────────────────────┘   │
│              │                               │                          │
│   ┌──────────┴───────────────────────────────┴───────────────────────┐  │
│   │ Knowledge-producer-service (FastAPI + LLM-workflow, этап 0)      │  │
│   └──────────────────────────────────────────────────────────────────┘  │
│              │                               │                          │
│              ▼                               ▼                          │
│   ┌──────────────────────────────┐   ┌──────────────────────────────┐   │
│   │ Knowledge-base-crud          │   │ Questions-crud               │   │
│   │ (Go + PostgreSQL)            │   │ (Go + PostgreSQL)            │   │
│   │ База знаний (~500)           │   │ База вопросов (~100)         │   │
│   └──────────────────────────────┘   └──────────────────────────────┘   │
│              │                               │                          │
│   ┌──────────────────────────────┐   ┌──────────────────────────────┐   │
│   │ Chat-crud                    │   │ Results-crud                 │   │
│   │ (Go + PostgreSQL)            │   │ (Go + PostgreSQL)            │   │
│   │ История чата                 │   │ Оценки, отчёты, пресеты      │   │
│   └──────────────────────────────┘   └──────────────────────────────┘   │
│                                                                         │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │ Observability: Prometheus | Grafana | Loki                      │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Описание контейнеров

### Frontend (React SPA)

- **Технологии**: React 19, TypeScript 5, Vite, Tailwind CSS
- **Ответственность**: Пользовательский интерфейс
- **Ключевые компоненты**:
  - Страница аутентификации
  - Дашборд с выбором параметров интервью
  - Чат-интерфейс для проведения интервью
  - Страница результатов с отчётом
  - История интервью

### BFF (Backend-for-frontend)

- **Технологии**: Go 1.21+, Gin
- **Ответственность**: Единая точка входа для frontend
- **Функции**:
  - Агрегация запросов к внутренним сервисам
  - CORS настройка
  - JWT валидация
  - WebSocket/polling для обновлений чата
  - Маршрутизация к CRUD-сервисам

### Auth-service

- **Технологии**: Go, JWT
- **Ответственность**: Аутентификация и авторизация
- **Функции**:
  - Регистрация пользователей
  - Логин/пароль аутентификация
  - Выпуск access/refresh токенов
  - Валидация токенов
- **Взаимодействие**: Только через user-store-service (нет прямого доступа к БД)

### User-store-service

- **Технологии**: Go, GORM, PostgreSQL
- **Ответственность**: Хранение данных пользователей
- **Функции**:
  - CRUD пользователей
  - Двойная идентификация (internal_id + external_uuid)
  - Хранение хешей паролей
- **БД**: user_store_db

### Session-service

- **Технологии**: Go, Redis
- **Ответственность**: Управление жизненным циклом сессии
- **Функции**:
  - Создание новой сессии
  - Кеширование активных интервью в Redis
  - Публикация запросов на формирование программы
  - Координация с session-crud-service
- **Хранилище**: Redis (кеш), session-crud-service (персистентность)

### Session-crud-service

- **Технологии**: Go, GORM, PostgreSQL
- **Ответственность**: Персистентное хранение сессий
- **Функции**:
  - Сохранение параметров интервью
  - Хранение сформированных программ
- **БД**: session_crud_db

### Interview-builder-service

- **Технологии**: Python 3.11+, FastAPI, LangGraph
- **Ответственность**: Агент-планировщик — программа интервью, тренировки или study (см. `docs/specs/interview-builder-service.md`)
- **Функции**:
  - Слушает Kafka `interview.build.request`
  - Вызывает tools к Questions-crud и Knowledge-base-crud (`search_questions`, `check_topic_coverage`, `validate_program`, `get_topic_relations`, `search_knowledge_base` — по режиму)
  - Публикует `interview.build.response`; программа сохраняется через Session-crud

### Agent-service

- **Технологии**: Python 3.11+, LangChain, LangGraph
- **Ответственность**: **Интервьюер** и **аналитик** (в коде и деплое позже разделяются; см. `docs/specs/interviewer-agent.md`, `docs/specs/analyst-agent.md`)
- **Интервьюер**: Kafka `chat.events.out` / `chat.events.in`, `evaluate_answer`, `search_knowledge_base`, `search_questions(related_to=...)`, `web_search`, `fetch_url`, `summarize_dialogue`; по завершении сессии — публикация `interview.session.completed`
- **Аналитик**: потребляет `interview.session.completed`, `get_evaluations`, `group_errors_by_topic`, материалы (KB + web), `generate_report_section`, `validate_report` (цикл до успеха или лимита), `save_draft_material` при необходимости; запись отчёта и `preset_training` в Results-crud; отчёт пользователю — `chat.events.in`

### Knowledge-producer-service

- **Технологии**: Python 3.11+, FastAPI, LLM
- **Ответственность**: **LLM-workflow этапа 0** — черновики для базы знаний, без LangGraph-агента (см. `docs/specs/knowledge-producer-service.md`)
- **Взаимодействие**: `web_search`, `fetch_url`, CRUD KB/Questions, `save_draft_material` (HITL); Kafka для этого сервиса в PoC может не использоваться

### Knowledge-base-crud-service

- **Технологии**: Go, GORM, PostgreSQL
- **Ответственность**: База знаний по ML
- **Функции**:
  - CRUD операций с концепциями
  - Поиск по фильтрам (сложность, тема, теги)
  - JSONB хранение
- **БД**: knowledge_base_crud_db
- **Данные**: ~500 концепций ML

### Questions-crud-service

- **Технологии**: Go, GORM, PostgreSQL
- **Ответственность**: База вопросов для интервью
- **Функции**:
  - CRUD вопросов
  - Фильтрация по сложности, теме, типу
  - JSONB хранение
- **БД**: questions_crud_db
- **Данные**: ~100 вопросов, 5 уровней, 8 тем

### Chat-crud-service

- **Технологии**: Go, GORM, PostgreSQL
- **Ответственность**: Хранение сообщений чата
- **Функции**:
  - Сохранение сообщений от BFF
  - Хранение полных дампов завершенных интервью
- **БД**: chat_crud_db

### Results-crud-service

- **Технологии**: Go, GORM, PostgreSQL
- **Ответственность**: Хранение результатов интервью
- **Функции**:
  - Сохранение оценок и отчёта (агент-аналитик после `validate_report`)
  - `preset_training` (слабые темы, материалы для рекомендаций тренировок / study)
- **БД**: results_crud_db

### Observability Stack

- **Prometheus**: Сбор метрик со всех сервисов
- **Grafana**: Визуализация метрик, дашборды
- **Loki**: Агрегация и поиск логов

## Потоки между контейнерами

1. **Синхронные (HTTP REST)**:
  - Frontend ↔ BFF
  - BFF ↔ Auth/User-store/Session-crud/Chat-crud/Results-crud
  - Interview-builder ↔ Questions-crud / Knowledge-base-crud
  - Knowledge-producer ↔ Knowledge-base-crud / Questions-crud (внутренние клиенты)
2. **Асинхронные (Kafka)**:
  - BFF → `chat.events.out` → Agent-service (Interviewer)
  - Agent-service → `chat.events.in` → BFF
  - Session-service → `interview.build.request` → Interview-builder
  - Interview-builder → `interview.build.response` → Session-service
  - Agent-service (Interviewer) → `interview.session.completed` → Agent-service (Analyst)
  - Agent-service (Analyst) → `chat.events.in` (отчёт)

