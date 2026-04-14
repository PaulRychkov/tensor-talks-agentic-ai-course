package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensor-talks/dialogue-aggregator/internal/kafka"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// §3: dialogue-aggregator — no local score / question generation
// ---------------------------------------------------------------------------

func TestModelServiceHasNoBusinessLogicFields(t *testing.T) {
	svc := &ModelService{}

	assert.Nil(t, svc.producer, "producer should be nil on zero-value struct")
	assert.Nil(t, svc.agentBridge, "agentBridge should be nil on zero-value struct")
	assert.Zero(t, svc.questionDelay, "questionDelay should be zero on zero-value struct")
}

func TestNewModelServiceSetsQuestionDelay(t *testing.T) {
	logger := zap.NewNop()

	svc := NewModelService(nil, nil, nil, nil, nil, nil, 5, logger)

	assert.Equal(t, 5_000_000_000, int(svc.questionDelay.Nanoseconds()),
		"questionDelay must equal 5 seconds in nanoseconds")
}

// ---------------------------------------------------------------------------
// extractResultsFromEvaluation — passthrough of agent payload
// ---------------------------------------------------------------------------

func TestExtractResults_FullEvaluation(t *testing.T) {
	logger := zap.NewNop()
	svc := &ModelService{logger: logger}

	metadata := map[string]interface{}{
		"evaluation": map[string]interface{}{
			"overall_score":        0.85,
			"evaluation_reasoning": "Good understanding of core concepts",
		},
		"recommendations": []interface{}{
			"Review distributed systems",
			"Practice coding interviews",
		},
	}

	score, feedback, recs := svc.extractResultsFromEvaluation(metadata)

	assert.Equal(t, 85, score)
	assert.Equal(t, "Good understanding of core concepts", feedback)
	assert.Equal(t, []string{"Review distributed systems", "Practice coding interviews"}, recs)
}

func TestExtractResults_NoEvaluation(t *testing.T) {
	logger := zap.NewNop()
	svc := &ModelService{logger: logger}

	metadata := map[string]interface{}{}

	score, feedback, recs := svc.extractResultsFromEvaluation(metadata)

	assert.Equal(t, 0, score)
	assert.Contains(t, feedback, "Не удалось получить оценку")
	assert.NotEmpty(t, recs)
}

func TestExtractResults_NilEvaluation(t *testing.T) {
	logger := zap.NewNop()
	svc := &ModelService{logger: logger}

	metadata := map[string]interface{}{
		"evaluation": nil,
	}

	score, feedback, recs := svc.extractResultsFromEvaluation(metadata)

	assert.Equal(t, 0, score)
	assert.NotEmpty(t, feedback)
	assert.NotEmpty(t, recs)
}

func TestExtractResults_ScoreClamping(t *testing.T) {
	logger := zap.NewNop()
	svc := &ModelService{logger: logger}

	tests := []struct {
		name     string
		score    float64
		expected int
	}{
		{"zero", 0.0, 0},
		{"mid", 0.5, 50},
		{"full", 1.0, 100},
		{"negative_clamps_to_zero", -0.5, 0},
		{"over_one_clamps_to_100", 1.5, 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metadata := map[string]interface{}{
				"evaluation": map[string]interface{}{
					"overall_score": tc.score,
				},
			}
			score, _, _ := svc.extractResultsFromEvaluation(metadata)
			assert.Equal(t, tc.expected, score)
		})
	}
}

func TestExtractResults_MissingPointsFallback(t *testing.T) {
	logger := zap.NewNop()
	svc := &ModelService{logger: logger}

	metadata := map[string]interface{}{
		"evaluation": map[string]interface{}{
			"overall_score": 0.6,
			"missing_points": []interface{}{
				"gradient descent",
				"backpropagation",
			},
		},
	}

	_, _, recs := svc.extractResultsFromEvaluation(metadata)

	assert.Len(t, recs, 2)
	assert.Contains(t, recs[0], "gradient descent")
	assert.Contains(t, recs[1], "backpropagation")
}

// ---------------------------------------------------------------------------
// AgentBridge event serialization / deserialization
// ---------------------------------------------------------------------------

func TestMessageFullEvent_Serialization(t *testing.T) {
	event := kafka.MessageFullEvent{
		EventID:   "evt-001",
		EventType: "message.full",
		Timestamp: "2025-01-17T12:00:00Z",
		Service:   "dialogue-aggregator",
		Version:   "1.0.0",
		Payload: map[string]interface{}{
			"chat_id":    "sess-123",
			"message_id": "msg-001",
			"role":       "user",
			"content":    "Explain backpropagation",
		},
		Metadata: map[string]interface{}{
			"correlation_id": "corr-001",
		},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded kafka.MessageFullEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, event.EventID, decoded.EventID)
	assert.Equal(t, event.EventType, decoded.EventType)
	assert.Equal(t, event.Service, decoded.Service)
	assert.Equal(t, event.Version, decoded.Version)
	assert.Equal(t, "sess-123", decoded.Payload["chat_id"])
	assert.Equal(t, "msg-001", decoded.Payload["message_id"])
	assert.Equal(t, "corr-001", decoded.Metadata["correlation_id"])
}

func TestPhraseGeneratedEvent_Deserialization(t *testing.T) {
	raw := `{
		"event_id": "evt-002",
		"event_type": "phrase.agent.generated",
		"timestamp": "2025-01-17T12:01:00Z",
		"service": "interviewer-agent-service",
		"version": "1.0.0",
		"payload": {
			"chat_id": "sess-123",
			"message_id": "msg-002",
			"generated_text": "Backpropagation is the algorithm for computing gradients.",
			"question_id": "q-1",
			"metadata": {
				"agent_decision": "next_question",
				"current_question_index": 1,
				"question_id": "q-2"
			}
		},
		"metadata": {
			"correlation_id": "corr-001"
		}
	}`

	var event kafka.PhraseGeneratedEvent
	err := json.Unmarshal([]byte(raw), &event)
	require.NoError(t, err)

	assert.Equal(t, "evt-002", event.EventID)
	assert.Equal(t, "phrase.agent.generated", event.EventType)
	assert.Equal(t, "interviewer-agent-service", event.Service)

	generatedText, _ := event.Payload["generated_text"].(string)
	assert.Equal(t, "Backpropagation is the algorithm for computing gradients.", generatedText)

	chatID, _ := event.Payload["chat_id"].(string)
	assert.Equal(t, "sess-123", chatID)

	md, _ := event.Payload["metadata"].(map[string]interface{})
	require.NotNil(t, md)

	decision, _ := md["agent_decision"].(string)
	assert.Equal(t, "next_question", decision)

	idx, _ := md["current_question_index"].(float64)
	assert.Equal(t, float64(1), idx)
}

func TestMessageFullEvent_EmptyPayload(t *testing.T) {
	event := kafka.MessageFullEvent{
		EventID:   "evt-003",
		EventType: "message.full",
		Payload:   map[string]interface{}{},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded kafka.MessageFullEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Empty(t, decoded.Payload)
}

func TestPhraseGeneratedEvent_MissingFields(t *testing.T) {
	raw := `{"event_id":"x","payload":{}}`

	var event kafka.PhraseGeneratedEvent
	err := json.Unmarshal([]byte(raw), &event)
	require.NoError(t, err)

	assert.Equal(t, "x", event.EventID)
	assert.Empty(t, event.EventType)
	assert.Nil(t, event.Metadata)

	generatedText, _ := event.Payload["generated_text"].(string)
	assert.Empty(t, generatedText)
}

func TestChatEvent_Serialization(t *testing.T) {
	event := kafka.ChatEvent{
		EventID:       "ce-001",
		EventType:     "chat.started",
		Timestamp:     "2025-01-17T12:00:00Z",
		Service:       "dialogue-aggregator",
		Version:       "1.0.0",
		CorrelationID: "corr-100",
		Payload: map[string]interface{}{
			"session_id": "s-1",
			"user_id":    "u-1",
		},
		Metadata: map[string]string{
			"source": "test",
		},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded kafka.ChatEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, event.EventID, decoded.EventID)
	assert.Equal(t, event.CorrelationID, decoded.CorrelationID)
	assert.Equal(t, "s-1", decoded.Payload["session_id"])
	assert.Equal(t, "test", decoded.Metadata["source"])
}

// ---------------------------------------------------------------------------
// Invalid / incomplete payloads — must not panic
// ---------------------------------------------------------------------------

func TestExtractResults_MalformedMetadata(t *testing.T) {
	logger := zap.NewNop()
	svc := &ModelService{logger: logger}

	tests := []struct {
		name     string
		metadata map[string]interface{}
	}{
		{"evaluation_is_string", map[string]interface{}{"evaluation": "not a map"}},
		{"evaluation_is_number", map[string]interface{}{"evaluation": 42}},
		{"overall_score_is_string", map[string]interface{}{
			"evaluation": map[string]interface{}{"overall_score": "high"},
		}},
		{"recommendations_is_string", map[string]interface{}{
			"evaluation":    map[string]interface{}{"overall_score": 0.5},
			"recommendations": "not an array",
		}},
		{"missing_points_not_array", map[string]interface{}{
			"evaluation": map[string]interface{}{
				"overall_score":  0.3,
				"missing_points": "not an array",
			},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				svc.extractResultsFromEvaluation(tc.metadata)
			})
		})
	}
}

func TestPhraseGeneratedEvent_InvalidJSON(t *testing.T) {
	var event kafka.PhraseGeneratedEvent
	err := json.Unmarshal([]byte(`{invalid json}`), &event)
	assert.Error(t, err)
}

func TestMessageFullEvent_NilMetadata(t *testing.T) {
	event := kafka.MessageFullEvent{
		EventID:   "evt-nil",
		EventType: "message.full",
		Payload:   map[string]interface{}{"chat_id": "c-1"},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded kafka.MessageFullEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Nil(t, decoded.Metadata, "metadata should be nil when omitted")
}

func TestAgentEvent_Serialization(t *testing.T) {
	event := kafka.AgentEvent{
		EventType:     "agent.request",
		SessionID:     "sess-abc",
		EventID:       "ae-001",
		CorrelationID: "corr-abc",
		Payload: map[string]interface{}{
			"action": "evaluate",
		},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded kafka.AgentEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "agent.request", decoded.EventType)
	assert.Equal(t, "sess-abc", decoded.SessionID)
	assert.Equal(t, "evaluate", decoded.Payload["action"])
}

