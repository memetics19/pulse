package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupsUpdateDelete(t *testing.T) {
	q := newQ(t)
	h := handlers.NewGroups(q)
	assert.Equal(t, http.StatusOK, do(h.List, "GET", "/x", nil).Code)

	cr := do(h.Create, "POST", "/x", map[string]any{"name": "prod", "display_order": 1})
	require.Equal(t, http.StatusCreated, cr.Code)
	var g generated.MonitorGroup
	require.NoError(t, json.NewDecoder(cr.Body).Decode(&g))

	assert.Equal(t, http.StatusOK, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", itoa(g.ID)))
	}, "PUT", "/x", map[string]any{"name": "prod2", "display_order": 2}).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", itoa(g.ID)))
	}, "DELETE", "/x", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", "bad"))
	}, "DELETE", "/x", nil).Code)
}

func TestMonitorsGetUpdateDelete(t *testing.T) {
	q := newQ(t)
	ctx := context.Background()
	mon, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "m", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)
	h := handlers.NewMonitors(q, true)
	id := itoa(mon.ID)

	assert.Equal(t, http.StatusOK, do(func(w http.ResponseWriter, r *http.Request) {
		h.Get(w, withChiID(r, "id", id))
	}, "GET", "/x", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(func(w http.ResponseWriter, r *http.Request) {
		h.Get(w, withChiID(r, "id", "9999"))
	}, "GET", "/x", nil).Code)
	assert.Equal(t, http.StatusOK, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", id))
	}, "PUT", "/x", map[string]any{"name": "m2", "url": "http://example.org", "type": "http", "interval_seconds": 30}).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", id))
	}, "DELETE", "/x", nil).Code)
}

func TestAgentsListDeleteMetrics(t *testing.T) {
	q := newQ(t)
	ah := handlers.NewAgents(q)
	id, _ := createAgent(t, ah)

	assert.Equal(t, http.StatusOK, do(ah.List, "GET", "/x", nil).Code)
	assert.Equal(t, http.StatusOK, do(func(w http.ResponseWriter, r *http.Request) {
		ah.GetMetrics(w, withChiID(r, "agentID", itoa(id)))
	}, "GET", "/x?days=2", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		ah.GetMetrics(w, withChiID(r, "agentID", "bad"))
	}, "GET", "/x", nil).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		ah.Delete(w, withChiID(r, "id", itoa(id)))
	}, "DELETE", "/x", nil).Code)
}

func TestIncidentsListCreateUpdateDelete(t *testing.T) {
	q := newQ(t)
	h := handlers.NewIncidents(q)
	assert.Equal(t, http.StatusOK, do(h.List, "GET", "/x", nil).Code)

	cr := do(h.Create, "POST", "/x", map[string]any{
		"title": "outage", "severity": "major", "affected_monitor_ids": []int64{1, 2},
	})
	require.Equal(t, http.StatusCreated, cr.Code)
	var inc generated.Incident
	require.NoError(t, json.NewDecoder(cr.Body).Decode(&inc))
	id := itoa(inc.ID)

	assert.Equal(t, http.StatusOK, do(func(w http.ResponseWriter, r *http.Request) {
		h.UpdateStatus(w, withChiID(r, "id", id))
	}, "PUT", "/x", map[string]any{"status": "investigating"}).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", id))
	}, "DELETE", "/x", nil).Code)
}

func TestMaintenanceListCreateUpdateDelete(t *testing.T) {
	q := newQ(t)
	h := handlers.NewMaintenance(q)
	assert.Equal(t, http.StatusOK, do(h.List, "GET", "/x", nil).Code)

	now := time.Now()
	cr := do(h.Create, "POST", "/x", map[string]any{
		"title": "db upgrade", "status": "scheduled", "affected_monitor_ids": []int64{1},
		"starts_at": now.Format(time.RFC3339), "ends_at": now.Add(time.Hour).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, cr.Code)
	var mw struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.NewDecoder(cr.Body).Decode(&mw))
	id := itoa(mw.ID)

	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.UpdateStatus(w, withChiID(r, "id", id))
	}, "PUT", "/x", map[string]any{"status": "in_progress"}).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", id))
	}, "DELETE", "/x", nil).Code)
}
