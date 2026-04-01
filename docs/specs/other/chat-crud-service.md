# Chat-crud-service Specification

## Обзор

**Назначение**: Хранение сообщений чата.

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- Сохранение сообщений от BFF
- Хранение полных дампов завершенных интервью

## БД

- chat_crud_db
- Table: messages (id, session_id, role, content, metadata JSONB, created_at)

## API

- POST /api/chat/messages
- GET /api/chat/sessions/{id}/messages

## Метрики

- Write latency (p95): < 20мс

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, DATABASE_URL).

**Секреты (Vault)**: DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Max writes: 500/минуту.

## Observability / Evals

**Метрики (Prometheus)**: db_writes_total, messages_saved_total.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: CRUD integration tests.
