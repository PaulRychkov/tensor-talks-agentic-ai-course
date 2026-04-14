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

// Result представляет результат интервью.
type Result struct {
	ID                  uint            `json:"id"`
	SessionID           uuid.UUID       `json:"session_id"`
	Score               int             `json:"score"`
	Feedback            string          `json:"feedback"`
	TerminatedEarly     bool            `json:"terminated_early"`
	ReportJSON          json.RawMessage `json:"report_json,omitempty"`
	PresetTraining      json.RawMessage `json:"preset_training,omitempty"`
	Evaluations         json.RawMessage `json:"evaluations,omitempty"`
	ResultFormatVersion int             `json:"result_format_version"`
	SessionKind         string          `json:"session_kind,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// GetResultResponse ответ с результатом.
type GetResultResponse struct {
	Result Result `json:"result"`
}

// GetResultsResponse ответ с несколькими результатами.
type GetResultsResponse struct {
	Results []Result `json:"results"`
}

// GetResult получает результат по session_id.
func (c *ResultsCRUDClient) GetResult(ctx context.Context, sessionID uuid.UUID) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/results/"+sessionID.String(), nil)
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
			return nil, fmt.Errorf("result not found")
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			return nil, fmt.Errorf("results crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var resultResp GetResultResponse
	if err := json.Unmarshal(body, &resultResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resultResp.Result, nil
}

// SubmitRating отправляет оценку пользователя (1-5) для сессии.
func (c *ResultsCRUDClient) SubmitRating(ctx context.Context, sessionID uuid.UUID, rating int, comment string) error {
	payload, _ := json.Marshal(map[string]any{"rating": rating, "comment": comment})
	req, err := http.NewRequestWithContext(ctx, "PATCH",
		c.baseURL+"/results/"+sessionID.String()+"/rating",
		bytes.NewReader(payload),
	)
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
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// GetResults получает результаты по списку session_id.
func (c *ResultsCRUDClient) GetResults(ctx context.Context, sessionIDs []uuid.UUID) ([]Result, error) {
	if len(sessionIDs) == 0 {
		return []Result{}, nil
	}

	// Формируем query параметр
	sessionIDsStr := ""
	for i, id := range sessionIDs {
		if i > 0 {
			sessionIDsStr += ","
		}
		sessionIDsStr += id.String()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/results?session_ids="+sessionIDsStr, nil)
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
			return nil, fmt.Errorf("results crud service error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var resultsResp GetResultsResponse
	if err := json.Unmarshal(body, &resultsResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return resultsResp.Results, nil
}
