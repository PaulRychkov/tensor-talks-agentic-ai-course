## Обзор backend-микросервисов

В этой директории находятся Go-микросервисы, обеспечивающие работу платформы TensorTalks.
Архитектура следует принципам **микросервисного разделения по зонам ответственности**:
отдельно хранение данных, аутентификация, управление сессиями и слой взаимодействия с фронтендом (BFF).

### Микросервисы

1. **user-crud-service**
   - Единственный сервис, имеющий прямой доступ к базе с логинами/паролями.
   - Предоставляет CRUD HTTP API над таблицей пользователей (создать/прочитать/обновить/удалить).
   - Реализует отладочный эндпоинт `GET /debug/users` с фильтрами по логину и поддержкой `limit/offset`.
   - Использует **Gin** (HTTP), **GORM** (PostgreSQL), **Viper** (конфигурация), **Testify** (тесты).

2. **auth-service**
   - Отвечает за регистрацию, логин и работу с JWT (access/refresh токены).
   - Никогда не ходит в базу напрямую — только через `user-crud-service` по HTTP.
   - Хеширует пароли с помощью **bcrypt** до записи в `user-crud-service`.
   - Выдаёт JWT с GUID пользователя, issuer/audience и ограниченным TTL.
   - Управляет активными логин-сессиями через Redis (хранение сессий с TTL, возможность отзыва токенов через logout).
   - При валидации токенов проверяет наличие активной сессии в Redis.

3. **session-service** (session-manager)
   - Управляет сессиями интервью.
   - Предоставляет REST API для создания сессий, получения программы интервью, закрытия сессий.
   - Работает с Redis для кэширования активных сессий.
   - Интегрируется с session-crud-service для хранения данных в БД.
   - Координирует создание программы интервью через Kafka с interview-builder-service.

4. **session-crud-service**
   - CRUD микросервис для работы с сессиями в PostgreSQL.
   - Хранит информацию о сессиях: параметры интервью, программу интервью, временные метки.
   - Предоставляет API для создания, получения, обновления и закрытия сессий.

5. **chat-crud-service**
   - CRUD микросервис для работы с чатами и сообщениями в PostgreSQL.
   - Хранит сообщения чатов и дампы завершенных чатов.
   - Предоставляет API для сохранения сообщений и получения истории чатов.

6. **results-crud-service**
   - CRUD микросервис для работы с результатами интервью в PostgreSQL.
   - Хранит финальный отчёт (`report_json`), placeholder-результат (score/feedback), `evaluations` по вопросам, `session_kind`, `result_format_version`.
   - Дополнительно содержит таблицы `presets` (рекомендации после interview-сессии) и `user_topic_progress` (прогресс по темам после study-сессии).
   - Предоставляет API для сохранения, получения результатов и работы с пресетами/прогрессом тем.

7. **interview-builder-service**
   - Python FastAPI сервис для динамического создания программы интервью / training / study.
   - Слушает Kafka топик `interview.build.request`.
   - Получает параметры (`topics`, `level`, `type`, `mode` ∈ `interview` | `training` | `study`).
   - Запрашивает вопросы из questions-crud-service и теорию из knowledge-base-crud-service.
   - Применяет пайплайн `filter → dedup → coverage → balance → mode-profile → sort → enrich`.
   - Возвращает программу + `program_meta` (`validation_passed`, `coverage`, `fallback_reason`, `generator_version`).
   - Отправляет результат в Kafka топик `interview.build.response`.

8. **knowledge-base-crud-service**
   - Go микросервис для работы с базой знаний в PostgreSQL.
   - CRUD операции над знаниями (создание, чтение, обновление, удаление).
   - Поиск знаний по фильтрам: complexity, concept, parent_id, tags.
   - Хранение структурированных знаний в JSONB формате.

9. **questions-crud-service**
   - Go микросервис для работы с базой вопросов в PostgreSQL.
   - CRUD операции над вопросами (создание, чтение, обновление, удаление).
   - Поиск вопросов по фильтрам: complexity, theory_id, question_type.
   - Хранение структурированных вопросов в JSONB формате.

10. **knowledge-producer-service**
    - Python FastAPI сервис для наполнения баз знаний и вопросов с HITL workflow `draft → review → publish`.
    - Поддерживает ингест из markdown / JSON / PDF / URL и web-search (arxiv, Semantic Scholar).
    - Дедупликация черновиков по similarity (`SequenceMatcher`).
    - Только опубликованные сущности уходят в `knowledge-base-crud-service` / `questions-crud-service`.

11. **dialogue-aggregator** (бывший `mock-model-service`)
   - Go-оркестратор диалога: читает `chat.events.out`, общается с `interviewer-agent-service` через AgentBridge (`messages.full.data` / `generated.phrases`), отправляет ответы в `chat.events.in`.
   - Сам решений не принимает — все `next` / `hint` / `clarify` / `skip` / `complete` приходят от `interviewer-agent-service`.
   - Получает программу + метаданные сессии (`session_mode`, `topics`, `level`) от `session-service` и хранит их в `SessionManager`.
   - Сохраняет все сообщения в `chat-crud-service`, placeholder-результат в `results-crud-service`, закрывает сессию в `session-service`.
   - Публикует `session.completed` (с реальными `session_kind`/`topics`/`level`) для `analyst-agent-service`.

12. **interviewer-agent-service**
   - Python LangGraph-сервис, ведущий интервью.
   - Принимает `messages.full.data` от `dialogue-aggregator`, прогоняет граф (классификация ответа → оценка → решение) и возвращает реплику в `generated.phrases`.
   - Хранит `question_evaluations` и состояние диалога в Redis, общается с `session-crud` / `chat-crud` для контекста.

13. **analyst-agent-service**
   - Python LangGraph-сервис для генерации финального отчёта.
   - Подписан на `session.completed`, по `session_kind` уходит в одну из веток:
     - `interview` → формирует `report_json` + рекомендации, обновляет `results` и создаёт запись в `presets`.
     - `training` → обновляет `results` финальной оценкой по training-сценарию.
     - `study` → обновляет `user_topic_progress` для пройденных тем.

14. **bff-service**
   - **Backend-for-frontend**, предоставляющий фронтенду стабильное REST API.
   - Проксирует запросы аутентификации в `auth-service`, скрывая внутреннюю топологию сервисов.
   - Управляет чатами: создаёт сессии через `session-service`, отправляет события в Kafka.
   - Читает события от модели из Kafka и обрабатывает их.
   - Проверяет JWT токены через middleware для защищенных endpoints (`/api/chat/*`, `/api/interviews/*`).
   - Конфигурирует CORS и не имеет доступа ни к базе данных, ни к `user-crud-service` напрямую.

### Базы данных

**user_crud_db** — хранит данные учётных записей:

| Колонка      | Тип        | Описание                                                     |
|--------------|-----------|--------------------------------------------------------------|
| id           | SERIAL PK | Внутренний числовой идентификатор                            |
| external_id  | UUID      | Внешний GUID, используемый между микросервисами             |
| login        | TEXT      | Уникальный логин пользователя                                |
| password_hash| TEXT      | Хеш пароля (bcrypt), прямой пароль нигде не хранится         |
| created_at   | TIMESTAMP | Время создания (заполняется GORM)                            |
| updated_at   | TIMESTAMP | Время обновления (заполняется GORM)                          |

**session_crud_db** — хранит данные о сессиях интервью:

| Колонка          | Тип        | Описание                                                     |
|------------------|-----------|--------------------------------------------------------------|
| session_id       | UUID PK   | Идентификатор сессии                                         |
| user_id          | UUID      | Идентификатор пользователя (indexed)                         |
| start_time       | TIMESTAMP | Время начала сессии                                          |
| end_time         | TIMESTAMP | Время окончания сессии (nullable)                            |
| params           | JSONB     | Параметры интервью (topics, level, type)                     |
| interview_program| JSONB     | Программа интервью (список вопросов с теорией)               |
| created_at       | TIMESTAMP | Время создания                                               |
| updated_at       | TIMESTAMP | Время обновления                                             |

**chat_crud_db** — хранит данные о чатах:

Таблица `messages`:
| Колонка   | Тип        | Описание                                                     |
|-----------|-----------|--------------------------------------------------------------|
| id        | SERIAL PK | Идентификатор сообщения                                      |
| session_id| UUID      | Идентификатор сессии (indexed)                               |
| type      | VARCHAR   | Тип сообщения (system/user)                                  |
| content   | TEXT      | Текст сообщения                                              |
| created_at| TIMESTAMP | Время создания                                               |

Таблица `chat_dumps`:
| Колонка   | Тип        | Описание                                                     |
|-----------|-----------|--------------------------------------------------------------|
| id        | SERIAL PK | Идентификатор дампа                                          |
| session_id| UUID      | Идентификатор сессии (unique indexed)                        |
| chat      | JSONB     | Структурированный дамп чата (массив сообщений)               |
| created_at| TIMESTAMP | Время создания                                               |
| updated_at| TIMESTAMP | Время обновления                                             |

**results_crud_db** — хранит результаты интервью / training / study:

Таблица `results`:
| Колонка               | Тип        | Описание                                                     |
|-----------------------|-----------|--------------------------------------------------------------|
| id                    | SERIAL PK | Идентификатор результата                                     |
| session_id            | UUID      | Идентификатор сессии (unique indexed)                        |
| user_id               | UUID      | Идентификатор пользователя (indexed)                         |
| session_kind          | VARCHAR   | `interview` / `training` / `study`                           |
| score                 | INTEGER   | Оценка (0-100), 0 для placeholder до обновления аналитиком   |
| feedback              | TEXT      | Текстовая обратная связь                                     |
| terminated_early      | BOOLEAN   | Сессия завершена досрочно                                    |
| report_json           | JSONB     | Финальный отчёт от analyst-agent-service (nullable)          |
| evaluations           | JSONB     | Покомпонентные оценки по вопросам                            |
| result_format_version | VARCHAR   | Версия схемы отчёта                                          |
| created_at            | TIMESTAMP | Время создания                                               |
| updated_at            | TIMESTAMP | Время обновления                                             |

Таблица `presets` — рекомендованные программы после interview-сессии (создаются analyst-agent).

Таблица `user_topic_progress` — прогресс пользователя по темам после study-сессии (обновляется analyst-agent).

**knowledge_base_crud_db** — хранит структурированные знания:

| Колонка   | Тип        | Описание                                                     |
|-----------|-----------|--------------------------------------------------------------|
| id        | VARCHAR PK| Идентификатор знания                                         |
| concept   | VARCHAR   | Название концепции                                           |
| complexity| INTEGER   | Сложность (1-3)                                              |
| parent_id | VARCHAR   | ID родительского знания (nullable, indexed)                  |
| data      | JSONB     | Структурированные данные знания (segments, relations, metadata) |
| version   | VARCHAR   | Версия знания                                                |
| created_at| TIMESTAMP | Время создания                                               |
| updated_at| TIMESTAMP | Время обновления                                             |

**questions_crud_db** — хранит вопросы интервью:

| Колонка      | Тип        | Описание                                                     |
|--------------|-----------|--------------------------------------------------------------|
| id           | VARCHAR PK| Идентификатор вопроса                                        |
| theory_id    | VARCHAR   | ID связанного знания (nullable, indexed)                    |
| question_type| VARCHAR   | Тип вопроса (conceptual, practical, coding)                  |
| complexity   | INTEGER   | Сложность (1-3)                                              |
| data         | JSONB     | Структурированные данные вопроса (content, ideal_answer, metadata) |
| version      | VARCHAR   | Версия вопроса                                               |
| created_at   | TIMESTAMP | Время создания                                               |
| updated_at   | TIMESTAMP | Время обновления                                             |

### Очереди Kafka

Используются следующие топики для асинхронной обработки событий:

- **chat.events.out** — события от BFF к dialogue-aggregator (старт чата, ответ пользователя, terminate)
- **chat.events.in** — события от dialogue-aggregator к BFF (вопрос модели, completed)
- **messages.full.data** — полный контекст диалога от dialogue-aggregator к interviewer-agent-service
- **generated.phrases** — реплика/решение от interviewer-agent-service к dialogue-aggregator
- **interview.build.request** — запрос на создание программы интервью (session-service → interview-builder)
- **interview.build.response** — ответ с программой интервью (interview-builder → session-service)
- **session.completed** — событие завершения сессии от dialogue-aggregator к analyst-agent-service (с `session_kind`, `topics`, `level`)

Подробнее см. [KAFKA.md](./KAFKA.md)

### Конфигурация

Каждый сервис читает конфигурацию через **Viper**:

- файл `config/config.yaml` внутри сервиса;
- переменные окружения с соответствующим префиксом:
  - `USER_CRUD_...` для `user-crud-service`;
  - `AUTH_...` для `auth-service`;
  - `SESSION_...` для `session-service`;
  - `SESSION_CRUD_...` для `session-crud-service`;
  - `CHAT_CRUD_...` для `chat-crud-service`;
  - `RESULTS_CRUD_...` для `results-crud-service`;
  - `INTERVIEW_BUILDER_...` для `interview-builder-service`;
  - `KNOWLEDGE_BASE_CRUD_...` для `knowledge-base-crud-service`;
  - `QUESTIONS_CRUD_...` для `questions-crud-service`;
  - `KNOWLEDGE_PRODUCER_...` для `knowledge-producer-service`;
  - `DIALOGUE_AGGREGATOR_...` для `dialogue-aggregator`;
  - `BFF_...` для `bff-service`.

Секреты (JWT-secret, пароли БД и т.п.) в проде должны передаваться только через переменные окружения
или секреты оркестратора, а не храниться в YAML.

### Контейнеры и оркестрация

- У каждого микросервиса и фронтенда есть свой `Dockerfile`.
- В корневом `docker-compose.yml` поднимаются:
  - React-фронтенд (Nginx + статические файлы);
  - `bff-service` — HTTP-шлюз для фронтенда;
  - `auth-service` — регистрация/логин/JWT;
  - `session-service` — управление сессиями (session-manager);
  - `session-crud-service` — CRUD для сессий;
  - `chat-crud-service` — CRUD для чатов;
  - `results-crud-service` — CRUD для результатов;
  - `interview-builder-service` — динамическое создание программы интервью (Python FastAPI);
  - `knowledge-base-crud-service` — CRUD для базы знаний;
  - `questions-crud-service` — CRUD для базы вопросов;
  - `knowledge-producer-service` — заполнение баз знаний и вопросов из JSON файлов (Python FastAPI);
  - `dialogue-aggregator` — оркестратор диалога между BFF и interviewer-agent-service;
  - `interviewer-agent-service` — interviewer-agent (LangGraph) для ведения интервью;
  - `analyst-agent-service` — генератор финальных отчётов / прогресса по `session.completed`;
  - `user-crud-service` — CRUD над таблицей пользователей;
  - PostgreSQL (несколько БД: user_crud_db, session_crud_db, chat_crud_db, results_crud_db, knowledge_base_crud_db, questions_crud_db);
  - Redis — кэширование активных сессий;
  - Kafka + Zookeeper для очередей;
  - Kafdrop — веб-интерфейс для просмотра Kafka (http://localhost:9000);
  - Prometheus, Grafana, Loki для мониторинга.

### Схема архитектуры backend

```text
                +-------------------------+
                |     React Frontend      |
                |  (браузер пользователя) |
                +------------+------------+
                             |
                             | HTTP /api/...
                             v
                    +--------+--------+
                    |    bff-service  |
                    |  (Gin, CORS)    |
                    +--------+--------+
                             |
        +---------------------+---------------------+
        |                     |                     |
        | HTTP /auth/...      | HTTP /sessions      | Kafka
        v                     v                     v
+-------+--------+   +--------+--------+   +--------+--------+
| auth-service   |   |session-service |   |  Kafka Broker  |
| (JWT, bcrypt)  |   |(session-mgr)   |   |  (очереди)     |
+-------+--------+   +--------+--------+   +--------+--------+
        |                     |                     |
        | HTTP /users...      | Redis (кэш)         | Kafka
        v                     |                     |
+-------+--------+            | HTTP                |
| user-crud-serv |            v                     |
| (GORM, PG)     |   +-----------------+            |
+-------+--------+   |session-crud-serv|            |
        |            |  (PostgreSQL)    |            |
        | SQL        +-----------------+            |
        v                     |                     |
  +-----+------+             | HTTP                 |
  | PostgreSQL |             v                      |
  +------------+   +-----------------+              |
        |          |chat-crud-service|              |
        |          |  (PostgreSQL)    |              |
        |          +-----------------+              |
        |                  |                        |
        |                  | HTTP                   |
        |                  v                        |
        |          +-----------------+              |
        |          |results-crud-serv|              |
        |          |  (PostgreSQL)    |              |
        |          +-----------------+              |
        |                  |                        |
        +------------------+                        |
                           |                        |
                    +------+------+                 |
                    | PostgreSQL  |                 |
                    | (3 БД)      |                 |
                    +-------------+                 |
                                                +-----+-----+
                                                | dialogue- |
                                                | aggregator|
                                                |  + agent  |
                                                |  + analyst|
                                                +-----------+
```

Кратко:

- фронтенд никогда не обращается напрямую к внутренним сервисам — только к `bff-service`;
- `auth-service` не имеет прямого доступа к PostgreSQL и использует `user-crud-service`;
- `user-crud-service` — единственная точка доступа к таблице `users`;
- `session-service` (session-manager) управляет жизненным циклом сессий, кэширует активные сессии в Redis, координирует создание программы интервью через Kafka с `interview-builder-service`;
- `session-crud-service`, `chat-crud-service`, `results-crud-service` — CRUD сервисы для персистентного хранения данных в отдельных PostgreSQL БД;
- `knowledge-base-crud-service`, `questions-crud-service` — CRUD сервисы для базы знаний и вопросов;
- `knowledge-producer-service` автоматически заполняет базы знаний и вопросов из JSON файлов при старте;
- `interview-builder-service` создаёт программу интервью через Kafka очереди (`interview.build.request/response`), запрашивая вопросы и знания из соответствующих CRUD сервисов;
- `bff-service` управляет чатами через `session-service`, получает историю из `chat-crud-service` и результаты из `results-crud-service`;
- `dialogue-aggregator` оркестрирует диалог: общается с `interviewer-agent-service` через `messages.full.data` / `generated.phrases`, не принимает решений сам, публикует `session.completed` для `analyst-agent-service`;
- `interviewer-agent-service` (LangGraph) ведёт интервью и возвращает реплики в `generated.phrases`;
- `analyst-agent-service` подписан на `session.completed` и формирует финальный `report_json` / обновляет `presets` или `user_topic_progress` в зависимости от `session_kind`;
- все сервисы конфигурируются через Viper (Go) или Pydantic Settings (Python) и запускаются в отдельных контейнерах.

### Мониторинг и логирование

Все микросервисы используют единую систему логирования и метрик:

- **Логирование**: Все логи выводятся в stdout в JSON-формате и собираются через Grafana Loki
- **Метрики**: Prometheus метрики доступны на `/metrics` endpoint каждого сервиса
- **Визуализация**: Grafana дашборды для просмотра логов и метрик
- **Kafka UI**: Kafdrop для просмотра сообщений в Kafka

**Доступ к инструментам мониторинга:**
- **Grafana**: http://localhost:3000 (логин: `admin`, пароль: `admin`)
- **Prometheus**: http://localhost:9090
- **Loki**: http://localhost:3100 (API)
- **Kafdrop**: http://localhost:9000 (Kafka UI)

Подробнее см.:
- [LOGGING.md](./LOGGING.md) — руководство по логированию
- [METRICS.md](./METRICS.md) — руководство по метрикам
- [KAFKA.md](./KAFKA.md) — информация об очередях Kafka

### Общие пакеты

В `common/` находятся переиспользуемые пакеты:
- `common/logger/go/` — единый логгер для Go-микросервисов
- `common/logger/python/` — единый логгер для Python-микросервисов
- `common/middleware/` — middleware для метрик и логирования

Подробные схемы и описание каждого сервиса см. в соответствующих `README.md`
в директориях:
- `auth-service`
- `user-crud-service`
- `bff-service`
- `session-service`
- `session-crud-service`
- `chat-crud-service`
- `results-crud-service`
- `interview-builder-service`
- `knowledge-base-crud-service`
- `questions-crud-service`
- `knowledge-producer-service`
- `dialogue-aggregator`
- `interviewer-agent-service`
- `analyst-agent-service`

Дополнительная документация:
- [WORKFLOW.md](./WORKFLOW.md) — полное описание workflow платформы
- [KAFKA.md](./KAFKA.md) — информация об очередях Kafka и форматах событий
