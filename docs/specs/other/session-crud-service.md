# Session-crud-service Specification

## Обзор

**Назначение**: Персистентное хранение сессий.

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- Сохранение параметров интервью
- Хранение сформированных программ

## БД

- session_crud_db
- Table: sessions (id, user_id, topic, level, program JSONB, status, created_at)

## API

- CRUD для сессий

## Метрики

- Write latency (p95): < 20мс

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, DATABASE_URL).

**Секреты (Vault)**: DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Max connections: 10.

## Observability / Evals

**Метрики (Prometheus)**: db_writes_total, write_duration.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: CRUD integration tests.
