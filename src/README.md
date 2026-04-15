# 🚀 TensorTalks

<div align="center">

**AI-симулятор технических ML-интервью для объективной оценки компетенций**

[![React](https://img.shields.io/badge/React-19.1.1-blue.svg)](https://reactjs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.9.3-blue.svg)](https://www.typescriptlang.org/)
[![Vite](https://img.shields.io/badge/Vite-7.1.7-646CFF.svg)](https://vitejs.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-4.1.14-38B2AC.svg)](https://tailwindcss.com/)

[🌐 Демо](https://www.tensor-talks.ru/) • [📧 Связаться с нами](#контакты) • [🎯 MVP 2025](#статус-разработки)

</div>

---

## 📋 О проекте

**TensorTalks** — это AI-симулятор технических ML-интервью для объективной оценки компетенций. Платформа моделирует реальные собеседования с AI-интервьюером, анализирует ответы, выявляет слабые стороны и предлагает индивидуальные рекомендации.

### 🎯 Основная ценность

TensorTalks помогает ML-специалистам и компаниям объективно оценивать и развивать технические компетенции в области машинного обучения через:
- 🤖 **Реалистичные AI-симуляции** интервью уровня FAANG и топ-стартапов
- 📊 **Персонализированный разбор** ответов и ошибок от AI-интервьюера
- 📈 **Аналитику сильных и слабых сторон** с конкретными рекомендациями
- 🎯 **Стандартизацию процесса** найма и развития для HR и TechLead

---

## 🎨 Возможности

### Интерфейс
- Современный дизайн с градиентами и анимациями
- Адаптивная верстка для всех устройств
- Интуитивная навигация

### Функциональность
- **База вопросов** — алгоритмы, ML-задачи, практические кейсы по уровням сложности от Junior до Senior
- **AI-разбор ошибок** — детальная обратная связь с объяснением ошибок и рекомендациями по улучшению
- **Стандартизация оценки** — единый формат оценки для HR и техлидов с объективными критериями
- **Персонализация** — индивидуальные планы развития на основе выявленных пробелов

---

## 🧱 Архитектура и технологии

### Общая схема

- **Frontend (React микросервис)**  
  Одностраничное приложение на React/TypeScript/Vite, отрисовывающее лендинг, экран аутентификации
  и пользовательские страницы. Общается только с BFF по HTTP.

- **BFF (bff-service)**  
  Backend-for-frontend на Go (Gin), который:
  - предоставляет REST API для фронтенда (`/api/...`);
  - проксирует запросы аутентификации в `auth-service`;
  - настраивает CORS и служит единственной точкой входа для браузера.

- **Auth (auth-service)**  
  Микросервис аутентификации на Go, отвечающий за:
  - регистрацию и логин по логину/паролю;
  - выпуск и валидацию JWT (access/refresh);
  - работу только через `user-crud-service` без прямого доступа к БД.

- **User Store (user-crud-service)**  
  Микросервис-хранилище пользователей на Go с PostgreSQL:
  - единственный сервис с прямым доступом к БД логинов/паролей;
  - предоставляет CRUD API и отладочный эндпоинт `/debug/users` (фильтр+пагинация);
  - хранит пользователей с внутренним int PK и внешним GUID (external_id).

- **Session Service (session-service)**  
  Микросервис управления сессиями (session-manager):
  - создаёт сессии с параметрами (topics, level, type, mode ∈ interview|training|study);
  - кэширует активные сессии в Redis;
  - координирует создание программы через Kafka;
  - отдаёт `session_mode`, `topics`, `level` в `GET /sessions/:id/program` для downstream-сервисов.

- **Session CRUD Service (session-crud-service)**  
  CRUD микросервис для сессий в PostgreSQL:
  - хранит информацию о сессиях, параметры интервью и программы;
  - предоставляет API для работы с сессиями.

- **Chat CRUD Service (chat-crud-service)**  
  CRUD микросервис для чатов и сообщений в PostgreSQL:
  - хранит все сообщения чатов и дампы завершенных чатов;
  - предоставляет API для сохранения и получения истории чатов.

- **Results CRUD Service (results-crud-service)**  
  CRUD микросервис для результатов, пресетов и прогресса в PostgreSQL:
  - хранит результаты любых сессий (interview/training/study) с `session_kind`, `report_json`, `evaluations`;
  - управляет `presets` (рекомендованные программы после interview);
  - управляет `user_topic_progress` (прогресс по темам после study).

- **Interview Builder Service (interview-builder-service)**  
  Python FastAPI сервис для динамического создания программы интервью:
  - слушает Kafka топик `interview.build.request`;
  - получает параметры интервью (topics, level, type);
  - запрашивает вопросы из questions-crud-service по фильтрам;
  - запрашивает знания из knowledge-base-crud-service для каждого вопроса;
  - собирает программу интервью (5 вопросов по умолчанию);
  - упорядочивает вопросы по логике;
  - отправляет программу в Kafka топик `interview.build.response`.

- **Knowledge Base CRUD Service (knowledge-base-crud-service)**  
  Go микросервис для работы с базой знаний в PostgreSQL:
  - CRUD операции над знаниями;
  - поиск знаний по фильтрам (complexity, concept, parent_id, tags);
  - хранение структурированных знаний в JSONB формате.

- **Questions CRUD Service (questions-crud-service)**  
  Go микросервис для работы с базой вопросов в PostgreSQL:
  - CRUD операции над вопросами;
  - поиск вопросов по фильтрам (complexity, theory_id, question_type);
  - хранение структурированных вопросов в JSONB формате.

- **Knowledge Producer Service (knowledge-producer-service)**  
  Python FastAPI сервис для наполнения баз знаний и вопросов с HITL workflow:
  - ингест markdown / JSON / PDF / URL и web-search (`arxiv`, `Semantic Scholar`);
  - 4-шаговый пайплайн `gather_context → structure → check_duplicates → save_draft`;
  - дедупликация черновиков по similarity;
  - HITL `draft → review → publish`, в `knowledge-base-crud-service` / `questions-crud-service` уходят только опубликованные сущности.

- **Dialogue Aggregator (dialogue-aggregator)**  
  Go-оркестратор диалога (бывший `mock-model-service`):
  - читает `chat.events.out`, общается с `interviewer-agent-service` через `messages.full.data` / `generated.phrases`, отдаёт реплики в `chat.events.in`;
  - сам бизнес-решений не принимает (`next` / `hint` / `clarify` / `skip` / `complete` приходят от `interviewer-agent-service`);
  - получает программу + метаданные (`session_mode`, `topics`, `level`) от `session-service`;
  - публикует `session.completed` в Kafka для `analyst-agent-service` с реальными `session_kind` / `topics` / `level`.

- **Interviewer Agent Service (interviewer-agent-service)**  
  Python LangGraph-сервис, ведущий интервью:
  - принимает контекст из `messages.full.data`, прогоняет граф (классификация → оценка → решение);
  - возвращает реплику и решение (`next` / `hint` / `clarify` / `skip` / `complete`) в `generated.phrases`;
  - хранит `question_evaluations` в Redis, ходит в `session-crud` / `chat-crud` / `knowledge-base-crud` за контекстом.

- **Analyst Agent Service (analyst-agent-service)**  
  Python LangGraph-сервис для формирования финального отчёта:
  - подписан на `session.completed`, по `session_kind` уходит в одну из веток:
    - `interview` → формирует `report_json`, обновляет `results` и создаёт запись в `presets`;
    - `training` → обновляет `results` итоговой оценкой;
    - `study` → обновляет `user_topic_progress` для пройденных тем.

- **PostgreSQL**  
  Хранит данные в нескольких базах:
  - `user_crud_db` — таблица `users` с полями `id`, `external_id` (UUID), `login`, `password_hash`;
  - `session_crud_db` — таблица `sessions` с информацией о сессиях и программах интервью;
  - `chat_crud_db` — таблицы `messages` и `chat_dumps` для истории чатов;
  - `results_crud_db` — таблицы `results` (с `report_json`, `evaluations`, `session_kind`, `result_format_version`), `presets`, `user_topic_progress`;
  - `knowledge_base_crud_db` — таблица `knowledge` для структурированных знаний;
  - `questions_crud_db` — таблица `questions` для вопросов интервью.

- **Redis**  
  Кэширование активных сессий для быстрого доступа к программам интервью.

- **Kafka**  
  Очереди для асинхронной обработки событий. Развертывается через Strimzi Operator в namespace `tensor-talks`:
  - `tensor-talks-chat.events.out` — события от BFF к dialogue-aggregator (старт чата, сообщения пользователя, terminate);
  - `tensor-talks-chat.events.in` — события от dialogue-aggregator к BFF (вопросы, completed);
  - `tensor-talks-messages.full.data` — полный контекст диалога (dialogue-aggregator → interviewer-agent-service);
  - `tensor-talks-generated.phrases` — реплика/решение (interviewer-agent-service → dialogue-aggregator);
  - `tensor-talks-interview.build.request` — запрос на создание программы интервью (session-service → interview-builder);
  - `tensor-talks-interview.build.response` — ответ с готовой программой и `program_meta`;
  - `tensor-talks-session.completed` — событие завершения сессии (dialogue-aggregator → analyst-agent-service) с `session_kind`, `topics`, `level`;
  - Kafka UI (Kafdrop) доступен для мониторинга топиков и сообщений.

### Стек backend

- Языки: **Go** (основные сервисы), **Python** (FastAPI для interview-builder и knowledge-producer)
- HTTP: **Gin** (Go), **FastAPI** (Python)
- ORM: **GORM** (Go)
- Конфигурация: **Viper** (Go), **Pydantic Settings** (Python)
- Логирование: **Zap** (Go), **structlog** (Python)
- Тестирование: **Testify** (Go)
- БД: **PostgreSQL** (JSONB для гибких схем)
- Очереди: **Kafka** (через Strimzi Operator, KRaft режим без Zookeeper)
- Мониторинг: **Prometheus**, **Grafana** (развертываются в namespace), **Kafdrop** (Kafka UI)
- Секреты: **Vault** (через External Secrets Operator)
- Контейнеризация: **Docker**, `docker-compose`
- Оркестрация: **Kubernetes**, **Helm**

Подробные схемы и описание каждого микросервиса см. в `backend/README.md`
и в отдельных `README.md` внутри `backend/auth-service`, `backend/user-crud-service`,
`backend/bff-service`, `backend/session-service`, `backend/interviewer-agent-service`, 
а также `frontend/README.md` для фронтенда.

---

## 🚀 Быстрый старт

### Локальная разработка (docker-compose)

```bash
# Клонируйте репозиторий
git clone https://github.com/PaulRychkov/TensorTalks.git
cd TensorTalks

# Запустите все сервисы
docker-compose up -d

# Откройте в браузере
# Frontend: http://localhost:5173
# API: http://localhost:8080
# Grafana: http://localhost:3000 (логин/пароль из Vault: tensor-talks/grafana)
```

### Развертывание в Kubernetes

См. подробные инструкции:
- [INSTALLATION_MANUAL.md](INSTALLATION_MANUAL.md) - установка и развертывание
- [DEVOPS_DEPLOY.md](DEVOPS_DEPLOY.md) - DevOps архитектура и CI/CD пайплайн

### Frontend отдельно

```bash
# Клонируйте репозиторий
git clone https://github.com/PaulRychkov/TensorTalks.git
cd TensorTalks/frontend

# Установите зависимости
npm install

# Запустите dev сервер
npm run dev
```

Откройте [http://localhost:5173](http://localhost:5173) в браузере.

---

## 🏗️ Технологический стек (кратко)

- **Frontend**: React, TypeScript, Vite, Tailwind CSS, React Router.
- **Backend**: Go (Gin, GORM, Viper, Testify), Python (FastAPI, LangChain, LangGraph), PostgreSQL.
- **Очереди**: Kafka (KRaft, через Strimzi Operator) для асинхронной обработки событий.
- **Мониторинг**: Prometheus (метрики), Grafana (визуализация), Loki (логи), Kafdrop (Kafka UI).
- **Инфраструктура**: Docker, docker-compose, Nginx (для фронтенда в контейнере).

---

## 🛠️ Структура проекта

```
frontend/
├── src/
│   ├── components/          # Компоненты
│   ├── pages/              # Страницы (лендинг, дашборд, чат, результаты)
│   ├── App.tsx             # Главная страница (лендинг)
│   └── main.tsx            # Точка входа
└── package.json           # Зависимости
```

---

## 🚧 Статус разработки

**Текущий этап**: MVP готов, interviewer-agent-service реализован, идет пилотная апробация

### ✅ Готово
- [x] Пользовательский интерфейс и навигация
- [x] Адаптивный дизайн с современными градиентами
- [x] Backend API и аутентификация (регистрация, логин, JWT)
- [x] MVP логика чатов (создание сессий, отправка сообщений)
- [x] Интеграция с Kafka для асинхронной обработки событий
- [x] AI-агент для проведения интервью (interviewer-agent-service на LangGraph)
- [x] Мониторинг (Prometheus, Grafana, Kafdrop) - развертывается в namespace
- [x] Собственный Kafka кластер через Strimzi Operator
- [x] Интеграция с Vault для управления секретами
- [x] Логирование и метрики во всех микросервисах
- [x] Хранение истории чатов и сессий (CRUD сервисы для чатов, результатов и сессий)
- [x] Управление сессиями с Redis кэшированием
- [x] Система создания программы интервью через Kafka
- [x] Просмотр истории завершенных интервью и результатов
- [x] Динамическая генерация программ интервью на основе уровня и темы
- [x] База знаний и вопросов с CRUD сервисами
- [x] Автоматическое заполнение баз при старте
- [x] Фильтрация вопросов по сложности и темам

### 🔄 В разработке
- [ ] WebSocket интеграция для real-time обновлений
- [ ] Расширенная база вопросов уровня FAANG
- [ ] Интеграции с HR-системами
- [ ] Голосовой интерфейс (ASR/TTS)
- [ ] Расширенная агентная система (course/agents-extension-plan.md)

**Текущий статус**: MVP 2025, пилотная апробация 2026

---

## 📚 Документация

- [INSTALLATION_MANUAL.md](INSTALLATION_MANUAL.md) — установка и развертывание
- [DEVOPS_DEPLOY.md](DEVOPS_DEPLOY.md) — DevOps архитектура и CI/CD
- [UI_ENDPOINTS.md](UI_ENDPOINTS.md) — все UI эндпоинты (Grafana, Prometheus, Kafdrop, pgAdmin, ArgoCD, Vault)
- [helm/README.md](helm/README.md) — Helm chart
- [argocd/README.md](argocd/README.md) — GitOps через ArgoCD
- [backend/README.md](backend/README.md) — Backend архитектура
- [backend/WORKFLOW.md](backend/WORKFLOW.md) — Workflow платформы
- [backend/KAFKA.md](backend/KAFKA.md) — Kafka топики и форматы событий
- [backend/METRICS.md](backend/METRICS.md) — Prometheus метрики
- [backend/LOGGING.md](backend/LOGGING.md) — Логирование

---

## ВКР и курс по агентам

Проект является выпускной квалификационной работой. В рамках курса по агентам делается PoC - агентная система поверх существующего функционала.

### Что покажем на демо

1. Агент-планировщик собирает программу интервью под заданные параметры.
2. Агент-интервьюер проводит интервью из 5 вопросов с уточнениями и подсказками, сам выбирает действия на основе контекста и инструментов (оценка ответа, поиск теории в KB, web search).
3. Агент-аналитик формирует структурированный отчёт: оценка, ошибки по темам, подобранные материалы, план подготовки.

### Что не входит в PoC

- Production-инфраструктура для evals
- Интеграции в HR-системы (ATS, SSO)
- WebSocket (работаем через polling)
- MCP-интеграции (Google Scholar, arxiv)

Документы курса в `course/`.

---

## 📞 Контакты

- **Сайт**: [tensor-talks.ru](https://www.tensor-talks.ru/)
- **Email**: contact@tensor-talks.ru

---

## 🎯 Целевые группы

### 👨‍💻 ML-специалисты (Middle-to-Senior)
- Объективная оценка своего уровня
- Практика в условиях реального интервью
- Структурированная обратная связь

### 🌱 Новички в ML
- Понять требования к ML-интервью
- Получить структурированный план подготовки
- Наставничество и гайды

### 🏢 Компании (Hiring)
- Стандартизация процесса оценки
- Экономия времени интервьюеров
- Объективные критерии отбора

### 📚 Компании (L&D)
- Системная диагностика навыков
- Отслеживание прогресса команды
- Целенаправленное развитие

---

<div align="center">

**Сделано с ❤️ для ML-сообщества**

</div>