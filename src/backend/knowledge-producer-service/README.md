## knowledge-producer-service — сервис для заполнения баз знаний и вопросов

`knowledge-producer-service` — Python FastAPI сервис для загрузки знаний и вопросов из JSON файлов в CRUD сервисы.

### Функциональность

- Загрузка знаний из `common/knowledge/*.json`
- Загрузка вопросов из `common/questions/*.json`
- Проверка на дубликаты по ID
- Проверка версий для обновления
- Сохранение в knowledge-base-crud-service и questions-crud-service

### Эндпоинты

- `POST /produce/knowledge` — загрузить все знания из файлов
- `POST /produce/questions` — загрузить все вопросы из файлов
- `POST /produce/all` — загрузить всё
- `GET /healthz` — health check

### Конфигурация

- `KNOWLEDGE_BASE_CRUD_URL` — URL knowledge-base-crud-service (по умолчанию http://localhost:8090)
- `QUESTIONS_CRUD_URL` — URL questions-crud-service (по умолчанию http://localhost:8091)
- `KNOWLEDGE_DATA_PATH` — путь к папке с JSON знаниями (по умолчанию /app/data/knowledge)
- `QUESTIONS_DATA_PATH` — путь к папке с JSON вопросами (по умолчанию /app/data/questions)
- `SERVER_HOST` — хост сервера (по умолчанию 0.0.0.0)
- `SERVER_PORT` — порт сервера (по умолчанию 8092)

### Автоматическая загрузка при старте

По умолчанию сервис автоматически загружает все данные при старте (`AUTO_LOAD_ON_STARTUP=true`). 
Сервис будет ждать готовности CRUD сервисов с повторными попытками.

### Ручная загрузка

Для ручной загрузки данных вызовите:
```bash
curl -X POST http://localhost:8092/produce/all
```

Сервис проверит все JSON файлы, сравнит версии и создаст/обновит записи в базах данных.

### Конфигурация автозагрузки

- `AUTO_LOAD_ON_STARTUP` — включить автозагрузку при старте (по умолчанию true)
- `STARTUP_LOAD_RETRY_DELAY` — задержка между попытками в секундах (по умолчанию 5)
- `STARTUP_LOAD_MAX_RETRIES` — максимальное количество попыток (по умолчанию 10)

