package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sentAlert struct {
	alert      alerter.Alert
	monitorIDs []int64
}

// fakeNotifier records every alert instead of sending it.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []sentAlert
}

func (f *fakeNotifier) Notify(_ context.Context, a alerter.Alert, monitorIDs ...int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentAlert{a, monitorIDs})
}

func (f *fakeNotifier) all() []sentAlert {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentAlert(nil), f.sent...)
}

func jsonRequest(t *testing.T, method string, v any) *http.Request {
	t.Helper()
	body, err := json.Marshal(v)
	require.NoError(t, err)
	req := httptest.NewRequest(method, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// A manual incident alerts when it opens, when its status moves, and when it
// is resolved, scoped to the monitors it affects.
func TestIncidentLifecycleSendsAlerts(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	n := &fakeNotifier{}
	h := handlers.NewIncidents(q).WithNotifier(n)

	rr := httptest.NewRecorder()
	h.Create(rr, jsonRequest(t, http.MethodPost, map[string]any{
		"title": "API down", "severity": "major", "affected_monitor_ids": []int64{4, 7},
	}))
	require.Equal(t, http.StatusCreated, rr.Code)
	var inc generated.Incident
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &inc))
	id := strconv.FormatInt(inc.ID, 10)

	rr = httptest.NewRecorder()
	h.UpdateStatus(rr, withChiID(jsonRequest(t, http.MethodPut, map[string]any{"status": "identified"}), "id", id))
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	h.UpdateStatus(rr, withChiID(jsonRequest(t, http.MethodPut, map[string]any{"status": "resolved", "rca": "bad deploy"}), "id", id))
	require.Equal(t, http.StatusOK, rr.Code)

	sent := n.all()
	require.Len(t, sent, 3)
	assert.Equal(t, inc.Status, sent[0].alert.Status)
	assert.Equal(t, "API down", sent[0].alert.Title)
	assert.Equal(t, "major", sent[0].alert.Severity)
	assert.Equal(t, inc.ID, sent[0].alert.IncidentID)
	assert.Equal(t, "identified", sent[1].alert.Status)
	assert.Equal(t, "resolved", sent[2].alert.Status)
	assert.Equal(t, "Resolved. Root cause: bad deploy", sent[2].alert.Message)
	for _, s := range sent {
		assert.Equal(t, []int64{4, 7}, s.monitorIDs)
	}
}

// A failed status change sends nothing.
func TestIncidentRejectedStatusSendsNoAlert(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	n := &fakeNotifier{}
	h := handlers.NewIncidents(q).WithNotifier(n)

	rr := httptest.NewRecorder()
	h.UpdateStatus(rr, withChiID(jsonRequest(t, http.MethodPut, map[string]any{"status": "resolved"}), "id", "1"))
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Empty(t, n.all())
}

// A timeline update alerts with the update's status and message.
func TestIncidentUpdateSendsAlert(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	inc, err := q.CreateIncident(context.Background(), generated.CreateIncidentParams{
		Title: "Checkout slow", Severity: "minor", AffectedMonitorIds: "[3]", Source: "internal",
	})
	require.NoError(t, err)
	n := &fakeNotifier{}
	h := handlers.NewIncidentUpdates(q).WithNotifier(n)

	rr := httptest.NewRecorder()
	h.Create(rr, withChiID(jsonRequest(t, http.MethodPost, map[string]any{
		"status": "monitoring", "message": "A fix is out.", "author": "ops",
	}), "incidentID", strconv.FormatInt(inc.ID, 10)))
	require.Equal(t, http.StatusCreated, rr.Code)

	sent := n.all()
	require.Len(t, sent, 1)
	assert.Equal(t, alerter.Alert{
		IncidentID: inc.ID, Title: "Checkout slow", Status: "monitoring", Severity: "minor", Message: "A fix is out.",
	}, sent[0].alert)
	assert.Equal(t, []int64{3}, sent[0].monitorIDs)
}
