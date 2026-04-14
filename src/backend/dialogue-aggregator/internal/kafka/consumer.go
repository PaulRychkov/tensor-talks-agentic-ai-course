package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Consumer читает события из Kafka (chat.events.out).
type Consumer struct {
	consumer     sarama.ConsumerGroup
	topicOut     string
	logger       *zap.Logger
	eventHandler EventHandler
}

// EventHandler обрабатывает события от BFF.
type EventHandler interface {
	HandleChatStarted(ctx context.Context, sessionID, userID string) error
	HandleUserMessage(ctx context.Context, sessionID, userID, content, messageID string) error
	HandleChatResumed(ctx context.Context, sessionID, userID string) error
	HandleChatTerminated(ctx context.Context, sessionID, userID string) error
}

// NewConsumer создаёт новый Kafka consumer.
func NewConsumer(brokers []string, topicOut, groupID string, logger *zap.Logger) (*Consumer, error) {
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
		topicOut: topicOut,
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

	topics := []string{c.topicOut}

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

	c.logger.Info("Kafka consumer started", zap.String("topic", c.topicOut))
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

			var event ChatEvent
			if err := json.Unmarshal(message.Value, &event); err != nil {
				h.logger.Error("Failed to unmarshal event",
					zap.Error(err),
					zap.ByteString("value", message.Value),
				)
				session.MarkMessage(message, "")
				continue
			}

			// Извлекаем session_id и user_id из payload
			sessionID, ok := event.Payload["session_id"].(string)
			if !ok {
				h.logger.Error("Invalid session_id type in payload")
				session.MarkMessage(message, "")
				continue
			}
			userID, ok := event.Payload["user_id"].(string)
			if !ok {
				h.logger.Error("Invalid user_id type in payload")
				session.MarkMessage(message, "")
				continue
			}

			h.logger.Info("Received Kafka event",
				zap.String("event_type", event.EventType),
				zap.String("session_id", sessionID),
				zap.String("user_id", userID),
			)

			session.MarkMessage(message, "")

			if h.consumer.eventHandler != nil {
				handler := h.consumer.eventHandler
				logger := h.logger
				eventType := event.EventType
				sessCtx := session.Context()
				if event.TraceID != "" {
					sessCtx = ContextWithTraceID(sessCtx, event.TraceID)
				}

				switch eventType {
				case "chat.started":
					go func(sid, uid string) {
						ctx, cancel := context.WithCancel(sessCtx)
						defer cancel()
						if err := handler.HandleChatStarted(ctx, sid, uid); err != nil {
							logger.Error("Failed to handle chat started",
								zap.Error(err),
								zap.String("session_id", sid),
							)
						}
					}(sessionID, userID)

				case "chat.user_message":
					content, ok := event.Payload["content"].(string)
					if !ok {
						h.logger.Error("Invalid content type in payload")
						continue
					}
					messageID, _ := event.Payload["message_id"].(string)
					go func(sid, uid, c, mid string) {
						ctx, cancel := context.WithCancel(sessCtx)
						defer cancel()
						if err := handler.HandleUserMessage(ctx, sid, uid, c, mid); err != nil {
							logger.Error("Failed to handle user message",
								zap.Error(err),
								zap.String("session_id", sid),
							)
						}
					}(sessionID, userID, content, messageID)

				case "chat.resumed":
					go func(sid, uid string) {
						ctx, cancel := context.WithCancel(sessCtx)
						defer cancel()
						if err := handler.HandleChatResumed(ctx, sid, uid); err != nil {
							logger.Error("Failed to handle chat resumed",
								zap.Error(err),
								zap.String("session_id", sid),
							)
						}
					}(sessionID, userID)

				case "chat.terminated":
					go func(sid, uid string) {
						ctx, cancel := context.WithCancel(sessCtx)
						defer cancel()
						if err := handler.HandleChatTerminated(ctx, sid, uid); err != nil {
							logger.Error("Failed to handle chat terminated",
								zap.Error(err),
								zap.String("session_id", sid),
							)
						}
					}(sessionID, userID)

				default:
					h.logger.Warn("Unknown event type",
						zap.String("event_type", eventType),
					)
				}
			}

		case <-session.Context().Done():
			return nil
		}
	}
}
