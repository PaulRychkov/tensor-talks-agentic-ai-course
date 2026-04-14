package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/metrics"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/models"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/repository"
	"github.com/tensor-talks/knowledge-base-crud-service/internal/service"
	"go.uber.org/zap"
)

// KnowledgeHandler обрабатывает HTTP-запросы для работы с базой знаний.
type KnowledgeHandler struct {
	svc    *service.KnowledgeService
	logger *zap.Logger
}

// NewKnowledgeHandler создаёт новый обработчик знаний.
func NewKnowledgeHandler(svc *service.KnowledgeService, logger *zap.Logger) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с базой знаний.
func (h *KnowledgeHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/knowledge", h.CreateKnowledge)
	router.GET("/knowledge/:id", h.GetKnowledgeByID)
	router.PUT("/knowledge/:id", h.UpdateKnowledge)
	router.DELETE("/knowledge/:id", h.DeleteKnowledge)
	router.GET("/knowledge", h.GetKnowledgeByFilters)
	router.GET("/knowledge/subtopics", h.GetSubtopics)
	// Semantic search endpoint (§10.3) — requires pgvector + embedding API
	router.POST("/knowledge/search-semantic", h.SearchSemantic)
}

type semanticSearchRequest struct {
	Query     string  `json:"query" binding:"required"`
	Limit     int     `json:"limit"`
	Threshold float64 `json:"threshold"`
	Topic     string  `json:"topic,omitempty"`
}

// SearchSemantic performs semantic similarity search in the knowledge base (§10.3).
// Returns segments ordered by cosine similarity to the query embedding.
// Requires the embedding model to be configured (EMBEDDING_API_URL env var).
func (h *KnowledgeHandler) SearchSemantic(c *gin.Context) {
	var req semanticSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.7
	}

	results, err := h.svc.SearchSemantic(c.Request.Context(), req.Query, req.Topic, req.Limit, req.Threshold)
	if err != nil {
		h.logger.Error("SearchSemantic failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "semantic search unavailable: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// CreateKnowledge создаёт новое знание.
func (h *KnowledgeHandler) CreateKnowledge(c *gin.Context) {
	var data models.KnowledgeJSONB
	if err := c.ShouldBindJSON(&data); err != nil {
		h.logger.Warn("CreateKnowledge: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	knowledge, err := h.svc.CreateKnowledge(c.Request.Context(), data)
	if err != nil {
		metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "create", "error").Inc()
		h.logger.Error("CreateKnowledge failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "create", "success").Inc()
	c.JSON(http.StatusCreated, gin.H{"knowledge": knowledge})
}

// GetKnowledgeByID возвращает знание по ID.
func (h *KnowledgeHandler) GetKnowledgeByID(c *gin.Context) {
	id := c.Param("id")

	knowledge, err := h.svc.GetKnowledgeByID(c.Request.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "get", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "knowledge not found"})
		} else {
			metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "get", "error").Inc()
			h.logger.Error("GetKnowledgeByID failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "get", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"knowledge": knowledge})
}

// UpdateKnowledge обновляет знание.
func (h *KnowledgeHandler) UpdateKnowledge(c *gin.Context) {
	id := c.Param("id")

	var data models.KnowledgeJSONB
	if err := c.ShouldBindJSON(&data); err != nil {
		h.logger.Warn("UpdateKnowledge: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	knowledge, err := h.svc.UpdateKnowledge(c.Request.Context(), id, data)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "update", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "knowledge not found"})
		} else {
			metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "update", "error").Inc()
			h.logger.Error("UpdateKnowledge failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "update", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"knowledge": knowledge})
}

// DeleteKnowledge удаляет знание.
func (h *KnowledgeHandler) DeleteKnowledge(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.DeleteKnowledge(c.Request.Context(), id); err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "delete", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "knowledge not found"})
		} else {
			metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "delete", "error").Inc()
			h.logger.Error("DeleteKnowledge failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "delete", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetSubtopics returns all knowledge items as subtopics with labels and topic grouping.
func (h *KnowledgeHandler) GetSubtopics(c *gin.Context) {
	subtopics, err := h.svc.GetSubtopics(c.Request.Context())
	if err != nil {
		h.logger.Error("GetSubtopics failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subtopics": subtopics})
}

// GetKnowledgeByFilters возвращает знания по фильтрам.
func (h *KnowledgeHandler) GetKnowledgeByFilters(c *gin.Context) {
	filters := repository.KnowledgeFilters{}

	if complexityStr := c.Query("complexity"); complexityStr != "" {
		complexity, err := strconv.Atoi(complexityStr)
		if err == nil {
			filters.Complexity = &complexity
		}
	}

	if concept := c.Query("concept"); concept != "" {
		filters.Concept = &concept
	}

	if parentID := c.Query("parent_id"); parentID != "" {
		filters.ParentID = &parentID
	}

	if tags := c.QueryArray("tags"); len(tags) > 0 {
		filters.Tags = tags
	}

	knowledge, err := h.svc.GetKnowledgeByFilters(c.Request.Context(), filters)
	if err != nil {
		metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "get_filters", "error").Inc()
		h.logger.Error("GetKnowledgeByFilters failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessKnowledgeOperationsTotal.WithLabelValues("knowledge-base-crud-service", "get_filters", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"knowledge": knowledge})
}
