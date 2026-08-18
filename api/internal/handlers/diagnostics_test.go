package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleBundle = `{"collected_at":"2026-08-18T03:14:02Z","sections":{"kernel":{"data":{"oom_kills":[{"process":"jellyfin","pid":1234}]}}}}`

func postDiagnostics(t *testing.T, q *generated.Queries, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/diagnostics", bytes.NewReader([]byte(body)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	handlers.NewIngest(q).PostDiagnostics(rr, req)
	return rr
}

func TestIngestDiagnosticsStoresBundleForAuthenticatedAgent(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	agentID, token := createAgent(t, handlers.NewAgents(q))

	rr := postDiagnostics(t, q, token, `{"bundle":`+sampleBundle+`}`)
	require.Equal(t, http.StatusNoContent, rr.Code)

	stored, err := q.ListAgentDiagnostics(t.Context(), generated.ListAgentDiagnosticsParams{
		AgentID: agentID, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Nil(t, stored[0].IncidentID, "an on-demand bundle belongs to no incident")
	// The server stores the payload verbatim so collectors can evolve without
	// a matching server-side schema change.
	assert.Contains(t, stored[0].Payload, "jellyfin")
}

func TestIngestDiagnosticsRejectsUnknownToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	_, _ = createAgent(t, handlers.NewAgents(q))

	rr := postDiagnostics(t, q, "not-a-real-token", `{"bundle":`+sampleBundle+`}`)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestIngestDiagnosticsRejectsMissingToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)

	rr := postDiagnostics(t, q, "", `{"bundle":`+sampleBundle+`}`)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestIngestDiagnosticsRejectsMalformedBody(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	_, token := createAgent(t, handlers.NewAgents(q))

	rr := postDiagnostics(t, q, token, `{"bundle":`)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestIngestDiagnosticsAttachesToIncidentWhenGiven(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	agentID, token := createAgent(t, handlers.NewAgents(q))

	_, err := db.ExecContext(t.Context(), `
		INSERT INTO incidents (id, title, severity, status)
		VALUES (7, 'Jellyfin is down', 'major', 'detected')`)
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{
		"incident_id": 7,
		"bundle":      json.RawMessage(sampleBundle),
	})
	rr := postDiagnostics(t, q, token, string(body))
	require.Equal(t, http.StatusNoContent, rr.Code)

	stored, err := q.ListIncidentDiagnostics(t.Context(), ptrInt64(7))
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, agentID, stored[0].AgentID)
}

func ptrInt64(v int64) *int64 { return &v }
