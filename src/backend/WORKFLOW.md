# Workflow TensorTalks

Полное описание всех workflow и процессов платформы TensorTalks.

## Содержание

1. [Общая архитектура](#общая-архитектура)
2. [Workflow интервью](#workflow-интервью)
3. [Workflow создания программы](#workflow-создания-программы)
4. [Workflow аутентификации](#workflow-аутентификации)
5. [Workflow сохранения результатов](#workflow-сохранения-результатов)
6. [Форматы событий](#форматы-событий)

---

## Общая архитектура

### Компоненты

```
┌─────────────────────────────────────────────────────────────┐
│                      Пользователь                            │
└────────────────────┬────────────────────────────────────────┘
                     │ HTTPS
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Frontend (React SPA)                                       │
│  - Лендинг                                                  │
│  - Аутентификация                                           │
│  - Интервью (чат)                                           │
│  - История/результаты                                       │
└────────────────────┬────────────────────────────────────────┘
                     │ HTTP/REST
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  BFF Service (Go, Gin)                                      │
│  - CORS                                                     │
│  - JWT валидация                                            │
│  - Роутинг запросов                                         │
└────────────┬──────────────────────┬─────────────────────────┘
             │                      │
    ┌────────▼────────┐    ┌────────▼────────┐
    │ Auth Service    │    │ Session Service │
    │ (JWT, bcrypt)   │    │ (Session Mgr)   │
    └────────┬────────┘    └────────┬────────┘
             │                      │
    ┌────────▼─────────────────────▼────────┐
    │         Redis                          │
    │  - JWT сессии                          │
    │  - Активные интервью                   │
    └───────────────────────────────────────┘
```

### Сервисы данных

```
┌─────────────────────────────────────────────────────────────┐
│  CRUD Services (Go)                                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ session-crud-service     │ PostgreSQL: session_crud │   │
│  │ chat-crud-service        │ PostgreSQL: chat_crud    │   │
│  │ results-crud-service     │ PostgreSQL: results_crud │   │
│  │ knowledge-base-crud      │ PostgreSQL: knowledge    │   │
│  │ questions-crud-service   │ PostgreSQL: questions    │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Python Services                                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ interview-builder-service  │ Генерация программы    │   │
│  │ knowledge-producer-service │ Заполнение баз (HITL)  │   │
│  │ interviewer-agent-service  │ AI-интервьюер          │   │
│  │ analyst-agent-service      │ Финальный отчёт        │   │
│  │ dialogue-aggregator (Go)   │ Оркестратор диалога    │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Workflow интервью

### 1. Начало интервью

```
┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐
│ User    │      │Frontend │      │   BFF   │      │ Session │
│         │      │         │      │         │      │ Service │
└────┬────┘      └────┬────┘      └────┬────┘      └────┬────┘
     │                │                │                │
     │ POST /api/     │                │                │
     │ chat/start     │                │                │
     │───────────────>│                │                │
     │                │ POST /api/     │                │
     │                │ chat/start     │                │
     │                │───────────────>│                │
     │                │                │ POST /sessions│
     │                │                │───────────────>│
     │                │                │                │
     │                │                │ 1. Проверка    │
     │                │                │    лимита      │
     │                │                │    (Redis)     │
     │                │                │                │
     │                │                │ 2. Создание    │
     │                │                │    сессии      │
     │                │                │                │
     │                │                │ 3. Отправка    │
     │                │                │    в Kafka     │
     │                │                │                │
```

**Детали шагов:**

1. **Проверка лимита активных сессий**
   ```
   GET redis://tensor-talks:sessions:user:{user_id}:active
   Если count >= 3 → ошибка "Достигнут лимит активных сессий"
   ```

2. **Создание сессии в БД**
   ```json
   POST http://session-crud-service:8085/sessions
   {
     "user_id": "uuid",
     "params": {
       "topics": ["ml", "nlp"],
       "level": "middle",
       "type": "ml",
       "mode": "interview"
     }
   }
   
   Response:
   {
     "session": {
       "session_id": "uuid",
       "user_id": "uuid",
       "start_time": "2026-03-31T10:00:00Z",
       "params": {...}
     }
   }
   ```

3. **Отправка запроса на построение программы**
   ```json
   Kafka: tensor-talks-interview.build.request
   {
     "event_id": "uuid",
     "event_type": "interview.build.request",
     "timestamp": "2026-03-31T10:00:00Z",
     "payload": {
       "session_id": "uuid",
       "params": {
         "topics": ["ml", "nlp"],
         "level": "middle",
         "type": "ml",
         "mode": "interview"
       }
     }
   }
   ```

### 2. Генерация программы интервью

```
┌────────────────┐      ┌──────────┐      ┌──────────┐
│ Session Service│      │ Interview│      │ Questions│
│                │      │ Builder  │      │   CRUD   │
└───────┬────────┘      └────┬─────┘      └────┬─────┘
        │                    │                 │
        │ 1. Consumer Kafka  │                 │
        │───────────────────>│                 │
        │                    │                 │
        │ 2. Поиск вопросов  │                 │
        │    по фильтрам     │                 │
        │────────────────────────────────────>│
        │                    │                 │
        │ 3. Вопросы         │                 │
        │    (5 шт)          │                 │
        │<────────────────────────────────────│
        │                    │                 │
        │ 4. Запрос знаний   │                 │
        │─────────────────────────────────────>│
        │                    │                 │
        │ 5. Знания          │                 │
        │<─────────────────────────────────────│
        │                    │                 │
        │ 6. Сборка программы│                 │
        │    (сортировка)    │                 │
        │                    │                 │
        │ 7. Ответ в Kafka   │                 │
        │<───────────────────│                 │
```

**Детали генерации программы:**

1. **Поиск вопросов по фильтрам**
   ```
   GET http://questions-crud-service:8091/questions/search
     ?complexity=2
     &theory_id=ml_basics
     &question_type=conceptual
     &limit=5
   ```

2. **Получение знаний для вопросов**
   ```
   GET http://knowledge-base-crud-service:8090/knowledge/{theory_id}
   ```

3. **Сборка программы**
   ```json
   {
     "session_id": "uuid",
     "interview_program": [
       {
         "question_id": "q1",
         "question_text": "...",
         "theory": {...},
         "ideal_answer": "..."
       },
       // 5 вопросов всего
     ]
   }
   ```

4. **Отправка ответа**
   ```json
   Kafka: tensor-talks-interview.build.response
   {
     "event_id": "uuid",
     "event_type": "interview.build.response",
     "timestamp": "2026-03-31T10:00:05Z",
     "payload": {
       "session_id": "uuid",
       "interview_program": [...]
     }
   }
   ```

### 3. Проведение интервью (AI Agent)

```
┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐
│ User    │      │Frontend │      │   BFF   │      │  Agent  │
│         │      │         │      │         │      │ Service │
└────┬────┘      └────┬────┘      └────┬────┘      └────┬────┘
     │                │                │                │
     │ Сообщение      │                │                │
     │───────────────>│                │                │
     │                │ POST /api/     │                │
     │                │ chat/message   │                │
     │                │───────────────>│                │
     │                │                │ Kafka:         │
     │                │                │ chat.events.out│
     │                │                │───────────────>│
     │                │                │                │
     │                │                │ LangGraph      │
     │                │                │ Workflow:      │
     │                │                │ 1. Загрузка    │
     │                │                │    контекста   │
     │                │                │ 2. Оценка      │
     │                │                │    ответа      │
     │                │                │ 3. Решение     │
     │                │                │    действия    │
     │                │                │ 4. Генерация   │
     │                │                │    ответа      │
     │                │                │                │
     │                │                │ Kafka:         │
     │                │                │ chat.events.in │
     │                │                │<───────────────│
     │                │                │                │
     │                │ SSE /stream    │                │
     │                │<───────────────│                │
     │ Response       │                │                │
     │<───────────────│                │                │
```

**LangGraph Workflow:**

```python
# State Schema
class InterviewState(TypedDict):
    messages: Annotated[list, add_messages]
    current_question: int
    attempts: int
    hints_given: int
    evaluations: list
    program: dict

# Graph
workflow = StateGraph(InterviewState)

workflow.add_node("load_context", load_context)
workflow.add_node("evaluate_answer", evaluate_answer)
workflow.add_node("decide_action", decide_action)
workflow.add_node("generate_response", generate_response)

workflow.set_entry_point("load_context")
workflow.add_edge("load_context", "evaluate_answer")
workflow.add_edge("evaluate_answer", "decide_action")
workflow.add_conditional_edges(
    "decide_action",
    should_continue,
    {
        "continue": "generate_response",
        "end": END
    }
)
workflow.add_edge("generate_response", "load_context")

app = workflow.compile()
```

**Действия агента:**

| Ответ пользователя | Действие агента |
|-------------------|-----------------|
| Полный ответ (score ≥ 0.8) | Переход к следующему вопросу |
| Частичный ответ (0.4-0.8) | Уточняющий вопрос или подсказка |
| Неполный ответ (< 0.4) | Развернутое объяснение |
| "Не знаю" | Подсказка с теорией |
| Off-topic | Возврат к теме вопроса |
| Пропуск | Запись в оценку, следующий вопрос |

---

## Форматы событий

### Kafka: tensor-talks-chat.events.out

**Событие: chat.started**
```json
{
  "event_id": "uuid",
  "event_type": "chat.started",
  "timestamp": "2026-03-31T10:00:00Z",
  "service": "bff-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "uuid",
    "user_id": "uuid",
    "program": [...]
  }
}
```

**Событие: user.message**
```json
{
  "event_id": "uuid",
  "event_type": "user.message",
  "timestamp": "2026-03-31T10:05:00Z",
  "service": "bff-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "uuid",
    "message_id": "uuid",
    "content": "Ответ пользователя..."
  }
}
```

### Kafka: tensor-talks-chat.events.in

**Событие: assistant.message**
```json
{
  "event_id": "uuid",
  "event_type": "assistant.message",
  "timestamp": "2026-03-31T10:05:05Z",
  "service": "interviewer-agent-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "uuid",
    "message_id": "uuid",
    "content": "Ответ агента...",
    "question_index": 0,
    "is_final": false
  }
}
```

**Событие: chat.completed** (BFF получает от dialogue-aggregator, score=0 placeholder, финальные данные приходят из results-crud после обработки analyst-agent)
```json
{
  "event_id": "uuid",
  "event_type": "chat.completed",
  "timestamp": "2026-03-31T10:30:00Z",
  "service": "dialogue-aggregator",
  "version": "1.0.0",
  "payload": {
    "session_id": "uuid",
    "user_id": "uuid",
    "results": {
      "score": 0,
      "feedback": "Оценка формируется аналитиком.",
      "recommendations": []
    },
    "completed_at": "2026-03-31T10:30:00Z"
  }
}
```

### Kafka: tensor-talks-session.completed (dialogue-aggregator → analyst-agent-service)

```json
{
  "event_id": "uuid",
  "event_type": "session.completed",
  "service": "dialogue-aggregator",
  "payload": {
    "session_id": "uuid",
    "session_kind": "interview",
    "user_id": "uuid",
    "chat_id": "uuid",
    "topics": ["ml", "nlp"],
    "level": "middle",
    "terminated_early": false,
    "answered_questions": 8,
    "total_questions": 10,
    "completed_at": "2026-03-31T10:30:00Z"
  }
}
```

`analyst-agent-service` подписан на этот топик; по `session_kind` он либо обновляет `results.report_json` + создаёт `presets` (interview), либо ставит финальную оценку (training), либо обновляет `user_topic_progress` (study).

---

## Ссылки

- [KAFKA.md](./KAFKA.md) - Детальное описание Kafka топиков
- [../DEVOPS_DEPLOY.md](../DEVOPS_DEPLOY.md) - DevOps архитектура
- [../README.md](../README.md) - Общая архитектура системы
