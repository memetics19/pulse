package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func newQ(t *testing.T) *generated.Queries {
	t.Helper()
	return generated.New(testutil.NewTestDB(t))
}

func do(h http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, target, bytes.NewReader(b))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	rr := httptest.NewRecorder()
	h(rr, r)
	return rr
}

func TestNotificationsCRUD(t *testing.T) {
	q := newQ(t)
	h := handlers.NewNotifications(q)

	assert.Equal(t, http.StatusOK, do(h.List, "GET", "/api/notifications", nil).Code)

	rr := do(h.Create, "POST", "/api/notifications",
		map[string]any{"channel": "slack", "config_json": `{"webhook_url":"https://x"}`})
	require.Equal(t, http.StatusCreated, rr.Code)
	var created generated.Notification
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&created))

	assert.Equal(t, http.StatusBadRequest, do(h.Create, "POST", "/x", "not json").Code)

	// Update
	upd := do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", itoa(created.ID)))
	}, "PUT", "/x", map[string]any{"channel": "email", "config_json": "{}"})
	assert.Equal(t, http.StatusOK, upd.Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", "abc"))
	}, "PUT", "/x", map[string]any{}).Code)

	// Delete
	del := do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", itoa(created.ID)))
	}, "DELETE", "/x", nil)
	assert.Equal(t, http.StatusNoContent, del.Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", "abc"))
	}, "DELETE", "/x", nil).Code)
}

func TestThemeGetUpdate(t *testing.T) {
	q := newQ(t)
	h := handlers.NewTheme(q)

	rr := do(h.Update, "PUT", "/api/theme",
		map[string]any{"preset": "dark", "custom_css": ":root{}", "config_json": "{}"})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, http.StatusOK, do(h.Get, "GET", "/api/theme", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(h.Update, "PUT", "/x", "not json").Code)
}

func TestCheckResultsListAndUptime(t *testing.T) {
	q := newQ(t)
	ctx := context.Background()
	mon, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "m", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)
	ms := int64(42)
	_, err = q.InsertCheckResult(ctx, generated.InsertCheckResultParams{
		MonitorID: mon.ID, CheckedAt: time.Now(), Status: "up", ResponseTimeMs: &ms,
	})
	require.NoError(t, err)

	h := handlers.NewCheckResults(q)
	id := itoa(mon.ID)

	list := do(func(w http.ResponseWriter, r *http.Request) {
		h.List(w, withChiID(r, "monitorID", id))
	}, "GET", "/x?days=7", nil)
	assert.Equal(t, http.StatusOK, list.Code)

	up := do(func(w http.ResponseWriter, r *http.Request) {
		h.Uptime(w, withChiID(r, "monitorID", id))
	}, "GET", "/x", nil)
	require.Equal(t, http.StatusOK, up.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(up.Body).Decode(&body))
	assert.Equal(t, float64(100), body["uptime_pct"], "one up check = 100%")

	// invalid monitorID -> 400
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.List(w, withChiID(r, "monitorID", "abc"))
	}, "GET", "/x", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Uptime(w, withChiID(r, "monitorID", "abc"))
	}, "GET", "/x", nil).Code)
}

func TestIncidentUpdatesCreateAndList(t *testing.T) {
	q := newQ(t)
	ctx := context.Background()
	inc, err := q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "down", Severity: "major", AffectedMonitorIds: "[]", StartedAt: time.Now(), Source: "internal",
	})
	require.NoError(t, err)

	h := handlers.NewIncidentUpdates(q)
	id := itoa(inc.ID)

	cr := do(func(w http.ResponseWriter, r *http.Request) {
		h.Create(w, withChiID(r, "incidentID", id))
	}, "POST", "/x", map[string]any{"status": "investigating", "message": "looking", "author": "admin"})
	assert.Equal(t, http.StatusCreated, cr.Code)

	ls := do(func(w http.ResponseWriter, r *http.Request) {
		h.List(w, withChiID(r, "incidentID", id))
	}, "GET", "/x", nil)
	assert.Equal(t, http.StatusOK, ls.Code)

	// error paths
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Create(w, withChiID(r, "incidentID", "abc"))
	}, "POST", "/x", map[string]any{}).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.List(w, withChiID(r, "incidentID", "abc"))
	}, "GET", "/x", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Create(w, withChiID(r, "incidentID", id))
	}, "POST", "/x", "not json").Code)
}
