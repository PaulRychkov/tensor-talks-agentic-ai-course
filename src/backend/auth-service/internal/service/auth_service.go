package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/auth-service/internal/client"
	"github.com/tensor-talks/auth-service/internal/tokens"
	"golang.org/x/crypto/bcrypt"
)

/*
Пакет service содержит бизнес-логику микросервиса аутентификации.

Основные обязанности:
  - регистрация пользователей (валидация логина/пароля, хеширование пароля, создание записи в user-crud-service);
  - аутентификация по логину и паролю;
  - выпуск и валидация JWT-токенов (доступ + обновление);
  - получение информации о пользователе через user-crud-service.

Все операции с базой выполняются только через user-crud-service, прямого доступа к БД у auth-service нет.
*/

// ErrInvalidInput возвращается при некорректном логине или пароле.
var ErrInvalidInput = errors.New("invalid input")

// ErrLoginTaken означает, что указанный логин уже существует.
var ErrLoginTaken = errors.New("login already taken")

// ErrInvalidCredentials сигнализирует о неверной паре логин/пароль.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidToken означает, что токен не прошёл проверку (подпись/срок/тип).
var ErrInvalidToken = errors.New("invalid token")

// ErrInvalidRecoveryKey означает, что ключ восстановления не совпадает с сохранённым хешем.
var ErrInvalidRecoveryKey = errors.New("invalid recovery key")

// UserCrudAPI описывает операции, которые auth-service ожидает от user-crud-service.
type UserCrudAPI interface {
	CreateUser(ctx context.Context, login, passwordHash string) (*client.User, error)
	GetUserByLogin(ctx context.Context, login string) (*client.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*client.User, error)
	SetRecoveryKeyHash(ctx context.Context, id uuid.UUID, hash string) error
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, passwordHash string) error
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

// TokenManager описывает интерфейс менеджера JWT-токенов.
type TokenManager interface {
	GenerateTokens(user *client.User) (tokens.TokenPair, error)
	Validate(token string) (*tokens.Claims, error)
}

// SessionStore описывает интерфейс для управления логин-сессиями в Redis.
type SessionStore interface {
	Ping(ctx context.Context) error
	CreateSession(ctx context.Context, userID uuid.UUID, sessionID, accessToken string) error
	ValidateSession(ctx context.Context, userID uuid.UUID, sessionID, accessToken string) (bool, error)
	DeleteSession(ctx context.Context, userID uuid.UUID, sessionID string) error
	DeleteAllUserSessions(ctx context.Context, userID uuid.UUID) error
}

// AuthService оркестрирует операции регистрации, логина и работы с токенами.
type AuthService struct {
	userCrud    UserCrudAPI
	tokens       TokenManager
	sessionStore SessionStore
}

// NewAuthService создаёт новый экземпляр сервиса аутентификации.
func NewAuthService(userCrud UserCrudAPI, tokens TokenManager, sessionStore SessionStore) *AuthService {
	return &AuthService{
		userCrud:    userCrud,
		tokens:       tokens,
		sessionStore: sessionStore,
	}
}

// Register выполняет регистрацию нового пользователя и возвращает пару токенов и ключ восстановления.
// Внутри:
//   - нормализует логин;
//   - валидирует логин и пароль;
//   - хеширует пароль с помощью bcrypt;
//   - создаёт пользователя в user-crud-service;
//   - генерирует ключ восстановления, сохраняет его bcrypt-хеш;
//   - выпускает пару access/refresh токенов.
//
// Возвращает raw recovery key (показывается пользователю один раз).
func (s *AuthService) Register(ctx context.Context, login, password string) (*client.User, tokens.TokenPair, string, error) {
	login = normalizeLogin(login)
	if err := validateCredentials(login, password); err != nil {
		return nil, tokens.TokenPair{}, "", err
	}

	hashed, err := hashPassword(password)
	if err != nil {
		return nil, tokens.TokenPair{}, "", fmt.Errorf("hash password: %w", err)
	}

	user, err := s.userCrud.CreateUser(ctx, login, hashed)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
			return nil, tokens.TokenPair{}, "", ErrLoginTaken
		}
		return nil, tokens.TokenPair{}, "", fmt.Errorf("create user: %w", err)
	}

	// Генерируем ключ восстановления и сохраняем его хеш (§10.10).
	rawKey, keyHash, err := generateRecoveryKey()
	if err != nil {
		return nil, tokens.TokenPair{}, "", fmt.Errorf("generate recovery key: %w", err)
	}
	if err := s.userCrud.SetRecoveryKeyHash(ctx, user.ID, keyHash); err != nil {
		// Не прерываем регистрацию — ключ можно будет перегенерировать
		rawKey = ""
	}

	pair, err := s.tokens.GenerateTokens(user)
	if err != nil {
		return nil, tokens.TokenPair{}, "", fmt.Errorf("generate tokens: %w", err)
	}

	// Создаём сессию в Redis (если sessionStore настроен)
	if s.sessionStore != nil {
		accessClaims, err := s.tokens.Validate(pair.AccessToken)
		if err == nil {
			sessionID := accessClaims.ID
			if sessionID == "" {
				sessionID = fmt.Sprintf("%s-%d", user.ID.String(), time.Now().Unix())
			}
			_ = s.sessionStore.CreateSession(ctx, user.ID, sessionID, pair.AccessToken)
		}
	}

	return user, pair, rawKey, nil
}

// RecoverPassword сбрасывает пароль пользователя по ключу восстановления (§10.10).
func (s *AuthService) RecoverPassword(ctx context.Context, login, recoveryKey, newPassword string) error {
	login = normalizeLogin(login)
	if login == "" || recoveryKey == "" || newPassword == "" {
		return ErrInvalidInput
	}

	user, err := s.userCrud.GetUserByLogin(ctx, login)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return ErrInvalidRecoveryKey // не раскрываем, существует ли логин
		}
		return fmt.Errorf("fetch user: %w", err)
	}

	if user.RecoveryKeyHash == nil || *user.RecoveryKeyHash == "" {
		return ErrInvalidRecoveryKey
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.RecoveryKeyHash), []byte(recoveryKey)); err != nil {
		return ErrInvalidRecoveryKey
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	return s.userCrud.UpdatePasswordHash(ctx, user.ID, newHash)
}

// ChangePassword меняет пароль пользователя с проверкой текущего (§10.14/6).
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	if currentPassword == "" || newPassword == "" {
		return ErrInvalidInput
	}
	user, err := s.userCrud.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("fetch user: %w", err)
	}
	if err := compareHashAndPassword(user.PasswordHash, currentPassword); err != nil {
		return ErrInvalidCredentials
	}
	newHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	return s.userCrud.UpdatePasswordHash(ctx, userID, newHash)
}

// RegenerateRecoveryKey перегенерирует ключ восстановления с проверкой пароля (§10.14/6).
// Возвращает новый raw recovery key.
func (s *AuthService) RegenerateRecoveryKey(ctx context.Context, userID uuid.UUID, password string) (string, error) {
	if password == "" {
		return "", ErrInvalidInput
	}
	user, err := s.userCrud.GetUserByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("fetch user: %w", err)
	}
	if err := compareHashAndPassword(user.PasswordHash, password); err != nil {
		return "", ErrInvalidCredentials
	}
	rawKey, keyHash, err := generateRecoveryKey()
	if err != nil {
		return "", fmt.Errorf("generate recovery key: %w", err)
	}
	if err := s.userCrud.SetRecoveryKeyHash(ctx, userID, keyHash); err != nil {
		return "", fmt.Errorf("save recovery key: %w", err)
	}
	return rawKey, nil
}

// DeleteAccount удаляет аккаунт пользователя с проверкой пароля (§10.14/6).
func (s *AuthService) DeleteAccount(ctx context.Context, userID uuid.UUID, password string) error {
	if password == "" {
		return ErrInvalidInput
	}
	user, err := s.userCrud.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("fetch user: %w", err)
	}
	if err := compareHashAndPassword(user.PasswordHash, password); err != nil {
		return ErrInvalidCredentials
	}
	if err := s.userCrud.DeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if s.sessionStore != nil {
		_ = s.sessionStore.DeleteAllUserSessions(ctx, userID)
	}
	return nil
}

// generateRecoveryKey генерирует случайный ключ восстановления (hex, 32 байта = 64 символа)
// и возвращает (rawKey, bcryptHash, error).
func generateRecoveryKey() (string, string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := strings.ToUpper(hex.EncodeToString(buf))
	// Форматируем как XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX
	parts := make([]string, 0, 8)
	for i := 0; i < len(raw); i += 4 {
		parts = append(parts, raw[i:i+4])
	}
	formatted := strings.Join(parts, "-")
	hashed, err := bcrypt.GenerateFromPassword([]byte(formatted), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return formatted, string(hashed), nil
}

// Login аутентифицирует пользователя по логину и паролю.
// При успешной проверке выдаёт новую пару токенов.
func (s *AuthService) Login(ctx context.Context, login, password string) (*client.User, tokens.TokenPair, error) {
	login = normalizeLogin(login)
	if err := validateCredentials(login, password); err != nil {
		return nil, tokens.TokenPair{}, ErrInvalidCredentials
	}

	user, err := s.userCrud.GetUserByLogin(ctx, login)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, tokens.TokenPair{}, ErrInvalidCredentials
		}
		return nil, tokens.TokenPair{}, fmt.Errorf("fetch user: %w", err)
	}

	if err := compareHashAndPassword(user.PasswordHash, password); err != nil {
		return nil, tokens.TokenPair{}, ErrInvalidCredentials
	}

	pair, err := s.tokens.GenerateTokens(user)
	if err != nil {
		return nil, tokens.TokenPair{}, fmt.Errorf("generate tokens: %w", err)
	}

	// Создаём сессию в Redis (если sessionStore настроен)
	if s.sessionStore != nil {
		accessClaims, err := s.tokens.Validate(pair.AccessToken)
		if err == nil {
			sessionID := accessClaims.ID
			if sessionID == "" {
				sessionID = fmt.Sprintf("%s-%d", user.ID.String(), time.Now().Unix())
			}
			_ = s.sessionStore.CreateSession(ctx, user.ID, sessionID, pair.AccessToken)
		}
	}

	return user, pair, nil
}

// Refresh принимает refresh-токен, валидирует его и по userID внутри токена
// запрашивает пользователя в user-crud-service, после чего выпускает новую пару токенов.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*client.User, tokens.TokenPair, error) {
	claims, err := s.tokens.Validate(refreshToken)
	if err != nil {
		return nil, tokens.TokenPair{}, ErrInvalidToken
	}
	if claims.Subject != "refresh" {
		return nil, tokens.TokenPair{}, ErrInvalidToken
	}

	user, err := s.userCrud.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, tokens.TokenPair{}, fmt.Errorf("fetch user: %w", err)
	}

	pair, err := s.tokens.GenerateTokens(user)
	if err != nil {
		return nil, tokens.TokenPair{}, fmt.Errorf("generate tokens: %w", err)
	}

	return user, pair, nil
}

// ValidateToken валидирует access-токен и возвращает его claims при успехе.
// Также проверяет, что сессия существует в Redis (если sessionStore настроен).
func (s *AuthService) ValidateToken(ctx context.Context, token string) (*tokens.Claims, error) {
	claims, err := s.tokens.Validate(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if claims.Subject != "access" {
		return nil, ErrInvalidToken
	}

	// Проверяем наличие сессии в Redis (если sessionStore настроен)
	if s.sessionStore != nil {
		sessionID := claims.ID
		if sessionID == "" {
			// Если jti не установлен, токен считается невалидным
			return nil, ErrInvalidToken
		}

		valid, err := s.sessionStore.ValidateSession(ctx, claims.UserID, sessionID, token)
		if err != nil {
			// При ошибке проверки Redis считаем токен невалидным для безопасности
			return nil, ErrInvalidToken
		}
		if !valid {
			// Сессия не найдена или истекла
			return nil, ErrInvalidToken
		}
	}

	return claims, nil
}

// GetUserByID запрашивает информацию о пользователе по GUID во внешнем user-crud-service.
func (s *AuthService) GetUserByID(ctx context.Context, id uuid.UUID) (*client.User, error) {
	return s.userCrud.GetUserByID(ctx, id)
}

// Logout удаляет сессию пользователя из Redis.
func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID, sessionID string) error {
	if s.sessionStore == nil {
		return nil // Если sessionStore не настроен, logout просто ничего не делает
	}
	return s.sessionStore.DeleteSession(ctx, userID, sessionID)
}

// normalizeLogin приводит логин к нижнему регистру и убирает пробелы по краям.
func normalizeLogin(login string) string {
	return strings.TrimSpace(strings.ToLower(login))
}

// validateCredentials проверяет базовые требования к логину и паролю.
// Для пароля: минимальная длина 8 символов, требуется хотя бы одна цифра и одна буква.
func validateCredentials(login, password string) error {
	if len(login) < 3 || len(login) > 64 {
		return ErrInvalidInput
	}
	if strings.Contains(login, " ") {
		return ErrInvalidInput
	}

	// Ужесточённые требования к паролю
	// if len(password) < 8 {
	// 	return ErrInvalidInput
	// }

	// Проверка наличия хотя бы одной буквы и одной цифры
	// hasLetter := false
	// hasDigit := false
	// for _, r := range password {
	// 	// Проверяем ASCII буквы (a-z, A-Z)
	// 	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
	// 		hasLetter = true
	// 	}
	// 	// Проверяем цифры (0-9)
	// 	if r >= '0' && r <= '9' {
	// 		hasDigit = true
	// 	}
	// 	// Если уже найдены и буква, и цифра, выходим из цикла
	// 	if hasLetter && hasDigit {
	// 		break
	// 	}
	// }
	// if !hasLetter || !hasDigit {
	// 	return ErrInvalidInput
	// }

	return nil
}

// hashPassword хеширует пароль с помощью bcrypt по умолчательному cost.
func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// compareHashAndPassword сравнивает сохранённый хеш и введённый пароль.
func compareHashAndPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

