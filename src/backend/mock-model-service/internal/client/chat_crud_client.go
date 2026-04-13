package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ChatCRUDClient клиент для работы с chat-crud-service.
type ChatCRUDClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewChatCRUDClient создаёт новый клиент для chat-crud-service.
func NewChatCRUDClient(baseURL string, timeoutSeconds int) *ChatCRUDClient {
	return &ChatCRUDClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// MessageType представляет тип сообщения.
type MessageType string

const (
	MessageTypeSystem MessageType = "system"
	MessageTypeUser   MessageType = "user"
)

// SaveMessageRequest запрос на сохранение сообщения.
type SaveMessageRequest struct {
	SessionID uuid.UUID   `json:"session_id"`
	Type      MessageType `json:"type"`
	Content   string      `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Message представляет сохранённое сообщение.
type Message struct {
	ID        uint      `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string    `json:"created_at"`
}

// SaveMessageResponse ответ при сохранении сообщения.
type SaveMessageResponse struct {
	Message Message `json:"message"`
}

// SaveMessage сохраняет сообщение в chat-crud-service.
func (c *ChatCRUDClient) SaveMessage(ctx context.Context, sessionID uuid.UUID, msgType MessageType, content string, metadata map[string]any) error {
	reqBody := SaveMessageRequest{
		SessionID: sessionID,
		Type:      msgType,
		Content:   content,
		Metadata:  metadata,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return fmt.Errorf("chat crud service error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetMessagesResponse ответ с сообщениями.
type GetMessagesResponse struct {
	Messages []Message `json:"messages"`
}

// GetMessages получает все сообщения сессии.
func (c *ChatCRUDClient) GetMessages(ctx context.Context, sessionID uuid.UUID) ([]Message, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/messages/"+sessionID.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("chat crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var messagesResp GetMessagesResponse
	if err := json.Unmarshal(body, &messagesResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return messagesResp.Messages, nil
}

// CreateChatDump создаёт дамп чата из сообщений (сигнализирует о завершении чата).
func (c *ChatCRUDClient) CreateChatDump(ctx context.Context, sessionID uuid.UUID) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat-dumps/"+sessionID.String(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return fmt.Errorf("chat crud service error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
