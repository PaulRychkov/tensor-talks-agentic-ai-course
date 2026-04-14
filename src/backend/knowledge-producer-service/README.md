## knowledge-producer-service — сервис для заполнения и генерации баз знаний и вопросов

`knowledge-producer-service` — Python FastAPI сервис для:

1. Загрузки знаний и вопросов из файлов в CRUD сервисы (batch produce)
2. LLM-powered пайплайна: ingestion → structuring → dedup → draft (§9 p.5)
3. Web-поиска с allowlist/rate limiting для обогащения контента

### Архитектура

```
                                   ┌────────────────┐
  POST /ingest/url  ──►  Ingestion ──►  LLM Pipeline ──► Draft
  POST /drafts/knowledge                                     │
  GET  /search/web  ──►  WebSearch                           ▼
                                                    CRUD (publish)
```

### Функциональность

#### Batch produce (v1)
- Загрузка знаний из `common/knowledge/*.json`
- Загрузка вопросов из `common/questions/*.json`
- Проверка на дубликаты по ID и текстовому сходству
- Проверка версий для обновления

#### LLM Pipeline (v2, §9 p.5.6)
- 4-шаговый пайплайн: `gather_context` → `structure` → `check_duplicates` → `save_draft`
- Детерминированный fallback при отсутствии LLM или исчерпании лимита вызовов
- Ограничение: `MAX_LLM_CALLS_PER_JOB` (по умолчанию 5)

#### Ingestion (§9 p.5.8)
- Поддерживаемые форматы: `.md`, `.json`, `.pdf`, URL
- PDF через `pdfplumber` (опциональная зависимость)
- HTML → plain text конвертация для URL
- Ограничения: `MAX_INGEST_FILE_BYTES`, `MAX_FETCH_URL_BYTES`

#### Web Search (§9 p.5.9)
- Провайдеры: `arxiv` (бесплатный), `semantic_scholar` (бесплатный), `none`
- Allowlist/denylist по доменам (`TRUSTED_DOMAIN_PATTERNS`, `DENIED_DOMAIN_PATTERNS`)
- Rate limiting: `WEB_SEARCH_RATE_LIMIT_PER_MINUTE`
- Защита от перерасхода: `MAX_WEB_SEARCH_PER_JOB`

#### Pydantic-модели черновиков (§9 p.5.7)
- `KnowledgeDraftContent` — зеркало Go `KnowledgeJSONB`
- `QuestionDraftContent` — зеркало Go `QuestionJSONB`
- `Draft` — конверт для draft workflow с review_status
- `RawDocument` — промежуточное представление ingestion

### Эндпоинты

| Метод | Путь | Описание |
|-------|------|----------|
| `POST` | `/produce/knowledge` | Загрузить все знания из файлов |
| `POST` | `/produce/questions` | Загрузить все вопросы из файлов |
| `POST` | `/produce/all` | Загрузить всё |
| `POST` | `/drafts/knowledge` | Создать черновик знания |
| `GET` | `/drafts` | Список черновиков с фильтрами |
| `GET` | `/drafts/{id}` | Получить черновик |
| `PUT` | `/drafts/{id}/review` | Утвердить/отклонить черновик |
| `POST` | `/drafts/{id}/publish` | Опубликовать в CRUD |
| `POST` | `/ingest/url` | Ingestion из URL через LLM pipeline |
| `GET` | `/search/web` | Web-поиск по configured провайдеру |
| `GET` | `/healthz` | Health check |

### Конфигурация

#### Основные
| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `KNOWLEDGE_BASE_CRUD_URL` | `http://localhost:8090` | URL knowledge-base-crud |
| `QUESTIONS_CRUD_URL` | `http://localhost:8091` | URL questions-crud |
| `SERVER_PORT` | `8092` | Порт сервера |
| `AUTO_LOAD_ON_STARTUP` | `true` | Автозагрузка данных |

#### LLM Pipeline
| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `LLM_BASE_URL` | — | URL LLM API |
| `LLM_API_KEY` | — | API ключ |
| `LLM_MODEL` | `gpt-4` | Модель |
| `LLM_TEMPERATURE` | `0.3` | Температура |
| `MAX_LLM_CALLS_PER_JOB` | `5` | Макс. вызовов LLM за один job |

#### Ingestion
| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `MAX_INGEST_FILE_BYTES` | `10000000` | Макс. размер файла (10 MB) |
| `MAX_FETCH_URL_BYTES` | `5000000` | Макс. размер контента URL (5 MB) |
| `FETCH_URL_TIMEOUT` | `15` | Таймаут загрузки URL (сек) |

#### Web Search
| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `WEB_SEARCH_PROVIDER` | `none` | Провайдер: `arxiv`, `semantic_scholar`, `none` |
| `TRUSTED_DOMAIN_PATTERNS` | `arxiv.org,pytorch.org,...` | Allowlist доменов |
| `DENIED_DOMAIN_PATTERNS` | — | Denylist доменов |
| `WEB_SEARCH_RATE_LIMIT_PER_MINUTE` | `10` | Rate limit |
| `MAX_WEB_SEARCH_PER_JOB` | `3` | Макс. поисков за job |

### Быстрый старт

```bash
# Batch produce
curl -X POST http://localhost:8092/produce/all

# Ingest from URL
curl -X POST http://localhost:8092/ingest/url \
  -H "Content-Type: application/json" \
  -d '{"url": "https://pytorch.org/docs/stable/tensors.html", "topic": "pytorch_tensors"}'

# Web search
curl "http://localhost:8092/search/web?query=attention+mechanism&topic=transformers"
```

