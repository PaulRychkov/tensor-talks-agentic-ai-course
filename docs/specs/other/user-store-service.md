# User-store-service Specification

## Обзор

**Назначение**: Хранение данных пользователей.

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- CRUD пользователей
- Двойная идентификация (internal_id + external_uuid)
- Хранение хешей паролей

## БД

- user_store_db
- Table: users (id, external_id, email, password_hash, created_at)

## API

- GET /api/users/{id}
- POST /api/users
- PUT /api/users/{id}
- DELETE /api/users/{id}

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

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: DB integration tests, migration tests.
