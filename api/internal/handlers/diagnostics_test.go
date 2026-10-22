package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
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
	assert.Equal(t, agentID, stored[0].AgentID)
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

// A bundle is a JSON object. Accepting null, numbers, strings, or arrays stores
// garbage that nothing downstream can render.
func TestIngestDiagnosticsRejectsNonObjectBundles(t *testing.T) {
	for _, body := range []string{
		`{"bundle":null}`,
		`{"bundle":123}`,
		`{"bundle":"just a string"}`,
		`{"bundle":[]}`,
	} {
		db := testutil.NewTestDB(t)
		q := generated.New(db)
		_, token := createAgent(t, handlers.NewAgents(q))

		rr := postDiagnostics(t, q, token, body)

		assert.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", body)
	}
}

// Uploading evidence is pointless if nothing can read it back — the whole
// reason to push bundles off the host is to see them without an SSH session.
func TestListAgentDiagnosticsReturnsStoredBundles(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	agentID, token := createAgent(t, handlers.NewAgents(q))

	require.Equal(t, http.StatusNoContent,
		postDiagnostics(t, q, token, `{"bundle":`+sampleBundle+`}`).Code)

	req := httptest.NewRequest(http.MethodGet,
		"/api/agents/"+strconv.FormatInt(agentID, 10)+"/diagnostics", nil)
	req = withURLParam(req, "agentID", strconv.FormatInt(agentID, 10))
	rr := httptest.NewRecorder()
	handlers.NewAgents(q).ListDiagnostics(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var got []struct {
		AgentID int64           `json:"agent_id"`
		Bundle  json.RawMessage `json:"bundle"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, agentID, got[0].AgentID)
	// The bundle comes back as JSON, not a re-encoded string a client must
	// parse twice.
	assert.Contains(t, string(got[0].Bundle), "jellyfin")
}

// withURLParam attaches a chi route param, which the handler reads directly.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Bundles are large — each can approach the 1 MiB ingest cap — so a generous
// row limit is a memory foot-gun rather than a feature. Asserting an exact
// count matters: "at most five" would also pass if the endpoint returned
// nothing at all.
func TestListAgentDiagnosticsClampsAndDefaultsTheRowCount(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	agentID, token := createAgent(t, handlers.NewAgents(q))

	for i := 0; i < 8; i++ {
		require.Equal(t, http.StatusNoContent,
			postDiagnostics(t, q, token, `{"bundle":`+sampleBundle+`}`).Code)
	}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"an oversized limit is clamped to the maximum", "?limit=50", 5},
		{"no limit uses the default", "", 3},
		{"a limit below the maximum is honoured", "?limit=2", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/agents/1/diagnostics"+tc.query, nil)
			req = withURLParam(req, "agentID", strconv.FormatInt(agentID, 10))
			rr := httptest.NewRecorder()
			handlers.NewAgents(q).ListDiagnostics(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			var got []map[string]any
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			assert.Len(t, got, tc.want)
		})
	}
}

// A misconfigured cron firing continuously would otherwise fill the database
// with near-1 MiB rows faster than retention can trim them. The limit is per
// agent and deliberately short — it stops a runaway loop, not a human running
// --diagnose twice.
func TestIngestDiagnosticsRateLimitsPerAgent(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	_, token := createAgent(t, handlers.NewAgents(q))
	ingest := handlers.NewIngest(q)

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/ingest/diagnostics",
			bytes.NewReader([]byte(`{"bundle":`+sampleBundle+`}`)))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		ingest.PostDiagnostics(rr, req)
		return rr.Code
	}

	assert.Equal(t, http.StatusNoContent, post(), "the first upload is accepted")
	assert.Equal(t, http.StatusTooManyRequests, post(), "an immediate repeat is refused")
}

// A rejected request must not consume the agent's next slot, or a malformed
// body locks out the corrected retry behind a 429.
func TestIngestDiagnosticsRejectedRequestDoesNotConsumeTheSlot(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	_, token := createAgent(t, handlers.NewAgents(q))
	ingest := handlers.NewIngest(q)

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/ingest/diagnostics",
			bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		ingest.PostDiagnostics(rr, req)
		return rr.Code
	}

	assert.Equal(t, http.StatusBadRequest, post(`{"bundle":null}`), "malformed body is refused")
	assert.Equal(t, http.StatusNoContent, post(`{"bundle":`+sampleBundle+`}`),
		"the corrected retry must be accepted immediately")
	assert.Equal(t, http.StatusTooManyRequests, post(`{"bundle":`+sampleBundle+`}`),
		"only a stored bundle starts the interval")
}
