# ArgoCD конфигурация для TensorTalks

Эта директория содержит манифесты для развертывания TensorTalks через ArgoCD (GitOps подход).

## Файлы

### `application.yaml`
Основной манифест ArgoCD Application, который:
- Отслеживает Git репозиторий с Helm chart
- Автоматически синхронизирует изменения
- Развертывает TensorTalks в namespace `tensor-talks`

**Настройки:**
- Auto-sync: включен
- Self-heal: включен
- Prune: включен
- Retry: до 5 попыток с экспоненциальной задержкой

### `repository-secret.yaml`
Secret для доступа к приватному GitHub репозиторию.
Содержит GitHub Personal Access Token.

**Важно:** Не коммитьте реальные токены в Git! Используйте External Secrets или создавайте Secret напрямую в кластере.

### `repository-secret-from-vault.yaml`
Альтернативная конфигурация для получения GitHub токена из Vault через External Secrets Operator.

### `vault-secretstore.yaml`
SecretStore для подключения External Secrets к Vault.
Используется для синхронизации секретов из Vault в Kubernetes Secrets.

### `github-repo-external-secret.yaml`
ExternalSecret для получения GitHub токена из Vault.
Автоматически создает Kubernetes Secret для ArgoCD.

## Развертывание

### Вариант 1: Прямое создание Secret (для локальной разработки)

```bash
# Создать Secret напрямую
kubectl create secret generic tensor-talks-repo \
  --from-literal=type=git \
  --from-literal=url=https://github.com/PaulRychkov/TensorTalks.git \
  --from-literal=username=PaulRychkov \
  --from-literal=password=<GITHUB_TOKEN> \
  -n argocd

# Применить ArgoCD Application
kubectl apply -f application.yaml
```

### Вариант 2: Через External Secrets (рекомендуется для production)

```bash
# Добавить GitHub токен в Vault
vault kv put kv/tensor-talks/github \
  username=PaulRychkov \
  token=<GITHUB_TOKEN>

# Применить SecretStore
kubectl apply -f vault-secretstore.yaml

# Применить ExternalSecret
kubectl apply -f github-repo-external-secret.yaml

# Применить ArgoCD Application
kubectl apply -f application.yaml
```

## Проверка статуса

```bash
# Проверка Application
kubectl get applications -n argocd

# Детальная информация
kubectl describe application tensor-talks -n argocd

# Логи ArgoCD
kubectl logs -l app.kubernetes.io/name=argocd-repo-server -n argocd
```

## Доступ к ArgoCD UI

- URL: https://argo.kubepractice.ru/
- Вход: через GitHub OAuth

## См. также

- [INSTALLATION.md](../INSTALLATION.md) - установка всех компонентов
- [helm/README.md](../helm/README.md) - руководство по развертыванию через Helm
