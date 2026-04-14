package client

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisStepClient reads agent processing steps from Redis.
// The interviewer-agent-service writes these under key "agent:step:{session_id}".
// This gives the BFF real-time visibility into which graph node is running.
type RedisStepClient struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// NewRedisStepClient creates a Redis step client. addr format: "host:port".
func NewRedisStepClient(addr, password string, db int, logger *zap.Logger) *RedisStepClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    password,
		DB:          db,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 500 * time.Millisecond,
	})
	return &RedisStepClient{rdb: rdb, logger: logger}
}

// GetAgentStep returns the current processing step for a session, or "" if not found/error.
func (r *RedisStepClient) GetAgentStep(sessionID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	val, err := r.rdb.Get(ctx, "agent:step:"+sessionID).Result()
	if err != nil {
		// redis.Nil = key not found — normal when no processing is happening
		return ""
	}
	return val
}

// Close closes the underlying Redis connection.
func (r *RedisStepClient) Close() {
	if err := r.rdb.Close(); err != nil {
		r.logger.Warn("Failed to close Redis step client", zap.Error(err))
	}
}
