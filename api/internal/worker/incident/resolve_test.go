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

// openAutoIncident records two down results so the detector opens its own
// incident for mon, and returns that incident.
func openAutoIncident(t *testing.T, q *store.Queries, mon store.Monitor, at time.Time) store.Incident {
	t.Helper()
	insertResult(t, q, mon.ID, "down", at)
	insertResult(t, q, mon.ID, "down", at.Add(time.Second))
	created, err := incident.NewDetector(q).MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	require.True(t, created)
	incidents, err := q.ListActiveIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	return incidents[0]
}

// resolve runs auto-resolve with a fresh detector, the way a restarted
// worker would, so only persisted rows drive the decision.
func resolve(t *testing.T, q *store.Queries, monitorID int64) int {
	t.Helper()
	resolved, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), q, monitorID)
	require.NoError(t, err)
	for _, inc := range resolved {
		require.Equal(t, "resolved", inc.Status)
	}
	return len(resolved)
}

func getIncident(t *testing.T, q *store.Queries, id int64) store.Incident {
	t.Helper()
	inc, err := q.GetIncident(context.Background(), id)
	require.NoError(t, err)
	return inc
}

func TestResolve_TwoAnsweringResultsResolveAutoIncident(t *testing.T) {
	tests := []struct {
		name   string
		first  string
		second string
	}{
		{name: "up then up", first: "up", second: "up"},
		{name: "degraded then up", first: "degraded", second: "up"},
		{name: "up then degraded", first: "up", second: "degraded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q)
			now := time.Now()
			inc := openAutoIncident(t, q, mon, now)

			insertResult(t, q, mon.ID, tt.first, now.Add(2*time.Second))
			assert.Equal(t, 0, resolve(t, q, mon.ID), "one answering result is not enough")
			insertResult(t, q, mon.ID, tt.second, now.Add(3*time.Second))
			assert.Equal(t, 1, resolve(t, q, mon.ID))

			got := getIncident(t, q, inc.ID)
			assert.Equal(t, "resolved", got.Status)
			require.NotNil(t, got.ResolvedAt)
			assert.WithinDuration(t, time.Now(), *got.ResolvedAt, time.Minute)
			require.NotNil(t, got.Rca)
			assert.Regexp(t,
				`^Recovered by itself at \d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ after \d+s\. No root cause was recorded\. Edit this if you know it\.$`,
				*got.Rca)

			updates, err := q.ListIncidentUpdates(context.Background(), inc.ID)
			require.NoError(t, err)
			require.Len(t, updates, 1)
			assert.Equal(t, "resolved", updates[0].Status)
			assert.Equal(t, "Recovered by itself.", updates[0].Message)
			assert.Equal(t, "Pulse", updates[0].Author)

			assert.Equal(t, 0, resolve(t, q, mon.ID), "a resolved incident is not resolved again")
		})
	}
}

func TestResolve_RCAStatesHowLongTheMonitorWasDown(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now().UTC()
	inc, err := q.CreateAutoIncident(context.Background(), store.CreateAutoIncidentParams{
		Title: "Test API is down", AffectedMonitorIds: "[" + stringID(mon.ID) + "]",
		StartedAt: now.Add(-(time.Hour + 19*time.Minute)), ExternalID: stringID(mon.ID),
	})
	require.NoError(t, err)
	insertResult(t, q, mon.ID, "up", now)
	insertResult(t, q, mon.ID, "up", now.Add(time.Second))

	assert.Equal(t, 1, resolve(t, q, mon.ID))

	got := getIncident(t, q, inc.ID)
	require.NotNil(t, got.Rca)
	assert.Contains(t, *got.Rca, " after 1h 19m. ")
}

func TestResolve_OneUpResultLeavesIncidentOpen(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	inc := openAutoIncident(t, q, mon, now)

	insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
	assert.Equal(t, 0, resolve(t, q, mon.ID))
	// A flap: up, then down again, then up. The latest two never both answer.
	insertResult(t, q, mon.ID, "down", now.Add(3*time.Second))
	assert.Equal(t, 0, resolve(t, q, mon.ID))
	insertResult(t, q, mon.ID, "up", now.Add(4*time.Second))
	assert.Equal(t, 0, resolve(t, q, mon.ID))

	got := getIncident(t, q, inc.ID)
	assert.Equal(t, "detected", got.Status)
	assert.Nil(t, got.ResolvedAt)
	assert.Nil(t, got.Rca)
}

func TestResolve_HumanTouchedIncidentStaysOpen(t *testing.T) {
	tests := []struct {
		name       string
		touch      func(t *testing.T, q *store.Queries, id int64)
		wantStatus string
	}{
		{
			name: "status moved to investigating",
			touch: func(t *testing.T, q *store.Queries, id int64) {
				_, err := q.UpdateIncidentStatus(context.Background(), store.UpdateIncidentStatusParams{
					Status: "investigating", ID: id,
				})
				require.NoError(t, err)
			},
			wantStatus: "investigating",
		},
		{
			name: "update posted",
			touch: func(t *testing.T, q *store.Queries, id int64) {
				_, err := q.CreateIncidentUpdate(context.Background(), store.CreateIncidentUpdateParams{
					IncidentID: id, Status: "detected", Message: "Looking into it.", Author: "admin",
				})
				require.NoError(t, err)
			},
			wantStatus: "detected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q)
			now := time.Now()
			inc := openAutoIncident(t, q, mon, now)
			tt.touch(t, q, inc.ID)

			insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
			insertResult(t, q, mon.ID, "up", now.Add(3*time.Second))
			assert.Equal(t, 0, resolve(t, q, mon.ID))

			got := getIncident(t, q, inc.ID)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Nil(t, got.ResolvedAt)
			assert.Nil(t, got.Rca)
		})
	}
}

func TestResolve_ManualIncidentStaysOpen(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	inc, err := q.CreateIncident(context.Background(), store.CreateIncidentParams{
		Title: "Manual incident", Severity: "minor",
		AffectedMonitorIds: "[" + stringID(mon.ID) + "]", StartedAt: now,
		Source: "internal", ExternalID: stringID(mon.ID),
	})
	require.NoError(t, err)

	insertResult(t, q, mon.ID, "up", now.Add(time.Second))
	insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
	assert.Equal(t, 0, resolve(t, q, mon.ID))

	got := getIncident(t, q, inc.ID)
	assert.Equal(t, "detected", got.Status)
	assert.Nil(t, got.ResolvedAt)
}

func TestResolve_IncidentCoveringSeveralMonitorsStaysOpen(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	other := createMonitor(t, q)
	now := time.Now().UTC()
	inc, err := q.CreateAutoIncident(context.Background(), store.CreateAutoIncidentParams{
		Title:              "Test API is down",
		AffectedMonitorIds: "[" + stringID(mon.ID) + "," + stringID(other.ID) + "]",
		StartedAt:          now, ExternalID: stringID(mon.ID),
	})
	require.NoError(t, err)

	insertResult(t, q, mon.ID, "up", now.Add(time.Second))
	insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
	assert.Equal(t, 0, resolve(t, q, mon.ID))

	got := getIncident(t, q, inc.ID)
	assert.Equal(t, "detected", got.Status)
	assert.Nil(t, got.ResolvedAt)
}

func TestResolve_LaterFailureOpensFreshIncident(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon := createMonitor(t, q)
	now := time.Now()
	first := openAutoIncident(t, q, mon, now)
	insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
	insertResult(t, q, mon.ID, "up", now.Add(3*time.Second))
	require.Equal(t, 1, resolve(t, q, mon.ID))

	insertResult(t, q, mon.ID, "down", now.Add(4*time.Second))
	insertResult(t, q, mon.ID, "down", now.Add(5*time.Second))
	created, err := incident.NewDetector(q).MaybeCreateIncident(context.Background(), mon.ID, "down")
	require.NoError(t, err)
	assert.True(t, created)

	active, err := q.ListActiveIncidents(context.Background())
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.NotEqual(t, first.ID, active[0].ID)
	assert.Equal(t, "detected", active[0].Status)
	assert.Equal(t, "resolved", getIncident(t, q, first.ID).Status)
}

func TestResolve_ReturnsDatabaseErrors(t *testing.T) {
	tests := []struct {
		name      string
		failAfter int
		want      string
	}{
		{name: "latest results", failAfter: 0, want: "list latest check results"},
		{name: "list incidents", failAfter: 1, want: "list auto-resolvable incidents"},
		{name: "resolve incident", failAfter: 2, want: "resolve incident"},
		{name: "add update", failAfter: 3, want: "add resolve update to incident"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := store.New(db)
			mon := createMonitor(t, q)
			now := time.Now()
			openAutoIncident(t, q, mon, now)
			insertResult(t, q, mon.ID, "up", now.Add(2*time.Second))
			insertResult(t, q, mon.ID, "up", now.Add(3*time.Second))

			faulty := store.New(testutil.FailAfter(db, tt.failAfter))
			_, err := incident.NewDetector(q).MaybeResolveIncidentsWithQueries(context.Background(), faulty, mon.ID)

			require.ErrorContains(t, err, tt.want)
		})
	}
}
