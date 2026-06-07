package incident_test

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetector_NoIncidentOnFirstDown(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	d := incident.NewDetector(q)

	created, err := d.MaybeCreateIncident(context.Background(), 42, "down")
	require.NoError(t, err)
	assert.False(t, created)
}

func TestDetector_IncidentOnSecondConsecutiveDown(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)

	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "Test API", Url: "http://example.com", Type: "http",
		IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: 1,
		Source: "internal",
	})
	require.NoError(t, err)

	d := incident.NewDetector(q)

	created1, err := d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.False(t, created1, "first down should not create incident")

	created2, err := d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.True(t, created2, "second consecutive down should create incident")

	// Third down — incident already active, should NOT create duplicate
	created3, err := d.MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.False(t, created3, "should not create duplicate incident")
}

func TestSuppressDuringMaintenance(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	// active maintenance covering monitor 7
	_, _ = q.CreateMaintenance(context.Background(), store.CreateMaintenanceParams{
		Title: "mw", Status: "in_progress", AffectedMonitorIds: "[7]",
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour),
	})
	d := incident.NewDetector(q)
	// two consecutive downs would normally create an incident on the 2nd
	_, _ = d.MaybeCreateIncident(context.Background(), 7, "down")
	created, err := d.MaybeCreateIncident(context.Background(), 7, "down")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("incident must be suppressed while monitor 7 is under active maintenance")
	}
}

func TestDetector_ResetsOnUp(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)

	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "Test API", Url: "http://example.com", Type: "http",
		IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: 1,
		Source: "internal",
	})
	require.NoError(t, err)

	d := incident.NewDetector(q)

	d.MaybeCreateIncident(context.Background(), mon.ID, "down")                 // 1st down
	d.MaybeCreateIncident(context.Background(), mon.ID, "up")                   // reset counter
	created, err := d.MaybeCreateIncident(context.Background(), mon.ID, "down") // 1st down again
	require.NoError(t, err)
	assert.False(t, created, "counter should reset after 'up'")
}
