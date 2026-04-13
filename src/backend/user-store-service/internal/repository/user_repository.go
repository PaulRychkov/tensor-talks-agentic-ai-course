package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/tensor-talks/user-store-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrNotFound обозначает, что пользователь по заданному условию не найден.
	ErrNotFound = errors.New("user not found")
	// ErrDuplicateLogin возвращается, когда логин уже существует в базе.
	ErrDuplicateLogin = errors.New("login already exists")
)

// UserRepository описывает интерфейс доступа к сущностям пользователей в хранилище.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByExternalID(ctx context.Context, externalID uuid.UUID) (*models.User, error)
	GetByLogin(ctx context.Context, login string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, externalID uuid.UUID) error
	// List возвращает список пользователей с возможностью фильтрации и пагинации.
	List(ctx context.Context, login *string, limit, offset int) ([]models.User, error)
}

// GormUserRepository — реализация UserRepository на основе GORM/PostgreSQL.
type GormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository создаёт новый экземпляр репозитория пользователей.
func NewGormUserRepository(db *gorm.DB) *GormUserRepository {
	return &GormUserRepository{db: db}
}

// Create вставляет новую запись о пользователе в базу данных.
func (r *GormUserRepository) Create(ctx context.Context, user *models.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return mapPGError(err)
	}
	return nil
}

// GetByExternalID возвращает пользователя по внешнему GUID-идентификатору.
func (r *GormUserRepository) GetByExternalID(ctx context.Context, externalID uuid.UUID) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("external_id = ?", externalID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetByLogin возвращает пользователя по логину.
func (r *GormUserRepository) GetByLogin(ctx context.Context, login string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("login = ?", login).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// Update сохраняет изменения пользователя, идентифицируя его по внешнему GUID.
func (r *GormUserRepository) Update(ctx context.Context, user *models.User) error {
	result := r.db.WithContext(ctx).Model(&models.User{}).Where("external_id = ?", user.ExternalID).Updates(map[string]any{
		"login":         user.Login,
		"password_hash": user.PasswordHash,
	})
	if err := mapPGError(result.Error); err != nil {
		return err
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete удаляет пользователя по внешнему GUID.
func (r *GormUserRepository) Delete(ctx context.Context, externalID uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("external_id = ?", externalID).Delete(&models.User{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List возвращает список пользователей с учётом фильтра по логину (если задан)
// и параметров пагинации limit/offset. Используется в основном для отладочного API.
// Фильтрация по логину реализована через ILIKE и подстроку, что удобно для поиска,
// но не предназначено для публичных производственных эндпоинтов без дополнительной защиты.
func (r *GormUserRepository) List(ctx context.Context, login *string, limit, offset int) ([]models.User, error) {
	var users []models.User

	query := r.db.WithContext(ctx).Model(&models.User{})
	if login != nil && *login != "" {
		// ILIKE используется для case-insensitive поиска по подстроке логина.
		query = query.Where("login ILIKE ?", "%"+*login+"%")
	}

	if err := query.
		Limit(limit).
		Offset(offset).
		Order("id ASC").
		Find(&users).Error; err != nil {
		return nil, err
	}

	return users, nil
}

func mapPGError(err error) error {
	if err == nil {
		return nil
	}

	// Проверяем PostgreSQL ошибку напрямую
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrDuplicateLogin
		}
		return err
	}

	// Проверяем GORM ошибку
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicateLogin
	}

	// Проверяем строковое представление ошибки (на случай, если GORM не оборачивает правильно)
	errStr := err.Error()
	if strings.Contains(errStr, "duplicate key value violates unique constraint") || strings.Contains(errStr, "23505") {
		return ErrDuplicateLogin
	}

	return err
}
