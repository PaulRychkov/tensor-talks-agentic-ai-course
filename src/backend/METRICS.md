# Метрики в микросервисах TensorTalks

## Обзор

Все микросервисы TensorTalks экспортируют метрики в формате **Prometheus** для централизованного сбора и визуализации в **Grafana**. Метрики собираются через Prometheus и отображаются на дашбордах в Grafana.

## Архитектура

```
┌─────────────────┐
│  Микросервисы    │
│  /metrics        │
│  (Prometheus)    │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Prometheus      │
│  (сбор метрик)   │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Grafana         │
│  (визуализация)  │
└─────────────────┘
```

## Единый формат метрик

Все метрики должны следовать соглашениям Prometheus и использовать единые префиксы и labels.

### Префиксы метрик

- `tensortalks_http_*` — HTTP-метрики (запросы, ответы, длительность)
- `tensortalks_business_*` — бизнес-метрики (регистрации, логины, операции)
- `tensortalks_db_*` — метрики базы данных (запросы, соединения, ошибки)
- `tensortalks_external_*` — метрики внешних вызовов (HTTP-клиенты, очереди)

### Обязательные labels

- `service` — имя микросервиса (например, `auth-service`)
- `version` — версия сервиса
- `environment` — окружение: `development`, `staging`, `production`

### Дополнительные labels

- `method` — HTTP метод (GET, POST, etc.)
- `endpoint` — путь эндпоинта
- `status_code` — HTTP статус код
- `error_type` — тип ошибки (для метрик ошибок)

## Стандартные метрики

### HTTP метрики

Каждый HTTP-сервис должен экспортировать:

```prometheus
# Количество HTTP-запросов
tensortalks_http_requests_total{service="auth-service", method="POST", endpoint="/auth/login", status_code="200"}

# Длительность HTTP-запросов (гистограмма)
tensortalks_http_request_duration_seconds{service="auth-service", method="POST", endpoint="/auth/login", quantile="0.5"}

# Размер тела запроса
tensortalks_http_request_size_bytes{service="auth-service", method="POST", endpoint="/auth/login"}

# Размер тела ответа
tensortalks_http_response_size_bytes{service="auth-service", method="POST", endpoint="/auth/login"}
```

### Бизнес-метрики

Примеры бизнес-метрик для auth-service:

```prometheus
# Количество регистраций
tensortalks_business_registrations_total{service="auth-service", status="success"}

# Количество логинов
tensortalks_business_logins_total{service="auth-service", status="success"}

# Количество выданных токенов
tensortalks_business_tokens_issued_total{service="auth-service", token_type="access"}

# Количество ошибок валидации токенов
tensortalks_business_token_validation_errors_total{service="auth-service", error_type="expired"}
```

### Метрики базы данных

```prometheus
# Количество запросов к БД
tensortalks_db_queries_total{service="user-store-service", operation="select", status="success"}

# Длительность запросов к БД
tensortalks_db_query_duration_seconds{service="user-store-service", operation="select"}

# Количество активных соединений
tensortalks_db_connections_active{service="user-store-service"}

# Количество ошибок БД
tensortalks_db_errors_total{service="user-store-service", error_type="connection_timeout"}
```

### Метрики внешних вызовов

```prometheus
# Количество вызовов внешних сервисов
tensortalks_external_requests_total{service="auth-service", target_service="user-store-service", status="success"}

# Длительность внешних вызовов
tensortalks_external_request_duration_seconds{service="auth-service", target_service="user-store-service"}
```

### Системные метрики

```prometheus
# Использование памяти
tensortalks_system_memory_bytes{service="auth-service", type="heap"}

# Количество горутин (для Go)
tensortalks_system_goroutines{service="auth-service"}
```

## Использование в Go-микросервисах

### Установка зависимостей

```go
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

### Базовый пример

```go
package main

import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "tensortalks_http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"service", "method", "endpoint", "status_code"},
    )

    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "tensortalks_http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"service", "method", "endpoint"},
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal)
    prometheus.MustRegister(httpRequestDuration)
}

func main() {
    // Регистрируем /metrics endpoint
    http.Handle("/metrics", promhttp.Handler())
    
    // ... остальной код сервера ...
}
```

### Middleware для HTTP-метрик

```go
package middleware

import (
    "strconv"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"
)

func PrometheusMiddleware(serviceName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        duration := time.Since(start).Seconds()
        statusCode := strconv.Itoa(c.Writer.Status())
        
        httpRequestsTotal.WithLabelValues(
            serviceName,
            c.Request.Method,
            c.FullPath(),
            statusCode,
        ).Inc()
        
        httpRequestDuration.WithLabelValues(
            serviceName,
            c.Request.Method,
            c.FullPath(),
        ).Observe(duration)
    }
}
```

### Бизнес-метрики

```go
var (
    registrationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "tensortalks_business_registrations_total",
            Help: "Total number of user registrations",
        },
        []string{"service", "status"},
    )
)

func (s *AuthService) Register(ctx context.Context, login, password string) {
    // ... регистрация ...
    
    if err != nil {
        registrationsTotal.WithLabelValues("auth-service", "error").Inc()
        return err
    }
    
    registrationsTotal.WithLabelValues("auth-service", "success").Inc()
}
```

## Использование в Python-микросервисах

### Установка зависимостей

```bash
pip install prometheus-client
```

### Базовый пример

```python
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from flask import Flask, Response

# Создание метрик
http_requests_total = Counter(
    'tensortalks_http_requests_total',
    'Total number of HTTP requests',
    ['service', 'method', 'endpoint', 'status_code']
)

http_request_duration = Histogram(
    'tensortalks_http_request_duration_seconds',
    'HTTP request duration in seconds',
    ['service', 'method', 'endpoint']
)

app = Flask(__name__)

@app.route('/metrics')
def metrics():
    return Response(generate_latest(), mimetype=CONTENT_TYPE_LATEST)

# Middleware для метрик
@app.before_request
def before_request():
    request.start_time = time.time()

@app.after_request
def after_request(response):
    duration = time.time() - request.start_time
    http_requests_total.labels(
        service='my-service',
        method=request.method,
        endpoint=request.endpoint,
        status_code=response.status_code
    ).inc()
    
    http_request_duration.labels(
        service='my-service',
        method=request.method,
        endpoint=request.endpoint
    ).observe(duration)
    
    return response
```

## Просмотр метрик в Grafana

### Доступ к Grafana

В Kubernetes:
```bash
kubectl port-forward -n tensor-talks svc/tensor-talks-grafana 3000:3000
```

Затем откройте в браузере: http://localhost:3000

Логин: `admin`, пароль: `admin` (измените в проде!)

### Настройка Prometheus как источника данных

Prometheus автоматически настроен как источник данных в Grafana через ConfigMap. Если нужно настроить вручную:

1. В Grafana: **Configuration** → **Data Sources** → **Add data source**
2. Выберите **Prometheus**
3. URL: `http://tensor-talks-prometheus:9090` (внутри кластера) или `http://localhost:9090` (через port-forward)
4. Нажмите **Save & Test**

### Создание дашборда

#### HTTP запросы в секунду

```promql
sum(rate(tensortalks_http_requests_total[5m])) by (service, endpoint)
```

#### Средняя длительность запросов

```promql
histogram_quantile(0.95, 
  sum(rate(tensortalks_http_request_duration_seconds_bucket[5m])) by (service, endpoint, le)
)
```

#### Количество ошибок

```promql
sum(rate(tensortalks_http_requests_total{status_code=~"5.."}[5m])) by (service)
```

#### Бизнес-метрики: регистрации

```promql
sum(rate(tensortalks_business_registrations_total[5m])) by (status)
```

#### Доступность сервисов

```promql
up{job="auth-service"}
```

### Готовые дашборды

Импортируйте готовые дашборды из Grafana Dashboard Library:
- **Node Exporter Full** (системные метрики)
- **Prometheus Stats** (метрики Prometheus)
- Создайте свой дашборд для TensorTalks метрик

## Конфигурация Prometheus

### В Kubernetes

Prometheus развертывается в namespace `tensor-talks` и автоматически собирает метрики из всех микросервисов через:
- Статическую конфигурацию в ConfigMap
- ServiceMonitor для автоматического обнаружения сервисов с аннотацией `prometheus.io/scrape: "true"`

Доступ к Prometheus:
```bash
kubectl port-forward -n tensor-talks svc/tensor-talks-prometheus 9090:9090
```

Затем откройте в браузере: http://localhost:9090

### Локальная разработка (Docker Compose)

Конфигурация в `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'auth-service'
    static_configs:
      - targets: ['auth-service:8081']
        labels:
          service: 'auth-service'
          environment: 'development'

  - job_name: 'bff-service'
    static_configs:
      - targets: ['bff-service:8080']
        labels:
          service: 'bff-service'
          environment: 'development'

  - job_name: 'user-store-service'
    static_configs:
      - targets: ['user-store-service:8082']
        labels:
          service: 'user-store-service'
          environment: 'development'

  - job_name: 'session-service'
    static_configs:
      - targets: ['session-service:8083']
        labels:
          service: 'session-service'
          environment: 'development'

  - job_name: 'mock-model-service'
    static_configs:
      - targets: ['mock-model-service:8084']
        labels:
          service: 'mock-model-service'
          environment: 'development'
```

## Alerting (опционально)

Настройте алерты в Prometheus для критических метрик:

```yaml
groups:
  - name: tensortalks_alerts
    rules:
      - alert: HighErrorRate
        expr: rate(tensortalks_http_requests_total{status_code=~"5.."}[5m]) > 0.1
        for: 5m
        annotations:
          summary: "High error rate in {{ $labels.service }}"
      
      - alert: ServiceDown
        expr: up{job=~"auth-service|bff-service|user-store-service|session-service|mock-model-service"} == 0
        for: 1m
        annotations:
          summary: "Service {{ $labels.job }} is down"
```

## Рекомендации

### Что метризовать

✅ **Обязательно:**
- HTTP-запросы (количество, длительность, статус-коды)
- Ошибки (количество, типы)
- Доступность сервисов (up/down)

✅ **Рекомендуется:**
- Бизнес-события (регистрации, логины, транзакции)
- Внешние вызовы (количество, длительность, ошибки)
- Использование ресурсов (память, CPU, соединения БД)

❌ **Не метризуйте:**
- Секретные данные (пароли, токены)
- Избыточные детали (каждый шаг алгоритма)
- Временные отладочные значения

### Best Practices

1. **Используйте гистограммы для длительностей** — они позволяют вычислять перцентили
2. **Группируйте метрики по labels** — не создавайте отдельную метрику для каждого значения
3. **Используйте единые имена** — следуйте соглашениям Prometheus
4. **Не перегружайте labels** — слишком много labels усложняет запросы
5. **Документируйте метрики** — используйте поле `Help` в описании метрик

## Troubleshooting

### Метрики не появляются в Prometheus

1. Проверьте, что `/metrics` endpoint доступен: `curl http://localhost:8081/metrics`
2. Проверьте конфигурацию Prometheus (`prometheus.yml`)
3. Проверьте логи Prometheus: `docker logs prometheus`

### Метрики не отображаются в Grafana

1. Убедитесь, что Prometheus добавлен как источник данных
2. Проверьте правильность PromQL запросов
3. Убедитесь, что временной диапазон выбран правильно

## Дополнительные ресурсы

- [Prometheus документация](https://prometheus.io/docs/)
- [PromQL язык запросов](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Grafana документация](https://grafana.com/docs/)
- [Prometheus Go client](https://github.com/prometheus/client_golang)
