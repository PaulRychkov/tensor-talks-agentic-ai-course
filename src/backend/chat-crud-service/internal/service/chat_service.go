package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/tensor-talks/chat-crud-service/internal/models"
	"github.com/tensor-talks/chat-crud-service/internal/repository"
)

// ChatService инкапсулирует бизнес-логику для чатов.
type ChatService struct {
	repo repository.ChatRepository
}

// NewChatService создаёт новый экземпляр сервиса чатов.
func NewChatService(repo repository.ChatRepository) *ChatService {
	return &ChatService{repo: repo}
}

// SaveMessage сохраняет сообщение в чат.
func (s *ChatService) SaveMessage(ctx context.Context, sessionID uuid.UUID, msgType models.MessageType, content string, metadata models.MessageMetadata) (*models.Message, error) {
	message := &models.Message{
		SessionID: sessionID,
		Type:      msgType,
		Content:   content,
		Metadata:  metadata,
	}

	if err := s.repo.CreateMessage(ctx, message); err != nil {
		return nil, err
	}

	return message, nil
}

// GetMessagesBySessionID возвращает все сообщения сессии.
func (s *ChatService) GetMessagesBySessionID(ctx context.Context, sessionID uuid.UUID) ([]models.Message, error) {
	return s.repo.GetMessagesBySessionID(ctx, sessionID)
}

// GetChatDump возвращает дамп чата по session_id.
func (s *ChatService) GetChatDump(ctx context.Context, sessionID uuid.UUID) (*models.ChatDump, error) {
	return s.repo.GetChatDumpBySessionID(ctx, sessionID)
}

// GetActiveChatJSON возвращает JSON структуру незавершенного чата из сообщений.
func (s *ChatService) GetActiveChatJSON(ctx context.Context, sessionID uuid.UUID) (*models.ChatJSONB, error) {
	// Получаем все сообщения
	messages, err := s.repo.GetMessagesBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Формируем JSONB структуру
	chatMessages := make([]models.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		chatMessages = append(chatMessages, models.ChatMessage{
			Type:      string(msg.Type),
			Content:   msg.Content,
			Metadata:  msg.Metadata,
			CreatedAt: msg.CreatedAt,
		})
	}

	return &models.ChatJSONB{
		Messages: chatMessages,
	}, nil
}

// CreateChatDump создаёт дамп чата из сообщений.
func (s *ChatService) CreateChatDump(ctx context.Context, sessionID uuid.UUID) error {
	// Получаем все сообщения
	messages, err := s.repo.GetMessagesBySessionID(ctx, sessionID)
	if err != nil {
		return err
	}

	// Формируем JSONB структуру
	chatMessages := make([]models.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		chatMessages = append(chatMessages, models.ChatMessage{
			Type:      string(msg.Type),
			Content:   msg.Content,
			Metadata:  msg.Metadata,
			CreatedAt: msg.CreatedAt,
		})
	}

	dump := &models.ChatDump{
		SessionID: sessionID,
		Chat: models.ChatJSONB{
			Messages: chatMessages,
		},
	}

	// Проверяем, существует ли уже дамп
	existingDump, err := s.repo.GetChatDumpBySessionID(ctx, sessionID)
	if err != nil && err == repository.ErrNotFound {
		// Создаём новый
		return s.repo.CreateChatDump(ctx, dump)
	} else if err != nil {
		return err
	}

	// Обновляем существующий
	existingDump.Chat = dump.Chat
	return s.repo.UpdateChatDump(ctx, existingDump)
}
