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
	ID              uint      `json:"id"`
	SessionID       uuid.UUID `json:"session_id"`
	Score           int       `json:"score"`
	Feedback        string    `json:"feedback"`
	TerminatedEarly bool      `json:"terminated_early"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
