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

// ResultsCRUDClient клиент для работы с results-crud-service.
type ResultsCRUDClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewResultsCRUDClient создаёт новый клиент для results-crud-service.
func NewResultsCRUDClient(baseURL string, timeoutSeconds int) *ResultsCRUDClient {
	return &ResultsCRUDClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// SaveResultRequest запрос на сохранение результата.
type SaveResultRequest struct {
	SessionID       uuid.UUID `json:"session_id"`
	Score           int       `json:"score"`
	Feedback        string    `json:"feedback"`
	TerminatedEarly bool      `json:"terminated_early,omitempty"`
}

// SaveResult сохраняет результат интервью в results-crud-service.
func (c *ResultsCRUDClient) SaveResult(ctx context.Context, sessionID uuid.UUID, score int, feedback string, terminatedEarly bool) error {
	reqBody := SaveResultRequest{
		SessionID:       sessionID,
		Score:           score,
		Feedback:        feedback,
		TerminatedEarly: terminatedEarly,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/results", bytes.NewBuffer(jsonData))
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
			return fmt.Errorf("results crud service error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
