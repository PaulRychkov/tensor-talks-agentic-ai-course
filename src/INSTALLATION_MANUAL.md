# Установка и развертывание TensorTalks

Полное руководство по установке, локальной разработке и развертыванию платформы TensorTalks.

## 📋 Содержание

1. [Быстрый старт](#быстрый-старт)
2. [Требования](#требования)
3. [Локальная разработка](#локальная-разработка)
4. [Установка Kubernetes](#установка-kubernetes)
5. [Установка компонентов](#установка-компонентов)
6. [Развертывание приложения](#развертывание-приложения)
7. [Управление секретами](#управление-секретами)
8. [Облачное развертывание](#облачное-развертывание)
9. [Мониторинг](#мониторинг)
10. [Troubleshooting](#troubleshooting)

---

## Быстрый старт

### Локальная разработка (docker-compose)

```bash
# Клонирование репозитория
git clone https://github.com/PaulRychkov/TensorTalks.git
cd TensorTalks

# Запуск всех сервисов
docker-compose up -d

# Доступ к сервисам
# Frontend: http://localhost:5173
# BFF API: http://localhost:8080
# Grafana: http://localhost:3000 (admin/admin)
# Kafdrop: http://localhost:9000
```

### Развертывание в Kubernetes

```bash
# Установка Helm chart
helm install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  --set localDevelopment=true

# Проверка
kubectl get pods -n tensor-talks
```

---

## Требования

### Для локальной разработки

- Docker Desktop или Docker Engine
- Docker Compose v2.0+
- Минимум 4 CPU и 8 GiB RAM

### Для Kubernetes

- Kubernetes кластер 1.24+
- kubectl настроен на кластер
- Helm 3.8+
- Минимум 4 CPU и 8 GiB RAM
- StorageClass для PersistentVolumes

---

## Локальная разработка

### Вариант 1: Docker Compose (рекомендуется)

**Запуск:**

```bash
docker-compose up -d
```

**Сервисы:**

| Сервис | URL | Описание |
|--------|-----|----------|
| Frontend | http://localhost:5173 | React SPA |
| BFF API | http://localhost:8080 | Backend API |
| Grafana | http://localhost:3000 | Метрики и логи |
| Prometheus | http://localhost:9090 | Метрики |
| Kafdrop | http://localhost:9000 | Kafka UI |
| pgAdmin | http://localhost:5050 | PostgreSQL UI |

**Остановка:**

```bash
docker-compose down
docker-compose down -v  # С удалением volumes
```

### Вариант 2: Локальный Kubernetes

**Настройка minikube:**

```bash
# Установка
choco install minikube  # Windows
brew install minikube   # macOS

# Запуск кластера
minikube start --cpus=4 --memory=8192

# Установка Helm chart
helm install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  --set postgresql.deployment.enabled=true \
  --set vault.deployment.enabled=true \
  --set localDevelopment=true
```

**Port-forward для доступа:**

```bash
# Frontend
kubectl port-forward svc/tensor-talks-frontend -n tensor-talks 5173:80

# BFF API
kubectl port-forward svc/tensor-talks-bff-service -n tensor-talks 8080:8080

# Grafana
kubectl port-forward svc/tensor-talks-grafana -n tensor-talks 3000:3000
```

### Настройка IDE

**VS Code расширения:**
- Kubernetes
- Docker
- Go (для backend)
- Python (для Python сервисов)
- Remote - Containers

**Отладка Go сервисов:**

```json
// .vscode/launch.json
{
  "name": "Debug BFF Service",
  "type": "go",
  "request": "launch",
  "mode": "debug",
  "program": "${workspaceFolder}/backend/bff-service"
}
```

---

## Установка Kubernetes

### Minikube (локально)

```bash
# Установка
choco install minikube  # Windows
brew install minikube   # macOS
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64  # Linux

# Запуск
minikube start --cpus=4 --memory=8192 --disk-size=20g

# Проверка
kubectl get nodes
```

### Kind (локально)

```bash
# Установка
choco install kind  # Windows
brew install kind   # macOS

# Создание кластера
kind create cluster --name tensor-talks

# Проверка
kubectl get nodes
```

### K3d (локально)

```bash
# Установка
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Создание кластера
k3d cluster create tensor-talks --servers=1 --agents=2

# Проверка
kubectl get nodes
```

### Облачные провайдеры

#### Яндекс.Облако

```bash
# Установка Yandex Cloud CLI
yc config profile create tensor-talks

# Создание кластера
yc managed-kubernetes cluster create \
  --name tensor-talks-cluster \
  --network-name default \
  --zone ru-central1-a \
  --service-account-name tensor-talks-sa \
  --node-service-account-name tensor-talks-node-sa \
  --release-channel regular \
  --version 1.24

# Создание node group
yc managed-kubernetes node-group create \
  --name tensor-talks-nodes \
  --cluster-name tensor-talks-cluster \
  --fixed-size 3 \
  --cores 2 \
  --memory 4 \
  --disk-size 50

# Настройка kubectl
yc managed-kubernetes cluster get-credentials tensor-talks-cluster --external
```

#### AWS EKS

```bash
# Установка eksctl
# Создание кластера
eksctl create cluster \
  --name tensor-talks-cluster \
  --region us-east-1 \
  --nodegroup-name tensor-talks-nodes \
  --node-type t3.medium \
  --nodes 3

# Настройка kubectl
aws eks update-kubeconfig --name tensor-talks-cluster
```

#### Google Cloud GKE

```bash
# Создание кластера
gcloud container clusters create tensor-talks-cluster \
  --zone us-central1-a \
  --num-nodes 3 \
  --machine-type e2-medium

# Настройка kubectl
gcloud container clusters get-credentials tensor-talks-cluster
```

#### Azure AKS

```bash
# Создание resource group
az group create --name tensor-talks-rg --location eastus

# Создание AKS кластера
az aks create \
  --resource-group tensor-talks-rg \
  --name tensor-talks-cluster \
  --node-count 3 \
  --node-vm-size Standard_B2s

# Настройка kubectl
az aks get-credentials --resource-group tensor-talks-rg --name tensor-talks-cluster
```

---

## Установка компонентов

### 1. Strimzi Operator (Kafka)

```bash
# Установка в namespace tensor-talks
kubectl create -f 'https://strimzi.io/install/latest?namespace=tensor-talks' -n tensor-talks

# Ожидание готовности
kubectl wait --for=condition=ready pod -l name=strimzi-cluster-operator -n tensor-talks --timeout=300s

# Проверка
kubectl get pods -l name=strimzi-cluster-operator -n tensor-talks
```

### 2. External Secrets Operator

```bash
# Установка через Helm
helm repo add external-secrets https://charts.external-secrets.io
helm repo update

helm install external-secrets \
  external-secrets/external-secrets \
  --namespace external-secrets-system \
  --create-namespace

# Проверка
kubectl get pods -n external-secrets-system
```

### 3. Prometheus Stack (опционально)

```bash
# Установка kube-prometheus-stack
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install monitoring \
  prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace
```

---

## Развертывание приложения

### Подготовка values.yaml

**Для локальной разработки:**

```yaml
localDevelopment: true

global:
  namespace: tensor-talks

postgresql:
  deployment:
    enabled: true
  persistence:
    enabled: false

vault:
  enabled: true
  deployment:
    enabled: true

serviceMonitor:
  enabled: false
```

**Для production:**

```yaml
localDevelopment: false

global:
  namespace: tensor-talks
  imageRegistry: ghcr.io

postgresql:
  deployment:
    enabled: false
  host: postgres.example.com

vault:
  enabled: true
  deployment:
    enabled: false
  server: https://vault.example.com
```

### Установка Helm chart

```bash
# Создание namespace
kubectl create namespace tensor-talks

# Локальное развертывание
helm install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  --set postgresql.deployment.enabled=true \
  --set vault.deployment.enabled=true \
  --set localDevelopment=true

# Production развертывание
helm upgrade --install tensor-talks ./helm \
  --namespace tensor-talks \
  --create-namespace \
  -f values-production.yaml
```

### Проверка развертывания

```bash
# Проверка подов
kubectl get pods -n tensor-talks

# Проверка сервисов
kubectl get svc -n tensor-talks

# Проверка StatefulSets
kubectl get statefulsets -n tensor-talks

# Проверка PersistentVolumeClaims
kubectl get pvc -n tensor-talks
```

---

## Управление секретами

### Добавление секретов в Vault

```bash
# Vault CLI
export VAULT_ADDR=https://vault.example.com
export VAULT_TOKEN=<root-token>

# PostgreSQL
vault kv put kv/tensor-talks/postgresql \
  host="postgres.example.com" \
  port="5432" \
  database="tensor_talks_db" \
  user="tensor_talks" \
  password="<secure-password>" \
  sslmode="require"

# JWT
vault kv put kv/tensor-talks/jwt \
  secret="<32-char-secret>" \
  issuer="tensor-talks-auth" \
  audience="tensor-talks-services"

# Grafana
vault kv put kv/tensor-talks/grafana \
  admin-user="admin" \
  admin-password="<secure-password>"

# GitHub (для ArgoCD)
vault kv put kv/tensor-talks/github \
  username="PaulRychkov" \
  token="<github-token>"

# LLM API Key
vault kv put kv/tensor-talks/llm \
  api-key="<llm-api-key>"
```

### Проверка External Secrets

```bash
# Статус External Secrets
kubectl get externalsecrets -n tensor-talks

# Детальная информация
kubectl describe externalsecret tensor-talks-postgres -n tensor-talks

# Проверка созданных Secrets
kubectl get secrets -n tensor-talks
```

---

## Облачное развертывание

### Яндекс.Облако

**StorageClass:**

```yaml
postgresql:
  persistence:
    storageClass: "yc-network-ssd"

kafka:
  storage:
    class: yc-network-ssd
```

**Ingress:**

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    yandex.cloud/load-balancer-type: "external"
  hosts:
    - host: tensor-talks.your-domain.ru
      paths:
        - path: /
          pathType: Prefix
```

### AWS EKS

**StorageClass:**

```yaml
postgresql:
  persistence:
    storageClass: "gp3"

kafka:
  storage:
    class: gp3
```

**Load Balancer:**

```bash
# Установка AWS Load Balancer Controller
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=tensor-talks-cluster
```

### Google Cloud GKE

**StorageClass:**

```yaml
postgresql:
  persistence:
    storageClass: "standard"
```

### Azure AKS

**StorageClass:**

```yaml
postgresql:
  persistence:
    storageClass: "managed-csi"
```

---

## Мониторинг

### Port-forward для доступа

```bash
# Grafana
kubectl port-forward svc/tensor-talks-grafana -n tensor-talks 3000:3000 &

# Prometheus
kubectl port-forward svc/tensor-talks-prometheus -n tensor-talks 9090:9090 &

# Kafdrop (Kafka UI)
kubectl port-forward svc/tensor-talks-kafdrop -n tensor-talks 9000:9000 &

# pgAdmin
kubectl port-forward svc/tensor-talks-pgadmin -n tensor-talks 5050:80 &
```

### Доступные UI

| Сервис | URL | Credentials |
|--------|-----|-------------|
| Grafana | http://localhost:3000 | Из Vault `tensor-talks/grafana` |
| Prometheus | http://localhost:9090 | - |
| Kafdrop | http://localhost:9000 | - |
| pgAdmin | http://localhost:5050 | `admin@tensor-talks.com` + пароль из Vault |
| Vault | http://localhost:8200 | Root token из логов |

### Проверка метрик

```bash
# Проверка доступности /metrics endpoint
curl http://tensor-talks-bff-service.tensor-talks.svc:8080/metrics

# Проверка в Prometheus
# Запрос: tensortalks_http_requests_total{namespace="tensor-talks"}
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

## Ссылки

- [DEVOPS_DEPLOY.md](DEVOPS_DEPLOY.md) - DevOps архитектура и CI/CD пайплайн
- [helm/README.md](helm/README.md) - Helm chart параметры
- [argocd/README.md](argocd/README.md) - GitOps через ArgoCD
- [UI_ENDPOINTS.md](UI_ENDPOINTS.md) - UI эндпоинты
- [backend/README.md](backend/README.md) - Backend архитектура
- [backend/WORKFLOW.md](backend/WORKFLOW.md) - Workflow платформы
