# Kafka очереди в микросервисах TensorTalks

## Обзор

Kafka используется для асинхронной обработки событий и обмена сообщениями между микросервисами TensorTalks. Это позволяет развязать сервисы и обеспечить надежную доставку сообщений.

## Архитектура

```
┌─────────────────┐
│  Producer        │
│  (микросервис)   │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Kafka Broker   │
│  (очередь)      │
└────────┬─────────┘
         │
         v
┌─────────────────┐
│  Consumer        │
│  (микросервис)   │
└─────────────────┘
```

## Основные концепции

### Topics (Топики)

Топик — это категория или канал сообщений. Каждый топик имеет имя и может содержать множество партиций для масштабирования.

**Топики для TensorTalks:**

- `chat.events.out` — события от BFF к модели (старт чата, ответ пользователя)
- `chat.events.in` — события от модели к BFF (вопрос от модели, результаты, окончание чата)
- `interview.build.request` — запрос на создание программы интервью (session-manager → interview-builder)
- `interview.build.response` — ответ с программой интервью (interview-builder → session-manager)

### Partitions (Партиции)

Партиция — это упорядоченная последовательность сообщений внутри топика. Партиции позволяют:
- Параллельную обработку сообщений
- Масштабирование топика
- Сохранение порядка сообщений в рамках партиции

### Consumer Groups (Группы потребителей)

Группа потребителей — это набор консьюмеров, которые совместно обрабатывают сообщения из топика. Каждое сообщение обрабатывается только одним консьюмером в группе.

## Формат сообщений

Все сообщения в Kafka должны быть в JSON-формате с единой структурой:

```json
{
  "event_id": "evt-abc123",
  "event_type": "user.registered",
  "timestamp": "2025-01-15T10:30:00.123Z",
  "service": "auth-service",
  "version": "1.0.0",
  "payload": {
    "user_id": "user-xyz789",
    "login": "user@example.com",
    "registered_at": "2025-01-15T10:30:00.123Z"
  },
  "metadata": {
    "request_id": "req-123",
    "correlation_id": "corr-456"
  }
}
```

### Обязательные поля

- `event_id` — уникальный ID события (UUID)
- `event_type` — тип события (например, `user.registered`, `interview.completed`)
- `timestamp` — время создания события (ISO 8601 UTC)
- `service` — имя сервиса, создавшего событие
- `version` — версия сервиса
- `payload` — данные события (зависит от типа события)

### Опциональные поля

- `metadata` — дополнительные метаданные (request_id, correlation_id и т.д.)

## Форматы событий чата

### Топик `chat.events.out` (BFF → Модель)

#### 1. Событие `chat.started`

Отправляется когда пользователь начинает новый чат.

```json
{
  "event_id": "evt-abc123",
  "event_type": "chat.started",
  "timestamp": "2025-01-15T10:30:00.123Z",
  "service": "bff-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "user_id": "user-123456",
    "started_at": "2025-01-15T10:30:00.123Z"
  },
  "metadata": {
    "request_id": "req-123"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии чата
- `user_id` (string, UUID) — идентификатор пользователя
- `started_at` (string, ISO 8601) — время начала чата

#### 2. Событие `chat.resumed`

Отправляется когда пользователь открывает активную сессию чата для продолжения интервью.

```json
{
  "event_id": "evt-resume-001",
  "event_type": "chat.resumed",
  "timestamp": "2025-01-15T10:35:00.123Z",
  "service": "bff-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "user_id": "user-123456",
    "resumed_at": "2025-01-15T10:35:00.123Z"
  },
  "metadata": {
    "request_id": "req-resume-123"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии чата
- `user_id` (string, UUID) — идентификатор пользователя
- `resumed_at` (string, ISO 8601) — время восстановления сессии

#### 3. Событие `chat.user_message`

Отправляется когда пользователь отправляет сообщение в чат.

```json
{
  "event_id": "evt-def456",
  "event_type": "chat.user_message",
  "timestamp": "2025-01-15T10:31:00.123Z",
  "service": "bff-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "user_id": "user-123456",
    "content": "Объясните разницу между L1 и L2 регуляризацией",
    "message_id": "msg-789",
    "timestamp": "2025-01-15T10:31:00.123Z"
  },
  "metadata": {
    "request_id": "req-456"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии чата
- `user_id` (string, UUID) — идентификатор пользователя
- `content` (string) — текст сообщения пользователя
- `message_id` (string, UUID) — уникальный идентификатор сообщения
- `timestamp` (string, ISO 8601) — время отправки сообщения

### Топик `chat.events.in` (Модель → BFF)

#### 1. Событие `chat.model_question`

Отправляется когда модель задает вопрос пользователю.

```json
{
  "event_id": "evt-ghi789",
  "event_type": "chat.model_question",
  "timestamp": "2025-01-15T10:32:00.123Z",
  "service": "model-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "user_id": "user-123456",
    "question": "Объясните разницу между L1 и L2 регуляризацией.",
    "question_id": "q-123",
    "timestamp": "2025-01-15T10:32:00.123Z"
  },
  "metadata": {
    "correlation_id": "evt-abc123"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии чата
- `user_id` (string, UUID) — идентификатор пользователя
- `question` (string) — текст вопроса от модели
- `question_id` (string, UUID) — уникальный идентификатор вопроса
- `timestamp` (string, ISO 8601) — время создания вопроса

#### 4. Событие `chat.terminated`

Отправляется когда пользователь досрочно завершает интервью.

```json
{
  "event_id": "evt-term-001",
  "event_type": "chat.terminated",
  "timestamp": "2025-01-15T10:40:00.123Z",
  "service": "bff-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "user_id": "user-123456",
    "terminated_at": "2025-01-15T10:40:00.123Z"
  },
  "metadata": {
    "request_id": "req-term-123"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии чата
- `user_id` (string, UUID) — идентификатор пользователя
- `terminated_at` (string, ISO 8601) — время досрочного завершения

#### 5. Событие `chat.completed`

Отправляется когда чат завершен (интервью окончено).

```json
{
  "event_id": "evt-jkl012",
  "event_type": "chat.completed",
  "timestamp": "2025-01-15T10:45:00.123Z",
  "service": "model-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "user_id": "user-123456",
    "results": {
      "score": 85,
      "feedback": "Хорошее понимание основ ML",
      "recommendations": ["Изучить продвинутые техники", "Практика с реальными данными"]
    },
    "completed_at": "2025-01-15T10:45:00.123Z"
  },
  "metadata": {
    "correlation_id": "evt-abc123"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии чата
- `user_id` (string, UUID) — идентификатор пользователя
- `results` (object) — результаты интервью:
  - `score` (number) — оценка (0-100)
  - `feedback` (string) — текстовая обратная связь
  - `recommendations` (array of strings) — рекомендации для улучшения
- `completed_at` (string, ISO 8601) — время завершения чата

### Новые топики для создания программы интервью

#### Топик `interview.build.request` (Session Manager → Interview Builder)

**Событие `interview.build.request`:**

Отправляется когда session-manager запрашивает создание программы интервью для новой сессии.

```json
{
  "event_id": "evt-build-001",
  "event_type": "interview.build.request",
  "timestamp": "2025-01-15T10:30:00.123Z",
  "service": "session-manager-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "params": {
      "topics": ["ml", "nlp"],
      "level": "middle",
      "type": "interview"
    }
  },
  "metadata": {
    "request_id": "req-build-123"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии
- `params` (object) — параметры интервьюируемого:
  - `topics` (array of strings) — темы интервью
  - `level` (string) — уровень сложности (junior, middle, senior)
  - `type` (string) — тип интервью (interview, training)

#### Топик `interview.build.response` (Interview Builder → Session Manager)

**Событие `interview.build.response`:**

Отправляется когда interview-builder готовит программу интервью.

```json
{
  "event_id": "evt-build-002",
  "event_type": "interview.build.response",
  "timestamp": "2025-01-15T10:30:05.456Z",
  "service": "interview-builder-service",
  "version": "1.0.0",
  "payload": {
    "session_id": "session-xyz789",
    "program": {
      "questions": [
        {
          "question": "Объясните разницу между L1 и L2 регуляризацией.",
          "theory": "L1 регуляризация (Lasso) добавляет сумму абсолютных значений весов...",
          "order": 1
        },
        {
          "question": "Как работает кросс-валидация k-fold?",
          "theory": "Кросс-валидация k-fold разделяет данные на k частей...",
          "order": 2
        }
      ]
    }
  },
  "metadata": {
    "correlation_id": "evt-build-001"
  }
}
```

**Поля payload:**
- `session_id` (string, UUID) — идентификатор сессии
- `program` (object) — программа интервью:
  - `questions` (array of objects) — упорядоченный список вопросов:
    - `question` (string) — текст вопроса
    - `theory` (string) — теория к вопросу
    - `order` (number) — порядковый номер вопроса

## Использование в Go-микросервисах

### Установка зависимостей

```go
go get github.com/IBM/sarama
```

### Producer (отправка сообщений)

```go
package kafka

import (
    "encoding/json"
    "github.com/IBM/sarama"
)

type Producer struct {
    producer sarama.SyncProducer
    serviceName string
    version string
}

func NewProducer(brokers []string, serviceName, version string) (*Producer, error) {
    config := sarama.NewConfig()
    config.Producer.Return.Successes = true
    config.Producer.RequiredAcks = sarama.WaitForAll
    
    producer, err := sarama.NewSyncProducer(brokers, config)
    if err != nil {
        return nil, err
    }
    
    return &Producer{
        producer: producer,
        serviceName: serviceName,
        version: version,
    }, nil
}

func (p *Producer) SendEvent(topic, eventType string, payload interface{}) error {
    event := Event{
        EventID:   uuid.New().String(),
        EventType: eventType,
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Service:   p.serviceName,
        Version:   p.version,
        Payload:   payload,
    }
    
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    msg := &sarama.ProducerMessage{
        Topic: topic,
        Value: sarama.StringEncoder(data),
    }
    
    _, _, err = p.producer.SendMessage(msg)
    return err
}
```

### Consumer (получение сообщений)

```go
package kafka

import (
    "context"
    "encoding/json"
    "github.com/IBM/sarama"
)

type Consumer struct {
    consumer sarama.ConsumerGroup
    handlers map[string]EventHandler
}

type EventHandler func(ctx context.Context, event Event) error

func NewConsumer(brokers []string, groupID string) (*Consumer, error) {
    config := sarama.NewConfig()
    config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
    config.Consumer.Offsets.Initial = sarama.OffsetOldest
    
    consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
    if err != nil {
        return nil, err
    }
    
    return &Consumer{
        consumer: consumer,
        handlers: make(map[string]EventHandler),
    }, nil
}

func (c *Consumer) RegisterHandler(eventType string, handler EventHandler) {
    c.handlers[eventType] = handler
}

func (c *Consumer) Consume(ctx context.Context, topics []string) error {
    handler := &consumerGroupHandler{
        handlers: c.handlers,
    }
    
    return c.consumer.Consume(ctx, topics, handler)
}
```

## Использование в Python-микросервисах

### Установка зависимостей

```bash
pip install kafka-python
```

### Producer

```python
from kafka import KafkaProducer
import json
import uuid
from datetime import datetime

class EventProducer:
    def __init__(self, brokers, service_name, version):
        self.producer = KafkaProducer(
            bootstrap_servers=brokers,
            value_serializer=lambda v: json.dumps(v).encode('utf-8')
        )
        self.service_name = service_name
        self.version = version
    
    def send_event(self, topic, event_type, payload):
        event = {
            "event_id": str(uuid.uuid4()),
            "event_type": event_type,
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "service": self.service_name,
            "version": self.version,
            "payload": payload
        }
        
        self.producer.send(topic, event)
        self.producer.flush()
```

### Consumer

```python
from kafka import KafkaConsumer
import json

class EventConsumer:
    def __init__(self, brokers, group_id):
        self.consumer = KafkaConsumer(
            bootstrap_servers=brokers,
            group_id=group_id,
            value_deserializer=lambda m: json.loads(m.decode('utf-8')),
            auto_offset_reset='earliest'
        )
        self.handlers = {}
    
    def register_handler(self, event_type, handler):
        self.handlers[event_type] = handler
    
    def consume(self, topics):
        self.consumer.subscribe(topics)
        
        for message in self.consumer:
            event = message.value
            event_type = event.get('event_type')
            
            if event_type in self.handlers:
                self.handlers[event_type](event)
```

## Примеры использования

### Отправка события регистрации

```go
// В auth-service после успешной регистрации
producer.SendEvent(
    "user.events",
    "user.registered",
    map[string]interface{}{
        "user_id": user.ID.String(),
        "login": user.Login,
        "registered_at": time.Now().UTC().Format(time.RFC3339),
    },
)
```

### Обработка события регистрации

```go
// В другом сервисе (например, notification-service)
consumer.RegisterHandler("user.registered", func(ctx context.Context, event Event) error {
    payload := event.Payload.(map[string]interface{})
    userID := payload["user_id"].(string)
    
    // Отправить приветственное письмо
    return sendWelcomeEmail(userID)
})
```

## Конфигурация Kafka

### Развертывание в Kubernetes через Strimzi

Kafka развертывается в namespace `tensor-talks` через Strimzi Operator с использованием KRaft режима (без Zookeeper).

#### Установка Strimzi Operator

```bash
kubectl create -f 'https://strimzi.io/install/latest?namespace=tensor-talks' -n tensor-talks
```

#### Развертывание Kafka кластера

Kafka кластер развертывается автоматически при установке Helm чарта:

```bash
helm install tensor-talks ./helm --namespace tensor-talks --create-namespace
```

Кластер создается с именем `tensor-talks-kafka` в namespace `tensor-talks`.

#### Bootstrap Server

После развертывания Kafka доступен по адресу:
```
tensor-talks-kafka-bootstrap.tensor-talks.svc.cluster.local:9092
```

#### Kafka UI (Kafdrop)

Kafdrop развертывается автоматически для мониторинга Kafka. Доступ через port-forward:

```bash
kubectl port-forward -n tensor-talks svc/tensor-talks-kafdrop 9000:9000
```

Затем откройте в браузере: http://localhost:9000

### Docker Compose (для локальной разработки)

```yaml
kafka:
  image: confluentinc/cp-kafka:7.5.0
  environment:
    KAFKA_BROKER_ID: 1
    KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
    KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092,PLAINTEXT_HOST://localhost:29092
    KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT
    KAFKA_INTER_BROKER_LISTENER_NAME: PLAINTEXT
    KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
  ports:
    - "29092:29092"
    - "9092:9092"
  depends_on:
    - zookeeper

zookeeper:
  image: confluentinc/cp-zookeeper:latest
  environment:
    ZOOKEEPER_CLIENT_PORT: 2181
    ZOOKEEPER_TICK_TIME: 2000
  ports:
    - "2181:2181"

kafdrop:
  image: obsidiandynamics/kafdrop:latest
  environment:
    KAFKA_BROKERCONNECT: kafka:9092
    SERVER_PORT: 9000
    JVM_OPTS: "-Xms32M -Xmx64M"
  ports:
    - "9000:9000"
  depends_on:
    - kafka
```

## Best Practices

### Именование топиков

Используйте формат: `{domain}.{entity}.{action}`

Примеры:
- `user.events` — события пользователей
- `interview.events` — события интервью
- `notification.commands` — команды для уведомлений

### Обработка ошибок

- Всегда логируйте ошибки обработки сообщений
- Используйте dead letter queue для сообщений, которые не удалось обработать
- Реализуйте retry механизм с экспоненциальной задержкой

### Производительность

- Используйте батчинг для отправки сообщений
- Настройте количество партиций в зависимости от нагрузки
- Мониторьте lag консьюмеров

### Безопасность

- Не отправляйте секретные данные (пароли, токены) в сообщениях
- Используйте шифрование для чувствительных данных
- Настройте ACL для ограничения доступа к топикам

## Мониторинг

### Веб-интерфейс Kafdrop

Для удобного просмотра Kafka доступен **Kafdrop** — веб-интерфейс для управления топиками и просмотра сообщений.

**URL**: http://localhost:9000

**Возможности:**
- Просмотр всех топиков
- Просмотр сообщений в топиках
- Просмотр Consumer Groups
- Информация о партициях и offset

### Метрики Kafka

- `kafka_consumer_lag` — отставание консьюмера
- `kafka_producer_messages_total` — количество отправленных сообщений
- `kafka_consumer_messages_total` — количество обработанных сообщений

### Просмотр в Grafana

Создайте дашборды для:
- Скорости обработки сообщений
- Lag консьюмеров
- Количества ошибок обработки
- Размера топиков

## Troubleshooting

### Сообщения не доставляются

1. Проверьте, что Kafka запущен: `docker ps | grep kafka`
2. Проверьте подключение к брокеру
3. Проверьте логи producer/consumer

### Сообщения обрабатываются медленно

1. Увеличьте количество партиций
2. Добавьте больше консьюмеров в группу
3. Оптимизируйте обработку сообщений

## Дополнительные ресурсы

- [Kafka документация](https://kafka.apache.org/documentation/)
- [Sarama Go client](https://github.com/IBM/sarama)
- [kafka-python](https://github.com/dpkp/kafka-python)
- [Confluent Kafka](https://www.confluent.io/)
