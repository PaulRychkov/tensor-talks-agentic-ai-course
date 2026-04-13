package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// AgentBridge отвечает за обмен с agent-service через Kafka:
// messages.full.data -> generated.phrases.
type AgentBridge struct {
	producer          sarama.SyncProducer
	consumer          sarama.ConsumerGroup
	topicMessagesFull string
	topicGenerated    string
	logger            *zap.Logger
}

type AgentBridgeConfig struct {
	Brokers        []string
	TopicMessages  string
	TopicGenerated string
	GroupID        string
}

// NewAgentBridge инициализирует producer/consumer для общения с agent-service.
func NewAgentBridge(cfg AgentBridgeConfig, logger *zap.Logger) (*AgentBridge, error) {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = []string{"kafka:9092"}
	}

	// Producer
	pCfg := sarama.NewConfig()
	pCfg.Producer.Return.Successes = true
	pCfg.Producer.RequiredAcks = sarama.WaitForAll
	pCfg.Producer.Retry.Max = 5

	producer, err := sarama.NewSyncProducer(cfg.Brokers, pCfg)
	if err != nil {
		return nil, fmt.Errorf("create agent bridge producer: %w", err)
	}

	// Consumer
	cCfg := sarama.NewConfig()
	cCfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	cCfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cCfg.Consumer.Return.Errors = true

	consumer, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, cCfg)
	if err != nil {
		_ = producer.Close()
		return nil, fmt.Errorf("create agent bridge consumer: %w", err)
	}

	return &AgentBridge{
		producer:          producer,
		consumer:          consumer,
		topicMessagesFull: cfg.TopicMessages,
		topicGenerated:    cfg.TopicGenerated,
		logger:            logger,
	}, nil
}

// MessageFullEvent минимальный формат события для messages.full.data.
type MessageFullEvent struct {
	EventID   string                 `json:"event_id"`
	EventType string                 `json:"event_type"`
	Timestamp string                 `json:"timestamp"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version"`
	Payload   map[string]interface{} `json:"payload"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// PhraseGeneratedEvent минимальный формат ответа агента.
type PhraseGeneratedEvent struct {
	EventID   string                 `json:"event_id"`
	EventType string                 `json:"event_type"`
	Timestamp string                 `json:"timestamp"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version"`
	Payload   map[string]interface{} `json:"payload"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SendMessageFull публикует событие message.full в messages.full.data.
func (b *AgentBridge) SendMessageFull(ctx context.Context, event MessageFullEvent, key string) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal message.full: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: b.topicMessagesFull,
		Value: sarama.ByteEncoder(data),
	}
	if key != "" {
		msg.Key = sarama.StringEncoder(key)
	}

	partition, offset, err := b.producer.SendMessage(msg)
	if err != nil {
		b.logger.Error("Failed to send message.full",
			zap.Error(err),
			zap.String("topic", b.topicMessagesFull),
		)
		return fmt.Errorf("send message.full: %w", err)
	}

	b.logger.Info("message.full sent",
		zap.String("topic", b.topicMessagesFull),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
	)

	return nil
}

// ReceiveOnePhrase ждёт одно событие phrase.agent.generated для заданного chat_id/корреляции.
func (b *AgentBridge) ReceiveOnePhrase(ctx context.Context, chatID string, correlationID string, timeout time.Duration) (*PhraseGeneratedEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	handler := &agentConsumerHandler{
		logger:       b.logger,
		chatID:       chatID,
		correlation:  correlationID,
		resultCh:     make(chan *PhraseGeneratedEvent, 1),
	}

	topics := []string{b.topicGenerated}

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := b.consumer.Consume(ctx, topics, handler); err != nil {
				b.logger.Error("Agent bridge consumer error", zap.Error(err))
				time.Sleep(time.Second)
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for phrase.agent.generated")
	case ev := <-handler.resultCh:
		return ev, nil
	}
}

type agentConsumerHandler struct {
	logger      *zap.Logger
	chatID      string
	correlation string
	resultCh    chan *PhraseGeneratedEvent
}

func (h *agentConsumerHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *agentConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *agentConsumerHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg := <-claim.Messages():
			if msg == nil {
				return nil
			}
			var ev PhraseGeneratedEvent
			if err := json.Unmarshal(msg.Value, &ev); err != nil {
				h.logger.Error("Failed to unmarshal phrase.agent.generated",
					zap.Error(err),
					zap.ByteString("value", msg.Value),
				)
				sess.MarkMessage(msg, "")
				continue
			}

			payloadChatID, _ := ev.Payload["chat_id"].(string)
			metaCorr := ""
			if ev.Metadata != nil {
				if v, ok := ev.Metadata["correlation_id"].(string); ok {
					metaCorr = v
				}
			}

			if payloadChatID == h.chatID && (h.correlation == "" || metaCorr == h.correlation) {
				select {
				case h.resultCh <- &ev:
				default:
				}
				sess.MarkMessage(msg, "")
				return nil
			}

			sess.MarkMessage(msg, "")
		case <-sess.Context().Done():
			return nil
		}
	}
}

