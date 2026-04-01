# BFF Specification

## Обзор

**Назначение**: Backend-for-frontend, единая точка входа.

**Технологии**: Go 1.21+, Gin, Kafka, WebSocket.

## Функции

- Агрегация запросов к внутренним сервисам
- CORS настройка
- JWT валидация
- Маршрутизация к CRUD-сервисам
- WebSocket/polling для обновлений чата

## API

- POST /api/sessions — создать сессию
- GET /api/sessions/{id} — получить сессию
- POST /api/sessions/{id}/message — отправить сообщение
- GET /api/sessions/{id}/results — получить результаты

## Метрики

- HTTP requests (p95): < 100мс
- Error rate: target < 1%

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, KAFKA_BOOTSTRAP_SERVERS).

**Секреты (Vault)**: JWT_SECRET.

**Версии моделей**: Не применимо.

**Лимиты**: Rate limits: 1000 запросов/минуту.

## Observability / Evals

**Метрики (Prometheus)**: http_requests_total, http_request_duration, errors.

**Логи (Loki)**: JSON логи, уровень INFO.

**Трейсы (OpenTelemetry)**: HTTP запросы, Kafka сообщения.

**Evals**: Integration tests, API contract tests.
