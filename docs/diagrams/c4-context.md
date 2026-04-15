# C4 Context Diagram

## Уровень 1: Контекст системы

```
┌─────────────────────────────────────────────────────────────────┐
│                     TensorTalks Platform                        │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Interview-builder-service (планировщик, ReAct LangGraph) │  │
│  │  Interviewer-agent-service (интервьюер, ReAct LangGraph)  │  │
│  │  Analyst-agent-service (аналитик, ReAct LangGraph)        │  │
│  │  Dialogue-aggregator (оркестрация Kafka)                  │  │
│  │  Knowledge-producer-service: LLM-workflow (не агент)      │  │
│  │  Admin-frontend + Admin-bff-service (админка)             │  │
│  │  User-crud-service (бывш. user-store-service)             │  │
│  │  Всего 17 сервисов                                        │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
         ▲                      ▲                   ▲
         │                      │                   │
         │                      │                   │
         ▲
         │ (HTTPS через Cloudflared tunnel: https://www.tensor-talks.ru/)
         │
    ┌────┴─────┐         ┌──────┴──────┐      ┌─────┴──────┐
    │          │         │             │      │            │
┌────────┐  ┌─────────┐  ┌─────────┐  ┌────────┐  ┌──────────┐
│ Пользо-│  │ LLM     │  │ Kafka   │  │ Postgre│  │ Внешние  │
│ ватель │  │ via     │  │ Cluster │  │ SQL +  │  │ API      │
│ (ML-   │  │ litellm-│  │ (7 то-  │  │ Redis  │  │ arxiv/   │
│ специа-│  │ proxy   │  │ пиков)  │  │        │  │ trusted  │
│ лист)  │  │         │  │         │  │        │  │ fetch    │
└────────┘  └─────────┘  └─────────┘  └────────┘  └──────────┘
```

## Акторы

### Пользователь (ML-специалист)
- **Роль**: Основной пользователь платформы
- **Цель**: Пройти симуляцию технического интервью, получить обратную связь и рекомендации
- **Взаимодействие**: 
  - Выбирает параметры (интервью: specialty, grade, use_previous_results; тренировка / study — по продукту)
  - Отвечает на вопросы в чат-интерфейсе
  - Получает отчёт с оценками и материалами

### LLM Provider (через litellm-proxy)
- **Роль**: Внешний провайдер языковых моделей (OpenAI-совместимый через litellm-proxy)
- **Цель**: Предоставление API для генерации ответов агентами
- **Взаимодействие**:
  - Сервисы с LLM (interview-builder-service, interviewer-agent-service, analyst-agent-service, knowledge-producer-service, admin-bff-service) вызывают модели по политике
  - Отправка промптов с контекстом интервью
  - Получение сгенерированных ответов

### Kafka Cluster
- **Роль**: Брокер сообщений для асинхронного взаимодействия
- **Цель**: Буферизация событий, масштабирование, отказоустойчивость
- **Взаимодействие**:
  - BFF публикует сообщения пользователя в `chat.events.out`
  - Dialogue-aggregator → `agent_requests` → Interviewer-agent-service; Interviewer-agent-service → `agent_responses` → Dialogue-aggregator → `chat.events.in`
  - Session-service → `interview.build.request` → Interview-builder-service → `interview.build.response`
  - По завершении сессии → `analyst_requests` → Analyst-agent-service → `analyst_responses` → Results-crud
  - Knowledge-producer-service публикует `knowledge_drafts` (HITL)
  - Всего 7 топиков: `chat.events.out`, `chat.events.in`, `agent_requests`, `agent_responses`, `analyst_requests`, `analyst_responses`, `knowledge_drafts` (+ `interview.build.request/response`)

### PostgreSQL + Redis
- **Роль**: Персистентное хранилище данных + эпизодическая и кратковременная память
- **Цель**: Надёжное хранение пользователей, сессий, вопросов, знаний, результатов; per-question state; долгосрочная эпизодическая память агентов
- **Взаимодействие**:
  - Каждый CRUD-сервис работает со своей БД
  - JSONB для вопросов и знаний
  - Redis хранит per-question state интервьюера (до завершения вопроса)
  - PostgreSQL — episodic memory (история сессий, evaluations, отчёты)

### Внешние API (Search)
- **Роль**: Инструмент для поиска внешних материалов
- **Цель**: Поиск актуальных статей, документации, примеров
- **Взаимодействие**:
  - Interviewer-agent-service, analyst-agent-service и knowledge-producer-service вызывают `web_search` (arxiv) и `fetch_url` (whitelist доверенных доменов)
  - Лимиты вызовов на сессию / задачу — по конфигурации (см. `docs/system-design.md`)
  - Только read-only доступ

### Cloudflared Tunnel (точка входа)
- **Роль**: Единая внешняя точка входа к платформе
- **URL**: https://www.tensor-talks.ru/
- **Взаимодействие**: Tunnel → Frontend (user) / Admin-frontend / BFF; TLS терминация на стороне Cloudflare

## Границы системы

**Внутри периметра**:
- Все микросервисы backend + frontend
- Kafka, Redis, PostgreSQL
- Interview-builder-service, Interviewer-agent-service, Analyst-agent-service, Dialogue-aggregator, Knowledge-producer-service, Admin-frontend, Admin-bff-service, User-crud-service

**Вне периметра**:
- LLM Provider (внешний API)
- Внешние поисковые API
- Пользователь (человек)

## Потоки данных (контекст)

1. **Пользователь → Система**: Выбор параметров, ответы на вопросы
2. **Система → Пользователь**: Вопросы, подсказки, отчёт
3. **Система → LLM Provider**: Промпты с контекстом
4. **LLM Provider → Система**: Сгенерированные ответы
5. **Система → Внешние API**: Поисковые запросы
6. **Внешние API → Система**: Результаты поиска, контент страниц
