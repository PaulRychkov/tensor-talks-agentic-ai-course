package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Consumer читает события из Kafka (interview.build.response).
type Consumer struct {
	consumer     sarama.ConsumerGroup
	topic        string
	logger       *zap.Logger
	eventHandler EventHandler
}

// EventHandler обрабатывает события создания программы интервью.
type EventHandler interface {
	HandleInterviewBuildResponse(ctx context.Context, sessionID string, program map[string]interface{}, programMeta map[string]interface{}) error
}

// NewConsumer создаёт новый Kafka consumer.
func NewConsumer(brokers []string, topic, groupID string, logger *zap.Logger) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Return.Errors = true

	consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &Consumer{
		consumer: consumer,
		topic:    topic,
		logger:   logger,
	}, nil
}

// SetEventHandler устанавливает обработчик событий.
func (c *Consumer) SetEventHandler(handler EventHandler) {
	c.eventHandler = handler
}

// Start запускает consumer и начинает чтение сообщений.
func (c *Consumer) Start(ctx context.Context) error {
	handler := &consumerGroupHandler{
		consumer: c,
		logger:   c.logger,
	}

	topics := []string{c.topic}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := c.consumer.Consume(ctx, topics, handler); err != nil {
					c.logger.Error("Error from consumer", zap.Error(err))
				}
			}
		}
	}()

	// Обработка ошибок
	go func() {
		for err := range c.consumer.Errors() {
			c.logger.Error("Consumer error", zap.Error(err))
		}
	}()

	c.logger.Info("Kafka consumer started", zap.String("topic", c.topic))
	return nil
}

// Close закрывает consumer.
func (c *Consumer) Close() error {
	return c.consumer.Close()
}

// consumerGroupHandler реализует sarama.ConsumerGroupHandler.
type consumerGroupHandler struct {
	consumer *Consumer
	logger   *zap.Logger
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			var event InterviewBuilderEvent
			if err := json.Unmarshal(message.Value, &event); err != nil {
				h.logger.Error("Failed to unmarshal event",
					zap.Error(err),
					zap.ByteString("value", message.Value),
				)
				session.MarkMessage(message, "")
				continue
			}

			if event.EventType != "interview.build.response" {
				h.logger.Warn("Unknown event type",
					zap.String("event_type", event.EventType),
				)
				session.MarkMessage(message, "")
				continue
			}

			// Извлекаем session_id и program из payload
			sessionID, ok := event.Payload["session_id"].(string)
			if !ok {
				h.logger.Error("Invalid session_id type in payload")
				session.MarkMessage(message, "")
				continue
			}

			// Support both "program" (legacy) and "interview_program" (new format)
			program, ok := event.Payload["interview_program"].(map[string]interface{})
			if !ok {
				program, ok = event.Payload["program"].(map[string]interface{})
				if !ok {
					h.logger.Error("Invalid program type in payload")
					session.MarkMessage(message, "")
					continue
				}
			}

			// Extract program_meta (optional, new field)
			var programMeta map[string]interface{}
			if meta, ok := event.Payload["program_meta"].(map[string]interface{}); ok {
				programMeta = meta
			}

			h.logger.Info("Received interview build response",
				zap.String("session_id", sessionID),
				zap.Bool("has_program_meta", programMeta != nil),
			)

			if h.consumer.eventHandler != nil {
				ctx := context.Background()
				if err := h.consumer.eventHandler.HandleInterviewBuildResponse(ctx, sessionID, program, programMeta); err != nil {
					h.logger.Error("Failed to handle interview build response",
						zap.Error(err),
						zap.String("session_id", sessionID),
					)
				}
			}

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}
