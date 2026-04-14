package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// KnowledgeCRUDClient клиент для работы с knowledge-base-crud-service.
type KnowledgeCRUDClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewKnowledgeCRUDClient создаёт новый клиент для knowledge-base-crud-service.
func NewKnowledgeCRUDClient(baseURL string, timeoutSeconds int) *KnowledgeCRUDClient {
	return &KnowledgeCRUDClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// SubtopicEntry represents a subtopic from the knowledge base.
type SubtopicEntry struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Topics []string `json:"topics"`
}

// GetSubtopics returns available subtopics from the knowledge base.
func (c *KnowledgeCRUDClient) GetSubtopics(ctx context.Context) ([]SubtopicEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/knowledge/subtopics", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Subtopics []SubtopicEntry `json:"subtopics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Subtopics, nil
}
