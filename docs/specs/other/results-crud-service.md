# Results-crud-service Specification

## Обзор

**Назначение**: Хранение результатов интервью.

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- Сохранение оценок от агента-аналитика
- Хранение структурированных отчётов

## БД

- results_crud_db
- Table: evaluations (id, session_id, question_index, score, errors JSONB, feedback JSONB)
- Table: reports (id, session_id, overall_score, summary JSONB, plan JSONB, materials JSONB)

## API

- POST /api/results/evaluations
- POST /api/results/reports
- GET /api/results/sessions/{id}

## Метрики

- Write latency (p95): < 20мс

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, DATABASE_URL).

**Секреты (Vault)**: DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Max writes: 100/минуту.

## Observability / Evals

**Метрики (Prometheus)**: db_writes_total, reports_generated_total.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, DB queries.

**Evals**: Report generation tests, data integrity tests.
