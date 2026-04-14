package kafka

import "context"

type traceIDKeyType struct{}

var traceIDKey = traceIDKeyType{}

// ContextWithTraceID returns a child context carrying the given trace ID.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext extracts the trace ID from context (empty string if absent).
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}
	return ""
}

// ChatEvent представляет событие чата.
type ChatEvent struct {
	EventID       string                 `json:"event_id"`
	EventType     string                 `json:"event_type"`
	Timestamp     string                 `json:"timestamp"`
	Service       string                 `json:"service"`
	Version       string                 `json:"version"`
	TraceID       string                 `json:"trace_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Payload       map[string]interface{} `json:"payload"`
	Metadata      map[string]string      `json:"metadata,omitempty"`
}

// AgentEvent represents a validated event structure for agent-service communication.
// All fields except Payload are required and must be validated before publishing.
type AgentEvent struct {
	EventType     string                 `json:"event_type"`
	SessionID     string                 `json:"session_id"`
	EventID       string                 `json:"event_id"`
	CorrelationID string                 `json:"correlation_id"`
	Payload       map[string]interface{} `json:"payload"`
}
