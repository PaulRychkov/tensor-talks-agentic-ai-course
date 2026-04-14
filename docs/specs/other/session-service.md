# Session-service Specification

## Обзор

**Назначение**: Управление жизненным циклом сессии (interview / training / study).

**Технологии**: Go 1.21+, Redis, Kafka.

## Функции

- Создание новой сессии с параметрами (topics, level, type, mode)
- Кеширование в Redis (TTL: 2 часа)
- Публикация запросов на формирование программы (Kafka)
- Координация с session-crud-service
- Возврат программы и метаданных сессии (`session_mode`, `topics`, `level`) по `GET /sessions/{id}/program`

## API

- POST /api/sessions — создать (topics, level, type, mode)
- GET /api/sessions/{id} — получить
- GET /api/sessions/{id}/program — получить программу и метаданные
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
