package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensor-talks/results-crud-service/internal/models"
)

// ---------------------------------------------------------------------------
// Mock repository
// ---------------------------------------------------------------------------

type mockResultRepo struct {
	createFn         func(ctx context.Context, result *models.Result) error
	getBySessionIDFn func(ctx context.Context, id uuid.UUID) (*models.Result, error)
	getBySessionIDs  func(ctx context.Context, ids []uuid.UUID) ([]models.Result, error)
}

func (m *mockResultRepo) Create(ctx context.Context, result *models.Result) error {
	if m.createFn != nil {
		return m.createFn(ctx, result)
	}
	return nil
}

func (m *mockResultRepo) GetBySessionID(ctx context.Context, id uuid.UUID) (*models.Result, error) {
	if m.getBySessionIDFn != nil {
		return m.getBySessionIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockResultRepo) GetBySessionIDs(ctx context.Context, ids []uuid.UUID) ([]models.Result, error) {
	if m.getBySessionIDs != nil {
		return m.getBySessionIDs(ctx, ids)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// §4: results-crud-service — validation & field handling
// ---------------------------------------------------------------------------

func TestCreateResult_DefaultSessionKind(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	result := &models.Result{
		SessionID: uuid.New(),
		Score:     80,
		Feedback:  "Good job",
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, "interview", result.SessionKind)
}

func TestCreateResult_SessionKindValues(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	kinds := []string{"interview", "training", "study"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			result := &models.Result{
				SessionID:   uuid.New(),
				Score:       75,
				Feedback:    "feedback",
				SessionKind: kind,
			}
			err := svc.CreateResult(context.Background(), result)
			require.NoError(t, err)
			assert.Equal(t, kind, result.SessionKind)
		})
	}
}

func TestCreateResult_DefaultResultFormatVersion(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	result := &models.Result{
		SessionID: uuid.New(),
		Score:     80,
		Feedback:  "OK",
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 1, result.ResultFormatVersion)
}

func TestCreateResult_PreservesExplicitFormatVersion(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	result := &models.Result{
		SessionID:           uuid.New(),
		Score:               80,
		Feedback:            "OK",
		ResultFormatVersion: 2,
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, 2, result.ResultFormatVersion)
}

func TestCreateResult_ValidReportJSON(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	report := map[string]interface{}{
		"summary":          "Interview went well",
		"errors_by_topic":  map[string]interface{}{},
		"preparation_plan": "Study more ML",
		"materials":        []string{"book1", "book2"},
	}
	reportBytes, _ := json.Marshal(report)

	result := &models.Result{
		SessionID:  uuid.New(),
		Score:      90,
		Feedback:   "Excellent",
		ReportJSON: json.RawMessage(reportBytes),
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
}

func TestCreateResult_ReportJSON_MissingRequiredSection(t *testing.T) {
	svc := NewResultService(&mockResultRepo{})

	for _, missing := range requiredReportSections {
		t.Run("missing_"+missing, func(t *testing.T) {
			report := map[string]interface{}{
				"summary":          "x",
				"errors_by_topic":  "x",
				"preparation_plan": "x",
				"materials":        "x",
			}
			delete(report, missing)
			reportBytes, _ := json.Marshal(report)

			result := &models.Result{
				SessionID:  uuid.New(),
				ReportJSON: json.RawMessage(reportBytes),
			}
			err := svc.CreateResult(context.Background(), result)
			require.Error(t, err)

			var ve *ValidationError
			assert.ErrorAs(t, err, &ve)
			assert.Contains(t, ve.Message, missing)
		})
	}
}

func TestCreateResult_ReportJSON_InvalidJSON(t *testing.T) {
	svc := NewResultService(&mockResultRepo{})

	result := &models.Result{
		SessionID:  uuid.New(),
		ReportJSON: json.RawMessage(`{not valid json`),
	}

	err := svc.CreateResult(context.Background(), result)
	require.Error(t, err)

	var ve *ValidationError
	assert.ErrorAs(t, err, &ve)
	assert.Contains(t, ve.Message, "not a valid JSON")
}

func TestCreateResult_EmptyReportJSON_SkipsValidation(t *testing.T) {
	svc := NewResultService(&mockResultRepo{})

	result := &models.Result{
		SessionID: uuid.New(),
		Score:     50,
		Feedback:  "meh",
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
}

func TestCreateResult_WithEvaluationsField(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	evals := []map[string]interface{}{
		{"question_id": "q-1", "score": 0.9, "feedback": "great"},
		{"question_id": "q-2", "score": 0.5, "feedback": "needs work"},
	}
	evalsBytes, _ := json.Marshal(evals)

	result := &models.Result{
		SessionID:   uuid.New(),
		Score:       70,
		Feedback:    "OK",
		Evaluations: json.RawMessage(evalsBytes),
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
}

func TestCreateResult_WithPresetTraining(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	preset := map[string]interface{}{
		"preset_id":   "preset-ml-101",
		"weak_topics": []string{"transformers", "attention"},
	}
	presetBytes, _ := json.Marshal(preset)

	result := &models.Result{
		SessionID:      uuid.New(),
		Score:          65,
		Feedback:       "needs improvement",
		PresetTraining: json.RawMessage(presetBytes),
		SessionKind:    "training",
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, "training", result.SessionKind)
}

func TestCreateResult_TerminatedEarly(t *testing.T) {
	repo := &mockResultRepo{}
	svc := NewResultService(repo)

	result := &models.Result{
		SessionID:       uuid.New(),
		Score:           0,
		Feedback:        "User terminated early",
		TerminatedEarly: true,
	}

	err := svc.CreateResult(context.Background(), result)
	require.NoError(t, err)
	assert.True(t, result.TerminatedEarly)
}

func TestGetResult_NotFound(t *testing.T) {
	repo := &mockResultRepo{
		getBySessionIDFn: func(_ context.Context, _ uuid.UUID) (*models.Result, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	svc := NewResultService(repo)

	_, err := svc.GetResult(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestGetResult_ReturnsResult(t *testing.T) {
	sid := uuid.New()
	expected := &models.Result{
		SessionID:   sid,
		Score:       88,
		Feedback:    "Well done",
		SessionKind: "interview",
	}

	repo := &mockResultRepo{
		getBySessionIDFn: func(_ context.Context, id uuid.UUID) (*models.Result, error) {
			if id == sid {
				return expected, nil
			}
			return nil, fmt.Errorf("not found")
		},
	}
	svc := NewResultService(repo)

	got, err := svc.GetResult(context.Background(), sid)
	require.NoError(t, err)
	assert.Equal(t, expected.Score, got.Score)
	assert.Equal(t, expected.SessionKind, got.SessionKind)
}

func TestGetResults_MultipleIDs(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	repo := &mockResultRepo{
		getBySessionIDs: func(_ context.Context, ids []uuid.UUID) ([]models.Result, error) {
			return []models.Result{
				{SessionID: id1, Score: 60},
				{SessionID: id2, Score: 90},
			}, nil
		},
	}
	svc := NewResultService(repo)

	results, err := svc.GetResults(context.Background(), []uuid.UUID{id1, id2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestValidationError_ImplementsError(t *testing.T) {
	ve := &ValidationError{Message: "test error"}
	assert.Equal(t, "test error", ve.Error())
}
