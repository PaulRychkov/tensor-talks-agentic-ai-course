# Session-service Specification

## Обзор

**Назначение**: Управление жизненным циклом сессии интервью.

**Технологии**: Go 1.21+, Redis, Kafka.

## Функции

- Создание новой сессии
- Кеширование в Redis (TTL: 2 часа)
- Публикация запросов на формирование программы (Kafka)
- Координация с session-crud-service

## API

- POST /api/sessions — создать
- GET /api/sessions/{id} — получить
- PUT /api/sessions/{id} — обновить

## Метрики

- Session create latency (p95): < 100мс
- Redis hit rate: target ≥ 95%

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, REDIS_HOST, KAFKA_BOOTSTRAP_SERVERS).

**Секреты (Vault)**: Нет.

**Версии моделей**: Не применимо.

**Лимиты**: Redis TTL: 2 часа.

## Observability / Evals

**Метрики (Prometheus)**: sessions_created_total, redis_hits_total, redis_misses_total.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, Redis операции, Kafka сообщения.

**Evals**: Session lifecycle tests, cache tests.
