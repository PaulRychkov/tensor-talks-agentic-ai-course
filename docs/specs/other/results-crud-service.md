# Results-crud-service Specification

## Обзор

**Назначение**: Хранение результатов сессий (interview / training / study).

**Технологии**: Go 1.21+, GORM, PostgreSQL.

## Функции

- Сохранение оценок от агента-аналитика
- Хранение структурированных отчётов (`report_json`)
- Хранение пресетов тренировок (`presets` — слабые темы, рекомендованные материалы)
- Хранение прогресса по темам (`user_topic_progress`)
- Маршрутизация по `session_kind`: interview → report + presets, training → результаты, study → user_topic_progress

## БД

- results_crud_db
- Table: evaluations (id, session_id, session_kind, question_index, score, errors JSONB, feedback JSONB)
- Table: reports (id, session_id, overall_score, report_json JSONB, summary JSONB, plan JSONB, materials JSONB)
- Table: presets (id, session_id, weak_topics JSONB, recommended_materials JSONB)
- Table: user_topic_progress (id, user_id, topic, level, progress JSONB)

## API

- POST /api/results/evaluations
- POST /api/results/reports
- GET /api/results/sessions/{id}
- POST /api/results/presets
- GET /api/results/presets/{session_id}
- POST /api/results/user-topic-progress
- GET /api/results/user-topic-progress/{user_id}

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
