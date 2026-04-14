package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensor-talks/session-service/internal/models"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// §1.3: session-service — ProgramMeta / CachedSession / ProgramResponse
// ---------------------------------------------------------------------------

func TestProgramMeta_Serialization(t *testing.T) {
	reason := "insufficient coverage for topic NLP"
	meta := models.ProgramMeta{
		ValidationPassed: true,
		Coverage:         map[string]int{"ml": 3, "nlp": 1},
		FallbackReason:   &reason,
		GeneratorVersion: "2.1.0",
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var decoded models.ProgramMeta
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.True(t, decoded.ValidationPassed)
	assert.Equal(t, 3, decoded.Coverage["ml"])
	assert.Equal(t, 1, decoded.Coverage["nlp"])
	require.NotNil(t, decoded.FallbackReason)
	assert.Equal(t, reason, *decoded.FallbackReason)
	assert.Equal(t, "2.1.0", decoded.GeneratorVersion)
}

func TestProgramMeta_NilFallbackReason(t *testing.T) {
	meta := models.ProgramMeta{
		ValidationPassed: true,
		GeneratorVersion: "1.0.0",
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var decoded models.ProgramMeta
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Nil(t, decoded.FallbackReason)
	assert.True(t, decoded.ValidationPassed)
}

func TestProgramMeta_FailedValidation(t *testing.T) {
	reason := "topics not found in knowledge base"
	meta := models.ProgramMeta{
		ValidationPassed: false,
		FallbackReason:   &reason,
		GeneratorVersion: "2.0.0",
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var decoded models.ProgramMeta
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.False(t, decoded.ValidationPassed)
	require.NotNil(t, decoded.FallbackReason)
	assert.Contains(t, *decoded.FallbackReason, "topics not found")
}

func TestProgramMetaFromMap_Valid(t *testing.T) {
	raw := map[string]interface{}{
		"validation_passed": true,
		"coverage":          map[string]interface{}{"ml": float64(5)},
		"fallback_reason":   nil,
		"generator_version": "3.0.0",
	}

	meta := models.ProgramMetaFromMap(raw)
	require.NotNil(t, meta)
	assert.True(t, meta.ValidationPassed)
	assert.Equal(t, 5, meta.Coverage["ml"])
	assert.Equal(t, "3.0.0", meta.GeneratorVersion)
}

func TestProgramMetaFromMap_NilInput(t *testing.T) {
	meta := models.ProgramMetaFromMap(nil)
	assert.Nil(t, meta)
}

func TestProgramMetaFromMap_EmptyMap(t *testing.T) {
	meta := models.ProgramMetaFromMap(map[string]interface{}{})
	require.NotNil(t, meta)
	assert.False(t, meta.ValidationPassed)
	assert.Nil(t, meta.FallbackReason)
}

func TestProgramMetaFromMap_WithFallbackReason(t *testing.T) {
	raw := map[string]interface{}{
		"validation_passed": false,
		"fallback_reason":   "model timeout",
		"generator_version": "2.5.0",
	}

	meta := models.ProgramMetaFromMap(raw)
	require.NotNil(t, meta)
	assert.False(t, meta.ValidationPassed)
	require.NotNil(t, meta.FallbackReason)
	assert.Equal(t, "model timeout", *meta.FallbackReason)
}

// ---------------------------------------------------------------------------
// CachedSession — model mapping with ProgramMeta, ProgramStatus, ProgramVersion
// ---------------------------------------------------------------------------

func TestCachedSession_FullSerialization(t *testing.T) {
	sid := uuid.New()
	uid := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	reason := "low coverage"

	cached := models.CachedSession{
		SessionID: sid,
		UserID:    uid,
		Params: models.SessionParams{
			Topics: []string{"ml", "nlp"},
			Level:  "senior",
			Type:   "llm",
			Mode:   "interview",
		},
		InterviewProgram: models.InterviewProgram{
			Questions: []models.QuestionItem{
				{ID: "q-1", Question: "Explain attention", Theory: "Attention is...", Order: 1},
				{ID: "q-2", Question: "What is BERT?", Theory: "BERT is...", Order: 2},
			},
		},
		ProgramStatus: "ready",
		ProgramMeta: &models.ProgramMeta{
			ValidationPassed: true,
			Coverage:         map[string]int{"ml": 3, "nlp": 2},
			FallbackReason:   &reason,
			GeneratorVersion: "2.0.0",
		},
		ProgramVersion: "1",
		CachedAt:       now,
	}

	data, err := json.Marshal(cached)
	require.NoError(t, err)

	var decoded models.CachedSession
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, sid, decoded.SessionID)
	assert.Equal(t, uid, decoded.UserID)
	assert.Equal(t, "senior", decoded.Params.Level)
	assert.Equal(t, "interview", decoded.Params.Mode)
	assert.Equal(t, []string{"ml", "nlp"}, decoded.Params.Topics)
	assert.Equal(t, "llm", decoded.Params.Type)
	assert.Len(t, decoded.InterviewProgram.Questions, 2)
	assert.Equal(t, "q-1", decoded.InterviewProgram.Questions[0].ID)
	assert.Equal(t, "Explain attention", decoded.InterviewProgram.Questions[0].Question)
	assert.Equal(t, 1, decoded.InterviewProgram.Questions[0].Order)
	assert.Equal(t, "ready", decoded.ProgramStatus)
	assert.Equal(t, "1", decoded.ProgramVersion)
	require.NotNil(t, decoded.ProgramMeta)
	assert.True(t, decoded.ProgramMeta.ValidationPassed)
	assert.Equal(t, 3, decoded.ProgramMeta.Coverage["ml"])
	require.NotNil(t, decoded.ProgramMeta.FallbackReason)
	assert.Equal(t, "low coverage", *decoded.ProgramMeta.FallbackReason)
	assert.Equal(t, now, decoded.CachedAt)
}

func TestCachedSession_NoProgramMeta(t *testing.T) {
	cached := models.CachedSession{
		SessionID: uuid.New(),
		UserID:    uuid.New(),
		Params: models.SessionParams{
			Topics: []string{"cv"},
			Level:  "junior",
			Mode:   "study",
		},
		InterviewProgram: models.InterviewProgram{
			Questions: []models.QuestionItem{
				{ID: "q-1", Question: "What is CNN?", Theory: "CNN...", Order: 1},
			},
		},
		ProgramStatus: "ready",
		CachedAt:      time.Now().UTC(),
	}

	data, err := json.Marshal(cached)
	require.NoError(t, err)

	var decoded models.CachedSession
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Nil(t, decoded.ProgramMeta)
	assert.Equal(t, "ready", decoded.ProgramStatus)
	assert.Empty(t, decoded.ProgramVersion)
}

func TestCachedSession_FailedProgramStatus(t *testing.T) {
	reason := "knowledge base unavailable"
	cached := models.CachedSession{
		SessionID:     uuid.New(),
		UserID:        uuid.New(),
		ProgramStatus: "failed",
		ProgramMeta: &models.ProgramMeta{
			ValidationPassed: false,
			FallbackReason:   &reason,
			GeneratorVersion: "1.0.0",
		},
	}

	data, err := json.Marshal(cached)
	require.NoError(t, err)

	var decoded models.CachedSession
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "failed", decoded.ProgramStatus)
	assert.False(t, decoded.ProgramMeta.ValidationPassed)
	assert.Equal(t, "knowledge base unavailable", *decoded.ProgramMeta.FallbackReason)
}

// ---------------------------------------------------------------------------
// ProgramResponse — GetInterviewProgram return type
// ---------------------------------------------------------------------------

func TestProgramResponse_Structure(t *testing.T) {
	program := &models.InterviewProgram{
		Questions: []models.QuestionItem{
			{ID: "q-1", Question: "What is GAN?", Theory: "GAN...", Order: 1},
			{ID: "q-2", Question: "Explain VAE", Theory: "VAE...", Order: 2},
		},
	}
	reason := "fallback used"
	meta := &models.ProgramMeta{
		ValidationPassed: false,
		FallbackReason:   &reason,
		GeneratorVersion: "1.5.0",
	}

	resp := ProgramResponse{
		Program: program,
		Status:  "ready",
		Meta:    meta,
	}

	assert.NotNil(t, resp.Program)
	assert.Len(t, resp.Program.Questions, 2)
	assert.Equal(t, "ready", resp.Status)
	assert.NotNil(t, resp.Meta)
	assert.False(t, resp.Meta.ValidationPassed)
}

func TestProgramResponse_NilMeta(t *testing.T) {
	resp := ProgramResponse{
		Program: &models.InterviewProgram{
			Questions: []models.QuestionItem{
				{ID: "q-1", Question: "test?", Theory: "...", Order: 1},
			},
		},
		Status: "ready",
		Meta:   nil,
	}

	assert.Nil(t, resp.Meta)
	assert.Equal(t, "ready", resp.Status)
}

// ---------------------------------------------------------------------------
// SessionParams — mode field values
// ---------------------------------------------------------------------------

func TestSessionParams_ModeValues(t *testing.T) {
	modes := []string{"interview", "training", "study"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			params := models.SessionParams{
				Topics: []string{"ml"},
				Level:  "middle",
				Mode:   mode,
			}

			data, err := json.Marshal(params)
			require.NoError(t, err)

			var decoded models.SessionParams
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, mode, decoded.Mode)
		})
	}
}

func TestSessionParams_TypeField(t *testing.T) {
	params := models.SessionParams{
		Topics: []string{"transformers"},
		Level:  "senior",
		Mode:   "interview",
		Type:   "nlp",
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var decoded models.SessionParams
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "nlp", decoded.Type)
}

func TestSessionParams_OmitEmptyType(t *testing.T) {
	params := models.SessionParams{
		Topics: []string{"basics"},
		Level:  "junior",
		Mode:   "study",
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	assert.NotContains(t, string(data), `"type"`)
}

// ---------------------------------------------------------------------------
// QuestionItem model
// ---------------------------------------------------------------------------

func TestQuestionItem_Serialization(t *testing.T) {
	q := models.QuestionItem{
		ID:       "q-abc",
		Question: "What is gradient descent?",
		Theory:   "Gradient descent is an optimization algorithm...",
		Order:    3,
	}

	data, err := json.Marshal(q)
	require.NoError(t, err)

	var decoded models.QuestionItem
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, q.ID, decoded.ID)
	assert.Equal(t, q.Question, decoded.Question)
	assert.Equal(t, q.Theory, decoded.Theory)
	assert.Equal(t, q.Order, decoded.Order)
}

// ---------------------------------------------------------------------------
// HandleInterviewBuildResponse — pending channel delivery
// ---------------------------------------------------------------------------

func TestHandleInterviewBuildResponse_DeliversToChannel(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	svc := &SessionManagerService{
		logger: logger,
	}

	sessionID := uuid.New().String()
	ch := make(chan *programResult, 1)
	svc.pendingSessions.Store(sessionID, ch)

	programPayload := map[string]interface{}{
		"questions": []interface{}{
			map[string]interface{}{
				"id":       "q-1",
				"question": "What is ML?",
				"theory":   "ML is...",
				"order":    float64(1),
			},
		},
	}
	metaPayload := map[string]interface{}{
		"validation_passed": true,
		"generator_version": "3.0.0",
	}

	err := svc.HandleInterviewBuildResponse(context.Background(), sessionID, programPayload, metaPayload)
	require.NoError(t, err)

	select {
	case res := <-ch:
		require.NotNil(t, res)
		require.NotNil(t, res.Program)
		assert.Len(t, res.Program.Questions, 1)
		assert.Equal(t, "q-1", res.Program.Questions[0].ID)
		assert.Equal(t, "What is ML?", res.Program.Questions[0].Question)
		assert.Equal(t, "ready", res.Status)
		require.NotNil(t, res.Meta)
		assert.True(t, res.Meta.ValidationPassed)
	default:
		t.Fatal("expected program result on channel")
	}
}

func TestHandleInterviewBuildResponse_FailedValidation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	svc := &SessionManagerService{
		logger: logger,
	}

	sessionID := uuid.New().String()
	ch := make(chan *programResult, 1)
	svc.pendingSessions.Store(sessionID, ch)

	programPayload := map[string]interface{}{
		"questions": []interface{}{
			map[string]interface{}{
				"id":       "q-fallback",
				"question": "Fallback question",
				"order":    float64(1),
			},
		},
	}
	metaPayload := map[string]interface{}{
		"validation_passed": false,
		"fallback_reason":   "topics not covered",
		"generator_version": "2.0.0",
	}

	err := svc.HandleInterviewBuildResponse(context.Background(), sessionID, programPayload, metaPayload)
	require.NoError(t, err)

	select {
	case res := <-ch:
		assert.Equal(t, "failed", res.Status)
		assert.False(t, res.Meta.ValidationPassed)
		require.NotNil(t, res.Meta.FallbackReason)
		assert.Equal(t, "topics not covered", *res.Meta.FallbackReason)
	default:
		t.Fatal("expected result on channel")
	}
}

func TestHandleInterviewBuildResponse_NoPendingSession(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	svc := &SessionManagerService{
		logger: logger,
	}

	programPayload := map[string]interface{}{
		"questions": []interface{}{},
	}

	err := svc.HandleInterviewBuildResponse(context.Background(), "non-existent", programPayload, nil)
	require.NoError(t, err)
}

func TestHandleInterviewBuildResponse_InvalidQuestionsFormat(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	svc := &SessionManagerService{
		logger: logger,
	}

	programPayload := map[string]interface{}{
		"questions": "not an array",
	}

	err := svc.HandleInterviewBuildResponse(context.Background(), uuid.New().String(), programPayload, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid questions format")
}
