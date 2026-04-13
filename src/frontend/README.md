## Frontend (React микросервис)

Этот каталог содержит фронтенд-микросервис TensorTalks, реализованный на **React + TypeScript + Vite**.
Фронт выступает отдельным микросервисом, который:

- отрисовывает лендинг, форму регистрации/логина и пользовательские страницы;
- общается только с `bff-service` по HTTP (`/api/...`);
- не обращается напрямую ни к `auth-service`, ни к `user-store-service`, ни к БД.

### Архитектура фронтенда

- `src/App.tsx` — публичный лендинг с описанием продукта, CTA и маркетинговыми секциями.
- `src/pages/Auth.tsx` — страница регистрации/логина:
  - валидация логина/пароля на клиенте;
  - запросы к `BFF /api/auth/register` и `/api/auth/login`;
  - сохранение пользователя и токенов в `localStorage` (ключи `tt_user`, `tt_tokens`).
- `src/pages/Dashboard.tsx`, `Chat.tsx`, `Results.tsx` — будущие функциональные страницы продукта.
- `src/services/auth.ts` — тонкий клиент для работы с BFF:
  - читает `VITE_API_BASE_URL` и формирует базовый URL;
  - инкапсулирует `fetch` и обработку ошибок.

### Схема взаимодействия фронта с backend

```text
+-----------------------------+         +------------------+        +------------------+
|        React Frontend       |  HTTP   |    bff-service   |  HTTP  |   auth-service   |
|  / (лендинг), /auth, ...    +-------->+  /api/auth/...   +------->+  /auth/...       |
|  Auth.tsx, auth.ts (client) |         |  CORS, JSON      |        |  JWT, bcrypt     |
+-----------------------------+         +---------+--------+        +---------+--------+
                                              |                            |
                                              | HTTP /users...             |
                                              v                            v
                                         +----+----------+         +-------+-------+
                                         | user-store    |  SQL    |  PostgreSQL   |
                                         |  - users      +-------->+  (таблица     |
                                         |    (GUID, PW) |         |   users)      |
                                         +---------------+         +---------------+
```

Основные моменты:

- фронтенд никогда не ходит напрямую в `auth-service`/`user-store-service` — только в `bff-service`;
- после успешной регистрации/логина фронт сохраняет пользователя и токены в `localStorage`,
  чтобы использовать их в будущих запросах (например, `GET /api/auth/me` для получения текущего пользователя);
- в будущем при вызове защищённых API фронт будет добавлять заголовок `Authorization: Bearer <access_token>`.

### Технологии

- **React** + **TypeScript** — UI и типизация.
- **Vite** — сборка и dev-сервер.
- **Tailwind CSS** — быстрая стилизация.
- **React Router** — маршрутизация между страницами (`/`, `/auth`, `/dashboard`, и т.д.).

### Взаимодействие с backend

- Все запросы идут на BFF (`/api/...` с точки зрения браузера).
- Примеры:
  - регистрация: `POST /api/auth/register { login, password }`;
  - логин: `POST /api/auth/login { login, password }`;
  - позже: `GET /api/auth/me` для получения текущего пользователя.
- BFF дальше проксирует запросы в `auth-service`, который уже работает с `user-store-service`.

### Локальный запуск

```bash
cd frontend
npm install
npm run dev
```

По умолчанию фронтенд ожидает BFF по пути `/api`. В docker-compose Nginx проксирует запросы
к BFF автоматически, в режиме разработки можно настроить прокси Vite или запускать BFF локально
на `http://localhost:8080` и выставить `VITE_API_BASE_URL=http://localhost:8080/api`.
