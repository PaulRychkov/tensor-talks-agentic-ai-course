package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

/*
Пакет models содержит ORM-модели для работы GORM с PostgreSQL.

Основная сущность:
  - User — учётная запись пользователя с внутренним числовым PK и внешним GUID (ExternalID),
    который используется другими микросервисами (auth, чат и т.п.).
*/

// User описывает зарегистрированного пользователя, сохраняемого в PostgreSQL.
type User struct {
	ID                   uint      `gorm:"primaryKey"`
	ExternalID           uuid.UUID `gorm:"type:uuid;uniqueIndex"`
	Login                string    `gorm:"type:varchar(64);uniqueIndex;not null"`
	PasswordHash         string    `gorm:"type:text;not null"`
	RecoveryKeyHash      *string   `gorm:"type:varchar(256)"` // §10.10: bcrypt hash of recovery key
	RecoveryAttempts     int       `gorm:"default:0"`
	RecoveryBlockedUntil *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// LoginAdjective represents an adjective in the login generator dictionary (§10.10).
type LoginAdjective struct {
	ID        uint      `gorm:"primaryKey"`
	Word      string    `gorm:"type:varchar(30);uniqueIndex;not null"`
	IsActive  bool      `gorm:"default:true;not null"`
	CreatedAt time.Time
}

// LoginNoun represents a noun in the login generator dictionary (§10.10).
type LoginNoun struct {
	ID        uint      `gorm:"primaryKey"`
	Word      string    `gorm:"type:varchar(30);uniqueIndex;not null"`
	IsActive  bool      `gorm:"default:true;not null"`
	CreatedAt time.Time
}

// BeforeCreate заполняет ExternalID перед вставкой записи, если он не задан.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ExternalID == uuid.Nil {
		u.ExternalID = uuid.New()
	}
	return nil
}

// PublicUser — "санитизированное" представление пользователя для ответов API.
// В текущем варианте сюда попадает и PasswordHash, т.к. этот микросервис
// не отдаётся напрямую во внешний мир, а используется только auth-service и отладочными инструментами.
type PublicUser struct {
	ExternalID      uuid.UUID `json:"id"`
	Login           string    `json:"login"`
	PasswordHash    string    `json:"password_hash"`
	RecoveryKeyHash *string   `json:"recovery_key_hash,omitempty"` // §10.10: for auth-service recovery flow
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ToPublic конвертирует ORM-модель в структуру, пригодную для JSON-ответа.
func (u User) ToPublic() PublicUser {
	return PublicUser{
		ExternalID:      u.ExternalID,
		Login:           u.Login,
		PasswordHash:    u.PasswordHash,
		RecoveryKeyHash: u.RecoveryKeyHash,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
	}
}

