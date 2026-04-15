# User-crud-service Specification

## Обзор

**Назначение**: Хранение данных пользователей (ранее назывался `user-store-service`).

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- CRUD пользователей
- Автогенерация логина (`{Прилагательное}{Существительное}{Число}`, напр. `BrightNeural42`) — без email/PII для соответствия 152-ФЗ
- Recovery key для восстановления аккаунта
- Двойная идентификация (internal_id + external_uuid)
- Хранение хешей паролей (bcrypt)

## БД

- `user_crud_db`
- Table: `users` (id, external_id, username, password_hash, recovery_key_hash, created_at)

## API

- GET /api/users/{id}
- POST /api/users
- PUT /api/users/{id}
- DELETE /api/users/{id}
- POST /api/users/recover (восстановление по recovery key)

## Метрики

- Query latency (p95): < 10мс
- Connection pool usage: target < 80%

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, DATABASE_URL).

**Секреты (Vault)**: DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Connection pool: 20 connections.

## Observability / Evals

**Метрики (Prometheus)**: db_queries_total, query_duration, connection_pool_usage.

**Логи (Loki)**: JSON логи, уровень INFO; PII не логируется.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: DB integration tests, migration tests.
