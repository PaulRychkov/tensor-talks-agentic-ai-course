# Helm Chart для TensorTalks

Эта директория содержит Helm chart для развертывания TensorTalks в Kubernetes.

## Структура

```
helm/
├── Chart.yaml                  # Метаданные чарта
├── values.yaml                 # Конфигурация по умолчанию
├── README.md                   # Этот файл
└── templates/                  # Kubernetes манифесты
    ├── namespace.yaml          # Namespace для развертывания
    ├── serviceaccount.yaml     # ServiceAccount для подов
    ├── vault/                  # Vault и External Secrets
    ├── postgresql/             # PostgreSQL (deployment или external)
    ├── kafka/                  # Kafka топики
    ├── monitoring/             # Prometheus, Grafana, Loki
    └── services/               # Микросервисы TensorTalks
```

## Быстрый старт

### Локальное развертывание

```bash
# Создание namespace
kubectl create namespace tensor-talks

# Установка с локальными PostgreSQL и Vault
helm install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  --set postgresql.deployment.enabled=true \
  --set vault.deployment.enabled=true \
  --set localDevelopment=true
```

### Развертывание в production

```bash
# Обновите values.yaml для production
# - localDevelopment: false
# - vault.enabled: true
# - imageRegistry: ghcr.io

helm upgrade --install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  -f values-production.yaml
```

## Конфигурация

### Флаг локальной разработки

**`localDevelopment: true`** (для разработки):
- Разрешает дефолтные значения для секретов
- `imageRegistry: ""` (без префикса)
- `pullPolicy: Never` (локальные образы)

**`localDevelopment: false`** (для production):
- Требует все секреты из Vault
- `imageRegistry: "ghcr.io"` (или другой registry)
- `pullPolicy: IfNotPresent`

### Основные параметры

| Параметр | Описание | Значение по умолчанию |
|----------|----------|----------------------|
| `localDevelopment` | Флаг локальной разработки | `false` |
| `imageRegistry` | Registry для Docker образов | `ghcr.io` |
| `postgresql.deployment.enabled` | Развернуть локальный PostgreSQL | `false` |
| `vault.enabled` | Использовать Vault для секретов | `true` |
| `vault.deployment.enabled` | Развернуть локальный Vault | `false` |
| `serviceMonitor.enabled` | Создать ServiceMonitor для Prometheus | `true` |

## Секреты

### Управление секретами через Vault

Все секреты должны храниться в Vault и синхронизироваться через External Secrets Operator:

- `tensor-talks/postgresql` - credentials для PostgreSQL
- `tensor-talks/jwt` - JWT секреты для auth-service
- `tensor-talks/grafana` - credentials для Grafana
- `tensor-talks/github` - GitHub токен для ArgoCD
- `tensor-talks/llm` - LLM API key для interviewer-agent-service

### Добавление секретов в Vault

```bash
# PostgreSQL
vault kv put kv/tensor-talks/postgresql \
  host="tensor-talks-postgresql" \
  port="5432" \
  database="tensor_talks_db" \
  user="tensor_talks" \
  password="<password>" \
  sslmode="disable"

# JWT
vault kv put kv/tensor-talks/jwt \
  secret="<jwt-secret-32-chars>" \
  issuer="tensor-talks-auth" \
  audience="tensor-talks-services"

# Grafana
vault kv put kv/tensor-talks/grafana \
  admin-user="admin" \
  admin-password="<password>"
```

## Мониторинг

### ServiceMonitor

ServiceMonitor создается в namespace `monitoring` и требует соответствующих прав:

```bash
# Проверка прав
kubectl auth can-i create servicemonitors -n monitoring

# Если прав нет - отключите в values.yaml
serviceMonitor:
  enabled: false
```

### Доступ к инструментам мониторинга

```bash
# Grafana
kubectl port-forward svc/tensor-talks-grafana -n tensor-talks 3000:3000

# Prometheus
kubectl port-forward svc/tensor-talks-prometheus -n tensor-talks 9090:9090

# Kafdrop (Kafka UI)
kubectl port-forward svc/tensor-talks-kafdrop -n tensor-talks 9000:9000
```

**Фильтры для Grafana:**
- Метрики: `{namespace="tensor-talks", project="tensor-talks"}`
- Логи: `{namespace="tensor-talks", project="tensor-talks"}`

## Безопасность

### Изоляция ресурсов

- **Namespace**: Все ресурсы в `tensor-talks`
- **Метки**: `project=tensor-talks`
- **Kafka топики**: Префикс `tensor-talks-*`
- **Resource limits**: Строгие лимиты для всех подов

### Анализ безопасности

Развертывание безопасно при соблюдении мер предосторожности, описанных в этом документе.

## ArgoCD (GitOps)

Для развертывания через ArgoCD:

```bash
# Применить ArgoCD Application
kubectl apply -f ../argocd/application.yaml
```

См. [../argocd/README.md](../argocd/README.md)

## Устранение неполадок

### Поды не запускаются

```bash
# Проверка событий
kubectl get events -n tensor-talks --sort-by='.lastTimestamp'

# Описание пода
kubectl describe pod <pod-name> -n tensor-talks

# Логи
kubectl logs <pod-name> -n tensor-talks
```

### Проблемы с External Secrets

```bash
# Проверка External Secrets
kubectl get externalsecrets -n tensor-talks

# Статус синхронизации
kubectl describe externalsecret <secret-name> -n tensor-talks

# Логи External Secrets Operator
kubectl logs -n external-secrets-system -l app.kubernetes.io/name=external-secrets
```

### Проверка секретов

```bash
# Проверка Kubernetes Secrets
kubectl get secrets -n tensor-talks

# Проверка значений (осторожно!)
kubectl get secret <secret-name> -n tensor-talks -o jsonpath='{.data}' | base64 -d
```

## См. также

- [INSTALLATION.md](../INSTALLATION.md) - **Полное руководство: установка, локальная разработка, облачное развертывание**
- [DEVOPS.md](../DEVOPS.md) - DevOps архитектура
- [argocd/README.md](../argocd/README.md) - GitOps через ArgoCD
