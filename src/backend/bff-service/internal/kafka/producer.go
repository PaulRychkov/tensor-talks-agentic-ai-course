package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/tensor-talks/bff-service/internal/metrics"
	"go.uber.org/zap"
)

// Producer отправляет события в Kafka.
type Producer struct {
	producer    sarama.SyncProducer
	topicOut    string
	serviceName string
	version     string
	logger      *zap.Logger
}

// NewProducer создаёт новый Kafka producer.
func NewProducer(brokers []string, topicOut, serviceName, version string, logger *zap.Logger) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &Producer{
		producer:    producer,
		topicOut:    topicOut,
		serviceName: serviceName,
		version:     version,
		logger:      logger,
	}, nil
}

// SendChatStarted отправляет событие начала чата.
func (p *Producer) SendChatStarted(sessionID, userID, requestID string) error {
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.started",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload: map[string]interface{}{
			"session_id": sessionID,
			"user_id":    userID,
			"started_at": time.Now().UTC().Format(time.RFC3339),
		},
		Metadata: map[string]string{
			"request_id": requestID,
		},
	}

	return p.sendEvent(event)
}

// SendUserMessage отправляет событие сообщения пользователя.
func (p *Producer) SendUserMessage(sessionID, userID, content, messageID, requestID string) error {
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.user_message",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload: map[string]interface{}{
			"session_id": sessionID,
			"user_id":    userID,
			"content":    content,
			"message_id": messageID,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		},
		Metadata: map[string]string{
			"request_id": requestID,
		},
	}

	return p.sendEvent(event)
}

// SendChatTerminated отправляет событие досрочного завершения чата пользователем.
func (p *Producer) SendChatTerminated(sessionID, userID, requestID string) error {
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.terminated",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload: map[string]interface{}{
			"session_id":    sessionID,
			"user_id":       userID,
			"terminated_at": time.Now().UTC().Format(time.RFC3339),
		},
		Metadata: map[string]string{
			"request_id": requestID,
		},
	}

	return p.sendEvent(event)
}

// SendChatResumed отправляет событие восстановления активной сессии чата.
func (p *Producer) SendChatResumed(sessionID, userID, requestID string) error {
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.resumed",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload: map[string]interface{}{
			"session_id": sessionID,
			"user_id":    userID,
			"resumed_at": time.Now().UTC().Format(time.RFC3339),
		},
		Metadata: map[string]string{
			"request_id": requestID,
		},
	}

	return p.sendEvent(event)
}

func (p *Producer) sendEvent(event ChatEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Извлекаем session_id для ключа партиционирования и логирования
	sessionID := ""
	if sid, ok := event.Payload["session_id"].(string); ok {
		sessionID = sid
	}

	msg := &sarama.ProducerMessage{
		Topic: p.topicOut,
		Value: sarama.StringEncoder(data),
		Key:   sarama.StringEncoder(sessionID), // Используем session_id как ключ для партиционирования
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.Error("Failed to send Kafka message",
			zap.String("event_type", event.EventType),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		metrics.KafkaMessagesProducedTotal.WithLabelValues(
			p.serviceName,
			p.topicOut,
			event.EventType,
			"error",
		).Inc()
		return fmt.Errorf("send message: %w", err)
	}

	p.logger.Info("Kafka message sent",
		zap.String("event_type", event.EventType),
		zap.String("session_id", sessionID),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
	)

	// Метрика
	metrics.KafkaMessagesProducedTotal.WithLabelValues(
		p.serviceName,
		p.topicOut,
		event.EventType,
		"success",
	).Inc()

	return nil
}

// Close закрывает producer.
func (p *Producer) Close() error {
	return p.producer.Close()
}
