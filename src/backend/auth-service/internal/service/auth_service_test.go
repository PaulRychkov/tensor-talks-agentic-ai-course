package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensor-talks/auth-service/internal/client"
	"github.com/tensor-talks/auth-service/internal/tokens"
)

type mockUserStore struct {
	usersByLogin map[string]*client.User
	lastCreated  *client.User
	createError  error
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{usersByLogin: make(map[string]*client.User)}
}

func (m *mockUserStore) CreateUser(_ context.Context, login, passwordHash string) (*client.User, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	user := &client.User{
		ID:           uuid.New(),
		Login:        login,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.usersByLogin[login] = user
	m.lastCreated = user
	return user, nil
}

func (m *mockUserStore) GetUserByLogin(_ context.Context, login string) (*client.User, error) {
	user, ok := m.usersByLogin[login]
	if !ok {
		return nil, &client.APIError{Status: 404, Message: "not found"}
	}
	return user, nil
}

func (m *mockUserStore) GetUserByID(_ context.Context, id uuid.UUID) (*client.User, error) {
	for _, user := range m.usersByLogin {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, &client.APIError{Status: 404}
}

type mockTokenManager struct {
	pair        tokens.TokenPair
	err         error
	validateErr error
	validateMap map[string]*tokens.Claims
}

func (m *mockTokenManager) GenerateTokens(_ *client.User) (tokens.TokenPair, error) {
	return m.pair, m.err
}

func (m *mockTokenManager) Validate(token string) (*tokens.Claims, error) {
	if m.validateErr != nil {
		return nil, m.validateErr
	}
	if m.validateMap == nil {
		return nil, errors.New("no claims registered")
	}
	claims, ok := m.validateMap[token]
	if !ok {
		return nil, errors.New("unknown token")
	}
	return claims, nil
}

func TestNormalizeLogin(t *testing.T) {
	assert.Equal(t, "user", normalizeLogin("  User "))
}

func TestValidateCredentials(t *testing.T) {
	// Валидные креденшалы
	assert.NoError(t, validateCredentials("user", "password1"))
	assert.NoError(t, validateCredentials("user", "pass1234"))

	// Невалидные логины
	assert.Error(t, validateCredentials("us", "password1"))
	assert.Error(t, validateCredentials("user name", "password1"))

	// Невалидные пароли
	assert.Error(t, validateCredentials("user", "123"))      // слишком короткий
	assert.Error(t, validateCredentials("user", "password")) // нет цифры
	assert.Error(t, validateCredentials("user", "12345678")) // нет буквы
	assert.Error(t, validateCredentials("user", "pass1"))    // слишком короткий (< 8 символов)
}

func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("password123")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NoError(t, compareHashAndPassword(hash, "password123"))
	assert.Error(t, compareHashAndPassword(hash, "wrong"))
}

func TestRegisterSuccess(t *testing.T) {
	store := newMockUserStore()
	tokens := &mockTokenManager{
		pair: tokens.TokenPair{
			AccessToken:  "access",
			RefreshToken: "refresh",
		},
	}
	svc := NewAuthService(store, tokens, nil)

	user, pair, err := svc.Register(context.Background(), "UserName", "password123")
	require.NoError(t, err)
	assert.Equal(t, "username", user.Login)
	assert.Equal(t, "access", pair.AccessToken)
	assert.NotEmpty(t, store.lastCreated.PasswordHash)
	assert.NotEqual(t, "password123", store.lastCreated.PasswordHash)
}

func TestLoginSuccess(t *testing.T) {
	store := newMockUserStore()
	hash, err := hashPassword("password123")
	require.NoError(t, err)
	userID := uuid.New()
	store.usersByLogin["username"] = &client.User{
		ID:           userID,
		Login:        "username",
		PasswordHash: hash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	tokens := &mockTokenManager{
		pair: tokens.TokenPair{
			AccessToken:  "access",
			RefreshToken: "refresh",
		},
	}
	svc := NewAuthService(store, tokens, nil)

	user, pair, err := svc.Login(context.Background(), "username", "password123")
	require.NoError(t, err)
	assert.Equal(t, userID, user.ID)
	assert.Equal(t, "access", pair.AccessToken)
}

func TestLoginInvalidPassword(t *testing.T) {
	store := newMockUserStore()
	hash, err := hashPassword("password123")
	require.NoError(t, err)
	store.usersByLogin["username"] = &client.User{
		ID:           uuid.New(),
		Login:        "username",
		PasswordHash: hash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	tokens := &mockTokenManager{
		pair: tokens.TokenPair{},
	}
	svc := NewAuthService(store, tokens, nil)

	_, _, err = svc.Login(context.Background(), "username", "wrong")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestRefreshSuccess(t *testing.T) {
	store := newMockUserStore()
	userID := uuid.New()
	store.usersByLogin["username"] = &client.User{
		ID:        userID,
		Login:     "username",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	tokensManager := &mockTokenManager{
		pair: tokens.TokenPair{
			AccessToken:  "new_access",
			RefreshToken: "new_refresh",
		},
		validateMap: map[string]*tokens.Claims{
			"refresh": {
				UserID: userID,
				Login:  "username",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "refresh",
				},
			},
		},
	}
	svc := NewAuthService(store, tokensManager, nil)

	user, pair, err := svc.Refresh(context.Background(), "refresh")
	require.NoError(t, err)
	assert.Equal(t, "username", user.Login)
	assert.Equal(t, "new_access", pair.AccessToken)
}

func TestRefreshInvalidToken(t *testing.T) {
	store := newMockUserStore()
	tokensManager := &mockTokenManager{
		validateErr: errors.New("invalid"),
	}
	svc := NewAuthService(store, tokensManager, nil)

	_, _, err := svc.Refresh(context.Background(), "refresh")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateToken(t *testing.T) {
	tokensManager := &mockTokenManager{
		validateMap: map[string]*tokens.Claims{
			"access": {
				UserID: uuid.New(),
				Login:  "username",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "access",
				},
			},
			"refresh": {
				UserID: uuid.New(),
				Login:  "username",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "refresh",
				},
			},
		},
	}
	svc := NewAuthService(newMockUserStore(), tokensManager, nil)

	claims, err := svc.ValidateToken(context.Background(), "access")
	require.NoError(t, err)
	assert.Equal(t, "username", claims.Login)

	_, err = svc.ValidateToken(context.Background(), "refresh")
	assert.ErrorIs(t, err, ErrInvalidToken)
}
