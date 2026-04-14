package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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

	// Router: one background consumer routes phrase.agent.generated events
	// to per-session waiting channels.
	waitersMu sync.Mutex
	waiters   map[string]chan *PhraseGeneratedEvent
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

	// Consumer — single background consumer for the router
	cCfg := sarama.NewConfig()
	cCfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	cCfg.Consumer.Offsets.Initial = sarama.OffsetNewest
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
		waiters:           make(map[string]chan *PhraseGeneratedEvent),
	}, nil
}

// StartRouter launches the single background Kafka consumer that routes
// phrase.agent.generated events to per-session waiting channels.
// Must be called once after creation; runs until ctx is cancelled.
func (b *AgentBridge) StartRouter(ctx context.Context) {
	handler := &routerConsumerHandler{bridge: b}
	topics := []string{b.topicGenerated}

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := b.consumer.Consume(ctx, topics, handler); err != nil {
				b.logger.Error("Agent bridge router consumer error", zap.Error(err))
				time.Sleep(time.Second)
			}
		}
	}()
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
	if event.EventType == "" {
		b.logger.Warn("Skipping event: missing event_type",
			zap.String("event_id", event.EventID),
		)
		return fmt.Errorf("missing required field: event_type")
	}
	if event.EventID == "" {
		b.logger.Warn("Skipping event: missing event_id")
		return fmt.Errorf("missing required field: event_id")
	}
	sessionID, _ := event.Payload["chat_id"].(string)
	if sessionID == "" {
		b.logger.Warn("Skipping event: missing session_id (chat_id) in payload",
			zap.String("event_id", event.EventID),
		)
		return fmt.Errorf("missing required field: session_id (chat_id in payload)")
	}
	if event.Metadata == nil {
		event.Metadata = make(map[string]interface{})
	}
	corrID, _ := event.Metadata["correlation_id"].(string)
	if corrID == "" {
		b.logger.Warn("Skipping event: missing correlation_id in metadata",
			zap.String("event_id", event.EventID),
		)
		return fmt.Errorf("missing required field: correlation_id")
	}

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
		zap.String("correlation_id", corrID),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
	)

	return nil
}

// ReceiveOnePhrase registers a per-session channel and waits for the router
// to deliver a matching phrase.agent.generated event.
func (b *AgentBridge) ReceiveOnePhrase(ctx context.Context, chatID string, correlationID string, timeout time.Duration) (*PhraseGeneratedEvent, error) {
	ch := make(chan *PhraseGeneratedEvent, 1)

	b.waitersMu.Lock()
	b.waiters[chatID] = ch
	b.waitersMu.Unlock()

	defer func() {
		b.waitersMu.Lock()
		delete(b.waiters, chatID)
		b.waitersMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for phrase.agent.generated")
	case ev := <-ch:
		return ev, nil
	}
}

// deliver routes an incoming event to the waiting goroutine (if any).
func (b *AgentBridge) deliver(chatID string, ev *PhraseGeneratedEvent) {
	b.waitersMu.Lock()
	ch, ok := b.waiters[chatID]
	b.waitersMu.Unlock()

	if ok {
		select {
		case ch <- ev:
		default:
		}
	}
}

// routerConsumerHandler — single background handler that routes events.
type routerConsumerHandler struct {
	bridge *AgentBridge
}

func (h *routerConsumerHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *routerConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *routerConsumerHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg := <-claim.Messages():
			if msg == nil {
				return nil
			}
			sess.MarkMessage(msg, "")

			var ev PhraseGeneratedEvent
			if err := json.Unmarshal(msg.Value, &ev); err != nil {
				h.bridge.logger.Error("Failed to unmarshal phrase.agent.generated",
					zap.Error(err),
					zap.ByteString("value", msg.Value),
				)
				continue
			}

			payloadChatID, _ := ev.Payload["chat_id"].(string)
			if payloadChatID != "" {
				h.bridge.deliver(payloadChatID, &ev)
			}

		case <-sess.Context().Done():
			return nil
		}
	}
}
