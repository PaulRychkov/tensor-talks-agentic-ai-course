// Package handler implements HTTP handlers for admin-bff-service (§10.1).
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tensor-talks/admin-bff-service/internal/config"
	"go.uber.org/zap"
)

// AdminHandler provides admin API endpoints that proxy to downstream services.
//
// All routes are under /admin/api/ and require admin/content_editor role (§10.1).
type AdminHandler struct {
	cfg    config.Config
	client *http.Client
	logger *zap.Logger
}

// New creates a new AdminHandler.
func New(cfg config.Config, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

// RegisterPublicRoutes registers routes that do not require authentication.
func (h *AdminHandler) RegisterPublicRoutes(router gin.IRouter) {
	router.POST("/admin/login", h.adminLogin)
}

type loginRequest struct {
	Secret string `json:"secret" binding:"required"`
}

// adminLogin issues a JWT with role=admin when the provided secret matches cfg.AdminSecret.
func (h *AdminHandler) adminLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "secret required"})
		return
	}
	if h.cfg.AdminSecret == "" || req.Secret != h.cfg.AdminSecret {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid secret"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "admin",
		"role": "admin",
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	})
	tokenStr, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		h.logger.Error("Failed to sign admin JWT", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": tokenStr})
}

// RegisterRoutes registers all admin API routes.
func (h *AdminHandler) RegisterRoutes(router gin.IRouter) {
	admin := router.Group("/admin/api")
	{
		// Knowledge ingestion routes
		ingestion := admin.Group("/knowledge")
		ingestion.POST("/upload", h.uploadKnowledge)
		ingestion.GET("/tasks", h.getKnowledgeTasks)
		ingestion.GET("/tasks/:id", h.getKnowledgeTask)
		ingestion.GET("", h.searchKnowledge)

		// Draft review routes
		drafts := admin.Group("/drafts")
		drafts.GET("", h.getDrafts)
		drafts.GET("/:id", h.getDraft)
		drafts.POST("/:id/approve", h.approveDraft)
		drafts.POST("/:id/reject", h.rejectDraft)

		// Questions CRUD
		admin.GET("/questions", h.searchQuestions)
		admin.POST("/questions", h.createQuestion)
		admin.GET("/questions/:id", h.getQuestion)
		admin.PUT("/questions/:id", h.updateQuestion)
		admin.DELETE("/questions/:id", h.deleteQuestion)

		// Login word dictionary management
		adjectives := admin.Group("/login-words/adjectives")
		adjectives.GET("", h.getAdjectives)
		adjectives.POST("", h.createAdjective)
		adjectives.PUT("/:id", h.updateAdjective)
		adjectives.DELETE("/:id", h.deleteAdjective)

		nouns := admin.Group("/login-words/nouns")
		nouns.GET("", h.getNouns)
		nouns.POST("", h.createNoun)
		nouns.PUT("/:id", h.updateNoun)
		nouns.DELETE("/:id", h.deleteNoun)

		// Operators management
		admin.POST("/operators", h.createOperator)

		// Knowledge CRUD (direct edit of existing articles)
		admin.GET("/knowledge/:id", h.getKnowledgeItem)
		admin.PUT("/knowledge/:id", h.updateKnowledgeItem)
		admin.DELETE("/knowledge/:id", h.deleteKnowledgeItem)

		// Metrics dashboard (3 types: technical/AI via Prometheus, product via results-crud)
		metrics := admin.Group("/metrics")
		metrics.GET("/product", h.getProductMetrics)
		metrics.GET("/technical", h.getTechnicalMetrics)
		metrics.GET("/ai", h.getAIMetrics)
	}
}

// ── Proxy helpers ──────────────────────────────────────────────────────────────

// proxyRequest forwards the request to a downstream service and writes the response.
func (h *AdminHandler) proxyRequest(
	c *gin.Context,
	targetBaseURL, targetPath string,
	queryParams url.Values,
) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	targetURL := strings.TrimRight(targetBaseURL, "/") + targetPath
	if len(queryParams) > 0 {
		targetURL += "?" + queryParams.Encode()
	}

	var bodyReader io.Reader
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
	}

	req, err := http.NewRequestWithContext(ctx, c.Request.Method, targetURL, bodyReader)
	if err != nil {
		h.logger.Error("Failed to create proxy request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "proxy error"})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Forward operator identity for audit logging
	if operatorID := c.GetString("operator_id"); operatorID != "" {
		req.Header.Set("X-Operator-ID", operatorID)
		req.Header.Set("X-Operator-Role", c.GetString("operator_role"))
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error("Proxy request failed", zap.String("url", targetURL), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "downstream service unavailable"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}

// ── Knowledge ingestion ────────────────────────────────────────────────────────

func (h *AdminHandler) uploadKnowledge(c *gin.Context) {
	// POST /admin/api/knowledge/upload → POST /ingest/url (URL) or /ingest/file (file)
	// Detect by Content-Type: multipart/form-data → file upload, else JSON URL upload
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		h.uploadKnowledgeFile(c)
		return
	}
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, "/ingest/url", nil)
}

func (h *AdminHandler) uploadKnowledgeFile(c *gin.Context) {
	// Forward multipart/form-data to knowledge-producer /ingest/file
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	topic := c.DefaultQuery("topic", c.PostForm("topic"))
	if topic == "" {
		topic = "general"
	}
	kind := c.DefaultQuery("kind", c.PostForm("kind"))
	if kind == "" {
		kind = "knowledge"
	}

	targetURL := strings.TrimRight(h.cfg.KnowledgeProducerSvc.BaseURL, "/") +
		fmt.Sprintf("/ingest/file?topic=%s&kind=%s", topic, kind)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, c.Request.Body)
	if err != nil {
		h.logger.Error("Failed to create file proxy request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "proxy error"})
		return
	}
	req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	if operatorID := c.GetString("operator_id"); operatorID != "" {
		req.Header.Set("X-Operator-ID", operatorID)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error("File proxy request failed", zap.String("url", targetURL), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "downstream service unavailable"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}

// getKnowledgeTasks returns pending drafts as the task queue.
// knowledge-producer-service has no /tasks endpoint; drafts with status=pending are the queue.
func (h *AdminHandler) getKnowledgeTasks(c *gin.Context) {
	q := url.Values{}
	q.Set("status", "pending")
	for k, v := range c.Request.URL.Query() {
		if k != "status" {
			q[k] = v
		}
	}
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, "/drafts", q)
}

func (h *AdminHandler) getKnowledgeTask(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, fmt.Sprintf("/drafts/%s", c.Param("id")), nil)
}

func (h *AdminHandler) searchKnowledge(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeBaseCrudSvc.BaseURL, "/knowledge", c.Request.URL.Query())
}

// ── Drafts ────────────────────────────────────────────────────────────────────

func (h *AdminHandler) getDrafts(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, "/drafts", c.Request.URL.Query())
}

func (h *AdminHandler) getDraft(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, fmt.Sprintf("/drafts/%s", c.Param("id")), nil)
}

func (h *AdminHandler) approveDraft(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, fmt.Sprintf("/drafts/%s/approve", c.Param("id")), nil)
}

func (h *AdminHandler) rejectDraft(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeProducerSvc.BaseURL, fmt.Sprintf("/drafts/%s/reject", c.Param("id")), nil)
}

// ── Questions ─────────────────────────────────────────────────────────────────

func (h *AdminHandler) searchQuestions(c *gin.Context) {
	h.proxyRequest(c, h.cfg.QuestionsCrudSvc.BaseURL, "/questions", c.Request.URL.Query())
}

func (h *AdminHandler) createQuestion(c *gin.Context) {
	h.proxyRequest(c, h.cfg.QuestionsCrudSvc.BaseURL, "/questions", nil)
}

func (h *AdminHandler) getQuestion(c *gin.Context) {
	h.proxyRequest(c, h.cfg.QuestionsCrudSvc.BaseURL, fmt.Sprintf("/questions/%s", c.Param("id")), nil)
}

func (h *AdminHandler) updateQuestion(c *gin.Context) {
	h.proxyRequest(c, h.cfg.QuestionsCrudSvc.BaseURL, fmt.Sprintf("/questions/%s", c.Param("id")), nil)
}

func (h *AdminHandler) deleteQuestion(c *gin.Context) {
	h.proxyRequest(c, h.cfg.QuestionsCrudSvc.BaseURL, fmt.Sprintf("/questions/%s", c.Param("id")), nil)
}

// ── Login words ───────────────────────────────────────────────────────────────

func (h *AdminHandler) getAdjectives(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, "/login-words/adjectives", c.Request.URL.Query())
}

func (h *AdminHandler) createAdjective(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, "/login-words/adjectives", nil)
}

func (h *AdminHandler) updateAdjective(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, fmt.Sprintf("/login-words/adjectives/%s", c.Param("id")), nil)
}

func (h *AdminHandler) deleteAdjective(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, fmt.Sprintf("/login-words/adjectives/%s", c.Param("id")), nil)
}

func (h *AdminHandler) getNouns(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, "/login-words/nouns", c.Request.URL.Query())
}

func (h *AdminHandler) createNoun(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, "/login-words/nouns", nil)
}

func (h *AdminHandler) updateNoun(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, fmt.Sprintf("/login-words/nouns/%s", c.Param("id")), nil)
}

func (h *AdminHandler) deleteNoun(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, fmt.Sprintf("/login-words/nouns/%s", c.Param("id")), nil)
}

// ── Operators ─────────────────────────────────────────────────────────────────

func (h *AdminHandler) createOperator(c *gin.Context) {
	h.proxyRequest(c, h.cfg.UserCrudSvc.BaseURL, "/users", nil)
}

// ── Knowledge CRUD (direct edit) ──────────────────────────────────────────────

func (h *AdminHandler) getKnowledgeItem(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeBaseCrudSvc.BaseURL, fmt.Sprintf("/knowledge/%s", c.Param("id")), nil)
}

func (h *AdminHandler) updateKnowledgeItem(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeBaseCrudSvc.BaseURL, fmt.Sprintf("/knowledge/%s", c.Param("id")), nil)
}

func (h *AdminHandler) deleteKnowledgeItem(c *gin.Context) {
	h.proxyRequest(c, h.cfg.KnowledgeBaseCrudSvc.BaseURL, fmt.Sprintf("/knowledge/%s", c.Param("id")), nil)
}

// ── Metrics dashboard ─────────────────────────────────────────────────────────

// getProductMetrics проксирует запрос к results-crud для получения продуктовых метрик.
// GET /admin/api/metrics/product
func (h *AdminHandler) getProductMetrics(c *gin.Context) {
	resultsCrudURL := h.cfg.ResultsCrudSvc.BaseURL
	if resultsCrudURL == "" {
		resultsCrudURL = "http://results-crud-service:8088"
	}
	h.proxyRequest(c, resultsCrudURL, "/results/metrics/product", nil)
}

// prometheusQuery выполняет instant query к Prometheus API и возвращает результат.
func (h *AdminHandler) prometheusQuery(ctx context.Context, promURL, query string) (interface{}, error) {
	target := strings.TrimRight(promURL, "/") + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// getTechnicalMetrics возвращает технические метрики из Prometheus.
// GET /admin/api/metrics/technical
func (h *AdminHandler) getTechnicalMetrics(c *gin.Context) {
	promURL := h.cfg.PrometheusURL
	if promURL == "" {
		promURL = "http://prometheus:9090"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	queries := map[string]string{
		"request_rate_5m":     `sum(rate(tensortalks_http_requests_total[5m])) by (service)`,
		"error_rate_5m":       `sum(rate(tensortalks_http_requests_total{status_code=~"5.."}[5m])) by (service)`,
		"p95_latency_5m":      `histogram_quantile(0.95, sum(rate(tensortalks_http_request_duration_seconds_bucket[5m])) by (le, service))`,
		"services_up":         `up`,
	}

	results := make(map[string]interface{}, len(queries))
	for key, q := range queries {
		res, err := h.prometheusQuery(ctx, promURL, q)
		if err != nil {
			h.logger.Warn("Prometheus query failed", zap.String("query", key), zap.Error(err))
			results[key] = nil
		} else {
			results[key] = res
		}
	}
	c.JSON(http.StatusOK, gin.H{"metrics": results})
}

// getAIMetrics возвращает AI-метрики агента из Prometheus.
// GET /admin/api/metrics/ai
func (h *AdminHandler) getAIMetrics(c *gin.Context) {
	promURL := h.cfg.PrometheusURL
	if promURL == "" {
		promURL = "http://prometheus:9090"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	queries := map[string]string{
		"llm_calls_1h":            `increase(agent_llm_calls_total[1h])`,
		"pii_blocks_1h":           `increase(pii_filter_triggered_total[1h])`,
		"decision_confidence_p50": `histogram_quantile(0.50, rate(agent_decision_confidence_bucket[5m]))`,
		"processing_p95_5m":       `histogram_quantile(0.95, rate(agent_processing_duration_seconds_bucket[5m]))`,
		"active_dialogues":        `agent_active_dialogues`,
		"message_feedback":        `tensortalks_message_feedback_total`,
	}

	results := make(map[string]interface{}, len(queries))
	for key, q := range queries {
		res, err := h.prometheusQuery(ctx, promURL, q)
		if err != nil {
			h.logger.Warn("Prometheus query failed", zap.String("query", key), zap.Error(err))
			results[key] = nil
		} else {
			results[key] = res
		}
	}
	c.JSON(http.StatusOK, gin.H{"metrics": results})
}
