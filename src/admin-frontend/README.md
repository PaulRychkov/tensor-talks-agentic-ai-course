# admin-frontend

Операторский веб-интерфейс для управления контентом TensorTalks (§10.1).

## Страницы

- **Dashboard** — сводка: количество pending-черновиков, последние загрузки
- **Загрузка материалов** — форма multipart upload + поле URL
- **Очередь черновиков** — таблица с Preview → Approve/Reject
- **Поиск по KB/Questions** — read-only поиск с фильтрами
- **Словари логинов** — CRUD прилагательных / существительных (§10.10)

## Запуск

```bash
cd admin-frontend
npm install
npm run dev
```

API: все запросы к `/admin/api/*` идут через `admin-bff-service` (порт 8096).

## Аутентификация

Требует JWT с `role: admin | content_editor` (§10.1).
Войти можно только через аккаунт с ролью оператора (создаётся через `POST /admin/api/operators`).
