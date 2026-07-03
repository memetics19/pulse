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
