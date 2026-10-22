package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
)

func TestIngestMalformedBody(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	_, token := createAgent(t, handlers.NewAgents(q))
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", bytes.NewReader([]byte("not json")))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handlers.NewIngest(q).PostMetrics(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
