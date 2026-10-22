package handlers_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pushBody() map[string]any {
	return map[string]any{"name": "hb", "type": "push", "interval_seconds": 60}
}

func TestMonitorCreatePushWithoutDatabase(t *testing.T) {
	q := newQ(t)

	noDB := handlers.NewMonitors(q, nil, true)
	assert.Equal(t, http.StatusInternalServerError, do(noDB.Create, "POST", "/x", pushBody()).Code)

	nilDB := handlers.NewMonitors(q, func() *sql.DB { return nil }, true)
	assert.Equal(t, http.StatusInternalServerError, do(nilDB.Create, "POST", "/x", pushBody()).Code)

	closed := testutil.NewTestDB(t)
	require.NoError(t, closed.Close())
	deadDB := handlers.NewMonitors(q, func() *sql.DB { return closed }, true)
	assert.Equal(t, http.StatusInternalServerError, do(deadDB.Create, "POST", "/x", pushBody()).Code)
}

func TestMonitorCreatePushDuplicateIdentityRollsBack(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)
	body := pushBody()
	body["source"] = "uptime-kuma"
	body["external_id"] = "monitor:1"

	require.Equal(t, http.StatusCreated, do(h.Create, "POST", "/x", body).Code)
	assert.Equal(t, http.StatusInternalServerError, do(h.Create, "POST", "/x", body).Code)

	monitors, err := generated.New(db).ListMonitors(context.Background())
	require.NoError(t, err)
	assert.Len(t, monitors, 1, "the failed create must not leave a second monitor behind")
}

func TestMonitorRotatePushTokenStoreFailure(t *testing.T) {
	db := testutil.NewTestDB(t)
	mon, err := generated.New(db).CreateMonitor(context.Background(), generated.CreateMonitorParams{
		Name: "hb", Type: "push", IntervalSeconds: 60, TimeoutSeconds: 30,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)

	// GetMonitor succeeds, then the token upsert fails.
	h := handlers.NewMonitors(generated.New(testutil.FailAfter(db, 1)), func() *sql.DB { return db }, true)
	code := do(func(w http.ResponseWriter, r *http.Request) {
		h.RotatePushToken(w, withChiID(r, "id", itoa(mon.ID)))
	}, "POST", "/x", nil).Code
	assert.Equal(t, http.StatusInternalServerError, code)
}

func TestMonitorGetUpdateDeleteErrorBranches(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)
	body := map[string]any{"name": "m", "url": "http://example.com", "type": "http", "interval_seconds": 60}

	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Get(w, withChiID(r, "id", "bad"))
	}, "GET", "/x", nil).Code, "Get with a non-numeric id")

	assert.Equal(t, http.StatusNotFound, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", "9999"))
	}, "PUT", "/x", body).Code, "Update of a missing monitor")

	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", "bad"))
	}, "DELETE", "/x", nil).Code, "Delete with a non-numeric id")

	mon, err := q.CreateMonitor(context.Background(), generated.CreateMonitorParams{
		Name: "m", Url: "http://example.com", Type: "http", IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)

	// GetMonitor succeeds, then the update query fails.
	failing := handlers.NewMonitors(generated.New(testutil.FailAfter(db, 1)), func() *sql.DB { return db }, true)
	assert.Equal(t, http.StatusInternalServerError, do(func(w http.ResponseWriter, r *http.Request) {
		failing.Update(w, withChiID(r, "id", itoa(mon.ID)))
	}, "PUT", "/x", body).Code, "Update when the write fails")
}
