package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/google/uuid"
	"github.com/tensor-talks/user-crud-service/internal/models"
	"github.com/tensor-talks/user-crud-service/internal/repository"
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

// SetRecoveryKeyHash сохраняет bcrypt-хеш ключа восстановления (§10.10).
func (s *UserService) SetRecoveryKeyHash(ctx context.Context, externalID uuid.UUID, hash string) error {
	return s.repo.SetRecoveryKeyHash(ctx, externalID, hash)
}

// UpdatePasswordHash обновляет только хеш пароля пользователя по его внешнему GUID (§10.10, recovery flow).
func (s *UserService) UpdatePasswordHash(ctx context.Context, externalID uuid.UUID, passwordHash string) error {
	if strings.TrimSpace(passwordHash) == "" {
		return ErrInvalidInput
	}
	_, err := s.UpdateUser(ctx, externalID, nil, &passwordHash)
	return err
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

// ── Login word dictionary methods (§10.10) ────────────────────────────────────

// WordEntry is a simplified response for login word lists.
type WordEntry struct {
	ID   uint   `json:"id"`
	Word string `json:"word"`
}

// GetActiveAdjectives returns all active adjectives for login generation.
func (s *UserService) GetActiveAdjectives(ctx context.Context) ([]WordEntry, error) {
	words, err := s.repo.GetActiveAdjectives(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]WordEntry, len(words))
	for i, w := range words {
		entries[i] = WordEntry{ID: w.ID, Word: w.Word}
	}
	return entries, nil
}

// GetActiveNouns returns all active nouns for login generation.
func (s *UserService) GetActiveNouns(ctx context.Context) ([]WordEntry, error) {
	words, err := s.repo.GetActiveNouns(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]WordEntry, len(words))
	for i, w := range words {
		entries[i] = WordEntry{ID: w.ID, Word: w.Word}
	}
	return entries, nil
}

// GenerateRandomLogin generates a unique Adjective+Noun+Number login (§10.10).
// Retries up to 10 times on collision.
func (s *UserService) GenerateRandomLogin(ctx context.Context) (string, error) {
	adjectives, err := s.repo.GetActiveAdjectives(ctx)
	if err != nil || len(adjectives) == 0 {
		return "", fmt.Errorf("no active adjectives available")
	}
	nouns, err := s.repo.GetActiveNouns(ctx)
	if err != nil || len(nouns) == 0 {
		return "", fmt.Errorf("no active nouns available")
	}

	for attempt := 0; attempt < 10; attempt++ {
		adj := adjectives[rand.Intn(len(adjectives))].Word
		noun := nouns[rand.Intn(len(nouns))].Word
		num := rand.Intn(999) + 1
		login := fmt.Sprintf("%s%s%d", adj, noun, num)

		_, err := s.repo.GetByLogin(ctx, strings.ToLower(login))
		if errors.Is(err, repository.ErrNotFound) {
			// Login not found in DB → it's available
			return login, nil
		}
		if err != nil {
			// Real DB error – stop and propagate
			return "", fmt.Errorf("checking login availability: %w", err)
		}
		// err == nil means login is taken; try again
	}
	return "", fmt.Errorf("could not generate unique login after 10 attempts")
}

// CheckLoginAvailability returns true if the login is not taken.
func (s *UserService) CheckLoginAvailability(ctx context.Context, login string) (bool, error) {
	normalized := normalizeLogin(login)
	_, err := s.repo.GetByLogin(ctx, normalized)
	if errors.Is(err, repository.ErrNotFound) {
		return true, nil // not found → available
	}
	if err != nil {
		return false, fmt.Errorf("checking login availability: %w", err)
	}
	return false, nil // found → taken
}

