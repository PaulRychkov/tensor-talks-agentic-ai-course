# DevOps и Развертывание TensorTalks

Полная документация по CI/CD, развертыванию и эксплуатации платформы TensorTalks.

## 📋 Оглавление

1. [Общая архитектура](#общая-архитектура)
2. [Инфраструктура](#инфраструктура)
   - [Kubernetes кластер](#kubernetes-кластер)
   - [Namespace и ресурсы](#namespace-и-ресурсы)
   - [Слои данных](#слои-данных)
3. [Микросервисы](#микросервисы)
   - [Backend сервисы](#backend-сервисы)
   - [AI сервисы](#ai-сервисы)
   - [Frontend](#frontend)
4. [CI/CD Пайплайн](#ci-cd-пайплайн)
   - [GitHub Actions Workflow](#github-actions-workflow)
   - [Матрица сборки](#матрица-сборки)
   - [Триггеры и события](#триггеры-и-события)
   - [Публикация образов](#публикация-образов)
5. [Развертывание](#развертывание)
   - [Helm Chart](#helm-chart)
   - [Kubernetes манифесты](#kubernetes-манифесты)
   - [ArgoCD GitOps](#argocd-gitops)
6. [Мониторинг и Observability](#мониторинг-и-observability)
   - [Prometheus (метрики)](#prometheus-метрики)
   - [Grafana (визуализация)](#grafana-визуализация)
   - [Loki (логи)](#loki-логи)
   - [Kafdrop (Kafka UI)](#kafdrop-kafka-ui)
7. [Управление секретами](#управление-секретами)
   - [Vault архитектура](#vault-архитектура)
   - [External Secrets Operator](#external-secrets-operator)
   - [Секреты и пути](#секреты-и-пути)
8. [Health Checks и Probes](#health-checks-и-probes)
9. [Resource Management](#resource-management)
   - [Лимиты ресурсов](#лимиты-ресурсов)
   - [Autoscaling (HPA)](#autoscaling-hpa)
10. [Troubleshooting](#troubleshooting)

---

## Общая архитектура

### Production среда (Kubernetes)

```
Namespace: tensor-talks

┌─────────────────────────────────────────────────────────────┐
│                     Ingress (Nginx)                         │
│  tensor-talks.example.com                                   │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│                    BFF Service (Go)                         │
│  Deployment: 2 реплики                                       │
│  Port: 8080                                                 │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Middleware:                                          │   │
│  │ - JWT Validation (auth-service)                     │   │
│  │ - Logging (Zap)                                      │   │
│  │ - Metrics (Prometheus)                               │   │
│  │ - CORS                                               │   │
│  └─────────────────────────────────────────────────────┘   │
└────────────┬──────────────────────┬─────────────────────────┘
             │                      │
    ┌────────▼────────┐    ┌────────▼────────┐
    │ Auth Service    │    │ Session Service │
    │ (Go, :8081)     │    │ (Go, :8083)     │
    │ Deployment: 2   │    │ Deployment: 2   │
    └────────┬────────┘    └────────┬────────┘
             │                      │
    ┌────────▼─────────────────────▼────────┐
    │         Redis Cluster                  │
    │  StatefulSet: 1 под                    │
    │  - JWT сессии (TTL: 24h)              │
    │  - Активные интервью (TTL: 2h)        │
    └───────────────────────────────────────┘
```

---

## Инфраструктура

### Kubernetes кластер

**Версия:** 1.24+

**Ноды:**
- Минимум 3 ноды
- 2 CPU / 4 GiB RAM каждая
- StorageClass: yc-network-ssd (Яндекс.Облако) или gp3 (AWS)

### Namespace и ресурсы

**Namespace:** `tensor-talks`

**Общие лимиты команды:**
- CPU: 3 cores total
- Memory: 6 GiB total

### Слои данных

```
┌─────────────────────────────────────────────────────────────┐
│                    PostgreSQL Cluster                        │
│  StatefulSet: 1 под (external в production)                 │
│  Схемы:                                                      │
│  - user_crud (auth-service, user-crud-service)             │
│  - session_crud (session-crud-service)                     │
│  - chat_crud (chat-crud-service)                           │
│  - results_crud (results-crud-service)                     │
│  - knowledge_base_crud (knowledge-base-crud-service)       │
│  - questions_crud (questions-crud-service)                 │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                      Kafka Cluster                           │
│  Strimzi Operator (external в production)                  │
│  Топики:                                                     │
│  - tensor-talks-chat.events.out (BFF → dialogue-aggregator)│
│  - tensor-talks-chat.events.in  (dialogue-aggregator → BFF)│
│  - tensor-talks-interview.build.request                     │
│      (session-service → interview-builder)                  │
│  - tensor-talks-interview.build.response                    │
│      (interview-builder → session-service)                  │
│  - messages.full.data                                       │
│      (dialogue-aggregator → interviewer-agent-service)      │
│  - generated.phrases                                        │
│      (interviewer-agent-service → dialogue-aggregator)      │
│  - session.completed                                        │
│      (dialogue-aggregator → analyst-agent-service)          │
│  Замечание: 4 первых топика декларированы в                 │
│  helm/templates/kafka/kafka-topics.yaml; последние 3 НЕ     │
│  декларированы (см. план §10.12).                           │
└─────────────────────────────────────────────────────────────┘
```

---

## Микросервисы

### Backend сервисы (Go)

| Сервис | Порт | Реплики | Описание |
|--------|------|---------|----------|
| `bff-service` | 8080 | 2 | Backend-for-frontend, API шлюз |
| `auth-service` | 8081 | 2 | Аутентификация, JWT |
| `user-crud-service` | 8082 | 1 | CRUD пользователей |
| `session-service` | 8083 | 2 | Управление сессиями интервью |
| `dialogue-aggregator` | 8084 | 1 | Оркестратор диалогов (бывший mock-model-service) |
| `session-crud-service` | 8085 | 1 | Хранение сессий в PostgreSQL |
| `chat-crud-service` | 8087 | 1 | Хранение чатов в PostgreSQL |
| `results-crud-service` | 8088 | 1 | Хранение результатов в PostgreSQL |
| `knowledge-base-crud-service` | 8090 | 1 | CRUD базы знаний |
| `questions-crud-service` | 8091 | 1 | CRUD базы вопросов |

### AI сервисы (Python)

```
┌─────────────────────────────────────────────────────────────┐
│              Interview Builder Service                       │
│  (Python FastAPI, :8089)                                    │
│  Deployment: 1 под                                          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Логика:                                              │   │
│  │ 1. Слушает Kafka: interview.build.request           │   │
│  │ 2. Запрос вопросов: questions-crud-service          │   │
│  │ 3. Запрос знаний: knowledge-base-crud-service       │   │
│  │ 4. Сборка программы (5 вопросов)                    │   │
│  │ 5. Ответ в Kafka: interview.build.response          │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                  Agent Service                               │
│  (Python + LangGraph, :8093)                                │
│  Deployment: 1 под                                          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ LangGraph Workflow:                                  │   │
│  │ 1. load_context → 2. evaluate_answer →              │   │
│  │ 3. decide_action → 4. generate_response             │   │
│  │                                                      │   │
│  │ Tools:                                               │   │
│  │ - search_knowledge_base                             │   │
│  │ - evaluate_answer                                   │   │
│  │ - web_search (external)                             │   │
│  │ - summarize_dialogue                                │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                  Analyst Agent Service                       │
│  (Python + LangGraph, :8094)                                │
│  Deployment: 1 под                                          │
│  Подписан на Kafka session.completed, формирует отчёт,      │
│  пишет evaluations/reports/presets в results-crud-service.  │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│              Knowledge Producer Service                      │
│  (Python, :8092)                                            │
│  Deployment: 1 под                                          │
│  AS-IS: загрузчик JSON-материалов в knowledge-base-crud /   │
│  questions-crud (см. план §3, цель — LangGraph + HITL).     │
└─────────────────────────────────────────────────────────────┘
```

### Frontend

| Сервис | Порт | Описание |
|--------|------|----------|
| `frontend` | 5173 | React SPA (Vite, TypeScript) |

---

## CI/CD Пайплайн

### GitHub Actions Workflow

**Файл:** `.github/workflows/build-and-push.yml`

```
Событие: push в main/develop
         │
         ▼
┌─────────────────────────────────────────┐
│  GitHub Actions Runner                  │
│  ubuntu-latest                          │
│                                         │
│  Jobs (parallel для всех сервисов):    │
│  ┌─────────────────────────────────┐   │
│  │ 1. Checkout                      │   │
│  │ 2. Docker Build (кэш слоев)     │   │
│  │ 3. Docker Scan (уязвимости)     │   │
│  │ 4. Docker Push (если ENABLE_PUSH)│  │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
         │
         ▼ (если ENABLE_PUSH=true)
┌─────────────────────────────────────────┐
│   GitHub Container Registry (ghcr.io)   │
│   Образы:                               │
│   - ghcr.io/paulrychkov/tensor-talks-  │
│     {service-name}:{tag}               │
│   Теги: latest, main-{sha}, develop-   │
└─────────────────────────────────────────┘
```

### Матрица сборки

| Сервис | Dockerfile | Путь | Зависимости |
|--------|-----------|------|-------------|
| auth-service | backend/auth-service/Dockerfile | backend/auth-service | - |
| bff-service | backend/bff-service/Dockerfile | backend/bff-service | - |
| user-crud-service | backend/user-crud-service/Dockerfile | backend/user-crud-service | - |
| session-service | backend/session-service/Dockerfile | backend/session-service | - |
| session-crud-service | backend/session-crud-service/Dockerfile | backend/session-crud-service | - |
| chat-crud-service | backend/chat-crud-service/Dockerfile | backend/chat-crud-service | - |
| results-crud-service | backend/results-crud-service/Dockerfile | backend/results-crud-service | - |
| knowledge-base-crud-service | backend/knowledge-base-crud-service/Dockerfile | backend/knowledge-base-crud-service | - |
| questions-crud-service | backend/questions-crud-service/Dockerfile | backend/questions-crud-service | - |
| interview-builder-service | backend/interview-builder-service/Dockerfile | backend/interview-builder-service | - |
| knowledge-producer-service | backend/knowledge-producer-service/Dockerfile | backend/knowledge-producer-service | - |
| interviewer-agent-service | backend/interviewer-agent-service/Dockerfile | backend/interviewer-agent-service | - |
| frontend | frontend/Dockerfile | frontend | - |

### Триггеры и события

**Push в main или develop:**
- Запускается сборка Docker образов для всех микросервисов
- Публикация образов отключена (ENABLE_PUSH: false)
- Образы собираются только для проверки работоспособности

**Pull Request в main:**
- Запускается сборка Docker образов
- Образы не публикуются (только проверка сборки)
- Позволяет проверить корректность сборки перед мерджем

**workflow_dispatch:**
- Ручной запуск через GitHub UI
- Полезно для отладки или принудительной пересборки

### Публикация образов

**Статус:** Отключено (`ENABLE_PUSH: false`)

**Для включения:**
1. Открыть `.github/workflows/build-and-push.yml`
2. Изменить `ENABLE_PUSH: false` на `ENABLE_PUSH: true`
3. Закоммитьтить изменения

**Формат образов:**
```
ghcr.io/paulrychkov/tensor-talks-{service-name}:{tag}
```

**Теги:**
- `latest` - последняя версия
- `main-{sha}` - коммит в main
- `develop-{sha}` - коммит в develop

---

## Развертывание

### Helm Chart

**Путь:** `helm/`

**Установка:**
```bash
# Локальное развертывание
helm install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  --set localDevelopment=true \
  --set postgresql.deployment.enabled=true \
  --set vault.deployment.enabled=true

# Production развертывание
helm upgrade --install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  -f values-production.yaml
```

**Основные параметры values.yaml:**
```yaml
localDevelopment: true  # false для production

global:
  namespace: tensor-talks
  imageRegistry: ""  # ghcr.io для production

postgresql:
  deployment:
    enabled: true  # false для external PostgreSQL

vault:
  enabled: true
  deployment:
    enabled: true  # false для external Vault
```

### Kubernetes манифесты

**Структура `helm/templates/`:**
- `namespace.yaml` - namespace tensor-talks
- `serviceaccount.yaml` - service account для подов
- `vault/` - Vault и External Secrets
- `postgresql/` - PostgreSQL deployment
- `kafka/` - Kafka топики
- `monitoring/` - Prometheus, Grafana, Loki
- `deployments/` - deployment каждого сервиса
- `services/` - services для каждого сервиса
- `ingress/` - ingress для frontend

### ArgoCD GitOps

**Файл:** `argocd/application.yaml`

**Настройки:**
- Auto-sync: включен
- Self-heal: включен
- Prune: включен
- Retry: до 5 попыток

**Развертывание:**
```bash
kubectl apply -f argocd/application.yaml
```

---

## Мониторинг и Observability

### Prometheus (метрики)

**StatefulSet:** 1 под

**Scrape interval:** 60s

**ServiceMonitor:**
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: tensor-talks
  namespace: tensor-talks
spec:
  selector:
    matchLabels:
      app: tensor-talks
  endpoints:
  - port: metrics
    path: /metrics
    interval: 60s
```

**Метрики:**
- `tensortalks_http_requests_total` - HTTP запросы
- `tensortalks_http_request_duration_seconds` - длительность запросов
- `tensortalks_business_*` - бизнес-метрики
- `tensortalks_db_*` - БД метрики
- `tensortalks_kafka_*` - Kafka метрики

### Grafana (визуализация)

**StatefulSet:** 1 под

**Дашборды:**
- Обзор системы
- Метрики по сервисам
- Kafka метрики
- БД метрики

**Доступ:**
```bash
kubectl port-forward svc/tensor-talks-grafana -n tensor-talks 3000:3000
```

**URL:** http://localhost:3000

**Credentials:** Из Vault `tensor-talks/grafana`

### Loki (логи)

**StatefulSet:** 1 под

**Promtail (DaemonSet):**
- Сбор логов из /var/log/containers
- Парсинг JSON формата
- Отправка в Loki

**Labels для фильтрации:**
- `namespace: tensor-talks`
- `app: {service-name}`
- `level: {log-level}`

**LogQL примеры:**
```logql
# Все логи сервиса
{namespace="tensor-talks", app="bff-service"}

# Только ошибки
{namespace="tensor-talks"} |= "error"

# Логи по request_id
{namespace="tensor-talks"} | json | request_id="req-123"
```

### Kafdrop (Kafka UI)

**Deployment:** 1 под

**Доступ:**
```bash
kubectl port-forward svc/tensor-talks-kafdrop -n tensor-talks 9000:9000
```

**URL:** http://localhost:9000

**Функционал:**
- Просмотр топиков
- Просмотр сообщений
- Consumer groups

---

## Управление секретами

### Vault архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                    HashiCorp Vault                           │
│  StatefulSet: 1 под (dev mode в local)                     │
│  External в production (vault.kubepractice.ru)             │
│                                                             │
│  Secrets Paths:                                            │
│  kv/tensor-talks/                                          │
│  ├── postgresql (host, port, database, user, password)    │
│  ├── jwt (secret, issuer, audience)                        │
│  ├── grafana (admin-user, admin-password)                 │
│  ├── github (username, token)                             │
│  └── llm (api-key)                                         │
└─────────────────────────────────────────────────────────────┘
         │
         │ External Secrets Operator
         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Kubernetes Secrets                         │
│  namespace: tensor-talks                                    │
│  Secrets:                                                   │
│  - tensor-talks-postgres                                   │
│  - tensor-talks-jwt                                        │
│  - tensor-talks-grafana                                    │
│  - tensor-talks-github                                     │
│  - tensor-talks-llm                                        │
└─────────────────────────────────────────────────────────────┘
```

### External Secrets Operator

**Установка:**
```bash
helm install external-secrets \
  external-secrets/external-secrets \
  --namespace external-secrets-system \
  --create-namespace
```

### SecretStore конфигурация

```yaml
apiVersion: external-secrets.io/v1
kind: SecretStore
metadata:
  name: vault-tensor-talks
  namespace: tensor-talks
spec:
  provider:
    vault:
      server: https://vault.kubepractice.ru
      path: Secrets/kv
      version: v2
      auth:
        kubernetes:
          mountPath: kubernetes
          role: tensor-talks
          serviceAccountRef:
            name: tensor-talks
```

### Секреты и пути

| Путь в Vault | Ключи | Описание |
|-------------|-------|----------|
| `tensor-talks/postgresql` | host, port, database, user, password, sslmode | PostgreSQL credentials |
| `tensor-talks/jwt` | secret, issuer, audience | JWT секреты |
| `tensor-talks/grafana` | admin-user, admin-password | Grafana credentials |
| `tensor-talks/github` | username, token | GitHub токен для ArgoCD |
| `tensor-talks/llm` | api-key | LLM API key для interviewer-agent-service |

**Добавление секретов:**
```bash
vault kv put kv/tensor-talks/postgresql \
  host="postgres.example.com" \
  port="5432" \
  database="tensor_talks_db" \
  user="tensor_talks" \
  password="<password>" \
  sslmode="require"
```

---

## Health Checks и Probes

### Endpoints здоровья

| Сервис | Endpoint | Проверка |
|--------|----------|----------|
| Все Go сервисы | GET /healthz | HTTP 200 |
| Все Python сервисы | GET /health | HTTP 200 |
| PostgreSQL | readinessProbe | TCP socket 5432 |
| Redis | readinessProbe | TCP socket 6379 |
| Kafka | readinessProbe | TCP socket 9092 |

### Liveness vs Readiness

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

---

## Resource Management

### Лимиты ресурсов

**Go сервисы:**
```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 300m
    memory: 384Mi
```

**Python сервисы:**
```yaml
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

**PostgreSQL:**
```yaml
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

**Kafka:**
```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 1000m
    memory: 1Gi
```

### Autoscaling (HPA)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: bff-service-hpa
  namespace: tensor-talks
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: bff-service
  minReplicas: 2
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

---

## Troubleshooting

### Поды не запускаются

```bash
# Проверка событий
kubectl get events -n tensor-talks --sort-by='.lastTimestamp'

# Описание пода
kubectl describe pod <pod-name> -n tensor-talks

# Проверка на OOMKilled
kubectl get pods -n tensor-talks -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[*].lastState.terminated.reason}{"\n"}{end}'
```

### Проблемы с Secret

```bash
# Проверка External Secrets
kubectl get externalsecrets -n tensor-talks

# Проверка SecretStore
kubectl get secretstore -n tensor-talks

# Логи External Secrets Operator
kubectl logs -n external-secrets-system -l app.kubernetes.io/name=external-secrets
```

### Проблемы с PostgreSQL

```bash
# Проверка подключения
kubectl exec -it deployment/tensor-talks-postgresql -n tensor-talks -- \
  psql -U tensor_talks -d tensor_talks_db -c '\dt'

# Проверка схем
kubectl exec -it deployment/tensor-talks-postgresql -n tensor-talks -- \
  psql -U tensor_talks -d tensor_talks_db -c '\dn'
```

### Проблемы с Kafka

```bash
# Проверка Kafka кластера
kubectl get kafka -n tensor-talks

# Проверка топиков
kubectl get kafkatopics -n tensor-talks

# Описание топика
kubectl exec -it tensor-talks-kafka-kafka-0 -n tensor-talks -- \
  kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic tensor-talks-chat.events.out
```

### Удаление и переустановка

```bash
# Удаление Helm release
helm uninstall tensor-talks -n tensor-talks

# Удаление PVC (осторожно! данные будут потеряны)
kubectl delete pvc -n tensor-talks -l app.kubernetes.io/instance=tensor-talks

# Переустановка
helm install tensor-talks ./helm --namespace tensor-talks
```

---

## Публикация в интернет через Cloudflare Tunnel

Проект публикуется через Cloudflare Tunnel (бесплатный, без необходимости белого IP).

### Текущая конфигурация

**Конфиг:** `tobedel/cloudflared-config.yml`  
**Туннель:** `tensor-talks-new`  
**Домен:** `tensor-talks.ru` → `http://127.0.0.1:8080` (kubectl port-forward к Kubernetes ingress)

### Запуск/остановка

```powershell
# Cloudflared запущен как Windows-сервис
# Статус:
Get-Service cloudflared

# Запустить:
Start-Service cloudflared

# Остановить:
Stop-Service cloudflared
```

### Установка (если сервис не установлен)

```powershell
# Установка через скрипт (см. tobedel/install-cloudflared-service.ps1)
# или вручную:
cloudflared service install
```

---

## Ссылки

- [INSTALLATION_MANUAL.md](INSTALLATION_MANUAL.md) - Полное руководство по установке
- [helm/README.md](helm/README.md) - Helm chart параметры
- [backend/README.md](backend/README.md) - Backend архитектура
- [backend/KAFKA.md](backend/KAFKA.md) - Kafka конфигурация
- [backend/METRICS.md](backend/METRICS.md) - Метрики сервисов
- [backend/LOGGING.md](backend/LOGGING.md) - Логирование
- [backend/WORKFLOW.md](backend/WORKFLOW.md) - Workflow платформы
- [argocd/README.md](argocd/README.md) - ArgoCD GitOps
