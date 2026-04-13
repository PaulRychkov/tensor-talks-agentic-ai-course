package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Producer отправляет события в Kafka (chat.events.in).
type Producer struct {
	producer    sarama.SyncProducer
	topicIn     string
	serviceName string
	version     string
	logger      *zap.Logger
}

// NewProducer создаёт новый Kafka producer.
func NewProducer(brokers []string, topicIn, serviceName, version string, logger *zap.Logger) (*Producer, error) {
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
		topicIn:     topicIn,
		serviceName: serviceName,
		version:     version,
		logger:      logger,
	}, nil
}

// SendModelQuestion отправляет вопрос от модели.
func (p *Producer) SendModelQuestion(sessionID, userID, question string) error {
	questionID := uuid.New().String()
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.model_question",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload: map[string]interface{}{
			"session_id":  sessionID,
			"user_id":     userID,
			"question":    question,
			"question_id": questionID,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		},
	}

	return p.sendEvent(event, sessionID)
}

// SendChatCompleted отправляет событие завершения чата.
func (p *Producer) SendChatCompleted(sessionID, userID string, score int, feedback string, recommendations []string) error {
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.completed",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload: map[string]interface{}{
			"session_id": sessionID,
			"user_id":    userID,
			"results": map[string]interface{}{
				"score":           score,
				"feedback":        feedback,
				"recommendations": recommendations,
			},
			"completed_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	return p.sendEvent(event, sessionID)
}

func (p *Producer) sendEvent(event ChatEvent, sessionID string) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: p.topicIn,
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
		return fmt.Errorf("send message: %w", err)
	}

	p.logger.Info("Kafka message sent",
		zap.String("event_type", event.EventType),
		zap.String("session_id", sessionID),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
	)

	return nil
}

// Close закрывает producer.
func (p *Producer) Close() error {
	return p.producer.Close()
}
