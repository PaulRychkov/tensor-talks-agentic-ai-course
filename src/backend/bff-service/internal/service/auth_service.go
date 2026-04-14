package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tensor-talks/bff-service/internal/client"
)

/*
Пакет service содержит бизнес-логику BFF, связанную с аутентификацией.

По сути, это тонкая обёртка над HTTP-клиентом к auth-service, которая:
  - маппит HTTP-статусы из auth-service на доменные ошибки BFF;
  - скрывает детали протокола и форматов ошибок от HTTP-слоя.
*/

// ErrInvalidCredentials означает, что auth-service отверг логин/пароль или токен.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrConflict обозначает конфликт ресурса (например, логин уже занят).
var ErrConflict = errors.New("resource conflict")

// ErrBadRequest указывает на ошибку валидации входных данных.
var ErrBadRequest = errors.New("bad request")

// AuthAPI описывает интерфейс клиента auth-service.
// Выделен как интерфейс, чтобы упростить тестирование AuthService (можно подставлять моки).
type AuthAPI interface {
	Register(ctx context.Context, login, password string) (*client.AuthResponse, error)
	Login(ctx context.Context, login, password string) (*client.AuthResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*client.AuthResponse, error)
	Me(ctx context.Context, accessToken string) (*client.User, error)
	Logout(ctx context.Context, accessToken string) error
	Recover(ctx context.Context, login, recoveryKey, newPassword string) error
	ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error
	RegenerateRecoveryKey(ctx context.Context, accessToken, password string) (string, error)
	DeleteAccount(ctx context.Context, accessToken, password string) error
}

// AuthService инкапсулирует бизнес-логику BFF, связанную с аутентификацией.
// Его задача — скрыть детали протокола auth-service от HTTP-слоя BFF и предоставить
// высокоуровневые операции регистрации, логина, обновления токенов и получения текущего пользователя.
type AuthService struct {
	client AuthAPI
}

// DetailedError хранит базовую ошибку и человекочитаемое сообщение, пришедшее из auth-service.
type DetailedError struct {
	base    error
	message string
}

func (e *DetailedError) Error() string {
	if e == nil {
		return ""
	}
	if e.message == "" {
		return e.base.Error()
	}
	return fmt.Sprintf("%s: %s", e.base.Error(), e.message)
}

// Unwrap позволяет использовать errors.Is / errors.As для DetailedError.
func (e *DetailedError) Unwrap() error {
	return e.base
}

// Message возвращает подробное сообщение об ошибке, если оно было задано.
func (e *DetailedError) Message() string {
	return e.message
}

// NewAuthService создаёт новый сервис аутентификации BFF.
// На вход принимает зависимость по интерфейсу AuthAPI (реальный HTTP-клиент или мок).
func NewAuthService(client AuthAPI) *AuthService {
	return &AuthService{client: client}
}

// Register регистрирует пользователя через auth-service.
// В случае ошибок HTTP-клиента или статус-кодов из auth-service возвращает
// доменные ошибки BFF через функцию mapError.
func (s *AuthService) Register(ctx context.Context, login, password string) (*client.AuthResponse, error) {
	resp, err := s.client.Register(ctx, login, password)
	if err != nil {
		return nil, mapError(err)
	}
	return resp, nil
}

// Login выполняет вход пользователя через auth-service.
// При успехе возвращает информацию о пользователе и пару токенов; при ошибках
// — доменные ошибки, удобные для HTTP-слоя.
func (s *AuthService) Login(ctx context.Context, login, password string) (*client.AuthResponse, error) {
	resp, err := s.client.Login(ctx, login, password)
	if err != nil {
		return nil, mapError(err)
	}
	return resp, nil
}

// Refresh запрашивает новые токены по refresh-токену.
// Внутренне вызывает соответствующий эндпоинт auth-service и маппит ошибки.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*client.AuthResponse, error) {
	resp, err := s.client.Refresh(ctx, refreshToken)
	if err != nil {
		return nil, mapError(err)
	}
	return resp, nil
}

// CurrentUser возвращает информацию о пользователе по access-токену.
// Используется для реализации /api/auth/me — BFF сам не валидирует токен, а
// полагается на auth-service.
func (s *AuthService) CurrentUser(ctx context.Context, accessToken string) (*client.User, error) {
	user, err := s.client.Me(ctx, accessToken)
	if err != nil {
		return nil, mapError(err)
	}
	return user, nil
}

// Logout инвалидирует сессию пользователя.
func (s *AuthService) Logout(ctx context.Context, accessToken string) error {
	return mapError(s.client.Logout(ctx, accessToken))
}

// Recover сбрасывает пароль по ключу восстановления.
func (s *AuthService) Recover(ctx context.Context, login, recoveryKey, newPassword string) error {
	return mapError(s.client.Recover(ctx, login, recoveryKey, newPassword))
}

// ChangePassword меняет пароль аутентифицированного пользователя.
func (s *AuthService) ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error {
	return mapError(s.client.ChangePassword(ctx, accessToken, currentPassword, newPassword))
}

// RegenerateRecoveryKey перегенерирует ключ восстановления.
func (s *AuthService) RegenerateRecoveryKey(ctx context.Context, accessToken, password string) (string, error) {
	key, err := s.client.RegenerateRecoveryKey(ctx, accessToken, password)
	if err != nil {
		return "", mapError(err)
	}
	return key, nil
}

// DeleteAccount удаляет аккаунт пользователя.
func (s *AuthService) DeleteAccount(ctx context.Context, accessToken, password string) error {
	return mapError(s.client.DeleteAccount(ctx, accessToken, password))
}

// mapError преобразует ошибку HTTP-клиента auth-service в одну из доменных
// ошибок BFF (ErrBadRequest, ErrInvalidCredentials, ErrConflict) с сохранением
// человекочитаемого сообщения.
func mapError(err error) error {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusBadRequest:
			return &DetailedError{base: ErrBadRequest, message: apiErr.Message}
		case http.StatusUnauthorized, http.StatusForbidden:
			return &DetailedError{base: ErrInvalidCredentials, message: apiErr.Message}
		case http.StatusConflict:
			return &DetailedError{base: ErrConflict, message: apiErr.Message}
		default:
			return fmt.Errorf("upstream error (status %d): %s", apiErr.Status, apiErr.Message)
		}
	}
	return err
}

// IsError — thin-wrapper над errors.Is, чтобы не тянуть пакет errors в handler.
func IsError(err, target error) bool {
	return errors.Is(err, target)
}

// ErrorMessage достаёт текстовое описание ошибки, если оно было вложено в DetailedError.
func ErrorMessage(err error) string {
	var detailed *DetailedError
	if errors.As(err, &detailed) {
		return detailed.Message()
	}
	return err.Error()
}
