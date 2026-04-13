package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/mock-model-service/internal/models"
)

// SessionManagerClient клиент для работы с session-manager-service.
type SessionManagerClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSessionManagerClient создаёт новый клиент для session-manager-service.
func NewSessionManagerClient(baseURL string, timeoutSeconds int) *SessionManagerClient {
	return &SessionManagerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// GetInterviewProgramResponse ответ с программой интервью.
type GetInterviewProgramResponse struct {
	Program models.InterviewProgram `json:"program"`
}

// GetInterviewProgram получает программу интервью по session_id.
func (c *SessionManagerClient) GetInterviewProgram(ctx context.Context, sessionID uuid.UUID) (*models.InterviewProgram, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/sessions/"+sessionID.String()+"/program", nil)
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
			return nil, fmt.Errorf("interview program not found")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("session manager service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var programResp GetInterviewProgramResponse
	if err := json.Unmarshal(body, &programResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &programResp.Program, nil
}

// CloseSession закрывает сессию.
func (c *SessionManagerClient) CloseSession(ctx context.Context, sessionID uuid.UUID) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/sessions/"+sessionID.String()+"/close", nil)
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
			return fmt.Errorf("session manager service error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
