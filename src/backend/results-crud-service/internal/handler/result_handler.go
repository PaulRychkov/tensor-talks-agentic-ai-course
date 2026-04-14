package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tensor-talks/results-crud-service/internal/metrics"
	"github.com/tensor-talks/results-crud-service/internal/models"
	"github.com/tensor-talks/results-crud-service/internal/repository"
	"github.com/tensor-talks/results-crud-service/internal/service"
	"go.uber.org/zap"
)

// ResultHandler обрабатывает HTTP-запросы для работы с результатами.
type ResultHandler struct {
	svc    *service.ResultService
	logger *zap.Logger
}

// NewResultHandler создаёт новый обработчик результатов.
func NewResultHandler(svc *service.ResultService, logger *zap.Logger) *ResultHandler {
	return &ResultHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с результатами.
// IMPORTANT: static paths (/results/user-summary etc.) must be registered
// BEFORE the parameterized route (/results/:session_id) to avoid shadowing.
func (h *ResultHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/results", h.CreateResult)
	router.GET("/results", h.GetResults)
	// Episodic memory endpoints (§10.6) — registered before :session_id to prevent shadowing
	router.GET("/results/user-summary", h.GetUserSummary)
	router.GET("/results/topic-scores", h.GetTopicScores)
	router.GET("/results/previous-report", h.GetPreviousReport)
	// Product metrics aggregation (admin dashboard)
	router.GET("/results/metrics/product", h.GetProductMetrics)
	// Parameterized route last to avoid swallowing static paths above
	router.GET("/results/:session_id", h.GetResult)
	router.PATCH("/results/:session_id/rating", h.UpdateRating)
}

type createResultRequest struct {
	SessionID           uuid.UUID       `json:"session_id" binding:"required"`
	Score               int             `json:"score"`
	Feedback            string          `json:"feedback"`
	TerminatedEarly     bool            `json:"terminated_early,omitempty"`
	ReportJSON          json.RawMessage `json:"report_json"`
	PresetTraining      json.RawMessage `json:"preset_training,omitempty"`
	ResultFormatVersion int             `json:"result_format_version,omitempty"`
	Evaluations         json.RawMessage `json:"evaluations,omitempty"`
	SessionKind         string          `json:"session_kind,omitempty"`
}

// CreateResult создаёт новый результат.
func (h *ResultHandler) CreateResult(c *gin.Context) {
	var req createResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("CreateResult: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload: " + err.Error()})
		return
	}

	result := &models.Result{
		SessionID:           req.SessionID,
		Score:               req.Score,
		Feedback:            req.Feedback,
		TerminatedEarly:     req.TerminatedEarly,
		ReportJSON:          req.ReportJSON,
		PresetTraining:      req.PresetTraining,
		ResultFormatVersion: req.ResultFormatVersion,
		Evaluations:         req.Evaluations,
		SessionKind:         req.SessionKind,
	}

	if err := h.svc.CreateResult(c.Request.Context(), result); err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			h.logger.Warn("CreateResult: validation failed", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Message})
			return
		}

		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "create", "error").Inc()
		h.logger.Error("CreateResult failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "create", "success").Inc()

	// Product metric: track session completion rate
	completedNaturally := "true"
	if result.TerminatedEarly {
		completedNaturally = "false"
	}
	sessionKind := result.SessionKind
	if sessionKind == "" {
		sessionKind = "interview"
	}
	metrics.SessionCompletedTotal.WithLabelValues(sessionKind, completedNaturally).Inc()

	c.JSON(http.StatusCreated, gin.H{"result": result})
}

// GetResult возвращает результат по session_id.
func (h *ResultHandler) GetResult(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		h.logger.Warn("GetResult: invalid session id", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	result, err := h.svc.GetResult(c.Request.Context(), sessionID)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
		} else {
			metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get", "error").Inc()
			h.logger.Error("GetResult failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// GetResults возвращает результаты по списку session_id (query параметр session_ids, разделённые запятыми).
func (h *ResultHandler) GetResults(c *gin.Context) {
	sessionIDsStr := c.Query("session_ids")
	if sessionIDsStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_ids parameter is required"})
		return
	}

	sessionIDStrs := strings.Split(sessionIDsStr, ",")
	sessionIDs := make([]uuid.UUID, 0, len(sessionIDStrs))
	for _, idStr := range sessionIDStrs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			h.logger.Warn("GetResults: invalid session id", zap.String("id", idStr), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id: " + idStr})
			return
		}
		sessionIDs = append(sessionIDs, id)
	}

	results, err := h.svc.GetResults(c.Request.Context(), sessionIDs)
	if err != nil {
		metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_many", "error").Inc()
		h.logger.Error("GetResults failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessResultOperationsTotal.WithLabelValues("results-crud-service", "get_many", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// ── Episodic memory endpoints (§10.6) ────────────────────────────────────────

// GetUserSummary returns an aggregated summary of a user's session history.
// Used by interview-builder-service for personalization.
// GET /results/user-summary?user_id=...&limit=N
func (h *ResultHandler) GetUserSummary(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	results, err := h.svc.GetResultsByUserID(c.Request.Context(), userID, 50)
	if err != nil {
		h.logger.Error("GetUserSummary failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Aggregate topic scores
	type topicHistory struct {
		Scores  []float64 `json:"scores"`
		LastSeen string   `json:"last_seen"`
	}
	topicMap := map[string]*topicHistory{}
	var weakTopics, strongTopics []string

	for _, r := range results {
		if len(r.Evaluations) == 0 {
			continue
		}
		var evals []map[string]interface{}
		if err := json.Unmarshal(r.Evaluations, &evals); err != nil {
			continue
		}
		for _, eval := range evals {
			topic, _ := eval["topic"].(string)
			if topic == "" {
				continue
			}
			score, _ := eval["score"].(float64)
			if _, ok := topicMap[topic]; !ok {
				topicMap[topic] = &topicHistory{}
			}
			topicMap[topic].Scores = append(topicMap[topic].Scores, score)
			topicMap[topic].LastSeen = r.CreatedAt.Format("2006-01-02")
		}
	}

	topicScores := map[string]float64{}
	for topic, th := range topicMap {
		var sum float64
		for _, s := range th.Scores {
			sum += s
		}
		avg := sum / float64(len(th.Scores))
		topicScores[topic] = avg
		if avg < 0.5 {
			weakTopics = append(weakTopics, topic)
		} else if avg >= 0.8 {
			strongTopics = append(strongTopics, topic)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":      userID,
		"total_sessions": len(results),
		"topic_scores": topicScores,
		"weak_topics":  weakTopics,
		"strong_topics": strongTopics,
	})
}

// GetTopicScores returns lightweight average scores per topic for a user.
// Used by interviewer-agent-service for context personalization.
// GET /results/topic-scores?user_id=...&topics=topic1,topic2
func (h *ResultHandler) GetTopicScores(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	topicsFilter := map[string]bool{}
	if topicsStr := c.Query("topics"); topicsStr != "" {
		for _, t := range strings.Split(topicsStr, ",") {
			topicsFilter[strings.TrimSpace(t)] = true
		}
	}

	results, err := h.svc.GetResultsByUserID(c.Request.Context(), userID, 20)
	if err != nil {
		h.logger.Error("GetTopicScores failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	topicSums := map[string]float64{}
	topicCounts := map[string]int{}
	for _, r := range results {
		if len(r.Evaluations) == 0 {
			continue
		}
		var evals []map[string]interface{}
		if err := json.Unmarshal(r.Evaluations, &evals); err != nil {
			continue
		}
		for _, eval := range evals {
			topic, _ := eval["topic"].(string)
			if topic == "" {
				continue
			}
			if len(topicsFilter) > 0 && !topicsFilter[topic] {
				continue
			}
			score, _ := eval["score"].(float64)
			topicSums[topic] += score
			topicCounts[topic]++
		}
	}

	avgScores := map[string]float64{}
	for topic, sum := range topicSums {
		avgScores[topic] = sum / float64(topicCounts[topic])
	}

	c.JSON(http.StatusOK, gin.H{"topic_scores": avgScores})
}

// UpdateRating сохраняет пользовательскую оценку сессии (1-5 звёзд).
// PATCH /results/:session_id/rating
func (h *ResultHandler) UpdateRating(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("session_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var req struct {
		Rating  int    `json:"rating" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}

	if err := h.svc.UpdateRating(c.Request.Context(), sessionID, req.Rating, req.Comment); err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Message})
			return
		}
		if err == repository.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
			return
		}
		h.logger.Error("UpdateRating failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Emit product metric (session_kind unknown at this point, use "any")
	metrics.SessionUserRating.WithLabelValues("any").Observe(float64(req.Rating))
	metrics.SessionRatingsTotal.WithLabelValues("any").Inc()

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetProductMetrics возвращает агрегированные продуктовые метрики (для admin-дашборда).
// GET /results/metrics/product
func (h *ResultHandler) GetProductMetrics(c *gin.Context) {
	m, err := h.svc.GetProductMetrics(c.Request.Context())
	if err != nil {
		h.logger.Error("GetProductMetrics failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"metrics": m})
}

// GetPreviousReport returns the most recent report for a user of a given session kind.
// Used by analyst-agent-service for comparative analysis.
// GET /results/previous-report?user_id=...&session_kind=interview
func (h *ResultHandler) GetPreviousReport(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	sessionKind := c.Query("session_kind")

	results, err := h.svc.GetResultsByUserID(c.Request.Context(), userID, 10)
	if err != nil {
		h.logger.Error("GetPreviousReport failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	for _, r := range results {
		if sessionKind != "" && r.SessionKind != sessionKind {
			continue
		}
		if len(r.ReportJSON) == 0 {
			continue
		}
		c.JSON(http.StatusOK, gin.H{"result": r})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "no previous report found"})
}
