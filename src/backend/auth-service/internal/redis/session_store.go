package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// SessionStore представляет Redis хранилище для активных логин-сессий.
type SessionStore struct {
	client *redis.Client
	logger *zap.Logger
	ttl    time.Duration // TTL для access токена (обычно меньше refresh)
}

// NewSessionStore создаёт новый экземпляр Redis хранилища сессий.
func NewSessionStore(addr, password string, db int, ttl time.Duration, logger *zap.Logger) *SessionStore {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &SessionStore{
		client: rdb,
		logger: logger,
		ttl:    ttl,
	}
}

// Ping проверяет подключение к Redis.
func (s *SessionStore) Ping(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	return s.client.Ping(ctx).Err()
}

// tokenFingerprint создаёт уникальный отпечаток токена для хранения в Redis.
func tokenFingerprint(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:16]) // Используем первые 16 байт
}

// CreateSession создаёт новую активную сессию логина.
// sessionID - уникальный идентификатор сессии (можно использовать jti из JWT или сгенерировать)
func (s *SessionStore) CreateSession(ctx context.Context, userID uuid.UUID, sessionID, accessToken string) error {
	// Используем два ключа:
	// 1. По userID для быстрого поиска всех сессий пользователя
	// 2. По sessionID для быстрой проверки конкретной сессии
	fingerprint := tokenFingerprint(accessToken)

	// Ключ для сессии: login_session:{user_id}:{session_id}
	sessionKey := fmt.Sprintf("login_session:%s:%s", userID.String(), sessionID)

	// Устанавливаем сессию с TTL
	if err := s.client.Set(ctx, sessionKey, fingerprint, s.ttl).Err(); err != nil {
		return fmt.Errorf("set session: %w", err)
	}

	// Также добавляем в set всех сессий пользователя для возможности инвалидации всех сессий
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	if err := s.client.SAdd(ctx, userSessionsKey, sessionID).Err(); err != nil {
		s.logger.Warn("Failed to add session to user sessions set", zap.Error(err))
	}
	// Устанавливаем TTL для set тоже
	s.client.Expire(ctx, userSessionsKey, s.ttl)

	s.logger.Info("Login session created",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID),
		zap.Duration("ttl", s.ttl),
	)

	return nil
}

// ValidateSession проверяет, активна ли сессия в Redis.
func (s *SessionStore) ValidateSession(ctx context.Context, userID uuid.UUID, sessionID, accessToken string) (bool, error) {
	sessionKey := fmt.Sprintf("login_session:%s:%s", userID.String(), sessionID)

	storedFingerprint, err := s.client.Get(ctx, sessionKey).Result()
	if err == redis.Nil {
		// Сессия не найдена в Redis (возможно, истекла или была удалена)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get session: %w", err)
	}

	// Сравниваем fingerprint текущего токена с сохранённым
	currentFingerprint := tokenFingerprint(accessToken)
	if storedFingerprint != currentFingerprint {
		// Fingerprint не совпадает - токен был изменён или сессия инвалидирована
		s.logger.Warn("Session fingerprint mismatch",
			zap.String("user_id", userID.String()),
			zap.String("session_id", sessionID),
		)
		return false, nil
	}

	return true, nil
}

// DeleteSession удаляет сессию из Redis.
func (s *SessionStore) DeleteSession(ctx context.Context, userID uuid.UUID, sessionID string) error {
	sessionKey := fmt.Sprintf("login_session:%s:%s", userID.String(), sessionID)
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())

	// Удаляем сессию
	if err := s.client.Del(ctx, sessionKey).Err(); err != nil {
		s.logger.Warn("Failed to delete session", zap.Error(err))
	}

	// Удаляем из set сессий пользователя
	if err := s.client.SRem(ctx, userSessionsKey, sessionID).Err(); err != nil {
		s.logger.Warn("Failed to remove session from user sessions set", zap.Error(err))
	}

	s.logger.Info("Login session deleted",
		zap.String("user_id", userID.String()),
		zap.String("session_id", sessionID),
	)

	return nil
}

// DeleteAllUserSessions удаляет все сессии пользователя (например, при смене пароля).
func (s *SessionStore) DeleteAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())

	// Получаем все session IDs
	sessionIDs, err := s.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get user sessions: %w", err)
	}

	// Удаляем каждую сессию
	for _, sessionID := range sessionIDs {
		sessionKey := fmt.Sprintf("login_session:%s:%s", userID.String(), sessionID)
		if err := s.client.Del(ctx, sessionKey).Err(); err != nil {
			s.logger.Warn("Failed to delete session",
				zap.String("user_id", userID.String()),
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Удаляем set
	if err := s.client.Del(ctx, userSessionsKey).Err(); err != nil {
		s.logger.Warn("Failed to delete user sessions set", zap.Error(err))
	}

	s.logger.Info("All user sessions deleted",
		zap.String("user_id", userID.String()),
		zap.Int("sessions_deleted", len(sessionIDs)),
	)

	return nil
}
