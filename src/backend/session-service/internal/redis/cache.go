package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/tensor-talks/session-service/internal/models"
	"go.uber.org/zap"
)

// Cache представляет Redis кэш для активных сессий.
type Cache struct {
	client *redis.Client
	logger *zap.Logger
	ttl    time.Duration
}

// NewCache создаёт новый экземпляр Redis кэша.
func NewCache(addr, password string, db, ttlHours int, logger *zap.Logger) *Cache {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &Cache{
		client: rdb,
		logger: logger,
		ttl:    time.Duration(ttlHours) * time.Hour,
	}
}

// Ping проверяет подключение к Redis.
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// GetActiveSessionsCount возвращает количество активных сессий.
func (c *Cache) GetActiveSessionsCount(ctx context.Context) (int, error) {
	keys, err := c.client.Keys(ctx, "session:*").Result()
	if err != nil {
		return 0, fmt.Errorf("get keys: %w", err)
	}
	return len(keys), nil
}

// SetSession сохраняет сессию в кэш.
func (c *Cache) SetSession(ctx context.Context, session *models.CachedSession) error {
	key := fmt.Sprintf("session:%s", session.SessionID.String())

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("set session: %w", err)
	}

	c.logger.Info("Session cached",
		zap.String("session_id", session.SessionID.String()),
		zap.Duration("ttl", c.ttl),
	)

	return nil
}

// GetSession возвращает сессию из кэша.
func (c *Cache) GetSession(ctx context.Context, sessionID uuid.UUID) (*models.CachedSession, error) {
	key := fmt.Sprintf("session:%s", sessionID.String())

	data, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Сессия не найдена в кэше
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var session models.CachedSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &session, nil
}

// DeleteSession удаляет сессию из кэша.
func (c *Cache) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	key := fmt.Sprintf("session:%s", sessionID.String())

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	c.logger.Info("Session deleted from cache",
		zap.String("session_id", sessionID.String()),
	)

	return nil
}

// Close закрывает подключение к Redis.
func (c *Cache) Close() error {
	return c.client.Close()
}
