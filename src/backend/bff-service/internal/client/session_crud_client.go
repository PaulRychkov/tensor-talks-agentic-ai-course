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

// SessionCRUDClient клиент для работы с session-crud-service.
type SessionCRUDClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSessionCRUDClient создаёт новый клиент для session-crud-service.
func NewSessionCRUDClient(baseURL string, timeoutSeconds int) *SessionCRUDClient {
	return &SessionCRUDClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// InterviewProgram представляет программу интервью.
type InterviewProgram struct {
	Questions []QuestionItem `json:"questions"`
}

// QuestionItem представляет один пункт программы.
type QuestionItem struct {
	Question string `json:"question"`
	Theory   string `json:"theory"`
	Order    int    `json:"order"`
}

// Session представляет сессию.
type Session struct {
	SessionID        uuid.UUID         `json:"session_id"`
	UserID           uuid.UUID         `json:"user_id"`
	StartTime        time.Time         `json:"start_time"`
	EndTime          *time.Time        `json:"end_time,omitempty"`
	Params           SessionParams     `json:"params"`
	InterviewProgram *InterviewProgram `json:"interview_program,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// GetSessionsByUserIDResponse ответ со списком сессий.
type GetSessionsByUserIDResponse struct {
	Sessions []Session `json:"sessions"`
}

// GetSessionsByUserID получает все сессии пользователя.
func (c *SessionCRUDClient) GetSessionsByUserID(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/sessions/user/"+userID.String(), nil)
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
			return nil, fmt.Errorf("session crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var sessionsResp GetSessionsByUserIDResponse
	if err := json.Unmarshal(body, &sessionsResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return sessionsResp.Sessions, nil
}

// GetSessionResponse ответ с сессией.
type GetSessionResponse struct {
	Session Session `json:"session"`
}

// GetSession получает сессию по session_id.
func (c *SessionCRUDClient) GetSession(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/sessions/"+sessionID.String(), nil)
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
			return nil, fmt.Errorf("session not found")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("session crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var sessionResp GetSessionResponse
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &sessionResp.Session, nil
}
