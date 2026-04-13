package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tensor-talks/questions-crud-service/internal/metrics"
	"github.com/tensor-talks/questions-crud-service/internal/models"
	"github.com/tensor-talks/questions-crud-service/internal/repository"
	"github.com/tensor-talks/questions-crud-service/internal/service"
	"go.uber.org/zap"
)

// QuestionHandler обрабатывает HTTP-запросы для работы с базой вопросов.
type QuestionHandler struct {
	svc    *service.QuestionService
	logger *zap.Logger
}

// NewQuestionHandler создаёт новый обработчик вопросов.
func NewQuestionHandler(svc *service.QuestionService, logger *zap.Logger) *QuestionHandler {
	return &QuestionHandler{svc: svc, logger: logger}
}

// RegisterRoutes регистрирует маршруты для работы с базой вопросов.
func (h *QuestionHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/questions", h.CreateQuestion)
	router.GET("/questions/:id", h.GetQuestionByID)
	router.PUT("/questions/:id", h.UpdateQuestion)
	router.DELETE("/questions/:id", h.DeleteQuestion)
	router.GET("/questions", h.GetQuestionsByFilters)
}

// CreateQuestion создаёт новый вопрос.
func (h *QuestionHandler) CreateQuestion(c *gin.Context) {
	var data models.QuestionJSONB
	if err := c.ShouldBindJSON(&data); err != nil {
		h.logger.Warn("CreateQuestion: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	question, err := h.svc.CreateQuestion(c.Request.Context(), data)
	if err != nil {
		metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "create", "error").Inc()
		h.logger.Error("CreateQuestion failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "create", "success").Inc()
	c.JSON(http.StatusCreated, gin.H{"question": question})
}

// GetQuestionByID возвращает вопрос по ID.
func (h *QuestionHandler) GetQuestionByID(c *gin.Context) {
	id := c.Param("id")

	question, err := h.svc.GetQuestionByID(c.Request.Context(), id)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "get", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		} else {
			metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "get", "error").Inc()
			h.logger.Error("GetQuestionByID failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "get", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"question": question})
}

// UpdateQuestion обновляет вопрос.
func (h *QuestionHandler) UpdateQuestion(c *gin.Context) {
	id := c.Param("id")

	var data models.QuestionJSONB
	if err := c.ShouldBindJSON(&data); err != nil {
		h.logger.Warn("UpdateQuestion: invalid payload", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	question, err := h.svc.UpdateQuestion(c.Request.Context(), id, data)
	if err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "update", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		} else {
			metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "update", "error").Inc()
			h.logger.Error("UpdateQuestion failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "update", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"question": question})
}

// DeleteQuestion удаляет вопрос.
func (h *QuestionHandler) DeleteQuestion(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.DeleteQuestion(c.Request.Context(), id); err != nil {
		if err == repository.ErrNotFound {
			metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "delete", "not_found").Inc()
			c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		} else {
			metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "delete", "error").Inc()
			h.logger.Error("DeleteQuestion failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
		return
	}

	metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "delete", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetQuestionsByFilters возвращает вопросы по фильтрам.
func (h *QuestionHandler) GetQuestionsByFilters(c *gin.Context) {
	filters := repository.QuestionFilters{}

	if complexityStr := c.Query("complexity"); complexityStr != "" {
		complexity, err := strconv.Atoi(complexityStr)
		if err == nil {
			filters.Complexity = &complexity
		}
	}

	if theoryID := c.Query("theory_id"); theoryID != "" {
		filters.TheoryID = &theoryID
	}

	if questionType := c.Query("question_type"); questionType != "" {
		filters.QuestionType = &questionType
	}

	questions, err := h.svc.GetQuestionsByFilters(c.Request.Context(), filters)
	if err != nil {
		metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "get_filters", "error").Inc()
		h.logger.Error("GetQuestionsByFilters failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	metrics.BusinessQuestionOperationsTotal.WithLabelValues("questions-crud-service", "get_filters", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}
