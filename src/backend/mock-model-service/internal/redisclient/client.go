package redisclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Client обёртка над go-redis для работы с состоянием диалога.
type Client struct {
	rdb *redis.Client
}

type Config struct {
	Addr     string
	Password string
	DB       int
}

// New создаёт новый Redis клиент.
func New(cfg Config) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	return &Client{rdb: rdb}
}

func messagesKey(chatID string) string {
	return fmt.Sprintf("dialogue:%s:messages", chatID)
}

func stateKey(chatID string) string {
	return fmt.Sprintf("dialogue:%s:state", chatID)
}

// AppendMessage добавляет сообщение в историю диалога.
// msg должен быть JSON-совместимой мапой (role, content, timestamp и т.п.).
func (c *Client) AppendMessage(ctx context.Context, chatID string, msg map[string]any) error {
	if c == nil || c.rdb == nil {
		return nil
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal redis message: %w", err)
	}

	if err := c.rdb.RPush(ctx, messagesKey(chatID), data).Err(); err != nil {
		return fmt.Errorf("redis RPUSH: %w", err)
	}

	return nil
}

// SetState сохраняет агрегированное состояние диалога.
func (c *Client) SetState(ctx context.Context, chatID string, state map[string]any) error {
	if c == nil || c.rdb == nil {
		return nil
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal redis state: %w", err)
	}

	if err := c.rdb.Set(ctx, stateKey(chatID), data, 0).Err(); err != nil {
		return fmt.Errorf("redis SET: %w", err)
	}

	return nil
}

// GetState возвращает агрегированное состояние диалога.
func (c *Client) GetState(ctx context.Context, chatID string) (map[string]any, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	data, err := c.rdb.Get(ctx, stateKey(chatID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("redis GET: %w", err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal redis state: %w", err)
	}
	return state, nil
}

