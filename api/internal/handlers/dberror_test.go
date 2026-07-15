package handlers_test

import (
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
)

// closedQ returns a Queries whose database is closed, so every query returns an
// error. This exercises the "database error -> 500" branch in each handler
// cheaply, without a mock, by driving the happy path up to its first query.
func closedQ(t *testing.T) *generated.Queries {
	t.Helper()
	db := testutil.NewTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return generated.New(db)
}

func TestHandlersReturn500OnDBError(t *testing.T) {
	q := closedQ(t)
	// Each of these calls a query immediately with no prior validation, so a
	// closed DB drives them into their error branch.
	lists := map[string]http.HandlerFunc{
		"monitors.List":      handlers.NewMonitors(q, true).List,
		"groups.List":        handlers.NewGroups(q).List,
		"incidents.List":     handlers.NewIncidents(q).List,
		"notifications.List": handlers.NewNotifications(q).List,
		"maintenance.List":   handlers.NewMaintenance(q).List,
		"pages.List":         handlers.NewPages(q).List,
		"agents.List":        handlers.NewAgents(q).List,
		"apikeys.List":       handlers.NewAPIKeys(q).List,
		"theme.Get":          handlers.NewTheme(q).Get,
		"overview.Get":       handlers.NewOverview(q).Get,
		"status.JSON":        nil, // placeholder; status handled elsewhere
	}
	for name, hf := range lists {
		if hf == nil {
			continue
		}
		code := do(hf, "GET", "/x", nil).Code
		assert.Equal(t, http.StatusInternalServerError, code, "%s should 500 on DB error", name)
	}

	// auth.Status: CountUsers fails -> 500
	assert.Equal(t, http.StatusInternalServerError,
		do(handlers.NewAuth(q, false).Status, "GET", "/x", nil).Code, "auth.Status")
}

func TestHandlerCreatesReturn500OnDBError(t *testing.T) {
	q := closedQ(t)
	creates := []struct {
		name string
		h    http.HandlerFunc
		body any
	}{
		{"monitors.Create", handlers.NewMonitors(q, true).Create,
			map[string]any{"name": "m", "url": "http://example.com", "type": "http", "interval_seconds": 60}},
		{"groups.Create", handlers.NewGroups(q).Create, map[string]any{"name": "g"}},
		{"incidents.Create", handlers.NewIncidents(q).Create,
			map[string]any{"title": "t", "severity": "major", "affected_monitor_ids": []int64{}}},
		{"notifications.Create", handlers.NewNotifications(q).Create,
			map[string]any{"channel": "slack", "config_json": "{}"}},
		{"pages.Create", handlers.NewPages(q).Create, map[string]any{"domain": "a.com", "title": "A"}},
		{"agents.Create", handlers.NewAgents(q).Create, map[string]any{"name": "h", "host_label": "web"}},
		{"apikeys.Create", handlers.NewAPIKeys(q).Create, map[string]any{"name": "k", "scopes": []string{}}},
		{"theme.Update", handlers.NewTheme(q).Update, map[string]any{"preset": "d", "custom_css": "", "config_json": "{}"}},
	}
	for _, c := range creates {
		assert.Equal(t, http.StatusInternalServerError, do(c.h, "POST", "/x", c.body).Code,
			"%s should 500 on DB error", c.name)
	}

	// Get/Delete with a valid numeric id but a dead DB -> 500 (or 404 for Get).
	m := handlers.NewMonitors(q, true)
	assert.Equal(t, http.StatusNotFound, do(func(w http.ResponseWriter, r *http.Request) {
		m.Get(w, withChiID(r, "id", "1"))
	}, "GET", "/x", nil).Code, "monitors.Get on dead DB -> 404")
	assert.Equal(t, http.StatusInternalServerError, do(func(w http.ResponseWriter, r *http.Request) {
		m.Delete(w, withChiID(r, "id", "1"))
	}, "DELETE", "/x", nil).Code, "monitors.Delete on dead DB -> 500")
}

func TestHandlerMutationsReturn500OnDBError(t *testing.T) {
	q := closedQ(t)
	// id-keyed Update/UpdateStatus/Delete/Revoke/List with valid input against a
	// dead DB all reach their query and 500.
	byID := []struct {
		name   string
		h      http.HandlerFunc
		key    string
		method string
		body   any
	}{
		{"monitors.Update", handlers.NewMonitors(q, true).Update, "id", "PUT",
			map[string]any{"name": "m", "url": "http://example.com", "type": "http", "interval_seconds": 60}},
		{"groups.Update", handlers.NewGroups(q).Update, "id", "PUT", map[string]any{"name": "g"}},
		{"groups.Delete", handlers.NewGroups(q).Delete, "id", "DELETE", nil},
		{"notifications.Update", handlers.NewNotifications(q).Update, "id", "PUT", map[string]any{"channel": "slack", "config_json": "{}"}},
		{"notifications.Delete", handlers.NewNotifications(q).Delete, "id", "DELETE", nil},
		{"incidents.UpdateStatus", handlers.NewIncidents(q).UpdateStatus, "id", "PUT", map[string]any{"status": "investigating"}},
		{"incidents.Delete", handlers.NewIncidents(q).Delete, "id", "DELETE", nil},
		{"maintenance.UpdateStatus", handlers.NewMaintenance(q).UpdateStatus, "id", "PUT", map[string]any{"status": "in_progress"}},
		{"maintenance.Delete", handlers.NewMaintenance(q).Delete, "id", "DELETE", nil},
		{"pages.Update", handlers.NewPages(q).Update, "id", "PUT", map[string]any{"domain": "a.com", "title": "A", "group_ids": []int64{}}},
		{"pages.Delete", handlers.NewPages(q).Delete, "id", "DELETE", nil},
		{"apikeys.Revoke", handlers.NewAPIKeys(q).Revoke, "id", "DELETE", nil},
		{"agents.Delete", handlers.NewAgents(q).Delete, "id", "DELETE", nil},
		{"checkResults.List", handlers.NewCheckResults(q).List, "monitorID", "GET", nil},
		{"checkResults.Uptime", handlers.NewCheckResults(q).Uptime, "monitorID", "GET", nil},
		{"incidentUpdates.List", handlers.NewIncidentUpdates(q).List, "incidentID", "GET", nil},
		{"incidentUpdates.Create", handlers.NewIncidentUpdates(q).Create, "incidentID", "POST", map[string]any{"status": "x", "message": "m"}},
	}
	for _, c := range byID {
		code := do(func(w http.ResponseWriter, r *http.Request) {
			c.h(w, withChiID(r, c.key, "1"))
		}, c.method, "/x", c.body).Code
		assert.Equal(t, http.StatusInternalServerError, code, "%s on dead DB", c.name)
	}
}
