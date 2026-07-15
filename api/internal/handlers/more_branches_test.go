package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaintenanceCreateValidation(t *testing.T) {
	h := handlers.NewMaintenance(newQ(t))
	now := time.Now().Format(time.RFC3339)

	// missing title
	assert.Equal(t, http.StatusBadRequest,
		do(h.Create, "POST", "/x", map[string]any{"starts_at": now, "ends_at": now}).Code)
	// invalid starts_at
	assert.Equal(t, http.StatusBadRequest,
		do(h.Create, "POST", "/x", map[string]any{"title": "t", "starts_at": "nope", "ends_at": now}).Code)
	// invalid ends_at
	assert.Equal(t, http.StatusBadRequest,
		do(h.Create, "POST", "/x", map[string]any{"title": "t", "starts_at": now, "ends_at": "nope"}).Code)
	// start_now -> created in_progress
	assert.Equal(t, http.StatusCreated,
		do(h.Create, "POST", "/x", map[string]any{"title": "t", "starts_at": now, "ends_at": now, "start_now": true}).Code)
	// bad JSON
	assert.Equal(t, http.StatusBadRequest, do(h.Create, "POST", "/x", "nope").Code)
}

func TestMonitorUpdateValidation(t *testing.T) {
	q := newQ(t)
	mon, err := q.CreateMonitor(context.Background(), generated.CreateMonitorParams{
		Name: "m", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)
	h := handlers.NewMonitors(q, false) // guard on
	// Update to a private URL is rejected.
	rr := do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", itoa(mon.ID)))
	}, "PUT", "/x", map[string]any{"name": "m", "url": "http://127.0.0.1/x", "type": "http", "interval_seconds": 60})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	// invalid id
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", "bad"))
	}, "PUT", "/x", map[string]any{}).Code)
	// bad body
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", itoa(mon.ID)))
	}, "PUT", "/x", "nope").Code)
}
