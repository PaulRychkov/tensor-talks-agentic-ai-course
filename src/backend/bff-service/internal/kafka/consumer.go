package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/tensor-talks/bff-service/internal/metrics"
	"go.uber.org/zap"
)

// Consumer читает события из Kafka.
type Consumer struct {
	consumer     sarama.ConsumerGroup
	topicIn      string
	logger       *zap.Logger
	eventHandler EventHandler
}

// EventHandler обрабатывает события от модели.
type EventHandler interface {
	HandleModelQuestion(ctx context.Context, sessionID, userID, question, questionID string, questionNumber, totalQuestions int, piiMaskedContent string) error
	HandleChatCompleted(ctx context.Context, sessionID, userID string, results ChatResults) error
}

// ChatResults представляет результаты завершенного чата.
type ChatResults struct {
	Score           int      `json:"score"`
	Feedback        string   `json:"feedback"`
	Recommendations []string `json:"recommendations"`
}

// NewConsumer создаёт новый Kafka consumer.
func NewConsumer(brokers []string, topicIn, groupID string, logger *zap.Logger) (*Consumer, error) {
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
		topicIn:  topicIn,
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

	topics := []string{c.topicIn}

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

	c.logger.Info("Kafka consumer started", zap.String("topic", c.topicIn))
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

			processingStart := time.Now()

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

			// Метрика получения сообщения
			metrics.KafkaMessagesConsumedTotal.WithLabelValues(
				"bff-service",
				h.consumer.topicIn,
				event.EventType,
				"received",
			).Inc()

			if h.consumer.eventHandler != nil {
				ctx := context.Background()

				switch event.EventType {
				case "chat.model_question":
					question, ok := event.Payload["question"].(string)
					if !ok {
						h.logger.Error("Invalid question type in payload")
						session.MarkMessage(message, "")
						continue
					}
					questionID, ok := event.Payload["question_id"].(string)
					if !ok {
						h.logger.Error("Invalid question_id type in payload")
						session.MarkMessage(message, "")
						continue
					}
					questionNumber := 0
					totalQuestions := 0
					if v, ok := event.Payload["question_number"].(float64); ok {
						questionNumber = int(v)
					}
					if v, ok := event.Payload["total_questions"].(float64); ok {
						totalQuestions = int(v)
					}
					piiMaskedContent, _ := event.Payload["pii_masked_content"].(string)
					if err := h.consumer.eventHandler.HandleModelQuestion(ctx, sessionID, userID, question, questionID, questionNumber, totalQuestions, piiMaskedContent); err != nil {
						h.logger.Error("Failed to handle model question",
							zap.Error(err),
							zap.String("session_id", sessionID),
						)
					}

				case "chat.completed":
					resultsData, ok := event.Payload["results"].(map[string]interface{})
					if !ok {
						h.logger.Error("Invalid results type in payload")
						session.MarkMessage(message, "")
						continue
					}
					score, _ := resultsData["score"].(float64)
					feedback, _ := resultsData["feedback"].(string)
					recommendationsRaw, _ := resultsData["recommendations"].([]interface{})
					results := ChatResults{
						Score:           int(score),
						Feedback:        feedback,
						Recommendations: convertToStringSlice(recommendationsRaw),
					}
					if err := h.consumer.eventHandler.HandleChatCompleted(ctx, sessionID, userID, results); err != nil {
						h.logger.Error("Failed to handle chat completed",
							zap.Error(err),
							zap.String("session_id", sessionID),
						)
					}

				default:
					h.logger.Warn("Unknown event type",
						zap.String("event_type", event.EventType),
					)
					metrics.KafkaMessagesConsumedTotal.WithLabelValues(
						"bff-service",
						h.consumer.topicIn,
						event.EventType,
						"unknown",
					).Inc()
				}
			}

			// Метрика длительности обработки
			processingDuration := time.Since(processingStart).Seconds()
			metrics.KafkaMessageProcessingDuration.WithLabelValues(
				"bff-service",
				event.EventType,
			).Observe(processingDuration)

			// Метрика успешной обработки
			metrics.KafkaMessagesConsumedTotal.WithLabelValues(
				"bff-service",
				h.consumer.topicIn,
				event.EventType,
				"success",
			).Inc()

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

func convertToStringSlice(arr []interface{}) []string {
	if arr == nil {
		return []string{}
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	return result
}
