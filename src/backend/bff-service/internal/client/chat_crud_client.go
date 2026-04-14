package client

import (
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

// ChatMessage представляет сообщение в чате.
type ChatMessage struct {
	Type      string                 `json:"type"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// GetMessagesResponse ответ с сообщениями.
type GetMessagesResponse struct {
	Messages []Message `json:"messages"`
}

// Message представляет сообщение из БД.
type Message struct {
	ID        uint      `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// GetChatDumpResponse ответ с дампом чата.
type GetChatDumpResponse struct {
	Dump ChatDump `json:"dump"`
}

// ChatDump представляет дамп чата.
type ChatDump struct {
	ID        uint                   `json:"id"`
	SessionID uuid.UUID              `json:"session_id"`
	Chat      map[string]interface{} `json:"chat"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// GetMessages получает все сообщения сессии.
func (c *ChatCRUDClient) GetMessages(ctx context.Context, sessionID uuid.UUID) ([]ChatMessage, error) {
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

	// Конвертируем в ChatMessage
	chatMessages := make([]ChatMessage, len(messagesResp.Messages))
	for i, msg := range messagesResp.Messages {
		chatMessages[i] = ChatMessage{
			Type:      msg.Type,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt,
		}
	}

	return chatMessages, nil
}

// GetActiveChatJSON получает JSON структуру незавершенного чата.
func (c *ChatCRUDClient) GetActiveChatJSON(ctx context.Context, sessionID uuid.UUID) ([]ChatMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/chat-active/"+sessionID.String(), nil)
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
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("active chat not found")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("chat crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var messagesResp struct {
		Messages []ChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &messagesResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return messagesResp.Messages, nil
}

// GetChatDump получает дамп завершенного чата.
func (c *ChatCRUDClient) GetChatDump(ctx context.Context, sessionID uuid.UUID) (*ChatDump, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/chat-dumps/"+sessionID.String(), nil)
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
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("chat dump not found")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("chat crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var dumpResp GetChatDumpResponse
	if err := json.Unmarshal(body, &dumpResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &dumpResp.Dump, nil
}
