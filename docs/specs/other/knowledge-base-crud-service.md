# Knowledge-base-crud-service Specification

## Обзор

**Назначение**: CRUD базы знаний по ML.

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- CRUD операций с концепциями
- Поиск по фильтрам (сложность, тема, теги)
- JSONB хранение

## БД

- knowledge_base_crud_db
- Table: knowledge_items (id, concept, difficulty, description JSONB, formulas JSONB, tags)

## API

- GET /api/knowledge?topic=X&difficulty=Y
- POST /api/knowledge
- PUT /api/knowledge/{id}
- DELETE /api/knowledge/{id}

## Метрики

- Search latency (p95): < 50мс

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, DATABASE_URL).

**Секреты (Vault)**: DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Max queries: 100/минуту.

## Observability / Evals

**Метрики (Prometheus)**: db_queries_total, search_latency, cache_hits.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: Search relevance tests, filter tests.
