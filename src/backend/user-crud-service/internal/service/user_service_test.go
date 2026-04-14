package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensor-talks/user-crud-service/internal/models"
	"github.com/tensor-talks/user-crud-service/internal/repository"
)

type mockUserRepository struct {
	lastCreated *models.User
	nextError   error
}

func (m *mockUserRepository) Create(_ context.Context, user *models.User) error {
	if m.nextError != nil {
		return m.nextError
	}
	m.lastCreated = user
	return nil
}

func (m *mockUserRepository) GetByExternalID(_ context.Context, _ uuid.UUID) (*models.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) GetByLogin(_ context.Context, _ string) (*models.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) Update(_ context.Context, _ *models.User) error {
	return errors.New("not implemented")
}

func (m *mockUserRepository) Delete(_ context.Context, _ uuid.UUID) error {
	return errors.New("not implemented")
}

func TestNormalizeLogin(t *testing.T) {
	assert.Equal(t, "user.name", normalizeLogin("  User.Name  "))
	assert.Equal(t, "", normalizeLogin("   "))
}

func TestValidateCredentials(t *testing.T) {
	assert.NoError(t, validateCredentials("user", "hash"))

	assert.ErrorIs(t, validateCredentials("", "hash"), ErrInvalidInput)
	assert.ErrorIs(t, validateCredentials("user name", "hash"), ErrInvalidInput)
	assert.ErrorIs(t, validateCredentials("user", "  "), ErrInvalidInput)
}

func TestCreateUser_Success(t *testing.T) {
	repo := &mockUserRepository{}
	svc := NewUserService(repo)

	user, err := svc.CreateUser(context.Background(), "  TestUser ", "hash")
	require.NoError(t, err)
	require.NotNil(t, user)

	assert.Equal(t, "testuser", repo.lastCreated.Login)
	assert.Equal(t, "testuser", user.Login)
	assert.Equal(t, "hash", user.PasswordHash)
}

func TestCreateUser_Duplicate(t *testing.T) {
	repo := &mockUserRepository{nextError: repository.ErrDuplicateLogin}
	svc := NewUserService(repo)

	user, err := svc.CreateUser(context.Background(), "login", "hash")
	assert.ErrorIs(t, err, repository.ErrDuplicateLogin)
	assert.Nil(t, user)
}

