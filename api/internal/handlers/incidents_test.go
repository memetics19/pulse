package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withChiID(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestIncidentResolveRequiresRCA(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewIncidents(q)

	body, _ := json.Marshal(map[string]any{
		"title":                "API down",
		"severity":             "critical",
		"affected_monitor_ids": "[]",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/incidents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	// Try resolve without RCA — must be 400
	updateBody, _ := json.Marshal(map[string]any{"status": "resolved", "rca": ""})
	req2 := withChiID(httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(updateBody)), "id", "1")
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.UpdateStatus(rr2, req2)
	assert.Equal(t, http.StatusBadRequest, rr2.Code)
}

func TestIncidentResolveWithRCA(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewIncidents(q)

	body, _ := json.Marshal(map[string]any{
		"title":                "API down",
		"severity":             "critical",
		"affected_monitor_ids": "[]",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/incidents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	updateBody, _ := json.Marshal(map[string]any{
		"status": "resolved",
		"rca":    "Root cause: misconfigured load balancer. Fixed by reverting config.",
	})
	req2 := withChiID(httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(updateBody)), "id", "1")
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.UpdateStatus(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code)
}
