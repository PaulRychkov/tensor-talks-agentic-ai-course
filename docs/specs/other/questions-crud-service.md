# Questions-crud-service Specification

## Обзор

**Назначение**: CRUD базы вопросов для интервью.

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- CRUD вопросов
- Фильтрация по сложности, теме, типу
- JSONB хранение

## БД

- questions_crud_db
- Table: questions (id, topic, difficulty, type, content JSONB, expected_answer JSONB)

## API

- GET /api/questions?topic=X&level=Y
- POST /api/questions
- PUT /api/questions/{id}
- DELETE /api/questions/{id}

## Метрики

- Search latency (p95): < 50мс

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, DATABASE_URL).

**Секреты (Vault)**: DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Max queries: 100/минуту.

## Observability / Evals

**Метрики (Prometheus)**: db_queries_total, search_latency.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: Search relevance tests, filter tests.
