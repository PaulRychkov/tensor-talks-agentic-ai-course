# UI Эндпоинты TensorTalks

Этот документ содержит актуальную информацию о всех доступных UI эндпоинтах для TensorTalks.

## 🐳 Docker-compose (локальная разработка)

| Сервис | URL | Логин/Пароль |
|--------|-----|--------------|
| Frontend (основной) | http://localhost:5173 | Регистрация через UI |
| BFF API | http://localhost:8080 | JWT токен |
| Admin Frontend | http://localhost:8097 | секрет `tt-admin-secret-2026` |
| Admin BFF API | http://localhost:8096 | — |
| Prometheus | http://localhost:9090 | — |
| Grafana | http://localhost:3000 | admin/admin |
| Kafdrop (Kafka UI) | http://localhost:9000 | — |
| pgAdmin | http://localhost:5050 | admin@admin.com / admin |

### Новое: Дашборд метрик в админке

Страница `/metrics` в Admin Frontend содержит три вкладки:
- **Продуктовые** — сессии, completion rate, avg score, рейтинги (из results-crud)
- **Технические** — RPS, error rate, p95 latency, статус сервисов (из Prometheus)
- **ИИ-метрики** — LLM вызовы, PII блокировки, уверенность агента (из Prometheus)

API admin-bff:
- `GET /admin/api/metrics/product` → результаты из results-crud
- `GET /admin/api/metrics/technical` → запросы к Prometheus  
- `GET /admin/api/metrics/ai` → запросы к Prometheus

## 🚀 Быстрый доступ (Kubernetes)

Запустите все port-forward команды в отдельных терминалах или используйте `&` для фонового режима.

## 📊 Мониторинг и Observability

### Grafana
- **URL**: http://localhost:3000
- **Логин/Пароль**: Из Vault (`tensor-talks/grafana`) или values.yaml
  - Локальная разработка (`localDevelopment: true`): дефолт `admin/admin`
  - Продакшен (`localDevelopment: false`): ОБЯЗАТЕЛЬНО из Vault!
- **Port-forward**: `kubectl port-forward svc/tensor-talks-grafana -n tensor-talks 3000:3000`
- **Описание**: Визуализация метрик и логов, дашборды для мониторинга сервисов

### Prometheus
- **URL**: http://localhost:9090
- **Port-forward**: `kubectl port-forward svc/tensor-talks-prometheus -n tensor-talks 9090:9090`
- **Описание**: Сбор и хранение метрик, PromQL запросы

### Kafdrop (Kafka UI)
- **URL**: http://localhost:9000
- **Port-forward**: `kubectl port-forward svc/tensor-talks-kafdrop -n tensor-talks 9000:9000`
- **Описание**: Просмотр Kafka топиков, сообщений, consumer groups

## 🗄️ База данных

### pgAdmin (PostgreSQL Admin UI)
- **URL**: http://localhost:5050
- **Логин**: `admin@tensor-talks.com` (из values.yaml)
- **Пароль**: Из Vault (`tensor-talks/pgadmin`) или values.yaml
  - Локальная разработка (`localDevelopment: true`): дефолт `admin`
  - Продакшен (`localDevelopment: false`): ОБЯЗАТЕЛЬНО из Vault!
- **Port-forward**: `kubectl port-forward svc/tensor-talks-pgadmin -n tensor-talks 5050:80`
- **Описание**: Веб-интерфейс для управления PostgreSQL
- **Подключение к БД**:
  - Host: `tensor-talks-postgresql` (внутри кластера) или `localhost` (через port-forward)
  - Port: `5432`
  - Database: `tensor_talks_db`
  - User: `tensor_talks`
  - Password: из Vault (`tensor-talks/postgresql`) или values.yaml

## 🔧 Инфраструктура

### ArgoCD (GitOps)
- **URL**: https://localhost:8081
- **Логин**: `admin`
- **Пароль**: получить через `kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d`
- **Port-forward**: `kubectl port-forward svc/argocd-server -n argocd 8081:443`
- **Описание**: GitOps развертывание, управление приложениями

### Vault UI
- **URL**: http://localhost:8200
- **Port-forward**: `kubectl port-forward svc/tensor-talks-vault -n tensor-talks 8200:8200`
- **Описание**: Управление секретами (требует токен для доступа)

## 🌐 Приложение

### Frontend
- **URL**: http://localhost:8080 (через `minikube tunnel`, без port-forward)
- **Описание**: Пользовательский интерфейс TensorTalks. Service имеет тип LoadBalancer,
  `minikube tunnel` (запущен Scheduled Task «TensorTalks-Tunnel») сам пробрасывает на 127.0.0.1:8080.

### BFF API
- **URL**: http://localhost:8080
- **Port-forward**: `kubectl port-forward svc/tensor-talks-bff-service -n tensor-talks 8080:8080`
- **Описание**: Backend-for-Frontend API, основной API для фронтенда

## 📋 Все команды port-forward

```bash
# Мониторинг
kubectl port-forward svc/tensor-talks-grafana -n tensor-talks 3000:3000 &
kubectl port-forward svc/tensor-talks-prometheus -n tensor-talks 9090:9090 &
kubectl port-forward svc/tensor-talks-kafdrop -n tensor-talks 9000:9000 &

# База данных
kubectl port-forward svc/tensor-talks-pgadmin -n tensor-talks 5050:80 &
kubectl port-forward svc/tensor-talks-postgresql -n tensor-talks 5432:5432 &

# Инфраструктура
kubectl port-forward svc/argocd-server -n argocd 8081:443 &
kubectl port-forward svc/tensor-talks-vault -n tensor-talks 8200:8200 &

# Приложение — port-forward НЕ нужен.
# Frontend (Service type: LoadBalancer) автоматически доступен на http://127.0.0.1:8080
# через minikube tunnel (Scheduled Task TensorTalks-Tunnel, install-autostart.ps1).
# BFF — через ingress изнутри кластера; наружу — через cloudflared (tensor-talks.ru).
```

## 🔍 Проверка статуса сервисов

```bash
# Проверка всех подов
kubectl get pods -n tensor-talks

# Проверка всех сервисов
kubectl get svc -n tensor-talks

# Проверка ArgoCD
kubectl get pods -n argocd
```

## ⚠️ Важные замечания

1. **Секреты в Vault**: Все пароли и секреты должны храниться в Vault для продакшена!
   - Локальная разработка (`localDevelopment: true`): можно использовать дефолты
   - Продакшен (`localDevelopment: false`): ОБЯЗАТЕЛЬНО из Vault!
2. **HTTPS для ArgoCD**: ArgoCD использует HTTPS, поэтому URL начинается с `https://`
3. **Vault токен**: Для доступа к Vault UI нужен токен (root token или user token)
4. **Локальный доступ**: Все port-forward работают только на localhost, для внешнего доступа используйте Ingress

## 📚 Дополнительная документация

- [INSTALLATION_MANUAL.md](INSTALLATION_MANUAL.md) - установка и развертывание
- [DEVOPS_DEPLOY.md](DEVOPS_DEPLOY.md) - DevOps архитектура
- [helm/README.md](helm/README.md) - документация по Helm chart
- [argocd/README.md](argocd/README.md) - GitOps развертывание через ArgoCD
