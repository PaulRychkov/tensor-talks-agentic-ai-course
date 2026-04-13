package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/tensor-talks/user-store-service/internal/models"
	"github.com/tensor-talks/user-store-service/internal/repository"
)

// ErrInvalidInput обозначает ошибку валидации входных данных (логин/пароль).
var ErrInvalidInput = errors.New("invalid input")

// ListFilters описывает параметры фильтрации и пагинации при выборке пользователей.
type ListFilters struct {
	// Login позволяет отфильтровать пользователей по подстроке логина (case-insensitive).
	Login *string
	// Limit максимальное количество записей в ответе.
	Limit int
	// Offset смещение от начала выборки.
	Offset int
}

// UserService инкапсулирует бизнес-логику, связанную с пользователями.
type UserService struct {
	repo repository.UserRepository
}

// NewUserService создаёт новый экземпляр сервиса пользователей.
func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// CreateUser создаёт нового пользователя в БД на основе логина и хеша пароля.
// Предполагается, что на этом уровне уже передаётся безопасный хеш (например, bcrypt),
// а "сырой" пароль никогда не попадает в данный сервис.
func (s *UserService) CreateUser(ctx context.Context, login, passwordHash string) (*models.User, error) {
	login = normalizeLogin(login)
	if err := validateCredentials(login, passwordHash); err != nil {
		return nil, err
	}

	user := &models.User{
		Login:        login,
		PasswordHash: passwordHash,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetByExternalID возвращает пользователя по его внешнему GUID-идентификатору.
func (s *UserService) GetByExternalID(ctx context.Context, externalID uuid.UUID) (*models.User, error) {
	return s.repo.GetByExternalID(ctx, externalID)
}

// GetByLogin возвращает пользователя по логину.
func (s *UserService) GetByLogin(ctx context.Context, login string) (*models.User, error) {
	login = normalizeLogin(login)
	return s.repo.GetByLogin(ctx, login)
}

// UpdateUser обновляет логин и/или хеш пароля пользователя по его внешнему GUID.
// Важно: здесь так же ожидается уже захешированный пароль.
func (s *UserService) UpdateUser(ctx context.Context, externalID uuid.UUID, login, passwordHash *string) (*models.User, error) {
	user, err := s.repo.GetByExternalID(ctx, externalID)
	if err != nil {
		return nil, err
	}

	if login != nil {
		sanitized := normalizeLogin(*login)
		if sanitized == "" {
			return nil, ErrInvalidInput
		}
		user.Login = sanitized
	}

	if passwordHash != nil {
		if strings.TrimSpace(*passwordHash) == "" {
			return nil, ErrInvalidInput
		}
		user.PasswordHash = *passwordHash
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteUser удаляет пользователя по его внешнему GUID.
func (s *UserService) DeleteUser(ctx context.Context, externalID uuid.UUID) error {
	return s.repo.Delete(ctx, externalID)
}

// ListUsers возвращает список пользователей для отладочного API с учётом фильтров.
// На этом слое дополнительно нормализуем и ограничиваем параметры пагинации:
//   - выставляем разумный defaultLimit и maxLimit, чтобы защититься от слишком тяжёлых запросов;
//   - не допускаем отрицательный offset;
//   - нормализуем фильтр по логину так же, как и в остальных операциях.
func (s *UserService) ListUsers(ctx context.Context, filters ListFilters) ([]models.User, error) {
	const (
		defaultLimit = 50
		maxLimit     = 200
	)

	limit := filters.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	var loginFilter *string
	if filters.Login != nil {
		normalized := normalizeLogin(*filters.Login)
		if normalized != "" {
			loginFilter = &normalized
		}
	}

	return s.repo.List(ctx, loginFilter, limit, offset)
}

// validateCredentials проверяет базовые ограничения на логин и хеш пароля.
func validateCredentials(login, passwordHash string) error {
	if login == "" || strings.Contains(login, " ") {
		return ErrInvalidInput
	}
	if strings.TrimSpace(passwordHash) == "" {
		return ErrInvalidInput
	}
	return nil
}

// normalizeLogin приводит логин к нижнему регистру и убирает лишние пробелы.
func normalizeLogin(login string) string {
	return strings.TrimSpace(strings.ToLower(login))
}
