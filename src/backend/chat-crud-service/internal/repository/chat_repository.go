package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tensor-talks/chat-crud-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrNotFound обозначает, что запись не найдена.
	ErrNotFound = errors.New("not found")
)

// ChatRepository описывает интерфейс доступа к чатам.
type ChatRepository interface {
	CreateMessage(ctx context.Context, message *models.Message) error
	GetMessagesBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Message, error)
	GetChatDumpBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.ChatDump, error)
	CreateChatDump(ctx context.Context, dump *models.ChatDump) error
	UpdateChatDump(ctx context.Context, dump *models.ChatDump) error
	MaskMessageContent(ctx context.Context, messageID uint, placeholder string) error
	MaskMessageContentByKafkaID(ctx context.Context, kafkaMessageID string, placeholder string) error
}

// GormChatRepository — реализация ChatRepository на основе GORM.
type GormChatRepository struct {
	db *gorm.DB
}

// NewGormChatRepository создаёт новый экземпляр репозитория чатов.
func NewGormChatRepository(db *gorm.DB) *GormChatRepository {
	return &GormChatRepository{db: db}
}

// CreateMessage создаёт новое сообщение.
func (r *GormChatRepository) CreateMessage(ctx context.Context, message *models.Message) error {
	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return err
	}
	return nil
}

// GetMessagesBySessionID возвращает все сообщения сессии.
func (r *GormChatRepository) GetMessagesBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Message, error) {
	var messages []models.Message
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// GetChatDumpBySessionID возвращает дамп чата по session_id.
func (r *GormChatRepository) GetChatDumpBySessionID(ctx context.Context, sessionID uuid.UUID) (*models.ChatDump, error) {
	var dump models.ChatDump
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&dump).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &dump, nil
}

// CreateChatDump создаёт новый дамп чата.
func (r *GormChatRepository) CreateChatDump(ctx context.Context, dump *models.ChatDump) error {
	if err := r.db.WithContext(ctx).Create(dump).Error; err != nil {
		return err
	}
	return nil
}

// MaskMessageContent заменяет содержимое сообщения плейсхолдером (для PII-маскирования).
func (r *GormChatRepository) MaskMessageContent(ctx context.Context, messageID uint, placeholder string) error {
	result := r.db.WithContext(ctx).Model(&models.Message{}).Where("id = ?", messageID).Update("content", placeholder)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MaskMessageContentByKafkaID заменяет содержимое сообщения по message_id из metadata (Kafka UUID).
func (r *GormChatRepository) MaskMessageContentByKafkaID(ctx context.Context, kafkaMessageID string, placeholder string) error {
	result := r.db.WithContext(ctx).Model(&models.Message{}).
		Where("metadata->>'message_id' = ?", kafkaMessageID).
		Update("content", placeholder)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateChatDump обновляет дамп чата.
func (r *GormChatRepository) UpdateChatDump(ctx context.Context, dump *models.ChatDump) error {
	result := r.db.WithContext(ctx).Model(&models.ChatDump{}).Where("session_id = ?", dump.SessionID).Updates(dump)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
