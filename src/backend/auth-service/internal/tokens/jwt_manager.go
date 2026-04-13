package tokens

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/tensor-talks/auth-service/internal/client"
	"github.com/tensor-talks/auth-service/internal/config"
)

/*
Пакет tokens отвечает за выпуск и валидацию JWT-токенов.

Особенности реализации:
  - используется HMAC (HS256) с секретом из конфигурации;
  - выдаются два типа токенов: access (subject = "access") и refresh (subject = "refresh");
  - в claims помещаются GUID пользователя и его логин.

Все настройки (issuer, audience, TTL, secret) берутся из config.JWTConfig.
*/

// TokenPair описывает пару сгенерированных access и refresh токенов.
// Обычно возвращается с ответом на успешную регистрацию или логин и используется
// фронтендом для хранения сессии и выполнения последующих запросов.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Claims расширяет стандартные JWT claims пользовательскими данными.
// Эти claims вшиваются как в access-, так и в refresh-токен и позволяют
// однозначно идентифицировать пользователя по GUID и логину.
type Claims struct {
	UserID uuid.UUID `json:"uid"`
	Login  string    `json:"login"`
	jwt.RegisteredClaims
}

// Manager реализует выпуск и валидацию JWT-токенов.
type Manager struct {
	cfg config.JWTConfig
}

// NewManager создаёт новый экземпляр менеджера токенов.
func NewManager(cfg config.JWTConfig) *Manager {
	return &Manager{cfg: cfg}
}

// GenerateTokens строит и подписывает пару access/refresh токенов для заданного пользователя.
// Важно: на вход функция ожидает уже проверенного пользователя, поэтому никакой
// дополнительной авторизации здесь не выполняется — только формирование JWT.
func (m *Manager) GenerateTokens(user *client.User) (TokenPair, error) {
	if user == nil {
		return TokenPair{}, errors.New("user is nil")
	}
	accessClaims := m.buildClaims(user, m.cfg.AccessTokenTTL)
	refreshClaims := m.buildClaims(user, m.cfg.RefreshTokenTTL)
	refreshClaims.Subject = "refresh"

	accessToken, err := m.sign(accessClaims)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, err := m.sign(refreshClaims)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Validate парсит и валидирует строку токена, возвращая claims при успехе.
// Проверяются:
//   - корректность подписи по секрету;
//   - срок действия токена (exp);
//   - соответствие формату Claims.
func (m *Manager) Validate(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(m.cfg.Secret), nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := parsed.Claims.(*Claims); ok && parsed.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// buildClaims формирует базовый набор claims (issuer, audience, ttl) и данные пользователя.
func (m *Manager) buildClaims(user *client.User, ttl time.Duration) *Claims {
	now := time.Now().UTC()
	// Генерируем уникальный ID для токена (jti) для идентификации сессии
	jti := uuid.New().String()
	return &Claims{
		UserID: user.ID,
		Login:  user.Login,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti, // JWT ID для идентификации сессии
			Subject:   "access",
			Issuer:    m.cfg.Issuer,
			Audience:  jwt.ClaimStrings{m.cfg.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
}

// sign подписывает переданные claims с использованием алгоритма HS256 и секрета.
func (m *Manager) sign(claims *Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.cfg.Secret))
}
