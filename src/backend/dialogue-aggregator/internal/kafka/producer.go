package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Producer отправляет события в Kafka (chat.events.in, session.completed).
type Producer struct {
	producer              sarama.SyncProducer
	topicIn               string
	topicSessionCompleted string
	serviceName           string
	version               string
	logger                *zap.Logger
}

// NewProducer создаёт новый Kafka producer.
func NewProducer(brokers []string, topicIn, topicSessionCompleted, serviceName, version string, logger *zap.Logger) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &Producer{
		producer:              producer,
		topicIn:               topicIn,
		topicSessionCompleted: topicSessionCompleted,
		serviceName:           serviceName,
		version:               version,
		logger:                logger,
	}, nil
}

// SendModelQuestion отправляет вопрос от модели.
func (p *Producer) SendModelQuestion(sessionID, userID, question string, questionNumber, totalQuestions int, piiMaskedContent ...string) error {
	questionID := uuid.New().String()
	payload := map[string]interface{}{
		"session_id":      sessionID,
		"user_id":         userID,
		"question":        question,
		"question_id":     questionID,
		"question_number": questionNumber,
		"total_questions": totalQuestions,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
	if len(piiMaskedContent) > 0 && piiMaskedContent[0] != "" {
		payload["pii_masked_content"] = piiMaskedContent[0]
	}
	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "chat.model_question",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		Payload:   payload,
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

// SendSessionCompleted публикует событие session.completed в Kafka для analyst-agent-service.
func (p *Producer) SendSessionCompleted(sessionID, sessionKind, userID, chatID string, topics []string, level string, terminatedEarly bool, answeredQuestions, totalQuestions int, traceID string) error {
	if p.topicSessionCompleted == "" {
		p.logger.Warn("topic_session_completed not configured, skipping publish",
			zap.String("session_id", sessionID),
		)
		return nil
	}

	event := ChatEvent{
		EventID:   uuid.New().String(),
		EventType: "session.completed",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   p.serviceName,
		Version:   p.version,
		TraceID:   traceID,
		Payload: map[string]interface{}{
			"session_id":              sessionID,
			"session_kind":            sessionKind,
			"user_id":                 userID,
			"chat_id":                 chatID,
			"topics":                  topics,
			"level":                   level,
			"terminated_early":        terminatedEarly,
			"answered_questions":      answeredQuestions,
			"total_questions":         totalQuestions,
			"completed_at":            time.Now().UTC().Format(time.RFC3339),
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal session.completed event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: p.topicSessionCompleted,
		Value: sarama.StringEncoder(data),
		Key:   sarama.StringEncoder(sessionID),
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.Error("Failed to send session.completed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return fmt.Errorf("send session.completed: %w", err)
	}

	p.logger.Info("session.completed published",
		zap.String("session_id", sessionID),
		zap.Bool("terminated_early", terminatedEarly),
		zap.Int("answered_questions", answeredQuestions),
		zap.Int("total_questions", totalQuestions),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
	)
	return nil
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
