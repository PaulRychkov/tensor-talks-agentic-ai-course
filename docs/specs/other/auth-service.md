# Auth-service Specification

## Обзор

**Назначение**: Аутентификация и авторизация пользователей.

**Технологии**: Go 1.21+, JWT.

## Функции

- Регистрация пользователей
- Логин/пароль аутентификация
- Выпуск access/refresh токенов
- Валидация токенов

## API

- POST /api/auth/register
- POST /api/auth/login
- POST /api/auth/refresh
- GET /api/auth/validate

## Метрики

- Auth latency (p95): < 50мс
- Error rate: target < 0.1%

## Serving / Config

**Запуск**: Docker (alpine), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (ENVIRONMENT, LOG_LEVEL, JWT_SECRET).

**Секреты (Vault)**: JWT_SECRET, DATABASE_PASSWORD.

**Версии моделей**: Не применимо.

**Лимиты**: Rate limits: 100 запросов/минуту на IP.

## Observability / Evals

**Метрики (Prometheus)**: http_requests_total, auth_attempts_total, errors.

**Логи (Loki)**: JSON логи, уровень INFO (без PII).

**Трейсы (OpenTelemetry)**: HTTP запросы.

**Evals**: Security tests, auth flow tests.
