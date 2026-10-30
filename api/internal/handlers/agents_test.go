package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/internal/keyauth"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createAgent(t *testing.T, h *handlers.Agents) (id int64, token string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": "host-1", "host_label": "web"})
	rr := httptest.NewRecorder()
	h.Create(rr, httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, rr.Code)
	var resp struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	return resp.ID, resp.Token
}

func TestAgentCreateStoresHashedToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewAgents(q)

	id, token := createAgent(t, h)
	require.Len(t, token, 48, "plaintext token should be 48 hex chars")

	agent, err := q.GetAgent(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, keyauth.Hash(token), agent.TokenHash, "DB should hold the hash")
	assert.NotEqual(t, token, agent.TokenHash, "DB must not hold the plaintext token")
}

func TestIngestMetricsAuth(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	_, token := createAgent(t, handlers.NewAgents(q))
	ih := handlers.NewIngest(q)

	post := func(bearer string) int {
		body, _ := json.Marshal(map[string]any{"cpu_percent": 10.5})
		req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", bytes.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rr := httptest.NewRecorder()
		ih.PostMetrics(rr, req)
		return rr.Code
	}

	assert.Equal(t, http.StatusNoContent, post(token), "valid token should ingest")
	assert.Equal(t, http.StatusUnauthorized, post("not-a-real-token"))
	assert.Equal(t, http.StatusUnauthorized, post(""))
}

func postAgent(t *testing.T, h *handlers.Agents, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.Create(rr, httptest.NewRequest(http.MethodPost, "/api/agents", bytes.NewReader([]byte(body))))
	return rr
}

// TestAgentCreateRequiresName covers #94. An empty or blank name used to
// create a nameless agent with 201.
func TestAgentCreateRequiresName(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewAgents(q)

	for _, body := range []string{`{}`, `{"name":""}`, `{"name":"   ","host_label":"web"}`, "{\"name\":\"\\t\\n\"}"} {
		rr := postAgent(t, h, body)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "body %s", body)
		assert.Contains(t, rr.Body.String(), "name is required", "body %s", body)
	}

	agents, err := q.ListAgents(t.Context())
	require.NoError(t, err)
	assert.Empty(t, agents, "no agent should be stored for a rejected request")
}

func TestAgentCreateTrimsName(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewAgents(q)

	rr := postAgent(t, h, `{"name":"  host-1  ","host_label":" web "}`)
	require.Equal(t, http.StatusCreated, rr.Code)
	var resp struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		HostLabel string `json:"host_label"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "host-1", resp.Name)
	assert.Equal(t, "web", resp.HostLabel)

	agent, err := q.GetAgent(t.Context(), resp.ID)
	require.NoError(t, err)
	assert.Equal(t, "host-1", agent.Name, "the stored name should be trimmed")
}

func TestAgentCreateRejectsDuplicateName(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewAgents(q)

	require.Equal(t, http.StatusCreated, postAgent(t, h, `{"name":"host-1"}`).Code)

	for _, name := range []string{"host-1", " host-1 ", "HOST-1"} {
		body, _ := json.Marshal(map[string]string{"name": name})
		rr := postAgent(t, h, string(body))
		assert.Equal(t, http.StatusConflict, rr.Code, "name %q", name)
		assert.Contains(t, rr.Body.String(), `an agent named`, "name %q", name)
		assert.Contains(t, rr.Body.String(), `already exists`, "name %q", name)
	}

	agents, err := q.ListAgents(t.Context())
	require.NoError(t, err)
	assert.Len(t, agents, 1, "a duplicate name must not create a second agent")

	// A different name is still accepted.
	assert.Equal(t, http.StatusCreated, postAgent(t, h, `{"name":"host-2"}`).Code)
}

// An agent stored before names were checked may carry surrounding spaces.
// The duplicate check compares trimmed names, so it still counts.
func TestAgentCreateDuplicateOfUntrimmedLegacyName(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	_, err := q.CreateAgent(t.Context(), generated.CreateAgentParams{Name: " legacy ", HostLabel: "", TokenHash: "x"})
	require.NoError(t, err)

	rr := postAgent(t, handlers.NewAgents(q), `{"name":"legacy"}`)
	assert.Equal(t, http.StatusConflict, rr.Code)
}
