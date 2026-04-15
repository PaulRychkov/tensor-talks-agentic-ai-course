## user-crud-service — микросервис-хранилище пользователей

`user-crud-service` — единственный сервис, который имеет прямой доступ к базе данных
с логинами и хешами паролей. Все остальные сервисы (включая `auth-service`) работают с этой БД
только через HTTP API данного микросервиса.

### Архитектура (общая схема)

```text
          +--------------------+
          |    auth-service    |
          |  HTTP /users/...   |
          +---------+----------+
                    |
                    | HTTP (JSON)
                    v
          +---------+----------+
          | user-crud-service |
          |  Gin + GORM + PG   |
          +---------+----------+
                    |
                    | SQL (GORM)
                    v
               +----+-----+
               |PostgreSQL|
               +----------+
```

### Архитектура модулей

- `cmd/user-crud-service/main.go`  
  Точка входа: загрузка конфигурации, сборка сервера, graceful shutdown.

- `internal/config`  
  Конфигурация через Viper (файл `config/config.yaml` + переменные окружения):
  - `server.host`, `server.port` — HTTP-сервер;
  - `database.host`, `database.port`, `database.user`, `database.password`, `database.name`, `database.ssl_mode` — подключение к PostgreSQL.

- `internal/models`  
  GORM-модель `User`:
  - `ID` (uint, внутренний PK);
  - `ExternalID` (UUID, внешний GUID для других микросервисов);
  - `Login` (уникальный логин);
  - `PasswordHash` (bcrypt-хеш пароля);
  - `RecoveryKeyHash` (*string, bcrypt-хеш ключа восстановления, nullable);
  - `CreatedAt`, `UpdatedAt` (timestamp, заполняются GORM).

- `internal/repository`  
  Репозиторий на основе GORM:
  - `Create`, `GetByExternalID`, `GetByLogin`, `Update`, `Delete`;
  - `SetRecoveryKeyHash(externalID, hash)` — обновление хеша ключа восстановления;
  - `List(login *string, limit, offset int)` — выборка пользователей с фильтром по логину и пагинацией;
  - аккуратная обработка ошибок PostgreSQL (конфликты по уникальному логину).

- `internal/service`  
  Бизнес-логика:
  - нормализация и валидация логина;
  - минимальная валидация хеша пароля (не пустой, без пробелов);
  - операции CRUD с пользователями;
  - `SetRecoveryKeyHash`, `UpdatePasswordHash` — точечное обновление полей безопасности;
  - `ListUsers` — обёртка над репозиторием с контролем `limit/offset`;
  - `GenerateRandomLogin` — выбирает случайное прилагательное + существительное + цифру из БД (`login_words`).

- `internal/handler`  
  HTTP-слой на Gin:
  - `POST /users` — создать пользователя (ожидает уже захешированный `password_hash`);
  - `GET /users/:id` — получить пользователя по GUID (`external_id`);
  - `GET /users/by-login/:login` — получить пользователя по логину;
  - `PUT /users/:id` — обновить логин и/или `password_hash`;
  - `DELETE /users/:id` — удалить пользователя;
  - `PATCH /users/:id/recovery-key-hash` — обновить хеш ключа восстановления;
  - `GET /login-words/adjectives` — список активных прилагательных;
  - `GET /login-words/nouns` — список активных существительных;
  - `POST /login-words/generate-random` — сгенерировать случайный логин (`{"login": "BraveNeural42"}`);
  - `POST /login-words/check-availability` — проверить доступность логина;
  - `GET /debug/users?login=&limit=&offset=` — отладочный список пользователей (не для публичного использования, не проксируется через BFF).

- `internal/server`  
  Подключение к PostgreSQL через GORM, миграции, создание зависимостей, настройка Gin и health-check `GET /healthz`.  
  При старте выполняет идемпотентное наполнение таблицы `login_words` (50 прилагательных + 96 существительных из ML-тематики).

### Модель данных

Таблица `users` в PostgreSQL:

| Поле          | Тип        | Описание                                                    |
|---------------|-----------|-------------------------------------------------------------|
| id            | SERIAL PK | Внутренний идентификатор записи                             |
| external_id   | UUID      | Внешний GUID, который используют другие микросервисы       |
| login              | TEXT      | Уникальный логин                                           |
| password_hash      | TEXT      | Хеш пароля (bcrypt), пароль в открытом виде не хранится    |
| recovery_key_hash  | TEXT NULL | Хеш ключа восстановления (bcrypt), nullable                |
| created_at         | TIMESTAMP | Время создания записи                                      |
| updated_at         | TIMESTAMP | Время обновления записи                                    |

### Назначение и ограничения

- Только этот сервис может читать/записывать БД с логинами и паролями.
- `auth-service` обращается сюда по HTTP для создания и получения пользователей.
- Отладочный эндпоинт `/debug/users` нужен для диагностики и не должен быть доступен
  из внешнего мира (BFF его не проксирует).


