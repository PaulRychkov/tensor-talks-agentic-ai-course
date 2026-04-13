package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensor-talks/bff-service/internal/client"
)

type stubAuthClient struct {
	resp *client.AuthResponse
	err  error
}

func (s *stubAuthClient) Register(ctx context.Context, login, password string) (*client.AuthResponse, error) {
	return s.resp, s.err
}

func (s *stubAuthClient) Login(ctx context.Context, login, password string) (*client.AuthResponse, error) {
	return s.resp, s.err
}

func (s *stubAuthClient) Refresh(ctx context.Context, refreshToken string) (*client.AuthResponse, error) {
	return s.resp, s.err
}

func (s *stubAuthClient) Me(ctx context.Context, token string) (*client.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &client.User{ID: "1", Login: "user"}, nil
}

func TestMapErrorBadRequest(t *testing.T) {
	err := mapError(&client.APIError{Status: 400, Message: "invalid"})
	require.Error(t, err)
	assert.True(t, IsError(err, ErrBadRequest))
	assert.Equal(t, "invalid", ErrorMessage(err))
}

func TestMapErrorUnauthorized(t *testing.T) {
	err := mapError(&client.APIError{Status: 401, Message: "invalid credentials"})
	assert.True(t, IsError(err, ErrInvalidCredentials))
	assert.Equal(t, "invalid credentials", ErrorMessage(err))
}

func TestAuthServiceRegisterError(t *testing.T) {
	client := &stubAuthClient{
		err: &client.APIError{Status: 409, Message: "exists"},
	}
	service := NewAuthService(client)

	_, err := service.Register(context.Background(), "login", "pass")
	require.Error(t, err)
	assert.True(t, IsError(err, ErrConflict))
}

func TestErrorMessageFallback(t *testing.T) {
	msg := ErrorMessage(errors.New("simple error"))
	assert.Equal(t, "simple error", msg)
}
