package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(rr.Body).Decode(v))
}

func TestMonitorValidation(t *testing.T) {
	q := newQ(t)
	h := handlers.NewMonitors(q, false) // allowPrivate=false -> SSRF guard active

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty url", map[string]any{"name": "m", "url": "", "type": "http", "interval_seconds": 60}},
		{"bad type", map[string]any{"name": "m", "url": "http://example.com", "type": "bogus", "interval_seconds": 60}},
		{"zero interval", map[string]any{"name": "m", "url": "http://example.com", "type": "http", "interval_seconds": 0}},
		{"private http url", map[string]any{"name": "m", "url": "http://127.0.0.1/x", "type": "http", "interval_seconds": 60}},
		{"private tcp target", map[string]any{"name": "m", "url": "10.0.0.1:6379", "type": "tcp", "interval_seconds": 60}},
		{"degraded >= down", map[string]any{"name": "m", "url": "http://example.com", "type": "http", "interval_seconds": 60, "degraded_threshold_ms": 3000, "down_threshold_ms": 2000}},
	}
	for _, c := range cases {
		code := do(h.Create, "POST", "/api/monitors", c.body).Code
		assert.Equal(t, http.StatusBadRequest, code, "%s should be rejected", c.name)
	}
	// bad JSON body -> 400
	assert.Equal(t, http.StatusBadRequest, do(h.Create, "POST", "/x", "not json").Code)
}

func TestPagesGroupAssociations(t *testing.T) {
	q := newQ(t)
	ctx := context.Background()
	g1, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "g1"})
	g2, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "g2"})
	h := handlers.NewPages(q)

	// Create with two groups.
	cr := do(h.Create, "POST", "/api/pages", map[string]any{
		"domain": "acme.com", "title": "Acme", "published": true, "group_ids": []int64{g1.ID, g2.ID},
	})
	require.Equal(t, http.StatusCreated, cr.Code)

	// List to obtain the id (view has group_ids inlined).
	var pages []struct {
		ID       int64   `json:"id"`
		GroupIDs []int64 `json:"group_ids"`
	}
	lr := do(h.List, "GET", "/api/pages", nil)
	require.Equal(t, http.StatusOK, lr.Code)
	decode(t, lr, &pages)
	var id string
	for _, p := range pages {
		if len(p.GroupIDs) == 2 {
			id = itoa(p.ID)
		}
	}
	require.NotEmpty(t, id, "created page with 2 groups should be listed")

	// Update to a single group: exercises the "remove existing, add new" loops.
	up := do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", id))
	}, "PUT", "/x", map[string]any{"domain": "acme.io", "title": "Acme2", "published": false, "group_ids": []int64{g1.ID}})
	assert.Equal(t, http.StatusOK, up.Code)
}
