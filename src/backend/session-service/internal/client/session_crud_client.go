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
	"github.com/tensor-talks/session-service/internal/models"
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

// CreateSessionRequest запрос на создание сессии.
type CreateSessionRequest struct {
	UserID uuid.UUID            `json:"user_id"`
	Params models.SessionParams `json:"params"`
}

// CreateSessionResponse ответ с сессией.
type CreateSessionResponse struct {
	Session struct {
		SessionID        uuid.UUID                `json:"session_id"`
		UserID           uuid.UUID                `json:"user_id"`
		StartTime        string                   `json:"start_time"`
		Params           models.SessionParams     `json:"params"`
		InterviewProgram *models.InterviewProgram `json:"interview_program,omitempty"`
	} `json:"session"`
}

// GetSessionResponse ответ с сессией.
type GetSessionResponse struct {
	Session struct {
		SessionID        uuid.UUID                `json:"session_id"`
		UserID           uuid.UUID                `json:"user_id"`
		StartTime        string                   `json:"start_time"`
		EndTime          *string                  `json:"end_time,omitempty"`
		Params           models.SessionParams     `json:"params"`
		InterviewProgram *models.InterviewProgram `json:"interview_program,omitempty"`
	} `json:"session"`
}

// UpdateProgramRequest запрос на обновление программы.
type UpdateProgramRequest struct {
	Program *models.InterviewProgram `json:"program"`
}

// CreateSession создаёт новую сессию.
func (c *SessionCRUDClient) CreateSession(ctx context.Context, userID uuid.UUID, params models.SessionParams) (*CreateSessionResponse, error) {
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
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("session crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var sessionResp CreateSessionResponse
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &sessionResp, nil
}

// GetSession возвращает сессию по ID.
func (c *SessionCRUDClient) GetSession(ctx context.Context, sessionID uuid.UUID) (*GetSessionResponse, error) {
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

	return &sessionResp, nil
}

// UpdateProgram обновляет программу интервью сессии.
func (c *SessionCRUDClient) UpdateProgram(ctx context.Context, sessionID uuid.UUID, program *models.InterviewProgram) error {
	reqBody := UpdateProgramRequest{
		Program: program,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/sessions/"+sessionID.String()+"/program", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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
			return fmt.Errorf("session crud service error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CloseSession закрывает сессию.
func (c *SessionCRUDClient) CloseSession(ctx context.Context, sessionID uuid.UUID) error {
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
			return fmt.Errorf("session crud service error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetSessionsByUserID возвращает все сессии пользователя.
func (c *SessionCRUDClient) GetSessionsByUserID(ctx context.Context, userID uuid.UUID) ([]GetSessionResponse, error) {
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

	var response struct {
		Sessions []struct {
			SessionID        uuid.UUID                `json:"session_id"`
			UserID           uuid.UUID                `json:"user_id"`
			StartTime        string                   `json:"start_time"`
			EndTime          *string                  `json:"end_time,omitempty"`
			Params           models.SessionParams     `json:"params"`
			InterviewProgram *models.InterviewProgram `json:"interview_program,omitempty"`
			CreatedAt        string                   `json:"created_at"`
			UpdatedAt        string                   `json:"updated_at"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Конвертируем в нужный формат
	sessions := make([]GetSessionResponse, len(response.Sessions))
	for i, s := range response.Sessions {
		sessions[i].Session.SessionID = s.SessionID
		sessions[i].Session.UserID = s.UserID
		sessions[i].Session.StartTime = s.StartTime
		sessions[i].Session.EndTime = s.EndTime
		sessions[i].Session.Params = s.Params
		sessions[i].Session.InterviewProgram = s.InterviewProgram
	}

	return sessions, nil
}

// GetActiveSessionByUserIDResponse ответ с активной сессией.
type GetActiveSessionByUserIDResponse struct {
	Session struct {
		SessionID        uuid.UUID                `json:"session_id"`
		UserID           uuid.UUID                `json:"user_id"`
		StartTime        string                   `json:"start_time"`
		EndTime          *string                  `json:"end_time,omitempty"`
		Params           models.SessionParams     `json:"params"`
		InterviewProgram *models.InterviewProgram `json:"interview_program,omitempty"`
	} `json:"session"`
}

// GetActiveSessionByUserID возвращает активную сессию пользователя (end_time IS NULL).
func (c *SessionCRUDClient) GetActiveSessionByUserID(ctx context.Context, userID uuid.UUID) (*GetActiveSessionByUserIDResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/sessions/user/"+userID.String()+"/active", nil)
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
			return nil, fmt.Errorf("active session not found")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("session crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var sessionResp GetActiveSessionByUserIDResponse
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &sessionResp, nil
}
