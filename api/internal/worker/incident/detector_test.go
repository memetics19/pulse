package incident_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createMonitor(t *testing.T, q *store.Queries) store.Monitor {
	t.Helper()
	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "Test API", Url: "http://example.com", Type: "http",
		IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal",
	})
	require.NoError(t, err)
	return mon
}

func insertResult(t *testing.T, q *store.Queries, monitorID int64, status string, checkedAt time.Time) {
	t.Helper()
	_, err := q.InsertCheckResult(context.Background(), store.InsertCheckResultParams{
		MonitorID: monitorID, Status: status, CheckedAt: checkedAt,
	})
	require.NoError(t, err)
}

func TestDetector_NoIncidentOnFirstPersistedDown(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	insertResult(t, q, mon.ID, "down", time.Now())

	created, err := incident.NewDetector(q).MaybeCreateIncident(context.Background(), mon.ID, "down")

	require.NoError(t, err)
	assert.False(t, created)
}

func TestDetector_UsesPersistedResultsAcrossDetectorRestarts(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()

	insertResult(t, q, mon.ID, "down", now)
	created, err := incident.NewDetector(q).MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.False(t, created)

	// A fresh detector represents another process or a worker restart. The
	// persisted result sequence, not detector memory, must drive detection.
	insertResult(t, q, mon.ID, "down", now.Add(time.Second))
	created, err = incident.NewDetector(q).MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.True(t, created)
}

func TestDetector_DoesNotDuplicateActiveIncident(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	d := incident.NewDetector(q)

	insertResult(t, q, mon.ID, "down", now)
	created, err := d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.False(t, created)

	insertResult(t, q, mon.ID, "down", now.Add(time.Second))
	created, err = d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.True(t, created)

	insertResult(t, q, mon.ID, "down", now.Add(2*time.Second))
	created, err = d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.False(t, created)

	incidents, err := q.ListActiveIncidents(context.Background())
	require.NoError(t, err)
	assert.Len(t, incidents, 1)
}

func TestDetector_SuppressesIncidentDuringMaintenance(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	_, err := q.CreateMaintenance(context.Background(), store.CreateMaintenanceParams{
		Title: "mw", Status: "in_progress", AffectedMonitorIds: "[" + stringID(mon.ID) + "]",
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	})
	require.NoError(t, err)

	insertResult(t, q, mon.ID, "down", now)
	insertResult(t, q, mon.ID, "down", now.Add(time.Second))
	created, err := incident.NewDetector(q).MaybeCreateIncident(context.Background(), mon.ID, "down")

	require.NoError(t, err)
	assert.False(t, created)
}

func TestDetector_RequiresLatestTwoPersistedResultsToBeDown(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	d := incident.NewDetector(q)

	insertResult(t, q, mon.ID, "down", now)
	insertResult(t, q, mon.ID, "up", now.Add(time.Second))
	insertResult(t, q, mon.ID, "down", now.Add(2*time.Second))
	created, err := d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.False(t, created)

	insertResult(t, q, mon.ID, "down", now.Add(3*time.Second))
	created, err = d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.True(t, created)
}

func stringID(id int64) string {
	return fmt.Sprintf("%d", id)
}
