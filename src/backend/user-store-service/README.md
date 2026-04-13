## user-store-service — микросервис-хранилище пользователей

`user-store-service` — единственный сервис, который имеет прямой доступ к базе данных
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
          | user-store-service |
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

- `cmd/user-store-service/main.go`  
  Точка входа: загрузка конфигурации, сборка сервера, graceful shutdown.

- `internal/config`  
  Конфигурация через Viper (файл `config/config.yaml` + переменные окружения `USER_STORE_*`):
  - `server.host`, `server.port` — HTTP-сервер;
  - `database.host`, `database.port`, `database.user`, `database.password`, `database.name`, `database.ssl_mode` — подключение к PostgreSQL.

- `internal/models`  
  GORM-модель `User`:
  - `ID` (uint, внутренний PK);
  - `ExternalID` (UUID, внешний GUID для других микросервисов);
  - `Login` (уникальный логин);
  - `PasswordHash` (bcrypt-хеш пароля);
  - `CreatedAt`, `UpdatedAt` (timestamp, заполняются GORM).

- `internal/repository`  
  Репозиторий на основе GORM:
  - `Create`, `GetByExternalID`, `GetByLogin`, `Update`, `Delete`;
  - `List(login *string, limit, offset int)` — выборка пользователей с фильтром по логину и пагинацией;
  - аккуратная обработка ошибок PostgreSQL (конфликты по уникальному логину).

- `internal/service`  
  Бизнес-логика:
  - нормализация и валидация логина;
  - минимальная валидация хеша пароля (не пустой, без пробелов);
  - операции CRUD с пользователями;
  - `ListUsers` — обёртка над репозиторием с контролем `limit/offset`.

- `internal/handler`  
  HTTP-слой на Gin:
  - `POST /users` — создать пользователя (ожидает уже захешированный `password_hash`);
  - `GET /users/:id` — получить пользователя по GUID (`external_id`);
  - `GET /users/by-login/:login` — получить пользователя по логину;
  - `PUT /users/:id` — обновить логин и/или `password_hash`;
  - `DELETE /users/:id` — удалить пользователя;
  - `GET /debug/users?login=&limit=&offset=` — отладочный список пользователей с фильтрами и пагинацией (не для публичного использования, не проксируется через BFF).

- `internal/server`  
  Подключение к PostgreSQL через GORM, миграции, создание зависимостей, настройка Gin и health-check `GET /healthz`.

### Модель данных

Таблица `users` в PostgreSQL:

| Поле          | Тип        | Описание                                                    |
|---------------|-----------|-------------------------------------------------------------|
| id            | SERIAL PK | Внутренний идентификатор записи                             |
| external_id   | UUID      | Внешний GUID, который используют другие микросервисы       |
| login         | TEXT      | Уникальный логин                                           |
| password_hash | TEXT      | Хеш пароля (bcrypt), пароль в открытом виде не хранится    |
| created_at    | TIMESTAMP | Время создания записи                                      |
| updated_at    | TIMESTAMP | Время обновления записи                                    |

### Назначение и ограничения

- Только этот сервис может читать/записывать БД с логинами и паролями.
- `auth-service` обращается сюда по HTTP для создания и получения пользователей.
- Отладочный эндпоинт `/debug/users` нужен для диагностики и не должен быть доступен
  из внешнего мира (BFF его не проксирует).


