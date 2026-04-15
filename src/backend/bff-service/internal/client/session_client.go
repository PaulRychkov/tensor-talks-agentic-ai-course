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

// SessionParams представляет параметры сессии.
type SessionParams struct {
	Topics             []string `json:"topics"`
	Level              string   `json:"level"`                          // junior, middle, senior
	Mode               string   `json:"mode"`                           // interview, training, study
	Type               string   `json:"type,omitempty"`                 // legacy; kept for backward compat
	Source             string   `json:"source,omitempty"`               // manual, preset
	PresetID           string   `json:"preset_id,omitempty"`
	Subtopics          []string `json:"subtopics,omitempty"`
	UsePreviousResults bool     `json:"use_previous_results,omitempty"` // episodic memory
	NumQuestions       *int     `json:"num_questions,omitempty"`        // interview duration: 5/10/15
	FocusPoints        []string `json:"focus_points,omitempty"`         // specific points for follow-up study
}

// SessionClient клиент для работы с session-manager-service.
type SessionClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSessionClient создаёт новый клиент для session-manager-service.
// Note: HTTP client timeout should be longer than context timeout to allow context to control cancellation.
func NewSessionClient(baseURL string, timeoutSeconds int) *SessionClient {
	// Use a longer timeout than context to let context control cancellation
	clientTimeout := time.Duration(timeoutSeconds+10) * time.Second
	if timeoutSeconds == 0 {
		clientTimeout = 45 * time.Second
	}
	return &SessionClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
	}
}

// CreateSessionRequest запрос на создание сессии с параметрами интервью.
type CreateSessionRequest struct {
	UserID uuid.UUID     `json:"user_id"`
	Params SessionParams `json:"params"`
}

// CreateSessionResponse ответ с ID сессии.
type CreateSessionResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Ready     bool      `json:"ready"`
}

// CreateSession создаёт новую сессию для пользователя с параметрами интервью.
func (c *SessionClient) CreateSession(ctx context.Context, userID uuid.UUID, params SessionParams) (*CreateSessionResponse, error) {
	reqBody := CreateSessionRequest{
		UserID: userID,
		Params: params,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/sessions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("max active sessions reached")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("session manager service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var sessionResp CreateSessionResponse
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &sessionResp, nil
}
