package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// §4.2: BFF results-crud client — DTO mapping & error handling
// ---------------------------------------------------------------------------

func newTestClient(handler http.Handler) (*ResultsCRUDClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	cl := NewResultsCRUDClient(srv.URL, 5)
	return cl, srv
}

func TestGetResult_DTOMapping(t *testing.T) {
	sid := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Contains(t, r.URL.Path, sid.String())

		resp := GetResultResponse{
			Result: Result{
				ID:                  42,
				SessionID:           sid,
				Score:               85,
				Feedback:            "Well done",
				TerminatedEarly:     false,
				ReportJSON:          json.RawMessage(`{"summary":"ok"}`),
				PresetTraining:      json.RawMessage(`{"preset_id":"p1"}`),
				Evaluations:         json.RawMessage(`[{"q":"q1","score":0.9}]`),
				ResultFormatVersion: 2,
				SessionKind:         "training",
				CreatedAt:           now,
				UpdatedAt:           now,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	result, err := cl.GetResult(context.Background(), sid)
	require.NoError(t, err)

	assert.Equal(t, uint(42), result.ID)
	assert.Equal(t, sid, result.SessionID)
	assert.Equal(t, 85, result.Score)
	assert.Equal(t, "Well done", result.Feedback)
	assert.False(t, result.TerminatedEarly)
	assert.Equal(t, 2, result.ResultFormatVersion)
	assert.Equal(t, "training", result.SessionKind)
	assert.JSONEq(t, `{"summary":"ok"}`, string(result.ReportJSON))
	assert.JSONEq(t, `{"preset_id":"p1"}`, string(result.PresetTraining))
	assert.JSONEq(t, `[{"q":"q1","score":0.9}]`, string(result.Evaluations))
}

func TestGetResult_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "result not found"})
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	_, err := cl.GetResult(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "result not found")
}

func TestGetResult_ServerError500(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	_, err := cl.GetResult(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal error")
}

func TestGetResult_Timeout(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cl := NewResultsCRUDClient(srv.URL, 1)

	_, err := cl.GetResult(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do request")
}

func TestGetResult_MalformedJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not json`))
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	_, err := cl.GetResult(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestGetResults_MultipleIDs(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)

		ids := r.URL.Query().Get("session_ids")
		assert.Contains(t, ids, id1.String())
		assert.Contains(t, ids, id2.String())

		resp := GetResultsResponse{
			Results: []Result{
				{SessionID: id1, Score: 60, SessionKind: "interview", CreatedAt: now, UpdatedAt: now},
				{SessionID: id2, Score: 90, SessionKind: "study", CreatedAt: now, UpdatedAt: now},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	results, err := cl.GetResults(context.Background(), []uuid.UUID{id1, id2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, 60, results[0].Score)
	assert.Equal(t, "study", results[1].SessionKind)
}

func TestGetResults_EmptyList(t *testing.T) {
	cl := NewResultsCRUDClient("http://unused", 5)

	results, err := cl.GetResults(context.Background(), []uuid.UUID{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestGetResults_ServerError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "db down"})
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	_, err := cl.GetResults(context.Background(), []uuid.UUID{uuid.New()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestGetResults_UpstreamTimeout(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	cl := NewResultsCRUDClient(srv.URL, 1)

	_, err := cl.GetResults(context.Background(), []uuid.UUID{uuid.New()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "do request")
}

func TestGetResult_UpstreamUnknownStatusCode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
	})

	cl, srv := newTestClient(handler)
	defer srv.Close()

	_, err := cl.GetResult(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}
