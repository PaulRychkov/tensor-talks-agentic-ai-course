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

- POST /api/chat/start — начать сессию чата (topics, level, type, mode)
- POST /api/chat/message — отправить сообщение
- GET /api/chat/history/:session_id — история сообщений
- POST /api/chat/terminate — досрочное завершение
- POST /api/chat/resume — возобновление сессии
- GET /api/results/sessions/:session_id — получить результаты

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
