# Frontend Specification

## Обзор

**Назначение**: Пользовательский интерфейс платформы.

**Технологии**: React 19, TypeScript 5, Vite, Tailwind CSS.

## Запуск

**Контейнеризация**:
- Docker (Nginx для раздачи статики)
- Многоступенчатая сборка (build → serve)


## Компоненты

- Страница аутентификации (регистрация, вход)
- Дашборд (выбор параметров интервью, история)
- Чат-интерфейс (проведение интервью)
- Страница результатов (отчёт, рекомендации)
- История интервью

## API

- BFF: HTTP REST + polling для обновлений чата

## Метрики

- Page load time (p95): < 3 секунд
- First contentful paint: < 1.5 секунд

## Serving / Config

**Запуск**: Docker (Nginx для статики), Kubernetes, Helm, ArgoCD.

**Конфигурация**: Переменные окружения (VITE_API_URL, VITE_WS_URL).

**Секреты (Vault)**: Нет (frontend не хранит секреты).

**Версии моделей**: Не применимо.

**Лимиты**: Max concurrent connections: 1000.

## Observability / Evals

**Метрики (Prometheus)**: page_load_duration, first_contentful_paint, errors.

**Логи (Loki)**: Browser console errors (отправка в Loki).

**Трейсы (OpenTelemetry)**: Web Vitals, HTTP запросы к BFF.

**Evals**: UX тесты, accessibility checks.
